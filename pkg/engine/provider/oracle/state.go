/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package oracle //nolint:cyclop // TODO: try to undestand calculation logic

// Struct for holding deserialized terraform.tfstate json.
type TfState struct {
	Data    []byte
	Outputs Outputs `json:"outputs"`
}

// TODO: should be fixed after testing on oracle cloud.
type Outputs struct {
	InstancePublicIps  InstanceIps `json:"instance_public_ips"`
	InstancePrivateIps InstanceIps `json:"instance_private_ips"`
}

// TODO: should be fixed after testing on oracle cloud.
type InstanceIps struct {
	Value [][]string
}
