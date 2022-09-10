package yandex

type TfState struct {
	TerraformVersion string     `json:"terraform_version"`
	Resources        []Resource `json:"resources"`
}

func (tfstate TfState) GetResource(name string) (*Resource, bool) {
	for _, resource := range tfstate.Resources {
		if resource.Name == name {
			return &resource, true
		}
	}

	return nil, false
}

func (tfstate TfState) GetResourcesByType(resourceType string) ([]*Resource, bool) {
	var (
		result []*Resource
		found  bool
	)

	for _, resource := range tfstate.Resources {
		if resource.Type == resourceType {
			result = append(result, &resource)
			found = true
		}
	}

	return result, found
}

type Resource struct {
	Type      string     `json:"type"`
	Name      string     `json:"name"`
	Instances []Instance `json:"instances"`
}

func (resources *Resource) GetInstance(name string) (*Instance, bool) {
	for _, inst := range resources.Instances {
		if inst.Attributes.Name == name {
			return &inst, true
		}
	}

	return nil, false
}

type Instance struct {
	Attributes Attributes `json:"attributes"`
}

type Attributes struct {
	BootDisk         []Disk             `json:"boot_disk"`
	Hostname         string             `json:"hostname"`
	Id               string             `json:"id"`
	GroupInstances   []GroupInstance    `json:"instances"`
	Metadata         Metadata           `json:"metadata"`
	Name             string             `json:"name"`
	NetworkInterface []NetworkInterface `json:"network_interface"`
	Resources        []InstanceResource `json:"resources"`
	ServiceAccountId string             `json:"service_account_id"`
	Status           string             `json:"status"`
	Zone             string             `json:"zone"`
}

func (attributes *Attributes) GetGroupInstance(name string) (*GroupInstance, bool) {
	for _, instance := range attributes.GroupInstances {
		if instance.Name == name {
			return &instance, true
		}
	}

	return nil, false
}

type GroupInstance struct {
	Fqdn             string             `json:"fqdn"`
	InstanceId       string             `json:"instance_id"`
	Name             string             `json:"name"`
	NetworkInterface []NetworkInterface `json:"network_interface"`
	Status           string             `json:"status"`
	ZoneId           string             `json:"zone_id"`
}

type Disk struct {
	DeviceName     string          `json:"device_name"`
	DeviceId       string          `json:"device_id"`
	DiskInitParams []DiskInitParam `json:"initialize_params"`
}

type DiskInitParam struct {
	BlockSize uint64 `json:"block_size"`
	Name      string `json:"name"`
	Size      uint64 `json:"size"`
	Type      string `json:"type"`
}

type Metadata struct {
	SshKeys string `json:"ssh-keys"`
}

type NetworkInterface struct {
	DnsRecord    []string `json:"dns_record"`
	Index        uint64   `json:"index"`
	IpAddress    string   `json:"ip_address"`
	NatIpAddress string   `json:"nat_ip_address"`
}

type InstanceResource struct {
	Cores  uint64 `json:"cores"`
	Gpus   uint64 `json:"gpus"`
	Memory uint64 `json:"memory"`
}


