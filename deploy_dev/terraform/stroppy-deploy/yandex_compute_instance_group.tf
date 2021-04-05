terraform {
  required_providers {
    yandex = {
      source = "yandex-cloud/yandex"
    }
  }
  required_version = ">= 0.13"
}

resource "yandex_iam_service_account" "instances" {
  name        = "instances"
  description = "service account to manage VMs"
}

resource "yandex_resourcemanager_folder_iam_binding" "editor" {
  folder_id = var.yc_folder_id

  role = "editor"

  members = [
    "serviceAccount:${yandex_iam_service_account.instances.id}",
  ]
  depends_on = [
    yandex_iam_service_account.instances,
  ]
}

resource "yandex_vpc_network" "internal" {
  name = "internal"
}

resource "yandex_vpc_subnet" "internal-a" {
  name           = "internal-a"
  zone           = "ru-central1-a"
  network_id     = yandex_vpc_network.internal.id
  v4_cidr_blocks = ["172.16.1.0/24"]
}

data "yandex_compute_image" "ubuntu_image" {
  family    = "ubuntu-2004-lts"
}

resource "yandex_compute_instance_group" "workers_1" {
  name               = "workers-1"
  service_account_id = yandex_iam_service_account.instances.id

  instance_template {
    platform_id = "standard-v2" # --platform-id
    resources {
      memory = 4 # --ram
      cores  = 2 # --cpu-count
      # Platform "standard-v1" allowed core number: 2, 4, 6, 8, 10, 12, 14, 16, 20, 24, 28, 32
      # Platform "standard-v2" allowed core number: 2, 4, 6, 8, 10, 12, 14, 16, 20, 24, 28, 32, 36, 40, 44, 48, 52, 56, 60, 64, 68, 72, 76, 80
    }
    boot_disk {
      mode = "READ_WRITE"
      initialize_params {
        image_id = data.yandex_compute_image.ubuntu_image.id
        size = 15 # --disk
        type = "network-ssd"
      }
    }
    network_interface {
      network_id = yandex_vpc_network.internal.id
      subnet_ids = [yandex_vpc_subnet.internal-a.id]
      nat = true # WHITE IP
    }

    metadata = {
      ssh-keys = "ubuntu:${file("id_rsa.pub")}"
    }
  }

  scale_policy {
    fixed_scale {
      size = 3 # --workers
    }
  }

  allocation_policy {
    zones = ["ru-central1-a"]
  }

  deploy_policy {
    max_unavailable = 1
    max_creating    = 3 # --workers
    max_expansion   = 1
    max_deleting    = 3 # --workers
  }

  # load_balancer {
  #   target_group_name = "workers-1"
  # }
  depends_on = [
    yandex_resourcemanager_folder_iam_binding.editor,
  ]
}

resource "yandex_compute_instance" "master" {
  name = "master"
  zone = "ru-central1-a"
  hostname = "master"
  platform_id = "standard-v1"

  resources {
    cores  = 2
    memory = 4
  }

  boot_disk {
    initialize_params {
      image_id = data.yandex_compute_image.ubuntu_image.id
      size = 10
      type = "network-ssd"
    }
  }

  network_interface {
    subnet_id = yandex_vpc_subnet.internal-a.id
    nat       = true
  }

  metadata = {
    ssh-keys = "ubuntu:${file("id_rsa.pub")}"
  }
}
