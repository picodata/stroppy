package terraform

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

const installableTerraformVersion = "0.15.4"

const (
	addStorageCmdTemplate = `
	sudo iscsiadm -m node -o new -T %v -p 169.254.2.2:3260
	sudo iscsiadm -m node -o update -T %v -n node.startup -v automatic
	sudo iscsiadm -m node -T %v -p 169.254.2.2:3260 -l
	sleep 5
	sudo parted /dev/sdb mklabel gpt
	sudo parted -a optimal /dev/sdb mkpart primary ext4 0% 100%
	sudo mkfs.ext4 /dev/sdb1
	sudo mount /dev/sdb1 /opt/local-path-provisioner/
	`
)
