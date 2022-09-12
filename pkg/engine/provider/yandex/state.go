/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package yandex //nolint:cyclop // won't fix

// Struct for holding deserialized terraform.tfstate json.
type TfState struct {
	TerraformVersion string     `json:"terraform_version"`
	Resources        []Resource `json:"resources"`
}

// Find resource with same name and return pointer or nil.
func (tfstate TfState) GetResource(name string) *Resource {
	for _, resource := range tfstate.Resources {
		if resource.Name == name {
			return &resource
		}
	}

	return nil
}

// Find all resourceType resources or return nil.
func (tfstate TfState) GetResourcesByType(resourceType string) []*Resource {
	var result []*Resource

	for _, resource := range tfstate.Resources {
		if resource.Type == resourceType {
			resource := resource
			result = append(result, &resource)
		}
	}

	return result
}

// Resource like yandex_vpc_subnet.
type Resource struct {
	Type      string     `json:"type"`
	Name      string     `json:"name"`
	Instances []Instance `json:"instances"`
}

// Get instance from resource.
func (resources *Resource) GetInstance(name string) *Instance {
	for _, inst := range resources.Instances { //nolint
		if inst.Attributes.Name == name {
			return &inst
		}
	}

	return nil
}

// Parent struct resource contains array of `Instance` objects.
type Instance struct {
	Attributes Attributes `json:"attributes"`
}

// There are two types of instance Attributes
// The first one are the attributes of a single instance and are stored as a flat structure.
// The second type refers to a set of instances, therefore Attributes of each instance
// will be stored in according instance group in InnerInstances.
type Attributes struct {
	Name             string             `json:"name"`
	ID               string             `json:"id"`
	InstanceTemplate []InstanceTemplate `json:"instance_template"`
	InnerInstances   []InnerInstance    `json:"instances"`
	ScalePolicy      []ScalePolicy      `json:"scale_policy"`
	NetworkInterface []NetworkInterface `json:"network_interface"`
	ServiceAccountID string             `json:"service_account_id"`
	Status           string             `json:"status"`
	V4CidrBlock      []string           `json:"v4_cidr_blocks"`
}

// Return inner instance if this resource has 'group' type.
func (attributes *Attributes) GetInnerInstance(name string) *InnerInstance {
	for _, instance := range attributes.InnerInstances {
		if instance.Name == name {
			return &instance
		}
	}

	return nil
}

// Hold part of parameters for resources like `yandex_compute_instance_group`.
type InstanceTemplate struct {
	BootDisk         []Disk              `json:"boot_disk"`
	Metadata         Metadata            `json:"metadata"`
	NetworkInterface []NetworkInterface  `json:"network_interface"`
	Resources        []ResourcesTemplate `json:"resources"`
}

// Block device parameters.
type Disk struct {
	DeviceName     string          `json:"device_name"`
	DeviceID       string          `json:"device_id"`
	DiskInitParams []DiskInitParam `json:"initialize_params"`
}

// Name, Size, And type of block device.
type DiskInitParam struct {
	BlockSize uint64 `json:"block_size"`
	Name      string `json:"name"`
	Size      uint64 `json:"size"`
	Type      string `json:"type"`
}

// Public ssh key snapshot.
type Metadata struct {
	SSHKeys string `json:"ssh-keys"`
}

// Network interface for `yandex_compute_instance_group` and `yandex_compute_instance`.
type NetworkInterface struct {
	DNSRecord    []string `json:"dns_record"`
	Index        uint64   `json:"index"`
	IPAddress    string   `json:"ip_address"`
	NatIPAddress string   `json:"nat_ip_address"`
}

// Resource template for `yandex_compute_instance_group`.
type ResourcesTemplate struct {
	Cores  uint64 `json:"cores"`
	Gpus   uint64 `json:"gpus"`
	Memory uint64 `json:"memory"`
}

// Similar to `Instance.Attributes` set of parameters for groupped resource types.
type InnerInstance struct {
	Fqdn             string             `json:"fqdn"`
	InstanceID       string             `json:"instance_id"`
	Name             string             `json:"name"`
	NetworkInterface []NetworkInterface `json:"network_interface"`
	Status           string             `json:"status"`
	ZoneID           string             `json:"zone_id"`
}

// Scaling policy for `yandex_compute_instance_group`.
type ScalePolicy struct {
	AutoScale  []Scale `json:"auto_scale"`
	FixedScale []Scale `json:"fixed_scale"`
}

// Target instances count.
type Scale struct {
	Size uint64 `json:"size"`
}
