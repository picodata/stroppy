package oracle

const (
	newTargetCmdTemplate    = "sudo iscsiadm -m discoverydb -t sendtargets -p 169.254.2.2:3260 -D"
	updateTargetCmdTemplate = "sudo iscsiadm -m node -o update -T %v -n node.startup -v automatic"
	loginTargetCmdTemplate  = "sudo iscsiadm -d 8 -m node -T %v -l"

	partedVolumeScript = `
sudo parted /dev/sdb mklabel gpt
sudo parted -a optimal /dev/sdb mkpart primary ext4 0% 100%
`

	checkExistFileSystemCmd = "sudo file -sL /dev/sdb1"

	checkAddedDiskCmd = "file /dev/sdb"
	// -s используем, чтобы не получать ошибку, когда ничего не найдено
	checkPartedCmd = "sudo parted -l"
	checkMountCmd  = "df -h"

	createFileSystemCmd     = "sudo mkfs.ext4 /dev/sdb1"
	addDirectoryCmdTemplate = "sudo mkdir -p /opt/local-path-provisioner/"
	mountLocalPathTemplate  = "sudo mount /dev/sdb1 /opt/local-path-provisioner/"
)
