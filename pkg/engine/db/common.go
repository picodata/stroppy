/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package db

import (
	"fmt"
	"net/url"
	"strings"

	"gitlab.com/picodata/stroppy/pkg/engine/kubeengine"
	engineSsh "gitlab.com/picodata/stroppy/pkg/engine/ssh"

	v1 "k8s.io/api/core/v1"

	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
	"gitlab.com/picodata/stroppy/pkg/kubernetes"
)

func createCommonCluster(sc engineSsh.Client, k *kubernetes.Kubernetes, wd, databaseTag, dbURL string,
	connectionPoolSize int, sharded bool,
) (fc *commonCluster) {
	fc = &commonCluster{
		k:                      k,
		sc:                     sc,
		wd:                     wd,
		tg:                     databaseTag,
		DBUrl:                  dbURL,
		portForwardControlChan: make(chan struct{}),

		clusterSpec: ClusterSpec{
			Pods: make([]*v1.Pod, 0, 10),
		},
		connectionPoolSize: connectionPoolSize,
		sharded:            sharded,
	}

	return
}

type commonCluster struct {
	sc engineSsh.Client
	k  *kubernetes.Kubernetes
	wd string
	tg string

	clusterSpec            ClusterSpec
	portForwardControlChan chan struct{}

	DBUrl string

	connectionPoolSize int
	addPool            int

	sharded bool
}

func (cc *commonCluster) deploy() (err error) {
	llog.Infof("Prepare '%s' deployment\n", cc.tg)

	deployConfigDirectory := cc.wd

	if err = cc.k.Engine.LoadDirectory(deployConfigDirectory, "/home/ubuntu/databases"); err != nil {
		return
	}

	llog.Infof("copying %s directory: success\n", cc.tg)
	llog.Infof("%s deploy started\n", cc.tg)

	deployCmd := fmt.Sprintf("chmod +x databases/%s/deploy_operator.sh && ./databases/%s/deploy_operator.sh", cc.tg, cc.tg)

	if err = cc.k.Engine.DebugCommand(deployCmd, false); err != nil {
		return
	}

	llog.Infof("%s deploy finished", cc.tg)

	return
}

func (cc *commonCluster) examineCluster(tag, targetNamespace, clusterMainPodName, clusterWorkerPodName string) (err error) {
	var pods []v1.Pod

	if pods, err = cc.k.Engine.ListPods(kubeengine.ResourceDefaultNamespace); err != nil {
		err = merry.Prepend(err, "list pods")

		return
	}

	printPodContainers := func(pod *v1.Pod) {
		for i := range pod.Spec.Containers {
			llog.Debugf("\tfound (%s, `%s`, '%s') container in pod '%s'",
				pod.Spec.Containers[i].Name, strings.Join(pod.Spec.Containers[i].Args, " "),
				strings.Join(pod.Spec.Containers[i].Command, " "), pod.Name)
		}

		llog.Debug("\t---------------------\n\n")
	}

	for i := 0; i < len(pods); i++ {
		pPod := &pods[i]

		llog.Debugf("examining pod: '%s'/'%s'", pPod.Name, pPod.GenerateName)

		if strings.HasPrefix(pPod.Name, clusterMainPodName) {
			llog.Infof("%s main pod is '%s'", tag, pPod.Name)
			printPodContainers(pPod)
			cc.clusterSpec.MainPod = pPod
		} else if strings.HasPrefix(pPod.Name, clusterWorkerPodName) {
			cc.clusterSpec.Pods = append(cc.clusterSpec.Pods, pPod)
			printPodContainers(pPod)
		}
	}

	if cc.clusterSpec.MainPod == nil {
		return fmt.Errorf("%s main pod does not exists", tag)
	}

	if cc.clusterSpec.MainPod.Status.Phase != v1.PodRunning {
		cc.clusterSpec.MainPod, err = cc.k.Engine.WaitPod(cc.clusterSpec.MainPod.Name,
			targetNamespace,
			kubeengine.PodWaitingWaitCreation,
			kubeengine.PodWaitingTime)
		if err != nil {
			return merry.Prependf(err, "%s pod wait", tag)
		}
	}

	llog.Debugf("%s main pod '%s' in status '%s', okay",
		tag, cc.clusterSpec.MainPod.Name, cc.clusterSpec.MainPod.Status.Phase)

	return
}

func (cc *commonCluster) openPortForwarding(name string, portMap []string) (err error) {
	var reqURL *url.URL

	reqURL, err = cc.k.Engine.GetResourceURL(kubeengine.ResourcePodName, kubeengine.ResourceDefaultNamespace, name, kubeengine.SubresourcePortForwarding)
	if err != nil {
		return
	}

	err = cc.k.Engine.OpenPortForward(cc.tg, portMap, reqURL, cc.portForwardControlChan)
	if err != nil {
		return merry.Prependf(err, "failed to started port-forward for '%s'", cc.tg)
	}

	llog.Infof("Port-forwarding for %s is started success", name)

	return
}
