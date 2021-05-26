# Administration > Tenancy Details (Name: owrussia)
variable "tenancy_ocid" {
  default = "ocid1.tenancy.oc1..aaaaaaaafqgn6pw772mo2ln2ywcaiijaufrnvh3bpetam34m2udykglixolq"
}
# Identity > Users > User Details (user: xdb)
variable "user_ocid" {
  default = "ocid1.user.oc1..aaaaaaaaxenvjykcpfynxtosbbmddulqvmb34ot2bbg5dq2d4flwvsmjsxsq"
}
# Identity > Users > User Details > API Keys
variable "fingerprint" {
  default = "ec:c8:5e:42:9c:71:a8:73:e5:c5:df:15:62:ee:f0:3c"
}
# ABSOLUTE path
variable "private_key_path" {
  type = string
  default = "picodata.pem"
}
# Manage Regions
variable "region" {
  default = "eu-frankfurt-1"
}
# Identity > Compartments (Name: XDB)
variable "compartment_ocid" {
  default = "ocid1.compartment.oc1..aaaaaaaa5soo5hr34y4i5hq3bkfxkg5t6zf7e23c3vlmempe6ppy34za76eq"
}

variable "ssh_public_key" {
  default = "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQCnPYxOLBLVVzRrWlw96AOzavA034a2tV1G5rtM6b7yUc5J9Vi2g3uvAj2idlRWnumEMrm1E6Pr6LHRr1oChDSCrcfIxl8oJZQW5eQsPPtRKj9fE8v6J3Nr8hMIAflG/SBqpGQmxhRqvcuuf7RHxs8EqsnOaXxUtbtZNDSo+VZj45rVh3BSg0TxSKfDrRNRw3/HO0KtYYlH8J1VIYl9t0tlrXZEndShS9LCat/EmBjSG1dtUdzz3jo3L67cJ7Qigcg1U2drzQ78yCJHRM6oTFEQkfO+WnjDm97+zxGordWhejaVzwARP4TjDBWAZVdxHUl3yAb02nHnRkHmtliuLBcX"
}

provider "oci" {
  tenancy_ocid     = var.tenancy_ocid
  user_ocid        = var.user_ocid
  fingerprint      = var.fingerprint
  private_key_path = var.private_key_path
  region           = var.region
}

# !!!
variable "num_instances" {
  default = "4" # instances count
}

variable "num_iscsi_volumes_per_instance" {
  default = "1"
}
# !!!
variable "instance_shape" {
  default = "VM.Standard.E3.Flex"
}
# !!!
variable "instance_ocpus" {
  default = 4 # CPU count
}
# !!!
variable "instance_shape_config_memory_in_gbs" {
  default = 4 # size in GBs
}

variable "flex_instance_image_ocid" {
  type = map(string)
  default = {
    # See https://docs.us-phoenix-1.oraclecloud.com/images/
    # Image "Canonical-Ubuntu-20.04-2021.04.15-0"
    eu-frankfurt-1   = "ocid1.image.oc1.eu-frankfurt-1.aaaaaaaaw4ap4pklk3lo5pls5rppt2vfhjvuukpi2fltc74ycmaz3w7bz2aq"
  }
}
# !!!
variable "db_size" {
  default = "50" # size in GBs
}

variable "tag_namespace_description" {
  default = "ns-stroppy-cluster"
}

variable "tag_namespace_name" {
  default = "ns-stroppy-cluster"
}

resource "oci_core_instance" "k8s_instance" {
  count               = var.num_instances
  availability_domain = data.oci_identity_availability_domain.ad.name
  compartment_id      = var.compartment_ocid
  display_name        = "K8sNode${count.index}"
  shape               = var.instance_shape

  shape_config {
    ocpus = var.instance_ocpus
    memory_in_gbs = var.instance_shape_config_memory_in_gbs
  }

  create_vnic_details {
    subnet_id                 = oci_core_subnet.k8s_cluster_subnet.id
    display_name              = "Primaryvnic"
    assign_public_ip          = true
    assign_private_dns_record = true
    hostname_label            = "k8s-node${count.index}"
  }

  source_details {
    source_type = "image"
    source_id = var.flex_instance_image_ocid[var.region]
  }

  metadata = {
    ssh_authorized_keys = var.ssh_public_key
    # user_data           = base64encode(file("./userdata/bootstrap"))
  }
  freeform_tags = {
    "freeformkey${count.index}" = "freeformvalue${count.index}"
  }

  preemptible_instance_config {
    preemption_action {
      type = "TERMINATE"
      preserve_boot_volume = false
    }
  }

  timeouts {
    create = "60m"
  }
}

# Define the volumes that are attached to the compute instances.

resource "oci_core_volume" "block_volume" {
  count               = var.num_instances * var.num_iscsi_volumes_per_instance
  availability_domain = data.oci_identity_availability_domain.ad.name
  compartment_id      = var.compartment_ocid
  display_name        = "BlockVolume${count.index}"
  size_in_gbs         = var.db_size
}

resource "oci_core_volume_attachment" "test_block_attach" {
  count           = var.num_instances * var.num_iscsi_volumes_per_instance
  attachment_type = "iscsi"
  instance_id     = oci_core_instance.k8s_instance[floor(count.index / var.num_iscsi_volumes_per_instance)].id
  volume_id       = oci_core_volume.block_volume[count.index].id
  device          = count.index == 0 ? "/dev/oracleoci/oraclevdb" : ""

  # Set this to enable CHAP authentication for an ISCSI volume attachment. The oci_core_volume_attachment resource will
  # contain the CHAP authentication details via the "chap_secret" and "chap_username" attributes.
  use_chap = true
  # Set this to attach the volume as read-only.
  #is_read_only = true
}

data "oci_core_instance_devices" "k8s_instance_devices" {
  count       = var.num_instances
  instance_id = oci_core_instance.k8s_instance[count.index].id
}

output "instance_private_ips" {
  value = [oci_core_instance.k8s_instance.*.private_ip]
}

output "instance_public_ips" {
  value = [oci_core_instance.k8s_instance.*.public_ip]
}

# Output the boot volume IDs of the instance
output "boot_volume_ids" {
  value = [oci_core_instance.k8s_instance.*.boot_volume_id]
}

# Output all the devices for all instances
output "instance_devices" {
  value = [data.oci_core_instance_devices.k8s_instance_devices.*.devices]
}

resource "oci_core_vcn" "k8s_cluster_vcn" {
  cidr_block     = "10.1.0.0/16"
  compartment_id = var.compartment_ocid
  display_name   = "K8sVcn"
  dns_label      = "k8svcn"
}

resource "oci_core_internet_gateway" "k8s_cluster_internet_gateway" {
  compartment_id = var.compartment_ocid
  display_name   = "K8sInternetGateway"
  vcn_id         = oci_core_vcn.k8s_cluster_vcn.id
}

resource "oci_core_default_route_table" "default_route_table" {
  manage_default_resource_id = oci_core_vcn.k8s_cluster_vcn.default_route_table_id
  display_name               = "DefaultRouteTable"

  route_rules {
    destination       = "0.0.0.0/0"
    destination_type  = "CIDR_BLOCK"
    network_entity_id = oci_core_internet_gateway.k8s_cluster_internet_gateway.id
  }
}

resource "oci_core_subnet" "k8s_cluster_subnet" {
  availability_domain = data.oci_identity_availability_domain.ad.name
  cidr_block          = "10.1.20.0/24"
  display_name        = "K8sSubnet"
  dns_label           = "k8ssubnet"
  security_list_ids   = [oci_core_security_list.etcd2_security_list.id]
  compartment_id      = var.compartment_ocid
  vcn_id              = oci_core_vcn.k8s_cluster_vcn.id
  route_table_id      = oci_core_vcn.k8s_cluster_vcn.default_route_table_id
  dhcp_options_id     = oci_core_vcn.k8s_cluster_vcn.default_dhcp_options_id
}

data "oci_identity_availability_domain" "ad" {
  compartment_id = var.compartment_ocid
  ad_number      = 1
}

resource "oci_identity_tag_namespace" "tag-namespace1" {
  #Required
  compartment_id = var.compartment_ocid
  description    = var.tag_namespace_description
  name           = var.tag_namespace_name
}

resource "oci_core_security_list" "etcd2_security_list" {
  compartment_id = var.compartment_ocid
  vcn_id         = oci_core_vcn.k8s_cluster_vcn.id
  display_name   = "Etcd2SecurityList"
# https://www.iana.org/assignments/protocol-numbers/protocol-numbers.xhtml
# Options are supported only for ICMP ("1"), TCP ("6"), UDP ("17"), and ICMPv6 ("58")

  egress_security_rules {
    protocol    = "6"
    destination = "0.0.0.0/0"
  }

  egress_security_rules {
    protocol    = "17"
    destination = "0.0.0.0/0"
  }

  ingress_security_rules {
    protocol = "6"
    source   = "0.0.0.0/0"
  }

  ingress_security_rules {
    protocol    = "17"
    source = "0.0.0.0/0"
  }
}
