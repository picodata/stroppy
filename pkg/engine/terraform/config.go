package terraform

import "errors"

const installableTerraformVersion = "0.15.4"

const yandexProvider = "yandex"

const oracleProvider = "oracle"

type TemplatesConfig struct {
	Yandex Configurations
	Oracle Configurations
}

type ConfigurationUnitParams struct {
	Description string
	Platform    string
	CPU         int
	RAM         int
	Disk        int
}

type Configurations struct {
	Small    []ConfigurationUnitParams
	Standard []ConfigurationUnitParams
	Large    []ConfigurationUnitParams
	Xlarge   []ConfigurationUnitParams
	Xxlarge  []ConfigurationUnitParams
	Maximum  []ConfigurationUnitParams
}

const TemplatesFileName = "templates.yaml"

var ErrChooseConfig = errors.New("failed to choose configuration. Unexpected configuration cluster template")

const (
	TargetLoginCmdTemplate = `
sudo iscsiadm -m node -o new -T %v -p 169.254.2.2:3260
sudo iscsiadm -m node -o update -T %v -n node.startup -v automatic
sudo iscsiadm -m node -T %v -p 169.254.2.2:3260 -l
sleep 5
`

	PartedVolumeCmd = `
sudo parted /dev/sdb mklabel gpt
sudo parted -a optimal /dev/sdb mkpart primary ext4 0% 100%
`

	CheckExistFileSystemCmd = "sudo file -sL /dev/sdb1"

	CheckAdddedDiskCmd = "file /dev/sdb"
	// -s используем, чтобы не получать ошибку, когда ничего не найдено
	CheckPartedCmd = "sudo parted -l"

	CheckMountCmd = "df -h"

	CreatefileSystemCmd = "sudo mkfs.ext4 /dev/sdb1"

	AddDirectoryCmdTemplate = "sudo mkdir -p /opt/local-path-provisioner/"

	MountLocalPathTemplate = "sudo mount /dev/sdb1 /opt/local-path-provisioner/"

	TerraformStateFileName = "terraform.tfstate"
)
