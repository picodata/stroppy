/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package chaos

import (
	"path/filepath"
	"strings"
	"sync"

	"github.com/ansel1/merry"

	v1 "k8s.io/api/core/v1"

	llog "github.com/sirupsen/logrus"
	"gitlab.com/picodata/stroppy/pkg/engine/kubeengine"
)

func createWorkableController(k *kubeengine.Engine, wd string) (c Controller) {
	c = &workableController{
		wd: filepath.Join(wd, "chaos"),
		k:  k,

		runningScenarios:     map[string]scenario{},
		runningScenariosLock: sync.Mutex{},

		portForwardStopChan: make(chan struct{}),
	}

	return
}

type workableController struct {
	k  *kubeengine.Engine
	wd string

	runningScenarios     map[string]scenario
	runningScenariosLock sync.Mutex

	portForwardStopChan chan struct{}

	controllerPod, dashboardPod *v1.Pod
}

func (chaos *workableController) executeAtomicCommand(scenarioName string) (err error) {
	llog.Infof("now starting chaos '%s' scenario", scenarioName)

	plan := createScenario(scenarioName, chaos.wd)
	if err = chaos.k.LoadFile(plan.sourcePath, plan.destinationPath); err != nil {
		return merry.Prepend(err, "load file failed")
	}

	llog.Debugf("full chaos command object is '%v'", plan)

	if err = chaos.k.ExecuteF("kubectl apply -f %s", plan.destinationPath); err != nil {
		return merry.Prepend(err, "scenario run failed")
	}

	chaos.runningScenariosLock.Lock()
	defer chaos.runningScenariosLock.Unlock()
	chaos.runningScenarios[plan.scenarioName] = plan

	return
}

func (chaos *workableController) ExecuteCommand(scenarioName string) (err error) {
	commandList := strings.Split(scenarioName, ",")
	for _, command := range commandList {
		if err = chaos.executeAtomicCommand(command); err != nil {
			return
		}
	}

	return
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
