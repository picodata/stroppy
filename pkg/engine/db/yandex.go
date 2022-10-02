package db

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"time"

	"github.com/ansel1/merry"
	helmclient "github.com/mittwald/go-helm-client"
	llog "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
	"gitlab.com/picodata/stroppy/pkg/database/cluster"
	engineSsh "gitlab.com/picodata/stroppy/pkg/engine/ssh"
	"gitlab.com/picodata/stroppy/pkg/kubernetes"
	"gitlab.com/picodata/stroppy/pkg/state"
	"gopkg.in/yaml.v2"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/repo"
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
func (yc *yandexCluster) Deploy(shellState *state.State) error {
	var err error

	if err = yc.deployYandexDBOperator(); err != nil {
		return merry.Prepend(err, "Error then deploying ydb operator")
	}

	if err = yc.deployStorage(shellState); err != nil {
		return merry.Prepend(err, "Error then deploying storage")
	}

	if err = waitObjectReady(
		path.Join(
			shellState.Settings.WorkingDirectory,
			databasesDir,
			yandexDirectory,
			"stroppy-storage.yml",
		),
		"storage",
	); err != nil {
		return merry.Prepend(err, "Error while waiting for YDB storage")
	}

	if err = yc.deployDatabase(shellState); err != nil {
		return merry.Prepend(err, "Error then deploying storage")
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

	return err
}

// Get clusterSpec.
func (yc *yandexCluster) GetSpecification() ClusterSpec {
	return yc.commonCluster.clusterSpec
}

// Deploy yandex db operator via helmclient library.
//
//nolint:funlen // because logic of this function is inseparable
func (yc *yandexCluster) deployYandexDBOperator() error {
	var (
		client helmclient.Client
		rel    *release.Release
	)

	kubeconfig, err := os.ReadFile(path.Join(os.Getenv("HOME"), ".kube/config"))
	if err != nil {
		return merry.Prepend(err, "Error then reading kubeconfig file")
	}

	options := &helmclient.KubeConfClientOptions{
		Options: &helmclient.Options{
			Namespace:        "stroppy",
			RepositoryCache:  "/tmp/.helmcache",
			RepositoryConfig: "/tmp/.helmrepo",
			RegistryConfig:   "/tmp/.config/helm",
			Debug:            true,
			Linting:          true,
			DebugLog:         func(format string, v ...interface{}) {},
			Output:           os.Stdout,
		},
		KubeContext: "",
		KubeConfig:  kubeconfig,
	}

	if client, err = helmclient.NewClientFromKubeConf(options); err != nil {
		return merry.Prepend(err, "Error then creating helm client")
	}

	// Add YandexDB helm repository
	chartRepo := repo.Entry{
		Name:                  "ydb",
		URL:                   ydbHelmRepo,
		Username:              "",
		Password:              "",
		CertFile:              "",
		KeyFile:               "",
		CAFile:                "",
		InsecureSkipTLSverify: false,
		PassCredentialsAll:    false,
	}

	// Add a chart-repository to the client.
	if err = client.AddOrUpdateChartRepo(chartRepo); err != nil {
		return merry.Prepend(err, "Error then adding ydb helm repository")
	}

	// Define the chart to be installed
	chartSpec := helmclient.ChartSpec{
		ReleaseName:      "ydb-operator",
		ChartName:        "ydb/operator",
		Namespace:        "stroppy",
		ValuesYaml:       "",
		Version:          "",
		CreateNamespace:  false,
		DisableHooks:     false,
		Replace:          false,
		Wait:             true,
		WaitForJobs:      false,
		DependencyUpdate: false,
		Timeout:          time.Duration(helmTimeout),
		GenerateName:     false,
		NameTemplate:     "",
		Atomic:           false,
		SkipCRDs:         false,
		UpgradeCRDs:      true,
		SubNotes:         false,
		Force:            false,
		ResetValues:      false,
		ReuseValues:      false,
		Recreate:         false,
		MaxHistory:       0,
		CleanupOnFail:    false,
		DryRun:           false,
	}

	if rel, err = client.InstallOrUpgradeChart(context.Background(), &chartSpec, nil); err != nil {
		return merry.Prepend(err, "Error then upgrading/installing ydb-operator release")
	}

	llog.Infof("Release '%s' with YDB operator successfully deployed", rel.Name)

	return nil
}

// Deploy YDB storage
// Parse manifest and deploy yandex db storage via kubectl.
//
//nolint:varnamelen // ok is typecasting boolean
func (yc *yandexCluster) deployStorage(shellState *state.State) error {
	var (
		err     error
		bytes   []byte
		storage map[interface{}]interface{}
	)

	mpath := path.Join(shellState.Settings.WorkingDirectory, databasesDir, yandexDirectory)

	if bytes, err = os.ReadFile(path.Join(mpath, "storage.yml")); err != nil {
		return merry.Prepend(err, "Error then reading file")
	}

	llog.Tracef("%v bytes read from storage.yml\n", len(bytes))

	if err = yaml.Unmarshal(bytes, &storage); err != nil {
		return merry.Prepend(err, "Error then deserizalizing storage manifest")
	}

	metadata, ok := storage["metadata"].(map[interface{}]interface{})
	if !ok {
		return merry.Prepend(err, castingError)
	}

	metadata["namespace"] = stroppyNamespaceName

	spec, ok := storage["spec"].(map[interface{}]interface{})
	if !ok {
		return merry.Prepend(err, castingError)
	}

	// TODO: get it from terraform.tfstate
	// https://github.com/picodata/stroppy/issues/94
	spec["nodes"] = 8
	spec["domain"] = "root"
	spec["erasure"] = erasureSpecies // TODO to constant

	var configuration string

	if configuration, ok = spec["configuration"].(string); !ok {
		return merry.Prepend(err, castingError)
	}

	if bytes, err = paramStorageConfig(configuration); err != nil {
		return merry.Prepend(err, "Error then parameterizing storage configuration")
	}

	spec["configuration"] = string(bytes)

	if bytes, err = yaml.Marshal(storage); err != nil {
		return merry.Prepend(err, "Error then serializing storage")
	}

	if err = os.WriteFile(
		path.Join(mpath, "stroppy-storage.yml"),
		bytes,
		fs.FileMode(roAll),
	); err != nil {
		return merry.Prepend(err, "Error then writing storage.yml")
	}

	return applyManifest(path.Join(mpath, "stroppy-storage.yml"))
}

// Deploy YDB database
// Parse manifest and deploy yandex db database via kubectl.
func (yc *yandexCluster) deployDatabase(shellState *state.State) error {
	var (
		err     error
		bytes   []byte
		storage map[interface{}]interface{}
	)

	mpath := path.Join(shellState.Settings.WorkingDirectory, databasesDir, yandexDirectory)

	bytes, err = os.ReadFile(path.Join(mpath, "database.yml"))
	if err != nil {
		return merry.Prepend(err, "Error then reading database.yml")
	}

	llog.Tracef("%v bytes read from database.yml\n", len(bytes))

	if err = yaml.Unmarshal(bytes, &storage); err != nil {
		return merry.Prepend(err, "Error then deserializing database manifest")
	}

	metadata, ok := storage["metadata"].(map[interface{}]interface{})
	if !ok {
		return merry.Prepend(err, castingError)
	}

	metadata["namespace"] = stroppyNamespaceName

	// TODO: get it from terraform.tfstate
	spec, ok := storage["spec"].(map[interface{}]interface{})
	if !ok {
		return merry.Prepend(err, castingError)
	}

	// TODO: replace based on tfstate resources
	// https://github.com/picodata/stroppy/issues/94
	spec["nodes"] = 1

	resources, ok := spec["resources"].(map[interface{}]interface{})
	if !ok {
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

	containerResources, ok := resources["containerResources"].(map[interface{}]interface{})
	if !ok {
		return merry.Prepend(err, castingError)
	}

	containerResources["limits"] = map[interface{}]interface{}{
		// TODO: replace to formula based on host resources
		// https://github.com/picodata/stroppy/issues/94
		// resources can fe fetched from terraform.tfstate
		"cpu": "100m",
	}

	if bytes, err = yaml.Marshal(storage); err != nil {
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

// Generate parameters for `storage` CRD.
func paramStorageConfig(storage string) ([]byte, error) {
	var (
		confMap map[interface{}]interface{}
		bytes   []byte
		err     error
	)

	if err = yaml.Unmarshal(
		[]byte(storage),
		&confMap,
	); err != nil {
		return nil, merry.Prepend(err, "Error then deserializing storage manifest")
	}

	// TODO: replace to config based on resources from terraform.tfstate
	// https://github.com/picodata/stroppy/issues/94
	hostConfigs, ok := confMap["host_configs"].([]interface{})
	if !ok {
		return nil, merry.Prepend(err, castingError)
	}

	hostConfigsFirst, ok := hostConfigs[0].(map[interface{}]interface{})
	if !ok {
		return nil, merry.Prepend(err, castingError)
	}

	drive, ok := hostConfigsFirst["drive"].([]interface{})
	if !ok {
		return nil, merry.Prepend(err, castingError)
	}

	drive[0] = map[interface{}]interface{}{
		"path": "/dev/kikimr_ssd_00",
		"type": "SSD",
	}

	// TODO: replace to config based on resources from terraform.tfstate
	// https://github.com/picodata/stroppy/issues/94
	domainsConfig, ok := confMap["domains_config"].(map[interface{}]interface{})
	if !ok {
		return nil, merry.Prepend(err, castingError)
	}

	stateStorage, ok := domainsConfig["state_storage"].([]interface{})
	if !ok {
		return nil, merry.Prepend(err, castingError)
	}

	stateStorage[0] = map[string]interface{}{
		"ring": map[string]interface{}{
			"node": []interface{}{
				1, 2, 3, 5, 6, 7, 8,
			},
			"nto_select": 5, //nolint // nto_select is parameter for nto
		},
		"ssid": 1,
	}

	// TODO: replace to config based on resources from terraform.tfstate
	// https://github.com/picodata/stroppy/issues/94
	blobStorageConfig, ok := confMap["blob_storage_config"].(map[interface{}]interface{})
	if !ok {
		return nil, merry.Prepend(err, castingError)
	}

	serviceSet, ok := blobStorageConfig["service_set"].(map[interface{}]interface{})
	if !ok {
		return nil, merry.Prepend(err, castingError)
	}

	failDomains := map[string]interface{}{
		"fail_domains": []interface{}{
			map[string]interface{}{
				"vdisk_locations": []interface{}{
					map[string]interface{}{
						"node_id":        1,
						"path":           "/dev/kikimr_ssd_00",
						"pdisk_category": "SSD",
					},
				},
			},
			map[string]interface{}{
				"vdisk_locations": []interface{}{
					map[string]interface{}{
						"node_id":        2, //nolint
						"path":           "/dev/kikimr_ssd_00",
						"pdisk_category": "SSD",
					},
				},
			},
			map[string]interface{}{
				"vdisk_locations": []interface{}{
					map[string]interface{}{
						"node_id":        3, //nolint
						"path":           "/dev/kikimr_ssd_00",
						"pdisk_category": "SSD",
					},
				},
			},
			map[string]interface{}{
				"vdisk_locations": []interface{}{
					map[string]interface{}{
						"node_id":        4, //nolint
						"path":           "/dev/kikimr_ssd_00",
						"pdisk_category": "SSD",
					},
				},
			},
			map[string]interface{}{
				"vdisk_locations": []interface{}{
					map[string]interface{}{
						"node_id":        5, //nolint
						"path":           "/dev/kikimr_ssd_00",
						"pdisk_category": "SSD",
					},
				},
			},
			map[string]interface{}{
				"vdisk_locations": []interface{}{
					map[string]interface{}{
						"node_id":        6, //nolint
						"path":           "/dev/kikimr_ssd_00",
						"pdisk_category": "SSD",
					},
				},
			},
			map[string]interface{}{
				"vdisk_locations": []interface{}{
					map[string]interface{}{
						"node_id":        7, //nolint
						"path":           "/dev/kikimr_ssd_00",
						"pdisk_category": "SSD",
					},
				},
			},
			map[string]interface{}{
				"vdisk_locations": []interface{}{
					map[string]interface{}{
						"node_id":        8, //nolint
						"path":           "/dev/kikimr_ssd_00",
						"pdisk_category": "SSD",
					},
				},
			},
		},
	}

	serviceSet["groups"] = []interface{}{
		map[string]interface{}{
			"erasure_species": erasureSpecies,
			"rings": []interface{}{
				failDomains,
			},
		},
	}

	// TODO: replace to config based on resources from terraform.tfstate
	// https://github.com/picodata/stroppy/issues/94
	chProfileConfig, ok := confMap["channel_profile_config"].(map[interface{}]interface{})
	if !ok {
		return nil, merry.Prepend(err, castingError)
	}

	profile, ok := chProfileConfig["profile"].([]interface{})
	if !ok {
		return nil, merry.Prepend(err, castingError)
	}

	profile[0] = map[string]interface{}{
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
	}

	if bytes, err = yaml.Marshal(confMap); err != nil {
		return []byte{}, merry.Prepend(err, "Error then serializing storage configuration")
	}

	return bytes, nil
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
		yc.commonCluster.connectionPoolSize,
	); err != nil {
		return nil, merry.Prepend(err, "Error then creating new YDB cluster")
	}

	llog.Debugln("Connection to YDB successfully created")

	return connection, nil
}
