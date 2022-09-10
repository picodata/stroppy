/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package provider

import "errors"

const (
	Oracle  = "oracle"
	Yandex  = "yandex"
	Neutral = "neutral"

	ClusterTemplateFileName = "templates.yaml"
)

var ErrChooseConfig = errors.New(
	"failed to choose configuration. Unexpected configuration cluster template",
)

type Provider interface {
	Prepare() error
	AddNetworkDisks(int) error
    GetInstanceAddress(string, string) (*Addresses, error)
	CheckSSHPrivateKey(string) error
	CheckSSHPublicKey(string) error
	RemoveProviderSpecificFiles()
	GetDeploymentCommands() (string, string)
    GetTfStateScheme() interface{}
	Name() string
}

type ClusterParameters struct {
	Description string
	Platform    string
	CPU         int
	RAM         int
	Disk        int
}

type ClusterConfigurations struct {
	Small    ClusterParameters
	Standard ClusterParameters
	Large    ClusterParameters
	XLarge   ClusterParameters
	XXLarge  ClusterParameters
	Maximum  ClusterParameters
}

type Addresses struct {
	Internal string
	External string
}
