/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package stroppy

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ansel1/merry"
	"github.com/ghodss/yaml"
	llog "github.com/sirupsen/logrus"
	engine "gitlab.com/picodata/stroppy/pkg/engine/kubeengine"
	"gitlab.com/picodata/stroppy/pkg/tools"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	applyconfig "k8s.io/client-go/applyconfigurations/core/v1"
	"k8s.io/client-go/kubernetes"
)

func CreateStroppyPod(e *engine.Engine) (pod *Pod) {
	pod = &Pod{
		e: e,
	}
	return
}

type Pod struct {
	internalPod *v1.Pod
	e           *engine.Engine
}

func (pod *Pod) Name() (name string) {
	name = pod.internalPod.Name
	return
}

func (pod *Pod) ContainerName(containerNum int) (contName string, err error) {
	contList := pod.internalPod.Spec.Containers
	if contList == nil {
		err = errors.New("not initialized")
		return
	}

	contCount := len(contList)
	if contCount < containerNum {
		err = fmt.Errorf("container num %d does not exists: %d", containerNum, contCount)
		return
	}

	contName = contList[containerNum].Name
	return
}

func (pod *Pod) DeployNamespace() error {
	var (
		err       error
		clientSet *kubernetes.Clientset
	)

	if clientSet, err = pod.e.GetClientSet(); err != nil {
		return merry.Prepend(err, "failed to get clientset for stroppy secret")
	}

	deployContext, cancel := context.WithCancel(context.Background())

	namespaceFilePath := filepath.Join(
		pod.e.WorkingDirectory,
		"third_party", "extra", "manifests", "stroppy",
		namespaceFile,
	)

	var namepaceFileBytes []byte

	if namepaceFileBytes, err = os.ReadFile(namespaceFilePath); err != nil {
		cancel()

		return merry.Prepend(err, "failed to read namespace manifest for stroppy")
	}

	stroppyNamespaceConfig := applyconfig.Namespace(NamespaceName)

	if err = yaml.Unmarshal(namepaceFileBytes, &stroppyNamespaceConfig); err != nil {
		cancel()

		return merry.Prepend(err, "failed to unmarshall stroppy namespace manifest")
	}

	var stroppyNamespace *v1.Namespace

	if stroppyNamespace, err = clientSet.CoreV1().Namespaces().Apply(
		deployContext,
		stroppyNamespaceConfig,
		metav1.ApplyOptions{
			TypeMeta: metav1.TypeMeta{
				Kind:       "",
				APIVersion: "",
			},
			DryRun:       []string{},
			Force:        false,
			FieldManager: fieldManagerName,
		},
	); err != nil {
		cancel()

		return merry.Prepend(err, fmt.Sprintf("Error then creating namespace %s", NamespaceName))
	}

	llog.Debugf("Namespace %s status %v", NamespaceName, stroppyNamespace.Status.Phase)
	llog.Infof("Applying stroppy namespace '%s': success", NamespaceName)

	cancel()

	return nil
}

//nolint // TODO: will be fixed in future funlen
func (pod *Pod) DeployPod() error {
	var (
		err       error
		clientSet *kubernetes.Clientset
	)

	if clientSet, err = pod.e.GetClientSet(); err != nil {
		return merry.Prepend(err, "failed to get clientset for stroppy secret")
	}

	deployContext, cancel := context.WithCancel(context.Background())

	deploymentFilePath := filepath.Join(
		pod.e.WorkingDirectory,
		"third_party", "extra", "manifests", "stroppy",
		deploymentFile,
	)

	var deployConfigBytes []byte

	if deployConfigBytes, err = os.ReadFile(deploymentFilePath); err != nil {
		cancel()

		return merry.Prepend(err, "failed to read config file for deploy stroppy")
	}

	stroppyPodConfig := applyconfig.Pod(PodName, engine.ResourceDefaultNamespace)
	if err = yaml.Unmarshal(deployConfigBytes, &stroppyPodConfig); err != nil {
		cancel()

		return merry.Prepend(err, "failed to unmarshall deploy stroppy configuration")
	}

	createPod := func() (err error) {
		if pod.internalPod, err = clientSet.CoreV1().
			Pods(engine.ResourceDefaultNamespace).
			Apply(deployContext,
				stroppyPodConfig,
				metav1.ApplyOptions{
					TypeMeta: metav1.TypeMeta{
						Kind:       "",
						APIVersion: "",
					},
					DryRun:       []string{},
					Force:        false,
					FieldManager: fieldManagerName,
				}); err != nil {
			cancel()

			return merry.Prepend(err, "failed to create stroppy pod: %v")
		}

		return nil
	}

	deletePod := func() (err error) {
		if err = clientSet.CoreV1().
			Pods(engine.ResourceDefaultNamespace).
			Delete(deployContext, PodName, metav1.DeleteOptions{}); err != nil { //nolint
			err = fmt.Errorf("failed to delete stroppy pod: %v", err)
			llog.Warn(err)
		}

		return nil
	}

	llog.Infoln("Applying stroppy pod...")
	err = tools.Retry("deploy stroppy pod",
		func() (err error) {
			if err = createPod(); err != nil {
				return
			}

			const podImagePullBackOff = "ImagePullBackOff"
			if pod.internalPod.Status.Phase == podImagePullBackOff {
				_ = deletePod()
				err = fmt.Errorf("stroppy pod '%s' in status '%s'",
					pod.internalPod.Name, podImagePullBackOff)
			}

			return
		},

		tools.RetryStandardRetryCount,
		tools.RetryStandardWaitingTime)
	if err != nil {
		return err
	}

	// на случай чуть большего времени на переход в running, ожидаем 5 минут, 
    // если не запустился - возвращаем ошибку
	if pod.internalPod.Status.Phase != v1.PodRunning {
		pod.internalPod, err = pod.e.WaitPod(PodName, engine.ResourceDefaultNamespace,
			engine.PodWaitingNotWaitCreation, engine.PodWaitingTimeTenMinutes)
		if err != nil {
			retryErr := tools.Retry("stroppy pod recreation",
				func() (err error) {
					time.Sleep(5 * time.Second)
					if err = deletePod(); err != nil {
						return
					}

					time.Sleep(5 * time.Second)
					err = createPod()
					return
				},

				tools.RetryStandardRetryCount,
				tools.RetryStandardWaitingTime)
			if retryErr != nil {
				return merry.Prepend(err, "stroppy pod running status check")
			}
		}
	}

	llog.Infoln("Applying the stroppy pod: success")

	cancel()

	return nil
}
