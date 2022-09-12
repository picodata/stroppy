/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package state

import (
	"gitlab.com/picodata/stroppy/pkg/database/config"
	"gitlab.com/picodata/stroppy/pkg/engine/provider"
)

type State struct {
	Settings *config.Settings

	// List of infrastructure nodes
	NodesInfo NodesInfo

	// network settings
	InstanceAddresses *provider.InstanceAddresses
	Subnet            string
}

type NodesInfo struct {
	MastersCnt int
	WorkersCnt int
	Params     map[string]*provider.Node
}
