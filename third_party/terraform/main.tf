
terraform {
  required_providers {
    yandex = {
      source = "yandex-cloud/yandex"
    }
  }
}

provider "yandex" {
  token = var.token
  cloud_id  = var.cloud_id
  folder_id = var.folder_id
  zone      = var.zone
}

data "yandex_iam_service_account" "srv_account" {
    service_account_id = var.service_account_id
}

data "yandex_iam_policy" "srv_iam_policy" {
    binding {
        role = "editor"

        members = [
            "serviceAccount:${data.yandex_iam_service_account.srv_account.id}",
        ]
    }
}

resource "yandex_iam_service_account_iam_policy" "srv_account_policy" {
    service_account_id = data.yandex_iam_service_account.srv_account.id
    policy_data        = data.yandex_iam_policy.srv_iam_policy.policy_data
}

resource "yandex_vpc_network" "internal_net" {
    name = "internal_net"
    depends_on =  [yandex_iam_service_account_iam_policy.srv_account_policy, ]
}

resource "yandex_vpc_subnet" "internal_subnet" {
    name           = "internal_subnet"
    zone           = var.zone
    network_id     = yandex_vpc_network.internal_net.id
    v4_cidr_blocks = ["172.16.25.0/24"]

    depends_on =  [yandex_iam_service_account_iam_policy.srv_account_policy, ]
}

data "yandex_compute_image" "ubuntu_image" {
    family = "ubuntu-2004-lts"
}

resource "yandex_compute_instance_group" "masters" {
    name               = "masters"
    service_account_id = data.yandex_iam_service_account.srv_account.id
    instance_template {
        name = "master-{instance.index}"
        hostname = "master-{instance.index}" 
        platform_id = "standard-v2"
        resources {
          memory = var.masters_memory
          cores  = var.masters_cpu
        }
        boot_disk {
          mode = "READ_WRITE"
          initialize_params {
            image_id = data.yandex_compute_image.ubuntu_image.id
            size     = var.masters_boot_disk
            type     = "network-ssd"
          }
        }
        secondary_disk {
          mode = "READ_WRITE"
          device_name = "database"
          initialize_params {
            size     = var.masters_secondary_disk
            type     = "network-ssd-nonreplicated"
          }
        }
        network_interface {
            ip_address = "172.16.25.1{instance.index}"
            subnet_ids = [yandex_vpc_subnet.internal_subnet.id]
            network_id = yandex_vpc_network.internal_net.id
            nat = true
        }
        metadata = { 
            ssh-keys = "ubuntu:${file("../../.ssh/id_rsa.pub")}"
      }
    }
    scale_policy {
      fixed_scale {
        size = var.masters_count
      }
    }
    allocation_policy {
      zones = [var.zone]
    }
    deploy_policy {
      max_unavailable = 0
      max_creating    = var.masters_count
      max_expansion   = 1
      max_deleting    = var.masters_count
    }
    depends_on =  [yandex_iam_service_account_iam_policy.srv_account_policy]
}

resource "yandex_compute_instance_group" "workers" {
    name               = "workers"
    service_account_id = data.yandex_iam_service_account.srv_account.id
    instance_template {
        name = "worker-{instance.index}"
        hostname = "worker-{instance.index}" 
        platform_id = "standard-v2"
        resources {
          memory = var.workers_memory
          cores  = var.workers_cpu
        }
        boot_disk {
          mode = "READ_WRITE"
          initialize_params {
            image_id = data.yandex_compute_image.ubuntu_image.id
            size     = var.workers_boot_disk
            type     = "network-ssd"
          }
        }
        secondary_disk {
          mode = "READ_WRITE"
          device_name = "database"
          initialize_params {
            size     = var.workers_secondary_disk
            type     = "network-ssd-nonreplicated"
          }
        }
        network_interface {
            ip_address = "172.16.25.10{instance.index}"
            subnet_ids = [yandex_vpc_subnet.internal_subnet.id]
            nat = true
        }
        metadata = { 
          ssh-keys = "ubuntu:${file("../../.ssh/id_rsa.pub")}"
    }
  }
  scale_policy {
    fixed_scale {
      size = var.workers_count
    }
  }
  allocation_policy {
    zones = [var.zone]
  }
  deploy_policy {
    max_unavailable = 1
    max_creating    = var.workers_count
    max_expansion   = 1
    max_deleting    = var.workers_count
  }
    depends_on =  [yandex_iam_service_account_iam_policy.srv_account_policy, ]
}

