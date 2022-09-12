/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package provider

import (
	"errors"
)

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
	GetInstancesAddresses() *InstanceAddresses
	GetSubnet() string
	GetNodes() map[string]*Node
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

type InstanceAddresses struct {
	Masters map[string]AddrPair
	Workers map[string]AddrPair
}

func (insAddr *InstanceAddresses) GetWorkersAndMastersAddrPairs() map[string]*AddrPair {
	pairs := make(map[string]*AddrPair)

	for nodeName, pair := range insAddr.Masters {
		pair := pair
		pairs[nodeName] = &pair
	}

	for nodeName, pair := range insAddr.Workers {
		pair := pair
		pairs[nodeName] = &pair
	}

	return pairs
}

func (insAddr *InstanceAddresses) GetFirstMaster() AddrPair {
	if value, ok := insAddr.Masters["master-1"]; ok {
		return value
	}

	if value, ok := insAddr.Masters["master"]; ok {
		return value
	}

	return AddrPair{
		Internal: "",
		External: "",
	}
}

func (insAddr *InstanceAddresses) MastersCnt() int {
	return len(insAddr.Masters)
}

func (insAddr *InstanceAddresses) WorkersCnt() int {
	return len(insAddr.Workers)
}

type AddrPair struct {
	Internal string
	External string
}

type Node struct {
	Fqdn      string
	Resources Resources
}

type Resources struct {
	CPU    uint64
	Memory uint64
	Disk   uint64
}
