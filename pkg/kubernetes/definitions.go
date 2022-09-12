/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package kubernetes

import (
	"gitlab.com/picodata/stroppy/pkg/engine/kubeengine"
	"gitlab.com/picodata/stroppy/pkg/engine/ssh"
	"gitlab.com/picodata/stroppy/pkg/engine/stroppy"
	"gitlab.com/picodata/stroppy/pkg/state"
)

func CreateKubernetes(
	sshClient ssh.Client,
	shellState *state.State,
) (k *Kubernetes, err error) {
	k = &Kubernetes{
		Engine:         &kubeengine.Engine{}, //nolint
		StroppyPod:     &stroppy.Pod{},       //nolint
		KubernetesPort: &ssh.Result{},        //nolint
		MonitoringPort: &ssh.Result{},        //nolint

	}

	k.Engine, err = kubeengine.CreateEngine(sshClient, shellState)
	return
}

type Kubernetes struct {
	Engine         *kubeengine.Engine
	StroppyPod     *stroppy.Pod
	KubernetesPort *ssh.Result
	MonitoringPort *ssh.Result
}
