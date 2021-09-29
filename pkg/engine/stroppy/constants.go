/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package stroppy

const (
	deployConfigFile = "stroppy-manifest.yaml"
	secretFile       = "stroppy-secret.yaml"

	fieldManagerName = "stroppy-deploy"
)

const PodName = "stroppy-client"

const dockerRepLoginCmd = "docker login -u stroppy_deploy -p k3xG2_xe_SDjyYDREML3 registry.gitlab.com"
