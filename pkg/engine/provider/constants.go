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

	CheckExistFileSystemCmd = "sudo file -sL /dev/sdb1 | grep -Eo 'ext4'"

	CheckAdddedDiskCmd = "file /dev/sdb"

	CheckPartedCmd = "sudo parted -l | grep 'ext4' | grep 'primary'"

	CheckMountCmd = "df -h| grep  '/opt/local-path-provisioner'"

	CreatefileSystemCmd = "sudo mkfs.ext4 /dev/sdb1"

	AddDirectoryCmdTemplate = "sudo mkdir -p /opt/local-path-provisioner/"

	MountLocalPathTemplate = "sudo mount /dev/sdb1 /opt/local-path-provisioner/"

	TerraformStateFileName = "terraform.tfstate"
)
