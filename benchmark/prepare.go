package main

import (
	"io/ioutil"

	"github.com/ansel1/merry"
	hcl "github.com/hashicorp/hcl/v2"
	hcl2 "github.com/hashicorp/hcl/v2/hclwrite"
	llog "github.com/sirupsen/logrus"
	"github.com/zclconf/go-cty/cty"
)

const defaultMasterCPU = 2

const defaultMasterRAM = 4

const defaultMasterDisk = 15

const providerFilePath = "deploy/yandex_compute_instance_group.tf"

// setTerraformBlock - задать блок требований к провайдеру
func setTerraformBlock(providerFileBody *hcl2.Body) {
	terraformBlock := providerFileBody.AppendNewBlock("terraform", nil)
	terraformBody := terraformBlock.Body()
	requiredProvidersBlock := terraformBody.AppendNewBlock("required_providers", nil)
	sourceMapCloud := cty.ObjectVal(map[string]cty.Value{"source": cty.StringVal("yandex-cloud/yandex")})
	requiredProvidersBlock.Body().SetAttributeValue("yandex", sourceMapCloud)
	providerFileBody.AppendNewline()
}

// setIamServiceAccountBlock - задать блок настроек управления сервисными аккаунтами (IAM)
func setIamServiceAccountBlock(providerFileBody *hcl2.Body) {
	iamServiceAccountBlock := providerFileBody.AppendNewBlock("resource",
		[]string{"yandex_iam_service_account", "instances"})
	iamServiceAccountBody := iamServiceAccountBlock.Body()
	iamServiceAccountBody.SetAttributeValue("name", cty.StringVal("instances"))
	iamServiceAccountBody.SetAttributeValue("description", cty.StringVal("service account to manage VMs"))
	providerFileBody.AppendNewline()
}

// setResourceManagerBlock - задать блок единой привязки в рамках IAM для существующей папки менеджера ресурсов.
func setResourceManagerBlock(providerFileBody *hcl2.Body) {
	resourceManagerParams := []string{"yandex_resourcemanager_folder_iam_binding", "editor"}
	resourceManagerBlock := providerFileBody.AppendNewBlock("resource", resourceManagerParams)
	resourceManagerBody := resourceManagerBlock.Body()
	/* здесь и далее структура hcl.Traversal используется для хранения переменных самого cloud вместо варианта с ${},
	где это возможно*/
	//nolint:exhaustivestruct
	folderID := hcl.Traversal{
		hcl.TraverseRoot{Name: "var"},
		hcl.TraverseAttr{Name: "yc_folder_id"},
	}
	resourceManagerBody.SetAttributeTraversal("folder_id", folderID)
	resourceManagerBody.SetAttributeValue("role", cty.StringVal("editor"))
	//nolint:exhaustivestruct
	members := hcl.Traversal{
		hcl.TraverseRoot{Name: "[\"serviceAccount:${yandex_iam_service_account.instances"},
		hcl.TraverseAttr{Name: "id}\",]"},
	}
	resourceManagerBody.SetAttributeTraversal("members", members)
	//nolint:exhaustivestruct
	dependsOn := hcl.Traversal{
		hcl.TraverseRoot{
			Name: "[\n  yandex_iam_service_account",
		},
		hcl.TraverseAttr{
			Name: "instances,\n   ]",
		},
	}
	resourceManagerBody.SetAttributeTraversal("depends_on", dependsOn)
	providerFileBody.AppendNewline()
}

// setVpcNetworkBody - задать блок управления сетью cloud
func setVpcNetworkBody(providerFileBody *hcl2.Body) {
	vpcNetworkBlock := providerFileBody.AppendNewBlock("resource", []string{"yandex_vpc_network", "internal"})
	vpcNetworkBody := vpcNetworkBlock.Body()
	vpcNetworkBody.SetAttributeValue("name", cty.StringVal("internal"))
	providerFileBody.AppendNewline()
}

// setVpcSubnetBody - задать блок управления подсетью cloud
func setVpcSubnetBody(providerFileBody *hcl2.Body) {
	vpcSubnetBlock := providerFileBody.AppendNewBlock("resource", []string{"yandex_vpc_subnet", "internal-a"})
	vpcSubnetBody := vpcSubnetBlock.Body()
	vpcSubnetBody.SetAttributeValue("name", cty.StringVal("internal-a"))
	vpcSubnetBody.SetAttributeValue("zone", cty.StringVal("ru-central1-a"))
	//nolint:exhaustivestruct
	vpcNetInternalID := hcl.Traversal{
		hcl.TraverseRoot{Name: "yandex_vpc_network.internal"},
		hcl.TraverseAttr{Name: "id"},
	}
	vpcSubnetBody.SetAttributeTraversal("network_id", vpcNetInternalID)
	var v4CidrBlocks []cty.Value
	v4CidrBlocks = append(v4CidrBlocks, cty.StringVal("172.16.1.0/24"))
	vpcSubnetBody.SetAttributeValue("v4_cidr_blocks", cty.ListVal(v4CidrBlocks))
	providerFileBody.AppendNewline()
}

// setComputeImageBlock - задать блок управления образами cloud
func setComputeImageBlock(providerFileBody *hcl2.Body) {
	computeImageBlock := providerFileBody.AppendNewBlock("data", []string{"yandex_compute_image", "ubuntu_image"})
	computeImageBody := computeImageBlock.Body()
	computeImageBody.SetAttributeValue("family", cty.StringVal("ubuntu-2004-lts"))
	providerFileBody.AppendNewline()
}

// setWorkersBlock - задать блок управления настройками worker-машин
func setWorkersBlock(providerFileBody *hcl2.Body, stringSSHKeys hcl.Traversal,
	cpu int, ram int, disk int, platform string, nodes int) {
	workersBlock := providerFileBody.AppendNewBlock("resource",
		[]string{"yandex_compute_instance_group", "workers_1"})
	workersBody := workersBlock.Body()
	workersBody.SetAttributeValue("name", cty.StringVal("workers-1"))
	//nolint:exhaustivestruct
	serviceAccInstanceID := hcl.Traversal{
		hcl.TraverseRoot{Name: "yandex_iam_service_account.instances"},
		hcl.TraverseAttr{Name: "id"},
	}
	workersBody.SetAttributeTraversal("service_account_id", serviceAccInstanceID)

	instanceTemplateWorkersBlock := workersBody.AppendNewBlock("instance_template", nil)
	instanceTemplateWorkersBody := instanceTemplateWorkersBlock.Body()
	instanceTemplateWorkersBody.SetAttributeValue("platform_id", cty.StringVal(platform))

	// Здесь задаются параметры cpu/count для worker-машин
	resourcesWorkersBlock := instanceTemplateWorkersBody.AppendNewBlock("resources", nil)
	resourcesWorkersBody := resourcesWorkersBlock.Body()
	resourcesWorkersBody.SetAttributeValue("memory", cty.NumberIntVal(int64(ram)))
	resourcesWorkersBody.SetAttributeValue("cores", cty.NumberIntVal(int64(cpu)))

	// Здесь задается режим жесткого диска worker-машин
	bootDiskWorkersBlock := instanceTemplateWorkersBody.AppendNewBlock("boot_disk", nil)
	bootDiskWorkersBody := bootDiskWorkersBlock.Body()
	bootDiskWorkersBody.SetAttributeValue("mode", cty.StringVal("READ_WRITE"))

	// Здесь задаются параметры инициализации жесткого диска
	initParamsDiskWorkersBlock := bootDiskWorkersBody.AppendNewBlock("initialize_params", nil)
	initParamsWorkersBody := initParamsDiskWorkersBlock.Body()
	//nolint:exhaustivestruct
	imageWorkersID := hcl.Traversal{
		hcl.TraverseRoot{Name: "data.yandex_compute_image.ubuntu_image"},
		hcl.TraverseAttr{Name: "id"},
	}
	initParamsWorkersBody.SetAttributeTraversal("image_id", imageWorkersID)
	// Здесь задается размер диска worker-машин
	initParamsWorkersBody.SetAttributeValue("size", cty.NumberIntVal(int64(disk)))
	initParamsWorkersBody.SetAttributeValue("type", cty.StringVal("network-ssd"))

	netInterfaseWorkersBlock := instanceTemplateWorkersBody.AppendNewBlock("network_interface", nil)
	netInterfaseWorkersBody := netInterfaseWorkersBlock.Body()
	//nolint:exhaustivestruct
	vpcNetInternalID := hcl.Traversal{
		hcl.TraverseRoot{Name: "yandex_vpc_network.internal"},
		hcl.TraverseAttr{Name: "id"},
	}
	netInterfaseWorkersBody.SetAttributeTraversal("network_id", vpcNetInternalID)
	//nolint:exhaustivestruct
	vpcSubNet := hcl.Traversal{
		hcl.TraverseRoot{Name: "[yandex_vpc_subnet.internal-a"},
		hcl.TraverseAttr{Name: "id,]"},
	}
	netInterfaseWorkersBody.SetAttributeTraversal("subnet_ids", vpcSubNet)
	netInterfaseWorkersBody.SetAttributeValue("nat", cty.BoolVal(true))
	instanceTemplateWorkersBody.SetAttributeTraversal("metadata", stringSSHKeys)
	providerFileBody.AppendNewline()
	scalePolicyWorkersBlock := workersBody.AppendNewBlock("scale_policy", nil)
	fixedScaleWorkersBlock := scalePolicyWorkersBlock.Body().AppendNewBlock("fixed_scale", nil)
	fixedScaleBody := fixedScaleWorkersBlock.Body()
	// здесь задается кол-во workers
	fixedScaleBody.SetAttributeValue("size", cty.NumberIntVal(int64(nodes)))
	providerFileBody.AppendNewline()
	allocPolicyWorkersBlock := workersBody.AppendNewBlock("allocation_policy", nil)
	allocPolicyWorkersBody := allocPolicyWorkersBlock.Body()
	var zones []cty.Value
	zones = append(zones, cty.StringVal("ru-central1-a"))
	allocPolicyWorkersBody.SetAttributeValue("zones", cty.ListVal(zones))
	providerFileBody.AppendNewline()
	deployPolicyWorkersBlock := workersBody.AppendNewBlock("deploy_policy", nil)
	deployPolicyWorkersBody := deployPolicyWorkersBlock.Body()
	deployPolicyWorkersBody.SetAttributeValue("max_unavailable", cty.NumberIntVal(1))
	// максимальное кол-во создаваемых воркеров
	deployPolicyWorkersBody.SetAttributeValue("max_creating", cty.NumberIntVal(int64(nodes)))
	deployPolicyWorkersBody.SetAttributeValue("max_expansion", cty.NumberIntVal(1))
	// максимальное кол-во удаляемых воркеров
	deployPolicyWorkersBody.SetAttributeValue("max_deleting", cty.NumberIntVal(int64(nodes)))
	//nolint:exhaustivestruct
	dependsOn := hcl.Traversal{
		hcl.TraverseRoot{
			Name: " [yandex_resourcemanager_folder_iam_binding",
		},
		hcl.TraverseAttr{
			Name: "editor, ]",
		},
	}
	workersBody.SetAttributeTraversal("depends_on", dependsOn)
	providerFileBody.AppendNewline()
}

// setMasterBlock - задать блок управления настройками master-машин
func setMasterBlock(providerFileBody *hcl2.Body, stringSSHKeys hcl.Traversal, platform string) {
	masterBlock := providerFileBody.AppendNewBlock("resource", []string{"yandex_compute_instance", "master"})
	masterBody := masterBlock.Body()
	masterBody.SetAttributeValue("name", cty.StringVal("master"))
	masterBody.SetAttributeValue("zone", cty.StringVal("ru-central1-a"))
	masterBody.SetAttributeValue("hostname", cty.StringVal("master"))
	masterBody.SetAttributeValue("platform_id", cty.StringVal(platform))

	// Здесь задаются параметры cpu/count для master-машины
	resourcesMasterBlock := masterBody.AppendNewBlock("resources", nil)
	resourceMasterBody := resourcesMasterBlock.Body()
	resourceMasterBody.SetAttributeValue("memory", cty.NumberIntVal(defaultMasterRAM))
	resourceMasterBody.SetAttributeValue("cores", cty.NumberIntVal(defaultMasterCPU))

	bootDiskMasterBlock := masterBody.AppendNewBlock("boot_disk", nil)
	bootDiskMasterBody := bootDiskMasterBlock.Body()

	// Здесь задаются параметры инициализации жесткого диска master-машины
	initParamsDiskMasterBlock := bootDiskMasterBody.AppendNewBlock("initialize_params", nil)
	initParamsDiskBody := initParamsDiskMasterBlock.Body()
	//nolint:exhaustivestruct
	imageMasterID := hcl.Traversal{
		hcl.TraverseRoot{Name: "data.yandex_compute_image.ubuntu_image"},
		hcl.TraverseAttr{Name: "id"},
	}
	initParamsDiskBody.SetAttributeTraversal("image_id", imageMasterID)
	// Здесь задается размер жесткого диска master-машины
	initParamsDiskBody.SetAttributeValue("size", cty.NumberIntVal(defaultMasterDisk))
	initParamsDiskBody.SetAttributeValue("type", cty.StringVal("network-ssd"))

	netInterfaceMasterBlock := masterBody.AppendNewBlock("network_interface", nil)
	netInterfaceMasterBody := netInterfaceMasterBlock.Body()
	//nolint:exhaustivestruct
	subnetMasterID := hcl.Traversal{
		hcl.TraverseRoot{Name: "yandex_vpc_subnet.internal-a"},
		hcl.TraverseAttr{Name: "id"},
	}
	netInterfaceMasterBody.SetAttributeTraversal("subnet_id", subnetMasterID)
	netInterfaceMasterBody.SetAttributeValue("nat", cty.BoolVal(true))
	masterBody.SetAttributeTraversal("metadata", stringSSHKeys)
}

// prepare - сформировать файл конфигурации для провайдера
func prepare(cpu int, ram int, disk int, platform string, nodes int) error {
	llog.Infoln("Starting generation provider configuration file")
	providerFile := hcl2.NewEmptyFile()
	providerFileBody := providerFile.Body()
	providerFileBody.AppendNewline()
	/* формируем строку вида { ssh-keys = "ubuntu:${file("id_rsa.pub")}"},
	чтобы не усложнять код преобразованиями из hcl в cty*/
	//nolint:exhaustivestruct
	stringSSHKeys := hcl.Traversal{
		hcl.TraverseRoot{Name: "{ \n ssh-keys = \"ubuntu:${file(\"id_rsa"},
		hcl.TraverseAttr{Name: "pub\")}\"\n}"},
	}
	setTerraformBlock(providerFileBody)
	setIamServiceAccountBlock(providerFileBody)
	setResourceManagerBlock(providerFileBody)
	setVpcNetworkBody(providerFileBody)
	setVpcSubnetBody(providerFileBody)
	setComputeImageBlock(providerFileBody)
	setWorkersBlock(providerFileBody, stringSSHKeys, cpu, ram, disk, platform, nodes)
	setMasterBlock(providerFileBody, stringSSHKeys, platform)
	err := ioutil.WriteFile(providerFilePath, providerFile.Bytes(), 0600)
	if err != nil {
		return merry.Prepend(err, "failed to write provider configuration file")
	}
	llog.Infoln("Generation provider configuration file: success")
	return nil
}
