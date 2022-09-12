/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package chaos

import (
	"path"
	"strings"
	"sync"

	"github.com/ansel1/merry"

	v1 "k8s.io/api/core/v1"

	llog "github.com/sirupsen/logrus"
	"gitlab.com/picodata/stroppy/pkg/engine/kubeengine"
	"gitlab.com/picodata/stroppy/pkg/state"
)

func createWorkableController(k *kubeengine.Engine, shellState state.State) Controller {
	return &workableController{
		k:                    k,
		runningScenarios:     map[string]scenario{},
		runningScenariosLock: sync.Mutex{},
		portForwardStopChan:  make(chan struct{}),
		controllerPod:        &v1.Pod{}, //nolint
		dashboardPod:         &v1.Pod{}, //nolint
	}
}

type workableController struct {
	k *kubeengine.Engine

	runningScenarios     map[string]scenario
	runningScenariosLock sync.Mutex

	portForwardStopChan chan struct{}

	controllerPod, dashboardPod *v1.Pod
}

func (chaos *workableController) executeAtomicCommand(
	scenarioName string,
	shellState *state.State,
) error {
	var err error

	llog.Infof("now starting chaos '%s' scenario", scenarioName)

	chaosScenario := createScenario(
		scenarioName,
		path.Join(shellState.Settings.WorkingDirectory, chaosDir),
	)
	if err = chaos.k.LoadFile(
		chaosScenario.sourcePath,
		chaosScenario.destinationPath,
		shellState,
	); err != nil {
		return merry.Prepend(err, "load file failed")
	}

	llog.Debugf("full chaos command object is '%v'", chaosScenario)

	if err = chaos.k.ExecuteF("kubectl apply -f %s", chaosScenario.destinationPath); err != nil {
		return merry.Prepend(err, "scenario run failed")
	}

	chaos.runningScenariosLock.Lock()
	defer chaos.runningScenariosLock.Unlock()
	chaos.runningScenarios[chaosScenario.scenarioName] = chaosScenario

	return nil
}

func (chaos *workableController) ExecuteCommand(
	scenarioName string,
	shellState *state.State,
) error {
	var err error

	commandList := strings.Split(scenarioName, ",")
	for _, command := range commandList {
		if err = chaos.executeAtomicCommand(command, shellState); err != nil {
			return merry.Prepend(err, "failed to executeAtomicCommand")
		}
	}

	return nil
}

func (chaos *workableController) Stop() {
	chaos.runningScenariosLock.Lock()
	defer chaos.runningScenariosLock.Unlock()

	var err error
	for _, s := range chaos.runningScenarios {
		if s.isRunning {
			llog.Infof("stopping chaos scenario '%s'\n", s.scenarioName)
			if err = chaos.k.ExecuteF("kubectl delete -f %s", s.destinationPath); err != nil {
				llog.Warnf("'%s' scenario not stopped: %v", s.destinationPath, err)
			}
		}
	}
}
