/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package chaos

import "path/filepath"

func createScenario(name, wd string) (s scenario) {
	scenarioFileName := name + ".yaml"

	s = scenario{
		scenarioName:     name,
		scenarioFileName: scenarioFileName,

		destinationPath: filepath.Join("/home/ubuntu", scenarioFileName),
		sourcePath:      filepath.Join(wd, scenarioFileName),

		isRunning: false,
	}

	return
}

type scenario struct {
	destinationPath string
	sourcePath      string

	scenarioName     string
	scenarioFileName string

	isRunning bool
}
