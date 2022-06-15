/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package yandex

import (
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"strings"

	"gitlab.com/picodata/stroppy/pkg/engine/provider"

	"github.com/ansel1/merry"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
	llog "github.com/sirupsen/logrus"
	"github.com/zclconf/go-cty/cty"
)

const (
	defaultMasterCPU = 2
	defaultMasterRAM = 4

	defaultMasterDisk = 15

	zoneName = "ru-central1-a"

	subnetAddressTemplate = "172.16.%d.0/24"
)

const providerFileName = "main.tf"
const varsFileName = "vars.tf"

func randStringID() string {
	letters := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

	const idLength = 5
	b := make([]rune, idLength)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

// setTerraformBlock - задать блок требований к провайдеру
func (yp *Provider) setTerraformBlock(providerFileBody *hclwrite.Body) {
	terraformBlock := providerFileBody.AppendNewBlock("terraform", nil)
	terraformBody := terraformBlock.Body()
	requiredProvidersBlock := terraformBody.AppendNewBlock("required_providers", nil)
	sourceMapCloud := cty.ObjectVal(map[string]cty.Value{"source": cty.StringVal("yandex-cloud/yandex")})
	requiredProvidersBlock.Body().SetAttributeValue("yandex", sourceMapCloud)
	providerFileBody.AppendNewline()
}

// setIamServiceAccountBlock - задать блок настроек управления сервисными аккаунтами (IAM)
func (yp *Provider) setIamServiceAccountBlock(providerFileBody *hclwrite.Body) {
	yp.serviceAccountName = fmt.Sprintf("instances8%s", strings.ToLower(randStringID()))

	iamServiceAccountBlock := providerFileBody.AppendNewBlock("resource",
		[]string{"yandex_iam_service_account", yp.serviceAccountName})

	iamServiceAccountBody := iamServiceAccountBlock.Body()
	iamServiceAccountBody.SetAttributeValue("name", cty.StringVal(yp.serviceAccountName))
	iamServiceAccountBody.SetAttributeValue("description", cty.StringVal("service account to manage VMs"))

	providerFileBody.AppendNewline()
}

// setResourceManagerBlock - задать блок единой привязки в рамках IAM для существующей папки менеджера ресурсов.
func (yp *Provider) setResourceManagerBlock(providerFileBody *hclwrite.Body) {
	resourceManagerParams := []string{"yandex_resourcemanager_folder_iam_binding", "editor"}
	resourceManagerBlock := providerFileBody.AppendNewBlock("resource", resourceManagerParams)
	resourceManagerBody := resourceManagerBlock.Body()

	/* здесь и далее структура hcl.Traversal используется для хранения переменных самого cloud вместо варианта с ${},
	где это возможно*/
	folderID := hcl.Traversal{
		hcl.TraverseRoot{Name: "var"},
		hcl.TraverseAttr{Name: "yc_folder_id"},
	}
	resourceManagerBody.SetAttributeTraversal("folder_id", folderID)
	resourceManagerBody.SetAttributeValue("role", cty.StringVal("editor"))

	members := hcl.Traversal{
		hcl.TraverseRoot{Name: fmt.Sprintf("[\"serviceAccount:${yandex_iam_service_account.%s",
			yp.serviceAccountName)},
		hcl.TraverseAttr{Name: "id}\",]"},
	}
	resourceManagerBody.SetAttributeTraversal("members", members)

	dependsOn := hcl.Traversal{
		hcl.TraverseRoot{
			Name: "[\n  yandex_iam_service_account",
		},
		hcl.TraverseAttr{
			Name: fmt.Sprintf("%s,\n   ]", yp.serviceAccountName),
		},
	}
	resourceManagerBody.SetAttributeTraversal("depends_on", dependsOn)
	providerFileBody.AppendNewline()
}

// setVpcNetworkBody - задать блок управления сетью cloud
func (yp *Provider) setVpcNetworkBody(providerFileBody *hclwrite.Body) {
	networkID := strings.ToLower(randStringID())
	yp.vpcSubnetBlockName = fmt.Sprintf("internal-a-%s", networkID)
	yp.vpcInternalNetworkName = fmt.Sprintf("internal-%s", networkID)

	vpcNetworkBlock := providerFileBody.AppendNewBlock("resource",
		[]string{"yandex_vpc_network", yp.vpcInternalNetworkName})
	vpcNetworkBody := vpcNetworkBlock.Body()
	vpcNetworkBody.SetAttributeValue("name", cty.StringVal(yp.vpcInternalNetworkName))
	providerFileBody.AppendNewline()
}

// setVpcSubnetBody - задать блок управления подсетью cloud
func (yp *Provider) setVpcSubnetBody(providerFileBody *hclwrite.Body) {
	vpcSubnetBlock := providerFileBody.AppendNewBlock("resource", []string{"yandex_vpc_subnet", yp.vpcSubnetBlockName})
	vpcSubnetBody := vpcSubnetBlock.Body()
	vpcSubnetBody.SetAttributeValue("name", cty.StringVal(yp.vpcSubnetBlockName))
	vpcSubnetBody.SetAttributeValue("zone", cty.StringVal(zoneName))

	vpcNetInternalID := hcl.Traversal{
		hcl.TraverseRoot{Name: fmt.Sprintf("yandex_vpc_network.%s", yp.vpcInternalNetworkName)},
		hcl.TraverseAttr{Name: "id"},
	}
	vpcSubnetBody.SetAttributeTraversal("network_id", vpcNetInternalID)

	var v4CidrBlocks []cty.Value
	v4CidrBlocks = append(v4CidrBlocks, cty.StringVal(fmt.Sprintf(subnetAddressTemplate, rand.Intn(254))))
	vpcSubnetBody.SetAttributeValue("v4_cidr_blocks", cty.ListVal(v4CidrBlocks))
	providerFileBody.AppendNewline()
}

// setComputeImageBlock - задать блок управления образами cloud
func (yp *Provider) setComputeImageBlock(providerFileBody *hclwrite.Body) {
	computeImageBlock := providerFileBody.AppendNewBlock("data", []string{"yandex_compute_image", "ubuntu_image"})
	computeImageBody := computeImageBlock.Body()
	computeImageBody.SetAttributeValue("family", cty.StringVal("ubuntu-2004-lts"))
	providerFileBody.AppendNewline()
}

// setWorkersBlock - задать блок управления настройками worker-машин
func (yp *Provider) setWorkersBlock(providerFileBody *hclwrite.Body, stringSSHKeys hcl.Traversal,
	cpu int, ram int, disk int, platform string, nodes int) {
	workersBlockName := fmt.Sprintf("workers-1%s", strings.ToLower(randStringID()))

	workersBlock := providerFileBody.AppendNewBlock("resource",
		[]string{"yandex_compute_instance_group", workersBlockName})
	workersBody := workersBlock.Body()
	workersBody.SetAttributeValue("name", cty.StringVal(workersBlockName))

	serviceAccInstanceID := hcl.Traversal{
		hcl.TraverseRoot{Name: fmt.Sprintf("yandex_iam_service_account.%s", yp.serviceAccountName)},
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

	imageWorkersID := hcl.Traversal{
		hcl.TraverseRoot{Name: "data.yandex_compute_image.ubuntu_image"},
		hcl.TraverseAttr{Name: "id"},
	}
	initParamsWorkersBody.SetAttributeTraversal("image_id", imageWorkersID)

	// Здесь задается размер диска worker-машин
	initParamsWorkersBody.SetAttributeValue("size", cty.NumberIntVal(int64(disk)))
	initParamsWorkersBody.SetAttributeValue("type", cty.StringVal("network-ssd"))

	netInterfaceWorkersBlock := instanceTemplateWorkersBody.AppendNewBlock("network_interface", nil)
	netInterfaceWorkersBody := netInterfaceWorkersBlock.Body()

	vpcNetInternalID := hcl.Traversal{
		hcl.TraverseRoot{Name: fmt.Sprintf("yandex_vpc_network.%s", yp.vpcInternalNetworkName)},
		hcl.TraverseAttr{Name: "id"},
	}
	netInterfaceWorkersBody.SetAttributeTraversal("network_id", vpcNetInternalID)

	vpcSubNet := hcl.Traversal{
		hcl.TraverseRoot{Name: fmt.Sprintf("[yandex_vpc_subnet.%s", yp.vpcSubnetBlockName)},
		hcl.TraverseAttr{Name: "id,]"},
	}

	netInterfaceWorkersBody.SetAttributeTraversal("subnet_ids", vpcSubNet)
	netInterfaceWorkersBody.SetAttributeValue("nat", cty.BoolVal(true))
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
	zones = append(zones, cty.StringVal(zoneName))
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
func (yp *Provider) setMasterBlock(providerFileBody *hclwrite.Body, stringSSHKeys hcl.Traversal, platform string) {
	const computeInstanceNameTemplate = "master%s"
	computeInstanceName := fmt.Sprintf(computeInstanceNameTemplate, strings.ToLower(randStringID()))

	masterBlock := providerFileBody.AppendNewBlock("resource", []string{"yandex_compute_instance", computeInstanceName})
	masterBody := masterBlock.Body()
	masterBody.SetAttributeValue("name", cty.StringVal(computeInstanceName))
	masterBody.SetAttributeValue("zone", cty.StringVal(zoneName))
	masterBody.SetAttributeValue("hostname", cty.StringVal(computeInstanceName))
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

	subnetMasterID := hcl.Traversal{
		hcl.TraverseRoot{Name: fmt.Sprintf("yandex_vpc_subnet.%s", yp.vpcSubnetBlockName)},
		hcl.TraverseAttr{Name: "id"},
	}
	netInterfaceMasterBody.SetAttributeTraversal("subnet_id", subnetMasterID)
	netInterfaceMasterBody.SetAttributeValue("nat", cty.BoolVal(true))
	masterBody.SetAttributeTraversal("metadata", stringSSHKeys)
}

// prepare формирует файл конфигурации для провайдера,
// для Yandex.Cloud поддерживается запуск нескольких конфигураций от разных пользователей
func (yp *Provider) prepare(template *provider.ClusterParameters, nodes int, wd string) (err error) {
	llog.Infoln("Starting generation provider configuration file")

    // At first we shold check that main.tf exists
    varsConfigPath := filepath.Join(wd, varsFileName)
    if _, err = os.Stat(varsConfigPath); err == nil {
        llog.Debugln("Terraform variables file 'vars.tf' founded: success")
    } else {
        llog.Debugln("Terraform variables file 'vars.tf' does not exists: waning")
    }

    providerConfigPath := filepath.Join(wd, providerFileName)
    // in some cases user should can create his own terraform script
    // if in workingDirectory main.tf is exists, file creation will be skipped
    if _, err = os.Stat(providerConfigPath); err == nil {
        llog.Infoln("Terraform script 'main.tf' already exists, skipping creation")
        return 
    }

	providerFile := hclwrite.NewEmptyFile()
	providerFileBody := providerFile.Body()
	providerFileBody.AppendNewline()

	/* формируем строку вида { ssh-keys = "ubuntu:${file("id_rsa.pub")}"},
	чтобы не усложнять код преобразованиями из hcl в cty*/
	stringSSHKeys := hcl.Traversal{
		hcl.TraverseRoot{Name: "{ \n ssh-keys = \"ubuntu:${file(\"id_rsa"},
		hcl.TraverseAttr{Name: "pub\")}\"\n}"},
	}

    llog.Warningf("TF file: %#v", providerFile)

	yp.setTerraformBlock(providerFileBody)
	yp.setIamServiceAccountBlock(providerFileBody)
	yp.setResourceManagerBlock(providerFileBody)
	yp.setVpcNetworkBody(providerFileBody)
	yp.setVpcSubnetBody(providerFileBody)
	yp.setComputeImageBlock(providerFileBody)
	yp.setWorkersBlock(providerFileBody, stringSSHKeys, template.CPU, template.RAM, template.Disk, template.Platform, nodes)
	yp.setMasterBlock(providerFileBody, stringSSHKeys, template.Platform)


	if err = ioutil.WriteFile(providerConfigPath, providerFile.Bytes(), 0o600); err != nil {
		return merry.Prepend(err, "failed to write provider configuration file")
	}

	llog.Infoln("Generation provider configuration file: success")
	return
}
