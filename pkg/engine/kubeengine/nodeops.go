/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package kubeengine

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
	"gitlab.com/picodata/stroppy/pkg/state"
	"gitlab.com/picodata/stroppy/pkg/tools"
	v1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
)

const waitTimeQuantum = 10 * time.Second

const (
	NodeNameMonitoring = "dbms-monitoring"
	NodeNameMaster     = "dbms-master"
	NodeNameDBMS       = "dbms-worker"
)

const trueVal = "true"

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
func (e *Engine) AddNodeLabels(shellState *state.State) error { //nolint
	var (
		clientSet *kubernetes.Clientset
		nodesList *v1.NodeList
		err       error
	)

	llog.Infoln("Starting of add labels to cluster nodes")

	if clientSet, err = e.GetClientSet(); err != nil {
		return merry.Prepend(err, "failed to get client set for deploy stroppy")
	}

	if err = tools.Retry(
		"get nodes list",
		func() (err error) {
			nodesList, err = clientSet.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
			return
		},
		tools.RetryStandardRetryCount,
		tools.RetryStandardWaitingTime,
	); err != nil {
		return merry.Prepend(err, "Failed to get nodes list")
	}

	for index, node := range nodesList.Items { //nolint
		newLabels := make(map[string]string)
		nodeLabels := nodesList.Items[index].GetLabels()

		switch {
		case node.Name == "master-1":
			if shellState.Settings.DeploymentSettings.AllWorkers {
				newLabels[NodeNameDBMS] = trueVal
			}

			newLabels[NodeNameMonitoring] = trueVal
			newLabels[NodeNameMaster] = trueVal
		case strings.Contains(node.Name, "master"):
			if shellState.Settings.DeploymentSettings.AllWorkers {
				newLabels[NodeNameDBMS] = trueVal
			}

			newLabels[NodeNameMaster] = trueVal
		case strings.Contains(node.Name, "worker"):
			newLabels[NodeNameDBMS] = trueVal
		}

		newLabels["topology.kubernetes.io/zone"] = node.Name

		for key, value := range newLabels {
			nodeLabels[key] = value
		}

		nodesList.Items[index].SetLabels(nodeLabels)
		nodesList.Items[index].CreationTimestamp = metav1.Time{} //nolint
		nodesList.Items[index].ResourceVersion = ""
		nodesList.Items[index].UID = types.UID("")

		llog.Tracef("Cluster node %s now has new labels: %v", node.Name, nodeLabels)
	}

	if err = applyNodeLabels(clientSet, nodesList); err != nil {
		return merry.Prepend(err, "failed to apply node labels")
	}

	llog.Infoln("Add labels to cluster nodes: success")

	return nil
}

func applyNodeLabels(clientSet *kubernetes.Clientset, nodesList *v1.NodeList) error {
	var err error

	for index := 0; index < len(nodesList.Items); index++ {
		if err = tools.Retry(
			"Adding labels to nodes",
			func() (err error) {
				_, err = clientSet.CoreV1().
					Nodes().
					Update(
						context.TODO(),
						&nodesList.Items[index],
						metav1.UpdateOptions{
							TypeMeta: metav1.TypeMeta{
								Kind:       "",
								APIVersion: "",
							},
							DryRun:          []string{},
							FieldManager:    "",
							FieldValidation: "",
						})

				return merry.Prepend(err, "Failed to update label on node")
			},
			tools.RetryStandardRetryCount,
			tools.RetryStandardWaitingTime,
		); err != nil {
			return merry.Prepend(err, "All retries was unsucessefull")
		}
	}

	return nil
}
