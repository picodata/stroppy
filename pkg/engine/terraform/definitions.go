/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package terraform

import (
	"errors"

	"gitlab.com/picodata/stroppy/pkg/database/config"
	"gitlab.com/picodata/stroppy/pkg/engine/provider"
)

const installableTerraformVersion = "0.15.4"

const stateFileName = "terraform.tfstate"

// Длина ответа terraform show при незапущенном кластере
const linesNotInitTerraformShow = 13

var errVersionParsed = errors.New("failed to parse version")

type Terraform struct {
	settings *config.DeploymentSettings

	exePath           string
	templatesFilePath string
	stateFilePath     string

	addressMap map[string]map[string]string
	isInit     bool

	WorkDirectory string

	version *version

	Provider provider.Provider

	data []byte
}

type version struct {
	major  int
	minor  int
	bugfix int
}
