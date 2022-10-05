package db

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"time"

	"gitlab.com/picodata/stroppy/pkg/database/cluster"
	"gitlab.com/picodata/stroppy/pkg/engine/kubeengine"
	engineSsh "gitlab.com/picodata/stroppy/pkg/engine/ssh"
	"gitlab.com/picodata/stroppy/pkg/kubernetes"
	"gitlab.com/picodata/stroppy/pkg/state"

	"github.com/ansel1/merry"
	helmclient "github.com/mittwald/go-helm-client"
	llog "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
	ydbApi "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	goYaml "gopkg.in/yaml.v3"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/repo"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	config "k8s.io/client-go/applyconfigurations/networking/v1"
	k8sClient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	k8sYaml "sigs.k8s.io/yaml"
)

const (
	databasesDir         string = "third_party/extra/manifests/databases"
	yandexDirectory      string = "yandexdb"
	ydbHelmRepo          string = "https://charts.ydb.tech"
	timeout              int    = 300
	step                 int    = 20
	helmTimeout          int    = 300000000000
	castingError         string = "Error then casting type into interface"
	roAll                int    = 0o644
	stroppyNamespaceName string = "stroppy"
	erasureSpecies       string = "block-4-2"
)

type yandexCluster struct {
	commonCluster *commonCluster
}

// Create createYandexDBCluster.
func createYandexDBCluster(
	sc engineSsh.Client,
	k *kubernetes.Kubernetes,
	shellState *state.State,
) Cluster {
	return &yandexCluster{
		commonCluster: createCommonCluster(
			sc,
			k,
			shellState,
		),
	}
}

// Deploy Yandex Database Cluster
// Implementation of YDB deploy completely differs from others,
// the helm operator will be connected first. The operator is always
// installed from the Yandex repository. Then the store and database
// manifests will be deserialized and parameterized.
func (yc *yandexCluster) Deploy(kube *kubernetes.Kubernetes, shellState *state.State) error {
	var err error

	if err = yc.deployYandexDBOperator(shellState); err != nil {
		return merry.Prepend(err, "Error then deploying ydb operator")
	}

	if err = deployStorage(yc.commonCluster.k, shellState); err != nil {
		return merry.Prepend(err, "Error then deploying storage")
	}

	if err = yc.deployDatabase(shellState); err != nil {
		return merry.Prepend(err, "Error then deploying storage")
	}

	if err = deployStatusIngress(yc.commonCluster.k, shellState); err != nil {
		return merry.Prepend(err, "failed to deploy status ingress")
	}

	if err = waitObjectReady(
		path.Join(
			shellState.Settings.WorkingDirectory,
			databasesDir,
			yandexDirectory,
			"stroppy-database.yml",
		),
		"database",
	); err != nil {
		return merry.Prepend(err, "Error while waiting for YDB database")
	}

	if err = deployStatusIngress(kube, shellState); err != nil {
		return merry.Prepend(err, "failed to deploy ydb status ingress ingress")
	}

	return err
}

// Get clusterSpec.
func (yc *yandexCluster) GetSpecification() ClusterSpec {
	return yc.commonCluster.clusterSpec
}

// Deploy yandex db operator via helmclient library.
func (yc *yandexCluster) deployYandexDBOperator(shellState *state.State) error { //nolint
	var (
		client helmclient.Client
		rel    *release.Release
		bytes  []byte
	)

	kubeconfig, err := os.ReadFile(path.Join(os.Getenv("HOME"), ".kube/config"))
	if err != nil {
		return merry.Prepend(err, "Error then reading kubeconfig file")
	}

	options := &helmclient.KubeConfClientOptions{
		Options: &helmclient.Options{ //nolint
			Namespace:        "stroppy",
			RepositoryCache:  "/tmp/.helmcache",
			RepositoryConfig: "/tmp/.helmrepo",
			RegistryConfig:   "/tmp/.config/helm",
			Debug:            true,
			Linting:          true,
		},
		KubeConfig: kubeconfig,
	}

	if client, err = helmclient.NewClientFromKubeConf(options); err != nil {
		return merry.Prepend(err, "Error then creating helm client")
	}

	// Add YandexDB helm repository
	chartRepo := repo.Entry{ //nolint
		Name: "ydb",
		URL:  ydbHelmRepo,
	}

	// Add a chart-repository to the client.
	if err = client.AddOrUpdateChartRepo(chartRepo); err != nil {
		return merry.Prepend(err, "Error then adding ydb helm repository")
	}

	if bytes, err = os.ReadFile(path.Join(
		shellState.Settings.WorkingDirectory,
		"third_party",
		"extra",
		"manifests",
		"databases",
		"yandexdb",
		"ydb-opeator-values-tpl.yml",
	)); err != nil {
		return merry.Prepend(err, "failed to read ydb operator values yaml")
	}

	// Define the chart to be installed
	chartSpec := helmclient.ChartSpec{ //nolint
		ReleaseName: "ydb-operator",
		ChartName:   "ydb/operator",
		Namespace:   "stroppy",
		ValuesYaml:  string(bytes),
		Wait:        true,
		Timeout:     time.Duration(helmTimeout),
		Atomic:      true,
		UpgradeCRDs: true,
		MaxHistory:  5, //nolint
	}

	if rel, err = client.InstallOrUpgradeChart(context.Background(), &chartSpec, nil); err != nil {
		return merry.Prepend(err, "Error then upgrading/installing ydb-operator release")
	}

	llog.Infof("Release '%s' with YDB operator successfully deployed", rel.Name)

	return nil
}

// Deploy YDB database
// Parse manifest and deploy yandex db database via kubectl.
func (yc *yandexCluster) deployDatabase(shellState *state.State) error {
	var (
		err     error
		bytes   []byte
		storage map[string]interface{}
	)

	mpath := path.Join(shellState.Settings.WorkingDirectory, databasesDir, yandexDirectory)

	bytes, err = os.ReadFile(path.Join(mpath, "database.yml"))
	if err != nil {
		return merry.Prepend(err, "Error then reading database.yml")
	}

	llog.Tracef("%v bytes read from database.yml\n", len(bytes))

	if err = k8sYaml.Unmarshal(bytes, &storage); err != nil {
		return merry.Prepend(err, "Error then deserializing database manifest")
	}

	metadata, statusOk := storage["metadata"].(map[string]interface{})
	if !statusOk {
		return merry.Prepend(err, castingError)
	}

	metadata["namespace"] = stroppyNamespaceName

	// TODO: get it from terraform.tfstate
	spec, statusOk := storage["spec"].(map[string]interface{})
	if !statusOk {
		return merry.Prepend(err, castingError)
	}

	// TODO: replace based on tfstate resources
	// https://github.com/picodata/stroppy/issues/94
	spec["nodes"] = 1

	resources, statusOk := spec["resources"].(map[string]interface{})
	if !statusOk {
		return merry.Prepend(err, castingError)
	}

	resources["storageUnits"] = []interface{}{
		map[string]interface{}{
			// TODO: replace to formula based on host resources
			// https://github.com/picodata/stroppy/issues/94
			// resources can fe fetched from terraform.tfstate
			"count":    1,
			"unitKind": "ssd",
		},
	}

	containerResources, ok := resources["containerResources"].(map[string]interface{})
	if !ok {
		return merry.Prepend(err, castingError)
	}

	containerResources["limits"] = map[string]interface{}{
		// TODO: replace to formula based on host resources
		// https://github.com/picodata/stroppy/issues/94
		// resources can fe fetched from terraform.tfstate
		"cpu": "100m",
	}

	if bytes, err = k8sYaml.Marshal(storage); err != nil {
		return merry.Prepend(err, "Error then serializing database.yml")
	}

	if err = os.WriteFile(
		path.Join(mpath, "stroppy-database.yml"),
		bytes,
		fs.FileMode(roAll),
	); err != nil {
		return merry.Prepend(err, "Error then writing stroppy-database.yml")
	}

	return applyManifest(path.Join(mpath, "stroppy-database.yml"))
}

// Run kubectl and apply manifest.
func applyManifest(manifestName string) error {
	var (
		stdout []byte
		err    error
	)

	// TODO: Replace with k8s api bindings
	// https://github.com/picodata/stroppy/issues/100
	cmd := exec.Command("kubectl", "apply", "-f", manifestName, "--output", "json")
	if stdout, err = cmd.Output(); err != nil {
		llog.Tracef("kubectl stdout:\n%v", stdout)

		return merry.Prepend(
			err,
			fmt.Sprintf("Error then applying %s manifest", manifestName),
		)
	}

	llog.Debugf("Manifest %s succesefully applied", manifestName)

	return nil
}

// Run `n` times `kubectl get -f path` until the `Ready` status is received.
func waitObjectReady(fpath, name string) error {
	var (
		err    error
		output []byte
	)

	for index := 0; index <= timeout; index += step {
		// TODO: https://github.com/picodata/stroppy/issues/99
		cmd := exec.Command("kubectl", "get", "-f", fpath, "--output", "json")
		if output, err = cmd.Output(); err != nil {
			llog.Warnf("Error then executing 'kubectl get': %s", err)
		}

		status := gjson.Get(string(output), "status.state").String()

		if status == "Ready" {
			llog.Infof("%s deployed successfully", name)

			break
		}

		if index == timeout {
			return merry.Prepend(err, "Timeout exceeded objects still in not 'Ready' state")
		}

		llog.Debugf(
			"Object %s in '%s' state, waiting %v seconds... \n",
			name,
			status,
			timeout-index,
		)
		time.Sleep(time.Duration(step) * time.Second)
	}

	return nil
}

// Connect to freshly deployed cluster.
func (yc *yandexCluster) Connect() (interface{}, error) {
	var (
		connection *cluster.YandexDBCluster
		err        error
	)

	if yc.commonCluster.DBUrl == "" {
		yc.commonCluster.DBUrl = "grpc://stroppy-ydb-database-grpc:2135/root/stroppy-ydb-database"

		llog.Infoln("Changed DBURL on", yc.commonCluster.DBUrl)
	}

	ydbContext, cancel := context.WithCancel(context.Background())

	defer cancel()

	if connection, err = cluster.NewYandexDBCluster(
		ydbContext,
		yc.commonCluster.DBUrl,
	); err != nil {
		return nil, merry.Prepend(err, "Error then creating new YDB cluster")
	}

	llog.Debugln("Connection to YDB successfully created")

	return connection, nil
}

func deployStatusIngress(kube *kubernetes.Kubernetes, shellState *state.State) error {
	var err error

	statusIngress := config.Ingress("ydb-status-ingress", stroppyNamespaceName)

	if err = kube.Engine.ToEngineObject(
		path.Join(
			shellState.Settings.WorkingDirectory,
			"third_party",
			"extra",
			"manifests",
			"databases",
			"yandexdb",
			"ydb-status-ingress.yml",
		),
		&statusIngress,
	); err != nil {
		return merry.Prepend(err, "failed to cast manifest into ingress")
	}

	kubeContext, ctxCloseFn := context.WithCancel(context.Background())
	defer ctxCloseFn()

	objectApplyFunc := func(clientSet *k8sClient.Clientset) error {
		if _, err = clientSet.NetworkingV1().Ingresses(stroppyNamespaceName).Apply(
			kubeContext,
			statusIngress,
			kube.Engine.GenerateDefaultMetav1(),
		); err != nil {
			return merry.Prepend(err, "failed to apply ingress manifest")
		}

		return nil
	}

	if err = kube.Engine.DeployObject(
		kubeContext, objectApplyFunc,
	); err != nil {
		return merry.Prepend(err, fmt.Sprintln("failed to deploy ingress"))
	}

	return nil
}

// Deploy YDB storage
// Parse manifest and deploy yandex db storage via kubectl.
func deployStorage(kube *kubernetes.Kubernetes, shellState *state.State) error { //nolint
	var (
		err             error
		ydbStorage      ydbApi.Storage
		restConfig      *rest.Config
		ydbRestClient   kubeengine.YDBV1Alpha1Interface
		storageQuantity resource.Quantity
	)

	if err = kube.Engine.ToEngineObject(
		path.Join(
			shellState.Settings.WorkingDirectory,
			"third_party",
			"extra",
			"manifests",
			"databases",
			"yandexdb",
			"storage.yml",
		),
		&ydbStorage,
	); err != nil {
		return merry.Prepend(err, "failed to deserialize ydb storage manifest")
	}

	persistentVolumeFilesystem := v1.PersistentVolumeFilesystem

	if err = k8sYaml.Unmarshal([]byte(
		fmt.Sprintf("%dGi", shellState.NodesInfo.GetFirstWorker().Resources.Disk-5)), //nolint
		&storageQuantity,
	); err != nil {
		return merry.Prepend(err, "failed to deserialize storageQuantity")
	}

	ydbStorage.Spec.DataStore = []v1.PersistentVolumeClaimSpec{{
		AccessModes: []v1.PersistentVolumeAccessMode{
			v1.PersistentVolumeAccessMode("ReadWriteOnce"),
		},
		Resources: v1.ResourceRequirements{ //nolint
			Requests: v1.ResourceList{"storage": storageQuantity},
		},
		VolumeMode: &persistentVolumeFilesystem,
	}}

	ydbStorage.Spec.Nodes = int32(shellState.NodesInfo.WorkersCnt)
	ydbStorage.Spec.Domain = "root"

	switch shellState.NodesInfo.WorkersCnt {
	case 8: //nolint
		ydbStorage.Spec.Erasure = ydbApi.ErasureBlock42
	case 9: //nolint
		ydbStorage.Spec.Erasure = ydbApi.ErasureMirror3DC
	default:
		ydbStorage.Spec.Erasure = ydbApi.None
	}

	if ydbStorage.Spec.Configuration, err = parametrizeStorageConfig(
		ydbStorage.Spec.Configuration,
		shellState,
	); err != nil {
		return merry.Prepend(err, "failed to parameterize storage configuration")
	}

	if restConfig, err = kube.Engine.GetKubeConfig(); err != nil {
		return merry.Prepend(err, "failed to get kube config")
	}

	if ydbRestClient, err = kubeengine.NewForConfig(restConfig); err != nil {
		return merry.Prepend(err, "failed to create rest client")
	}

	kubeContext, ctxCloseFn := context.WithCancel(context.Background())
	defer ctxCloseFn()

	if _, err = ydbRestClient.Storage("stroppy").Apply(
		kubeContext,
		&ydbStorage,
		&metav1.ApplyOptions{
			TypeMeta: metav1.TypeMeta{
				Kind:       ydbStorage.Kind,
				APIVersion: ydbStorage.APIVersion,
			},
			DryRun:       []string{},
			Force:        true,
			FieldManager: "Merge",
		}, //nolint
	); err != nil {
		return merry.Prepend(err, "failed to apply storage manifers")
	}

	llog.Infof("Storage %s deploy status: success", ydbStorage.Name)

	return nil
}

// Generate parameters for `storage` CRD.
func parametrizeStorageConfig(storage string, shellState *state.State) (string, error) { //nolint
	var (
		err     error
		confMap map[string]interface{}
	)

	if err = goYaml.Unmarshal(
		[]byte(storage),
		&confMap,
	); err != nil {
		return "", merry.Prepend(err, "failed to deserialize ydb configuration")
	}

	domainsConfig, statusOk := confMap["domains_config"].(map[string]interface{})
	if !statusOk {
		return "", merry.Prepend(err, castingError)
	}

	stateStorage, statusOk := domainsConfig["state_storage"].([]interface{})
	if !statusOk {
		return "", merry.Prepend(err, castingError)
	}

	blobStorageConfig, statusOk := confMap["blob_storage_config"].(map[string]interface{})
	if !statusOk {
		return "", merry.Prepend(err, castingError)
	}

	serviceSet, statusOk := blobStorageConfig["service_set"].(map[string]interface{})
	if !statusOk {
		return "", merry.Prepend(err, castingError)
	}

	serviceSet["availability_domains"] = 1

	failDomainsFunc := func(count int) []map[string]interface{} {
		failDomains := []map[string]interface{}{}

		for index := 1; index < count+1; index++ {
			failDomains = append(
				failDomains,
				map[string]interface{}{
					"vdisk_locations": []interface{}{
						map[string]interface{}{
							"node_id":        index,
							"path":           "/dev/kikimr_ssd_00",
							"pdisk_category": "SSD",
						},
					},
				},
			)
		}

		return failDomains
	}

	chProfileConfig, statusOk := confMap["channel_profile_config"].(map[string]interface{})
	if !statusOk {
		return "", merry.Prepend(err, castingError)
	}

	profile, statusOk := chProfileConfig["profile"].([]interface{})
	if !statusOk {
		return "", merry.Prepend(err, castingError)
	}

	channelProfileFunc := func(erasureSpecies string) map[string]interface{} {
		return map[string]interface{}{
			"channel": []interface{}{
				map[string]interface{}{
					"erasure_species":   erasureSpecies,
					"pdisk_category":    1,
					"storage_pool_kind": "ssd",
				},
				map[string]interface{}{
					"erasure_species":   erasureSpecies,
					"pdisk_category":    1,
					"storage_pool_kind": "ssd",
				},
				map[string]interface{}{
					"erasure_species":   erasureSpecies,
					"pdisk_category":    1,
					"storage_pool_kind": "ssd",
				},
			},
			"profile_id": 0,
		}
	}

	switch len(shellState.NodesInfo.NodesParams) {
	case 8: //nolint
		stateStorage[0] = map[string]interface{}{
			"ring": map[string]interface{}{
				"node": []interface{}{
					1, 2, 3, 4, 5, 6, 7, 8,
				},
				"nto_select": 5, //nolint // nto_select is parameter for nto
			},
			"ssid": 1,
		}

		serviceSet["groups"] = []interface{}{
			map[string]interface{}{
				"group_id":         0,
				"group_generation": 1,
				"erasure_species":  erasureSpecies,
				"rings": []map[string]interface{}{
					{
						"fail_domains": failDomainsFunc(8), //nolint
					},
				},
			},
		}

		profile[0] = channelProfileFunc(string(ydbApi.ErasureBlock42))
	case 9: //nolint
		stateStorage[0] = map[string]interface{}{
			"ring": map[string]interface{}{
				"node": []interface{}{
					1, 2, 3, 4, 5, 6, 7, 8, 9,
				},
				"nto_select": 5, //nolint // nto_select is parameter for nto
			},
			"ssid": 1,
		}

		serviceSet["groups"] = []interface{}{
			map[string]interface{}{
				"group_id":         0,
				"group_generation": 1,
				"erasure_species":  erasureSpecies,
				"rings": []map[string]interface{}{
					{
						"fail_domains": failDomainsFunc(9), //nolint
					},
				},
			},
		}

		profile[0] = channelProfileFunc(string(ydbApi.ErasureMirror3DC))
	default:
		stateStorage[0] = map[string]interface{}{
			"ring": map[string]interface{}{
				"node":       []interface{}{1},
				"nto_select": 1, //nolint // nto_select is parameter for nto
			},
			"ssid": 1,
		}

		serviceSet["groups"] = []interface{}{
			map[string]interface{}{
				"group_id":         0,
				"group_generation": 1,
				"erasure_species":  erasureSpecies,
				"rings": []map[string]interface{}{
					{
						"fail_domains": failDomainsFunc(1),
					},
				},
			},
		}
		profile[0] = channelProfileFunc(string(ydbApi.None))
	}

	var data []byte

	if data, err = goYaml.Marshal(confMap); err != nil {
		return "", merry.Prepend(err, "Error then serializing storage configuration")
	}

	return string(data), nil
}
