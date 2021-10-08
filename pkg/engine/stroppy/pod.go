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

	stroppy := applyconfig.Pod(PodName, engine.ResourceDefaultNamespace)

	if err = yaml.Unmarshal(deployConfigBytes, &stroppy); err != nil {
		return merry.Prepend(err, "failed to unmarshall deploy stroppy configuration")
	}

	llog.Infoln("Applying stroppy pod...")
	err = tools.Retry("deploy stroppy pod",
		func() (err error) {
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
			engine.PodWaitingNotWaitCreation, engine.PodWaitingTimeTenMinutes)
		if err != nil {
			retryErr := tools.Retry("stroppy pod recreation",
				func() (err error) {
					// x := pod.e.GetClientSet()
					return
				},
				tools.RetryStandardRetryCount,
				tools.RetryStandardWaitingTime)
			if retryErr != nil {
				err = merry.Errorf("")
			}
			return merry.Prepend(err, "stroppy pod running status")
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