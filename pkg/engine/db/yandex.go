package db

import (
	"context"
	"fmt"
	"os"
	"path"
	"time"

	"gitlab.com/picodata/stroppy/pkg/database/cluster"
	"gitlab.com/picodata/stroppy/pkg/engine/kubeengine"
	engineSsh "gitlab.com/picodata/stroppy/pkg/engine/ssh"
	"gitlab.com/picodata/stroppy/pkg/kubernetes"
	"gitlab.com/picodata/stroppy/pkg/state"

	"github.com/ansel1/merry"
	helmclient "github.com/mittwald/go-helm-client"
	"github.com/pkg/errors"
	llog "github.com/sirupsen/logrus"
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

const logLevelTrace = "trace"

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

	if err = deployDatabase(yc.commonCluster.k, shellState); err != nil {
		return merry.Prepend(err, "Error then deploying database")
	}

	if err = deployStatusIngress(yc.commonCluster.k, shellState); err != nil {
		return merry.Prepend(err, "failed to deploy status ingress")
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

// Connect to freshly deployed cluster.
func (yc *yandexCluster) Connect() (interface{}, error) {
	var (
		connection *cluster.YdbCluster
		err        error
	)

	if yc.commonCluster.DBUrl == "" {
		yc.commonCluster.DBUrl = "grpc://stroppy-ydb-database-grpc:2135/root/stroppy-ydb-database"

		llog.Infoln("Changed DBURL to", yc.commonCluster.DBUrl)
	}

	ydbContext, ctxCloseFn := context.WithTimeout(context.Background(), time.Second)
	defer ctxCloseFn()

	if connection, err = cluster.NewYdbCluster(
		ydbContext,
		yc.commonCluster.DBUrl,
		yc.commonCluster.connectionPoolSize,
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

	kubeContext, ctxCloseFn := context.WithTimeout(context.Background(), time.Second*5) //nolint
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
		ydbRestClient   *kubeengine.YDBV1Alpha1Client
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

	ydbStorage.Namespace = stroppyNamespaceName

	persistentVolumeFilesystem := v1.PersistentVolumeBlock

	if err = k8sYaml.Unmarshal([]byte(
		fmt.Sprintf("%dGi", shellState.NodesInfo.GetFirstWorker().Resources.SecondaryDisk)), //nolint
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

	if shellState.Settings.LogLevel == logLevelTrace {
		var data []byte

		if data, err = k8sYaml.Marshal(ydbStorage); err != nil {
			return errors.Wrap(err, "failed to serialize storage manifest")
		}

		llog.Tracef("YDB storage manifest:\n%s\n", string(data))
	}

	if restConfig, err = kube.Engine.GetKubeConfig(); err != nil {
		return merry.Prepend(err, "failed to get kube config")
	}

	if ydbRestClient, err = kubeengine.NewForConfig(restConfig); err != nil {
		return merry.Prepend(err, "failed to create rest client")
	}

	kubeContext, ctxCloseFn := context.WithTimeout(context.Background(), time.Second*5) //nolint
	defer ctxCloseFn()

	if _, err = ydbRestClient.YDBV1Alpha1().Storages("stroppy").Apply(
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

	switch shellState.NodesInfo.WorkersCnt {
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

// Deploy YDB database
// Parse manifest and deploy yandex db database via RESTapi.
func deployDatabase(kube *kubernetes.Kubernetes, shellState *state.State) error {
	var (
		err               error
		ydbDatabase       ydbApi.Database
		restConfig        *rest.Config
		ydbRestClient     *kubeengine.YDBV1Alpha1Client
		databaseCPULimits resource.Quantity
	)

	if err = kube.Engine.ToEngineObject(
		path.Join(
			shellState.Settings.WorkingDirectory,
			"third_party",
			"extra",
			"manifests",
			"databases",
			"yandexdb",
			"database.yml",
		),
		&ydbDatabase,
	); err != nil {
		return merry.Prepend(err, "failed to deserialize ydb database manifest")
	}

	ydbDatabase.Namespace = stroppyNamespaceName

	switch len(shellState.NodesInfo.NodesParams) {
	case 8, 9: //nolint
		ydbDatabase.Spec.Nodes = int32(shellState.NodesInfo.WorkersCnt)
	default:
		ydbDatabase.Spec.Nodes = 1 //nolint
	}

	var cpu string

	switch {
	case shellState.NodesInfo.GetFirstWorker().Resources.CPU == 1|2|3:
		cpu = "100m"
	default:
		cpu = fmt.Sprintf(
			"%dm",
			int(shellState.NodesInfo.GetFirstWorker().Resources.CPU/2)*1000, //nolint
		)
	}

	llog.Debugf("Database CPU: %v", cpu)

	if err = k8sYaml.Unmarshal([]byte(cpu), databaseCPULimits); err != nil {
		return merry.Prepend(err, "failed to deserialize storageQuantity")
	}

	ydbDatabase.Spec.Resources.ContainerResources.Limits = v1.ResourceList{
		v1.ResourceCPU: databaseCPULimits,
	}

	if shellState.Settings.LogLevel == logLevelTrace {
		var data []byte

		if data, err = k8sYaml.Marshal(ydbDatabase); err != nil {
			return errors.Wrap(err, "failed to serialize database manifest")
		}

		llog.Tracef("YDB database manifest:\n%s\n", string(data))
	}

	if restConfig, err = kube.Engine.GetKubeConfig(); err != nil {
		return merry.Prepend(err, "failed to get kube config")
	}

	if ydbRestClient, err = kubeengine.NewForConfig(restConfig); err != nil {
		return merry.Prepend(err, "failed to create rest client")
	}

	kubeContext, ctxCloseFn := context.WithTimeout(context.Background(), time.Second*5) //nolint
	defer ctxCloseFn()

	if _, err = ydbRestClient.YDBV1Alpha1().Databases(stroppyNamespaceName).Apply(
		kubeContext,
		&ydbDatabase,
		&metav1.ApplyOptions{
			TypeMeta: metav1.TypeMeta{
				Kind:       ydbDatabase.Kind,
				APIVersion: ydbDatabase.APIVersion,
			},
			DryRun:       []string{},
			Force:        true,
			FieldManager: "Merge",
		},
	); err != nil {
		return merry.Prepend(err, "failed to apply storage manifers")
	}

	llog.Infof("Database %s deploy status: success", ydbDatabase.Name)

	return nil
}
