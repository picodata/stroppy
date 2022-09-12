/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package chaos

import (
	"gitlab.com/picodata/stroppy/pkg/engine/kubeengine"
	"gitlab.com/picodata/stroppy/pkg/state"
)

type Controller interface {
	Deploy(*state.State) error
	ExecuteCommand(string, *state.State) error
	Stop()
}

func CreateController(
	k *kubeengine.Engine,
	shellState *state.State,
) Controller {
	if shellState.Settings.UseChaos {
		return createWorkableController(k, *shellState)
	}

	return createDummyChaos()
}
