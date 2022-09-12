/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package stroppy

import (
	"context"
	"errors"
	"fmt"
	"path"

	engine "gitlab.com/picodata/stroppy/pkg/engine/kubeengine"
	"gitlab.com/picodata/stroppy/pkg/state"

	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
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

func (pod *Pod) DeployNamespace(shellState *state.State) error {
	var err error

	stroppyNSConfig := applyconfig.Namespace(StroppyClientNSName)

	if err = pod.e.ToEngineObject(
		StroppyClientNSName,
		path.Join(
			shellState.Settings.WorkingDirectory,
			"third_party", "extra", "manifests", "stroppy",
			stroppyClientNSManifestFile,
		),
		stroppyNSConfig,
	); err != nil {
		return merry.Prepend(err, "failed to cast to k8s engine object")
	}

	deployContext, cancel := context.WithCancel(context.Background())

	defer cancel()

	var namespaceResult *v1.Namespace

	objectApplyFunc := func(clientSet *kubernetes.Clientset) error {
		if namespaceResult, err = clientSet.CoreV1().Namespaces().Apply(
			deployContext,
			stroppyNSConfig,
			pod.e.GenerateDefaultMetav1(),
		); err != nil {
			return merry.Prepend(err, "Error then executing namespace creation")
		}

		return nil
	}

	if err = pod.e.DeployObject(deployContext, objectApplyFunc); err != nil {
		return merry.Prepend(
			err,
			fmt.Sprintf("Error then creating namespace %s", StroppyClientNSName),
		)
	}

	llog.Debugf("Namespace %s status %v", StroppyClientNSName, namespaceResult.Status.Phase)
	llog.Infof("Applying stroppy namespace manifest '%s': success", StroppyClientNSName)

	return nil
}

func (pod *Pod) DeployPod(shellState *state.State) error {
	var err error

	stroppyClientPodConfig := applyconfig.Pod(
		StroppyClientPodName,
		engine.ResourceDefaultNamespace,
	)

	if err = pod.e.ToEngineObject(
		StroppyClientPodName,
		path.Join(
			shellState.Settings.WorkingDirectory,
			"third_party", "extra", "manifests", "stroppy",
			stoppyClientManifestFile,
		),
		stroppyClientPodConfig,
	); err != nil {
		return merry.Prepend(err, "failed to cast to k8s engine object")
	}

	deployContext, cancel := context.WithCancel(context.Background())

	defer cancel()

	var podResult *v1.Pod

	if err = pod.e.DeployAndWaitObject(
		deployContext,
		StroppyClientPodName,
		engine.ResourceDefaultNamespace,
		func(clientSet *kubernetes.Clientset) error {
			if podResult, err = clientSet.CoreV1().Pods(StroppyClientNSName).Apply(
				deployContext,
				stroppyClientPodConfig,
				pod.e.GenerateDefaultMetav1(),
			); err != nil {
				return merry.Prepend(err, "Error then executing namespace creation")
			}

			return nil
		},
		func(clientSet *kubernetes.Clientset) error {
			if err = clientSet.CoreV1().Pods(StroppyClientNSName).Delete(
				deployContext, StroppyClientPodName, pod.e.GenerateDefaultDeleteOptions(),
			); err != nil {
				return merry.Prepend(err, "failed to delete pod")
			}

			return nil
		},
	); err != nil {
		return merry.Prepend(
			err,
			fmt.Sprintf("Error then deploying pod %s", StroppyClientNSName),
		)
	}

	llog.Debugf("Pod %s status %v", StroppyClientPodName, podResult.Status.Phase)
	llog.Infof("Applying stroppy pod '%s': success", StroppyClientPodName)

	return nil
}
