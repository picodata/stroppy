/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package chaos

import (
	llog "github.com/sirupsen/logrus"
	"gitlab.com/picodata/stroppy/pkg/state"
)

func createDummyChaos() (c Controller) {
	c = &dummyChaos{}
	return
}

type dummyChaos struct{}

func (*dummyChaos) Deploy(_ *state.State) (_ error) {
	llog.Infof("Dummy chaos successfully deployed\n")
	return
}

func (*dummyChaos) ExecuteCommand(scenarioName string, _ *state.State) (_ error) {
	llog.Infof("dummy chaos successfully execute `%s` scenario\n", scenarioName)
	return
}

func (*dummyChaos) Stop() {
	llog.Infof("dummy chaos successfully stopped\n")
}
