/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package kubeengine

import (
	"context"
	"fmt"
	"time"

	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
	"gitlab.com/picodata/stroppy/pkg/tools"
	v1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const waitTimeQuantum = 10 * time.Second

const (
	workerTypeKey = "worker-type"

	workerTypeStroppy = "stroppy-worker"
	workerTypeDBMS    = "dbms-worker"
)

func (e *Engine) waitPodCreation(clientSet *kubernetes.Clientset,
	creationWait bool, waitTime time.Duration, podName, namespace string,
) (targetPod *v1.Pod, err error) {
	targetPod, err = clientSet.CoreV1().Pods(namespace).Get(context.TODO(),
		podName,
		metav1.GetOptions{
			TypeMeta:        metav1.TypeMeta{},
			ResourceVersion: "",
		})
	if err == nil {
		return
	}
	if k8s_errors.IsNotFound(err) && creationWait {

		llog.Infof("WaitPod: go wait '%s/%s' pod creation...",
			namespace, podName)

		creationWaitTime := waitTime
		for k8s_errors.IsNotFound(err) && creationWaitTime > 0 {

			creationWaitTime -= waitTimeQuantum
			time.Sleep(waitTimeQuantum)

			targetPod, err = clientSet.CoreV1().Pods(namespace).Get(context.TODO(),
				podName,
				metav1.GetOptions{
					TypeMeta:        metav1.TypeMeta{},
					ResourceVersion: "",
				})
		}

		if err != nil {
			err = merry.Prependf(err, "'%s/%s' pod creation failed", namespace, podName)
			return
		}

		if targetPod == nil {
			err = fmt.Errorf("pod '%s/%s' still not created", namespace, podName)
			return
		}

	} else {
		err = merry.Prepend(err, "get pod")
		return
	}

	return
}

func (e *Engine) WaitPodPhase(podName, namespace string,
	creationWait bool, waitTime time.Duration, phase v1.PodPhase,
) (*v1.Pod, error) {
	var (
		targetPod *v1.Pod
		clientSet *kubernetes.Clientset
		err       error
	)

	if waitTime < waitTimeQuantum {
        return nil, fmt.Errorf( //nolint // will be fixed in future
			"input wait time %v (s) is less than quantum 10 seconds",
			waitTime.Seconds(),
		)
	}

	if clientSet, err = e.GetClientSet(); err != nil {
		return nil, merry.Prepend(err, "Error then `GetClientSet`")
	}

	if targetPod, err = e.waitPodCreation(
		clientSet,
		creationWait,
		waitTime,
		podName,
		namespace,
	); err != nil {
		return nil, merry.Prepend(err, "Error then waiting pod creation")
	}

	if targetPod.Status.Phase == phase {
		llog.Debugf("WaitPod: pod '%s/%s' already in status '%s'", namespace, podName, phase)

		return targetPod, nil
	}

	for targetPod.Status.Phase != phase {
		targetPod, err = clientSet.CoreV1().Pods(namespace).Get(context.TODO(),
			podName,
			metav1.GetOptions{
				TypeMeta:        metav1.TypeMeta{},
				ResourceVersion: "",
			})
		if err != nil {
			llog.Warnf("WaitPod: failed to update information: %v", err)
			continue
		}
		waitTime -= waitTimeQuantum
		time.Sleep(waitTimeQuantum)

		llog.Infof("WaitPod: '%s' pod status: %v", targetPod.Name, targetPod.Status.Phase)
	}

	if targetPod.Status.Phase != phase {
		return nil, merry.Errorf(
			"pod still not in status '%s', %d minutes left, current status: '%v'",
			phase,
			waitTime/time.Minute,
			targetPod.Status.Phase,
		)
	}

	return targetPod, nil
}

func (e *Engine) WaitPod(podName, namespace string,
	creationWait bool, waitTime time.Duration,
) (targetPod *v1.Pod, err error) {
	targetPod, err = e.WaitPodPhase(podName, namespace, creationWait, waitTime, v1.PodRunning)
	return
}

// AddNodeLabels - добавить labels worker-нодам кластера для разделения stroppy и СУБД
func (e Engine) AddNodeLabels(_ string) (err error) {
	var nodesList *v1.NodeList

	llog.Infoln("Starting of add labels to cluster nodes")

	clientSet, err := e.GetClientSet()
	if err != nil {
		return merry.Prepend(err, "failed to get client set for deploy stroppy")
	}

	// используем получения списка нод ради точного кол-ва нод кластера.
	// deploySettings.nodes не используем из-за разного кол-ва nodes
	// для одинакового кол-ва воркеров в yc и oc
	if err = tools.Retry(
		"get nodes list",
		func() (err error) {
			nodesList, err = clientSet.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
			return
		},
		tools.RetryStandardRetryCount,
		tools.RetryStandardWaitingTime,
	); err != nil {
		return merry.Prepend(err, "failed to get nodes list")
	}

	workerNodeList := nodesList.Items[1:]

	for i := 0; i < len(workerNodeList); i++ {

		currentLabels := workerNodeList[i].GetLabels()

		if _, ok := currentLabels[workerTypeKey]; ok {
			llog.Infoln("this node already been marked")
			continue
		}

		currentLabels[workerTypeKey] = workerTypeDBMS
		workerNodeList[i].SetLabels(currentLabels)

		// последний воркер оставляем для stroppy
		if i == len(workerNodeList)-1 {

			currentLabels[workerTypeKey] = workerTypeStroppy
			workerNodeList[i].SetLabels(currentLabels)
		}

		// применяем изменения на ноду
		err = tools.Retry("adding labels to nodes",
			func() (err error) {
				_, err = clientSet.CoreV1().
					Nodes().
					//nolint // error with imports
					Update(context.TODO(), &workerNodeList[i], metav1.UpdateOptions{})
				return
			},
			tools.RetryStandardRetryCount,
			tools.RetryStandardWaitingTime)
		if err != nil {
			return merry.Prepend(err, "failed to update node")
		}
	}

	llog.Infoln("Add labels to cluster nodes: success")
	return
}
