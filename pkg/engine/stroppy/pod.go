/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package stroppy

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
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

	if contCount := len(contList); contCount < containerNum {
		err = fmt.Errorf("container num %d does not exists: %d", containerNum, contCount)

		return
	}

	contName = contList[containerNum].Name

	return
}

func (pod *Pod) Deploy() (err error) {
	var clientSet *kubernetes.Clientset

	if clientSet, err = pod.e.GetClientSet(); err != nil {
		return merry.Prepend(err, "failed to get clientset for stroppy secret")
	}

	if err = pod.prepareDeploy(clientSet); err != nil {
		return
	}

	deployConfigStroppyPath := filepath.Join(pod.e.WorkingDirectory, "cluster", deployConfigFile)

	var deployConfigBytes []byte

	if deployConfigBytes, err = ioutil.ReadFile(deployConfigStroppyPath); err != nil {
		return merry.Prepend(err, "failed to read config file for deploy stroppy")
	}

	stroppyPodConfig := applyconfig.Pod(PodName, engine.ResourceDefaultNamespace)
	if err = yaml.Unmarshal(deployConfigBytes, &stroppyPodConfig); err != nil {
		return merry.Prepend(err, "failed to unmarshall deploy stroppy configuration")
	}

	createPod := func() (err error) {
		pod.internalPod, err = clientSet.CoreV1().
			Pods(engine.ResourceDefaultNamespace).
			Apply(context.TODO(),
				stroppyPodConfig,
				metav1.ApplyOptions{
					TypeMeta: metav1.TypeMeta{
						Kind:       "",
						APIVersion: "",
					},
					DryRun:       []string{},
					Force:        false,
					FieldManager: fieldManagerName,
				})
		if err != nil {
			err = fmt.Errorf("failed to create stroppy pod: %v", err)
		}

		return
	}

	deletePod := func() (err error) {
		err = clientSet.CoreV1().
			Pods(engine.ResourceDefaultNamespace).
			Delete(context.TODO(), PodName, metav1.DeleteOptions{})
		if err != nil {
			err = fmt.Errorf("failed to delete stroppy pod: %v", err)
			llog.Warn(err)
		}

		return
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

	// на случай чуть большего времени на переход в running, ожидаем 5 минут, если не запустился - возвращаем ошибку
	if pod.internalPod.Status.Phase != v1.PodRunning {
		pod.internalPod, err = pod.e.WaitPod(PodName, engine.ResourceDefaultNamespace,
			engine.PodWaitingNotWaitCreation, engine.PodWaitingTime)
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

	return
}

func (pod *Pod) prepareDeploy(clientSet *kubernetes.Clientset) (err error) {
	llog.Infoln("Preparing of stroppy pod deploy")

	if err = pod.e.ExecuteCommand(dockerRepLoginCmd); err != nil {
		return merry.Prepend(err, "failed to login in prvivate repository")
	}

	llog.Infoln("logging in private repository: success")

	secretFilePath := filepath.Join(pod.e.WorkingDirectory, "cluster", secretFile)

	var secretFile []byte

	if secretFile, err = ioutil.ReadFile(secretFilePath); err != nil {
		return merry.Prepend(err, "failed to read config file for stroppy secret")
	}

	secret := applyconfig.Secret("stroppy-secret", "default")

	// используем github.com/ghodss/yaml, т.к она поддерживает работу с зашифрованными строками
	if err = yaml.Unmarshal(secretFile, &secret); err != nil {
		return merry.Prepend(err, "failed to unmarshall stroppy secret configuration")
	}

	llog.Infoln("Applying the stroppy secret...")

	err = tools.Retry("apply stroppy secret",
		func() (err error) {
			_, err = clientSet.CoreV1().Secrets("default").Apply(context.TODO(), secret, metav1.ApplyOptions{
				TypeMeta:     metav1.TypeMeta{},
				DryRun:       []string{},
				Force:        false,
				FieldManager: fieldManagerName,
			})

			return
		},
		tools.RetryStandardRetryCount,
		tools.RetryStandardWaitingTime)
	if err != nil {
		return merry.Prepend(err, "failed to apply stroppy secret")
	}

	llog.Infoln("applying of k8s secret: success")

	return
}
