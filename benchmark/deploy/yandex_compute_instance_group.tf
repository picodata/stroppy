
terraform {
  required_providers {
    yandex = {
      source = "yandex-cloud/yandex"
    }
  }
}

resource "yandex_iam_service_account" "instances" {
  name        = "instances"
  description = "service account to manage VMs"
}

resource "yandex_resourcemanager_folder_iam_binding" "editor" {
  folder_id  = var.yc_folder_id
  role       = "editor"
  members    = ["serviceAccount:${yandex_iam_service_account.instances.id}",]
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
  family = "ubuntu-2004-lts"
}

resource "yandex_compute_instance_group" "workers_1" {
  name               = "workers-1"
  service_account_id = yandex_iam_service_account.instances.id
  instance_template {
    platform_id = "standard-v1"
    resources {
      memory = 4
      cores  = 4
    }
    boot_disk {
      mode = "READ_WRITE"
      initialize_params {
        image_id = data.yandex_compute_image.ubuntu_image.id
        size     = 15
        type     = "network-ssd"
      }
    }
    network_interface {
      network_id = yandex_vpc_network.internal.id
      subnet_ids = [yandex_vpc_subnet.internal-a.id,]
      nat        = true
    }
    metadata = { 
 ssh-keys = "ubuntu:${file("id_rsa.pub")}"
}
  }
  scale_policy {
    fixed_scale {
      size = 3
    }
  }
  allocation_policy {
    zones = ["ru-central1-a"]
  }
  deploy_policy {
    max_unavailable = 1
    max_creating    = 3
    max_expansion   = 1
    max_deleting    = 3
  }
  depends_on =  [yandex_resourcemanager_folder_iam_binding.editor, ]
}




resource "yandex_compute_instance" "master" {
  name        = "master"
  zone        = "ru-central1-a"
  hostname    = "master"
  platform_id = "standard-v1"
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
    subnet_id = yandex_vpc_subnet.internal-a.id
    nat       = true
  }
  metadata = { 
 ssh-keys = "ubuntu:${file("id_rsa.pub")}"
}
}
