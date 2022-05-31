terraform {
  required_version = ">= 0.14.0"
    required_providers {
      openstack = {
        source  = "terraform-provider-openstack/openstack"
        version = "~> 1.46.0"
      }
      selectel = {
        source  = "selectel/selectel"
        version = "~> 3.7.1"
      }
   }
}

provider "selectel" {
  token = var.sel_token
  region = var.os_region
}

provider "openstack" {
  domain_name = var.sel_account
  user_name   = var.user_name
  password    = var.user_password
  auth_url    = var.os_auth_url
  tenant_id   = var.project_id
  region      = var.os_region
}

resource openstack_compute_keypair_v2 "dtravyan" {
  name = "dtravyan"
}

data "openstack_networking_network_v2" "external_net" {
  name = "external-network"
}

resource "openstack_networking_router_v2" "stroppy_router" {
  name = "stroppy_router"
  external_network_id = data.openstack_networking_network_v2.external_net.id
}

resource "openstack_networking_network_v2" "stroppy_network" {
  name = "stroppy_network"
}

resource "openstack_networking_subnet_v2" "stroppy_subnet" {
  network_id = openstack_networking_network_v2.stroppy_network.id
  name       = "stroppy_subnet"
  cidr       = var.subnet_cidr
  enable_dhcp = false
  dns_nameservers = ["8.8.8.8"]
}

resource "openstack_networking_router_interface_v2" "stroppy_router_interface" {
  router_id = openstack_networking_router_v2.stroppy_router.id
  subnet_id = openstack_networking_subnet_v2.stroppy_subnet.id
}

data "openstack_images_image_v2" "centos_image" {
  most_recent = true
  visibility  = "public"
  name        = "CentOS 8 Stream 64-bit"
}

data "cloudinit_config" "cloud_init" {
  gzip = false
  base64_encode = false
  part {
    content_type = "text/cloud-config" 
    content = file("${path.module}/cloud-init/cloud-init.yaml")
  }
}

resource "openstack_compute_flavor_v2" "stroppy_flavor" {
  name      = "stroppy-flavour"
  ram       = var.stroppy_hosts_ram_mb
  vcpus     = var.stroppy_hosts_vcpus
  disk      = var.stroppy_hosts_root_disk_gb
  is_public = "false"
}

resource "openstack_compute_instance_v2" "stroppy_host" {
  name              = "stroppy_host"
  flavor_id         = openstack_compute_flavor_v2.stroppy_flavor.id
  key_pair          = openstack_compute_keypair_v2.dtravyan.id
  availability_zone = var.server_zone
  network {
    fixed_ip_v4 = "192.168.16.11"
    uuid = openstack_networking_network_v2.stroppy_network.id
  }

  image_id = data.openstack_images_image_v2.centos_image.id

  vendor_options {
    ignore_resize_confirmation = true
  }

  lifecycle {
    ignore_changes = [image_id]
  }

  user_data = data.cloudinit_config.cloud_init.rendered
}

resource "openstack_networking_floatingip_v2" "fns_floating_it" {
  pool = "external-network"
}

resource "openstack_compute_floatingip_associate_v2" "fns_floating_it" {
  floating_ip = openstack_networking_floatingip_v2.fns_floating_it.address
  instance_id = openstack_compute_instance_v2.stroppy_host.id
}

output "stroppy_host_floating_ip" {
  value = "ssh picoadm@${openstack_compute_floatingip_associate_v2.fns_floating_it.floating_ip}"
}