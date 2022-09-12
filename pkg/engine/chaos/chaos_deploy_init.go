/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package chaos

import (
	"errors"
	"net/url"
	"path"
	"path/filepath"
	"strings"

	"gitlab.com/picodata/stroppy/pkg/engine/kubeengine"
	"gitlab.com/picodata/stroppy/pkg/state"
	v1 "k8s.io/api/core/v1"

	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
)

func (chaos *workableController) Deploy(shellState *state.State) error {
	var err error

	llog.Infoln("Starting chaos-mesh deployment...")

	const deployChaosMesh = "chmod +x cluster/deploy_chaos.sh && ./cluster/deploy_chaos.sh"
	if err = chaos.k.ExecuteCommand(deployChaosMesh); err != nil {
		return merry.Prepend(err, "chaos-mesh deployment failed")
	}
	llog.Debugln("chaos-mesh prepared successfully")

	if err = chaos.enumChaosParts(); err != nil {
		return merry.Prepend(err, "failed to enumChaosParts")
	}

	const (
		chaosConfigDirName = "_config"
		rbacFileName       = "rbac.yaml"
	)

	rbacFileSourcePath := filepath.Join(
		path.Join(shellState.Settings.WorkingDirectory, chaosDir),
		chaosConfigDirName,
		rbacFileName,
	)
	rbacFileKubemasterPath := filepath.Join("home", "stroppy", rbacFileName)

	if err = chaos.k.LoadFile(rbacFileSourcePath, rbacFileKubemasterPath, shellState); err != nil {
		return merry.Prepend(err, "rbac.yaml copying")
	}

	const rbacApplyCommand = "kubectl apply -f %s"
	if err = chaos.k.ExecuteF(rbacApplyCommand, rbacFileKubemasterPath); err != nil {
		return merry.Prepend(err, "rbac.yaml applying")
	}

	llog.Warnf(
		"to access chaos dashboard please login to cloud master machine and run command\n%s\n",
		"kubectl -n chaos-testing describe secret account-cluster-manager-picodata",
	)

	if err = chaos.establishDashboardAvailability(shellState); err != nil {
		return merry.Prepend(err, "failed to establishDashboardAvailability")
	}

	llog.Infoln("chaos-mesh deployed successfully")

	return nil
}

// ----------------------------

func (chaos *workableController) enumChaosParts() (err error) {
	var pods []v1.Pod
	if pods, err = chaos.k.ListPods(chaosNamespace); err != nil {
		return
	}

	for i := 0; i < len(pods); i++ {
		pod := pods[i]
		llog.Debugf("examining pod: '%s'/'%s'", pod.Name, pod.GenerateName)

		if strings.HasPrefix(pod.Name, chaosDashboardResourceName) {
			chaos.dashboardPod = pod.DeepCopy()
			llog.Infof("chaos dashboard pod is '%s'", pod.Name)
		} else if strings.HasPrefix(pod.Name, chaosControlManagerName) {
			chaos.controllerPod = pod.DeepCopy()
			llog.Infof("chaos control management pod is '%s'", pod.Name)
		}
	}

	if chaos.dashboardPod == nil {
		return errors.New("chaos dashboard pod not found")
	}
	if chaos.controllerPod == nil {
		return errors.New("chaos control manager pod not found")
	}

	return
}

func (chaos *workableController) establishDashboardAvailability(
	shellState *state.State,
) error {
	// прокидываем порты, что бы можно было открыть веб-интерфейс
	var (
		err    error
		reqURL *url.URL
	)

	reqURL, err = chaos.k.GetResourceURL(kubeengine.ResourceService,
		chaosNamespace,
		chaos.dashboardPod.Name,
		kubeengine.SubresourcePortForwarding)
	if err != nil {
		return merry.Prepend(err, "failed to get url")
	}

	err = chaos.k.OpenPortForward(chaos.dashboardPod.Name,
		[]string{"2333:2333"},
		reqURL,
		chaos.portForwardStopChan)
	if err != nil {
		// return merry.Prepend(err, "port-forward is not established")
		llog.Errorf("chaos dashboard pf fail: %v", err)
		err = nil
	}

	_ = chaos.k.OpenSecureShellTunnel(
		chaosDashboardResourceName,
		shellState.InstanceAddresses.GetFirstMaster().External,
		chaosPort,
	)

	return nil
}
