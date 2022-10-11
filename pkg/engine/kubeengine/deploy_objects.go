/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package kubeengine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"reflect"
	"strings"
	"time"

	"gitlab.com/picodata/stroppy/pkg/state"
	"gitlab.com/picodata/stroppy/pkg/tools"
	"helm.sh/helm/v3/pkg/repo"

	"github.com/ansel1/merry"
	helmclient "github.com/mittwald/go-helm-client"
	llog "github.com/sirupsen/logrus"
	goYaml "gopkg.in/yaml.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	k8sYaml "sigs.k8s.io/yaml"
)

const lokiPort int = 3100

// Accept path to manifest file, read it to bytes and try to cast into `objectType`
// objectType should match one of the kubernetes Kind eg Pod, Deployment, Namespace, etc.
func (k8sEngine *Engine) ToEngineObject(
	manifestPath string,
	objectType interface{},
) error {
	var (
		err             error
		objectFileBytes []byte
	)

	if objectFileBytes, err = os.ReadFile(manifestPath); err != nil {
		return merry.Prepend(err, "failed to read config file for deploy stroppy")
	}

	if err = k8sYaml.Unmarshal(objectFileBytes, objectType); err != nil {
		return merry.Prepend(
			err,
			fmt.Sprintf(
				"failed to unmarshall object to kind %s configuration",
				reflect.TypeOf(objectType),
			),
		)
	}

	return nil
}

// Accept context and function for deploying some kubernetes Kind
// This function is intended for objects whose readiness should not be expected.
// Such as namespaces, configmaps, service accounts, etc.
func (k8sEngine *Engine) DeployObject(
	deployContext context.Context,
	objectApplyFunc func(*kubernetes.Clientset) error,
) error {
	var (
		err       error
		clientSet *kubernetes.Clientset
	)

	if clientSet, err = k8sEngine.GetClientSet(); err != nil {
		return merry.Prepend(err, "failed to get clientset for stroppy secret")
	}

	if err = objectApplyFunc(clientSet); err != nil {
		return merry.Prepend(err, "failed to deploy")
	}

	return nil
}

// Accept context and two functions for deploying object with wait loop
// The second function is designed to delete an object if it does not reach
// the status of 'Ready' in the allowed time.
func (k8sEngine *Engine) DeployAndWaitObject(
	deployContext context.Context,
	objectName, objectType string,
	objectApplyFunc func(*kubernetes.Clientset) error,
	objectDeleteFunc func(*kubernetes.Clientset) error,
) error {
	var (
		err       error
		clientSet *kubernetes.Clientset
	)

	llog.Infof(
		"Start deploying new %s object %s with %d attempts",
		objectType,
		objectName,
		tools.RetryStandardRetryCount,
	)

	if clientSet, err = k8sEngine.GetClientSet(); err != nil {
		return merry.Prepend(err, "failed to get clientset for stroppy secret")
	}

	if err = tools.Retry(
		fmt.Sprintf("deploy %s object %s", objectType, objectName),
		func() error {
			if err = objectApplyFunc(clientSet); err != nil {
				return merry.Prepend(
					err,
					fmt.Sprintf("error then waiting ready status for object %s", objectName),
				)
			}

			return nil
		},
		tools.RetryStandardRetryCount,
		tools.RetryStandardWaitingTime,
	); err != nil {
		if deleteErr := objectDeleteFunc(clientSet); deleteErr != nil {
			return merry.Prepend(
				deleteErr,
				fmt.Errorf(
					"failed to deploy and delete k8s object %s: %w", objectName, err,
				).Error())
		}

		return merry.Prepend(err, "failed to retry deploing k8s object")
	}

	llog.Infof("Object %s successefully deployed", objectName)

	return nil
}

// Returns `metav1.ApplyOptions` with default parametres
// `FieldManager` equals stroppy.
func (*Engine) GenerateDefaultMetav1() metav1.ApplyOptions {
	return metav1.ApplyOptions{
		TypeMeta: metav1.TypeMeta{
			Kind:       "",
			APIVersion: "",
		},
		DryRun:       []string{},
		Force:        false,
		FieldManager: "stroppy",
	}
}

// Returns `metav1.DeleteOptions` with default parametres
// `PropagationPolicy` equals "background".
func (*Engine) GenerateDefaultDeleteOptions() metav1.DeleteOptions {
	propagationPolicy := metav1.DeletePropagationBackground

	return metav1.DeleteOptions{
		TypeMeta: metav1.TypeMeta{
			Kind:       "",
			APIVersion: "",
		},
		GracePeriodSeconds: new(int64),
		Preconditions:      &metav1.Preconditions{}, //nolint
		OrphanDependents:   new(bool),
		PropagationPolicy:  &propagationPolicy,
		DryRun:             []string{},
	}
}

type InstallOptions struct {
	ChartName      string
	ChartVersion   string
	ChartNamespace string
	ReleaseName    string
	RepositoryURL  string
	RepositoryName string
	ValuesYaml     string
	Timeout        time.Duration
}

func (k8sEngine *Engine) DeployChart(
	installOptions *InstallOptions, shellState *state.State,
) error {
	var (
		err        error
		client     helmclient.Client
		debug      bool
		kubeConfig []byte
	)

	switch strings.ToLower(shellState.Settings.LogLevel) {
	case "trace", "debug":
		debug = true
	case "info", "warn", "error":
		debug = false
	}

	if kubeConfig, err = os.ReadFile(k8sEngine.clusterConfigFile); err != nil {
		return merry.Prepend(err, "failed to read kubeconfig file")
	}

	options := &helmclient.KubeConfClientOptions{ //nolint
		Options: &helmclient.Options{ //nolint
			Namespace: installOptions.ChartNamespace,
			Debug:     debug,
		},
		KubeConfig: kubeConfig,
	}

	if client, err = helmclient.NewClientFromKubeConf(options); err != nil {
		return merry.Prepend(err, "Error then creating helm client")
	}

	// Add YandexDB helm repository
	chartRepo := repo.Entry{ //nolint
		Name: installOptions.RepositoryName,
		URL:  installOptions.RepositoryURL,
	}

	// Add a chart-repository to the client.
	if err = client.AddOrUpdateChartRepo(chartRepo); err != nil {
		return merry.Prepend(err, "Error then adding ydb helm repository")
	}

	if installOptions.Timeout == 0 {
		installOptions.Timeout = time.Minute * 5 //nolint
	}

	// Define the chart to be installed
	chartSpec := helmclient.ChartSpec{ //nolint
		ReleaseName: installOptions.ReleaseName,
		ChartName:   installOptions.ChartName,
		Namespace:   installOptions.ChartNamespace,
		ValuesYaml:  installOptions.ValuesYaml,
		Timeout:     installOptions.Timeout,
		Atomic:      true,
		UpgradeCRDs: true,
		MaxHistory:  5, //nolint
	}

	if _, err = client.InstallOrUpgradeChart(context.Background(), &chartSpec, nil); err != nil {
		return merry.Prepend(
			err,
			fmt.Sprintf("Error then upgrading/installing %s release", installOptions.ReleaseName),
		)
	}

	llog.Infof("Helm chart '%s' deploy: success", installOptions.ChartName)

	return nil
}

var errWrongValues = errors.New("failed to cast map")

func GetPromtailValues(shellState *state.State) ([]byte, error) {
	var (
		bytes  []byte
		err    error
		values map[string]interface{}
	)

	if bytes, err = os.ReadFile(path.Join(
		shellState.Settings.WorkingDirectory,
		"third_party",
		"monitoring",
		"promtail-values-tpl.yml",
	)); err != nil {
		return nil, merry.Prepend(err, "failed to open promtail values")
	}

	if err = k8sYaml.Unmarshal(bytes, &values); err != nil {
		return nil, merry.Prepend(err, "failed to deserialize values")
	}

	config, success := values["config"].(map[string]interface{})
	if !success {
		return nil, errWrongValues
	}

	config["clients"] = []interface{}{
		map[string]string{
			"url": fmt.Sprintf( //nolint
				"http://%s:%d/loki/api/v1/push",
				shellState.NodesInfo.IPs.FirstMasterIP.Internal,
				lokiPort,
			),
		},
	}

	if bytes, err = goYaml.Marshal(&values); err != nil {
		return nil, merry.Prepend(err, "failed to serialize values")
	}

	return bytes, nil
}

func GetPrometheusValues(shellState *state.State) ([]byte, error) {
	var (
		bytes  []byte
		err    error
		values map[string]interface{}
	)

	if bytes, err = os.ReadFile(path.Join(
		shellState.Settings.WorkingDirectory,
		"third_party",
		"monitoring",
		"prometheus-values-tpl.yml",
	)); err != nil {
		return nil, merry.Prepend(err, "failed to open prometheus values template")
	}

	if err = k8sYaml.Unmarshal(bytes, &values); err != nil {
		return nil, merry.Prepend(err, "failed to deserialize values")
	}

	if bytes, err = goYaml.Marshal(&values); err != nil {
		return nil, merry.Prepend(err, "failed to serialize values")
	}

	return bytes, nil
}

func GetIngressValues(shellState *state.State) ([]byte, error) {
	var (
		bytes  []byte
		err    error
		values map[string]interface{}
	)

	if bytes, err = os.ReadFile(path.Join(
		shellState.Settings.WorkingDirectory,
		"third_party",
		"extra",
		"values",
		"ingress-nginx-values-tpl.yml",
	)); err != nil {
		return nil, merry.Prepend(err, "failed to open nginx-ingress values")
	}

	if err = k8sYaml.Unmarshal(bytes, &values); err != nil {
		return nil, merry.Prepend(err, "failed to deserialize values")
	}

	controller, success := values["controller"].(map[string]interface{})
	if !success {
		return nil, merry.Prepend(errWrongValues, "failed to get controller block")
	}

	service, success := controller["service"].(map[string]interface{})
	if !success {
		return nil, merry.Prepend(errWrongValues, "failed to het service block")
	}

	service["nodePort"] = []interface{}{
		map[interface{}]interface{}{
			"http":  shellState.Settings.DeploymentSettings.PromPort,
			"https": shellState.Settings.DeploymentSettings.PromSPort,
		},
	}

	if bytes, err = goYaml.Marshal(&values); err != nil {
		return nil, merry.Prepend(err, "failed to serialize values")
	}

	return bytes, nil
}
