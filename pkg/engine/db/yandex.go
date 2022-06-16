package db

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"time"

	"github.com/ansel1/merry"
	helmclient "github.com/mittwald/go-helm-client"
	llog "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
	"gitlab.com/picodata/stroppy/pkg/database/cluster"
	engineSsh "gitlab.com/picodata/stroppy/pkg/engine/ssh"
	"gitlab.com/picodata/stroppy/pkg/kubernetes"
	"gopkg.in/yaml.v2"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/repo"
)

const (
	databasesDir    string = "third_party/extra/manifests/databases"
	yandexDirectory string = "yandexdb"
	ydbHelmRepo     string = "https://charts.ydb.tech"
	timeout         int    = 300
	step            int    = 20
)

type yandexCluster struct {
	wd            string
	commonCluster *commonCluster
}

func createYandexDBCluster(
	sc engineSsh.Client,
	k *kubernetes.Kubernetes,
	wd string,
	dbURL string,
	connectionPoolSize int,
) (yandex Cluster) {
	yandex = &yandexCluster{
		commonCluster: createCommonCluster(
			sc,
			k,
			filepath.Join(wd, dbWorkingDirectory, yandexDirectory),
			yandexDirectory,
			dbURL,
			connectionPoolSize,
			false,
		),
	}

	return
}

// Deploy Yandex Database Cluster
// This realization completely different then another when deploying,
// the helm operator will be connected first. The operator is always
// installed from the Yandex repository. Then the store and database
// manifests will be deserialized and parameterized.
func (yc *yandexCluster) Deploy() error {
	var err error

	if err = yc.deployYandexDBOperator(); err != nil {
		return merry.Prepend(err, "Error then deploying ydb operator")
	}

	if _, err = yc.deployStorage(); err != nil {
		return merry.Prepend(err, "Error then deploying storage")
	}

	if err = waitObjectReady(
		path.Join(yc.wd, databasesDir, yandexDirectory, "stroppy-storage.yml"),
		"storage",
	); err != nil {
		return merry.Prepend(err, "Error then waiting YDB storage")
	}
	if _, err = yc.deployDatabase(); err != nil {
		return merry.Prepend(err, "Error then deploying storage")
	}

	if err = waitObjectReady(
		path.Join(yc.wd, databasesDir, yandexDirectory, "stroppy-database.yml"),
		"database",
	); err != nil {
		return merry.Prepend(err, "Error then waiting YDB database")
	}

	return err
}

// Get clusterSpec.
func (yc *yandexCluster) GetSpecification() ClusterSpec {
	return yc.commonCluster.clusterSpec
}

// Deploy yandex db operator via helmclient library.
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
			Namespace:        "default", //nolint //TODO: Change this to the namespace you wish the client to operate in.
			RepositoryCache:  "/tmp/.helmcache",
			RepositoryConfig: "/tmp/.helmrepo",
			Debug:            true,
			Linting:          true,
			DebugLog:         func(format string, v ...interface{}) {},
		},
		KubeContext: "",
		KubeConfig:  kubeconfig,
	}

	if client, err = helmclient.NewClientFromKubeConf(options); err != nil {
		return merry.Prepend(err, "Error then creating helm client")
	}

	// Add YandexDB helm repository
	chartRepo := repo.Entry{
		Name: "ydb",
		URL:  ydbHelmRepo,
	}

	// Add a chart-repository to the client.
	if err = client.AddOrUpdateChartRepo(chartRepo); err != nil {
		return merry.Prepend(err, "Error then adding ydb helm repository")
	}

	// Define the chart to be installed
	chartSpec := helmclient.ChartSpec{
		ReleaseName: "ydb-operator",
		ChartName:   "ydb/operator",
		Namespace:   "default",
		UpgradeCRDs: true,
		Wait:        true,
		Timeout:     time.Duration(time.Duration(5 * time.Minute)),
	}

	if rel, err = client.InstallOrUpgradeChart(context.Background(), &chartSpec, nil); err != nil {
		return merry.Prepend(err, "Error then upgrading/installing ydb-operator release")
	}
	llog.Infof("Release '%s' with YDB operator successfully deployed", rel.Name)

	return err
}

// Deploy YDB storage
// Parse manifest and deploy yandex db storage via kubectl.
func (yc *yandexCluster) deployStorage() ([]byte, error) {
	var err error
	var bytes []byte
	var storage map[interface{}]interface{}

	mpath := path.Join(yc.wd, databasesDir, yandexDirectory)

	bytes, err = os.ReadFile(path.Join(mpath, "storage.yml"))
	if err != nil {
		return []byte{}, merry.Prepend(err, "Error then reading file")
	}
	llog.Tracef("%v bytes read from storage.yml\n", len(bytes))

	if err = yaml.Unmarshal(bytes, &storage); err != nil {
		return []byte{}, merry.Prepend(err, "Error then deserizalizing storage manifest")
	}
	spec := storage["spec"].(map[interface{}]interface{})
	spec["resources"].(map[interface{}]interface{})["limits"] = map[string]interface{}{
		"cpu":    "1000m",  //nolint //TODO: replace to formula based on host resources #issue94
		"memory": "2048Mi", //  resources can fe fetched from terraform.tfstate
	}
	spec["nodes"] = 4 //nolint //TODO: get it from terraform.tfstate #issue94

	if bytes, err = paramStorageConfig(storage); err != nil {
		return []byte{}, merry.Prepend(err, "Error then parameterizing storage configuration")
	}

	storage["spec"].(map[interface{}]interface{})["configuration"] = string(bytes)

	if bytes, err = yaml.Marshal(storage); err != nil {
		return []byte{}, merry.Prepend(err, "Error then serializing storage")
	}

	if err = os.WriteFile(path.Join(mpath, "stroppy-storage.yml"), bytes, 0644); err != nil {
		return []byte{}, merry.Prepend(err, "Error then writing storage.yml")
	}

	return applyManifest(path.Join(mpath, "stroppy-storage.yml"))
}

// Deploy YDB database
// Parse manifest and deploy yandex db database via kubectl.
func (yc *yandexCluster) deployDatabase() ([]byte, error) {
	var err error
	var bytes []byte
	var storage map[interface{}]interface{}

	mpath := path.Join(yc.wd, databasesDir, yandexDirectory)

	bytes, err = os.ReadFile(path.Join(mpath, "database.yml"))
	if err != nil {
		return []byte{}, merry.Prepend(err, "Error then reading database.yml")
	}
	llog.Tracef("%v bytes read from database.yml\n", len(bytes))

	if err = yaml.Unmarshal(bytes, &storage); err != nil {
		return []byte{}, merry.Prepend(err, "Error then deserializing database manifest")
	}

	//nolint //TODO: get it from terraform.tfstate
	storage["spec"].(map[interface{}]interface{})["nodes"] = 1

	resources := storage["spec"].(map[interface{}]interface{})["resources"]
	resources.(map[interface{}]interface{})["storageUnits"] = []interface{}{
		map[string]interface{}{
			"count":    1,     //nolint //TODO: replace to formula based on host resources #issue94
			"unitKind": "ssd", //  resources can fe fetched from terraform.tfstate
		},
	}

	containerResources := resources.(map[interface{}]interface{})["containerResources"]
	containerResources.(map[interface{}]interface{})["limits"] = map[interface{}]interface{}{
		"cpu":    "500m",  //nolint //TODO: replace to formula based on host resources #issue94
		"memory": "512Mi", // resources can fe fetched from terraform.tfstate
	}

	if bytes, err = yaml.Marshal(storage); err != nil {
		return []byte{}, merry.Prepend(err, "Error then serializing database.yml")
	}

	if err = os.WriteFile(path.Join(mpath, "stroppy-database.yml"), bytes, 0644); err != nil {
		return []byte{}, merry.Prepend(err, "Error then writing database.yml")
	}
	return applyManifest(path.Join(mpath, "stroppy-database.yml"))
}

// Run kubectl and apply manifest.
func applyManifest(manifestName string) ([]byte, error) {
	var err error
	var cmd *exec.Cmd
	var stdout []byte
	cmd = exec.Command("kubectl", "apply", "-f", manifestName, "--output", "json")
	if stdout, err = cmd.Output(); err != nil {
		return stdout, merry.Prepend(
			err,
			fmt.Sprintf("Error then applying %s manifest", manifestName),
		)
	}
	llog.Debugf("Manifest %s succesefully applyed", manifestName)
	return stdout, err
}

// Run `n` times `kubectl get -f path` until the `Ready` status is received.
func waitObjectReady(path string, name string) error {
	var err error
	var output []byte

	for index := 0; index <= timeout; index += step {
		cmd := exec.Command("kubectl", "get", "-f", path, "--output", "json")
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
	// to be able to connect to the cluster from localhost
	//nolint //TODO: Replace to right YandexDB url and add connection to database issue95
	if yc.commonCluster.DBUrl == "" {
		yc.commonCluster.DBUrl = "ydb://stroppy:stroppy@localhost:2135/stroppy?sslmode=disable"
		llog.Infoln("changed DBURL on", yc.commonCluster.DBUrl)
	}
	connetion, err := cluster.NewYandexDBCluster(
		yc.commonCluster.DBUrl,
		yc.commonCluster.connectionPoolSize,
	)
	return connetion, err
}

func paramStorageConfig(storage map[interface{}]interface{}) ([]byte, error) {
	var (
		configuration map[interface{}]interface{}
		bytes         []byte
		err           error
	)

	if err = yaml.Unmarshal(
		[]byte(storage["spec"].(map[interface{}]interface{})["configuration"].(string)),
		&configuration,
	); err != nil {
		return []byte{}, merry.Prepend(err, "Error then deserializing storage manifest")
	}

	//nolint //TODO: replace to config based on resources from terraform.tfstate #issue94
	drive := configuration["host_configs"].([]interface{})[0]
	drive.(map[interface{}]interface{})["drive"].([]interface{})[0] = map[interface{}]interface{}{
		"path": "SectorMap:1:64",
		"type": "SSD",
	}

	//nolint //TODO: replace to config based on resources from terraform.tfstate #issue94
	ring := configuration["domains_config"].(map[interface{}]interface{})["state_storage"]
	ring.([]interface{})[0] = map[string]interface{}{
		"ring": map[string]interface{}{
			"node": []interface{}{
				1,
			},
			"nto_select": 1,
		},
		"ssid": 1,
	}

	//nolint //TODO: replace to config based on resources from terraform.tfstate #issue94
	serviceSet := configuration["blob_storage_config"].(map[interface{}]interface{})["service_set"]
	serviceSet.(map[interface{}]interface{})["groups"] = []interface{}{
		map[string]interface{}{
			"erasure_species": "none",
			"rings": []interface{}{
				map[string]interface{}{
					"fail_domains": []interface{}{
						map[string]interface{}{
							"vdisk_locations": []interface{}{
								map[string]interface{}{
									"node_id":        1,
									"path":           "SectorMap:1:64",
									"pdisk_category": "SSD",
								},
							},
						},
					},
				},
			},
		},
	}
	//nolint //TODO: replace to config based on resources from terraform.tfstate #issue94
	profile := configuration["channel_profile_config"].(map[interface{}]interface{})["profile"]
	profile.([]interface{})[0] = map[string]interface{}{
		"channel": []interface{}{
			map[string]interface{}{
				"erasure_species":   "none",
				"pdisk_category":    1,
				"storage_pool_kind": "ssd",
			},
			map[string]interface{}{
				"erasure_species":   "none",
				"pdisk_category":    1,
				"storage_pool_kind": "ssd",
			},
			map[string]interface{}{
				"erasure_species":   "none",
				"pdisk_category":    1,
				"storage_pool_kind": "ssd",
			},
		},
	}

	if bytes, err = yaml.Marshal(configuration); err != nil {
		return []byte{}, merry.Prepend(err, "Error then serializing storage configuration")
	}

	return bytes, nil
}
