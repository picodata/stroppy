
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
            size     = var.workers_disk
            type     = "network-ssd"
          }
        }
        network_interface {
            ip_address = "172.16.25.10{instance.index}"
            subnet_ids = [yandex_vpc_subnet.internal_subnet.id]
            network_id = yandex_vpc_network.internal_net.id
            nat = true
        }
        metadata = { 
          ssh-keys = "ubuntu:${file("id_rsa.pub")}"
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
    max_creating    = 3
    max_expansion   = 1
    max_deleting    = 3
  }
    depends_on =  [yandex_iam_service_account_iam_policy.srv_account_policy, ]
}

resource "yandex_compute_instance" "master" {
    name        = "master"
    hostname    = "master"
    zone        = var.zone
    platform_id = "standard-v2"
    service_account_id = data.yandex_iam_service_account.srv_account.id
    resources {
      memory = 4
      cores  = 2
    }
    boot_disk {
        initialize_params {
            image_id = data.yandex_compute_image.ubuntu_image.id
            size     = 15
            type     = "network-ssd"
        }
    }
    network_interface {
        ip_address = "172.16.25.99"
        subnet_id = yandex_vpc_subnet.internal_subnet.id
        nat       = true
    }
    metadata = { 
        ssh-keys = "ubuntu:${file("id_rsa.pub")}"
    }
    depends_on =  [yandex_iam_service_account_iam_policy.srv_account_policy, ]
}
