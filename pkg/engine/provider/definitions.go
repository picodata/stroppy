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
	WaitNodes() error
	GetNodesInfo() map[string]*NodeParams
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
	return AddrPair{
		Internal: insAddr.Masters["master-1"].Internal,
		External: insAddr.Masters["master-1"].External,
	}
}

func (insAddr *InstanceAddresses) MastersCnt(allMasters bool) int {
	if allMasters {
		return len(insAddr.Workers) + len(insAddr.Masters)
	}

	return len(insAddr.Masters)
}

func (insAddr *InstanceAddresses) WorkersCnt(allWorkers bool) int {
	if allWorkers {
		return len(insAddr.Workers) + len(insAddr.Masters)
	}

	return len(insAddr.Workers)
}

func (insAddr *InstanceAddresses) GetFirstWorker() AddrPair {
	return AddrPair{
		Internal: insAddr.Workers["worker-1"].Internal,
		External: insAddr.Workers["worker-1"].External,
	}
}

type AddrPair struct {
	Internal string
	External string
}

type NodeParams struct {
	Index      int
	InstanceID string
	Fqdn       string
	Resources  Resources
}

type Resources struct {
	CPU           uint64
	Memory        uint64
	BootDisk      uint64
	SecondaryDisk uint64
}
