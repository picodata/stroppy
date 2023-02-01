variable "token" {
  type = string
  description = "Yandex Cloud API key"
}

variable "zone" {
  type = string
  description = "Yandex Cloud Region (i.e. ru-central1-a)"
}

variable "cloud_id" {
  type = string
  description = "Yandex Cloud id"
}

variable "folder_id" {
  type = string
  description = "Yandex Cloud folder id"
}

variable "service_account" {
    type = string
    description = "Yandex Cloud service account name"
    default = "stroppy"
}

variable "service_account_id" {
  type = string
  description = "Yandex Cloud service account id"
  default = "ajevdf69bgecucb0tn7n"
}

# Master params
variable "masters_count" {
    type = number
    description = "Yandex Cloud disk size gigabytes per master"
    default = 1
}

variable "masters_cpu" {
    type = number
    description = "Yandex Cloud cpu cores per master"
    default = 16
}

variable "masters_memory" {
    type = number
    description = "Yandex Cloud memory gigabytes per master"
    default = 48
}

variable "masters_boot_disk" {
    type = number
    description = "Yandex Cloud disk size gigabytes per master"
    default = 20
}

variable "masters_secondary_disk" {
    type = number
    description = "Yandex Cloud disk size gigabytes per master"
    default = 465
}

# Worker params
variable "workers_count" {
    type = number
    description = "Yandex Cloud count of workers"
    default = 9
}

variable "workers_cpu" {
    type = number
    description = "Yandex Cloud cpu in cores per worker"
    default = 16
}

variable "workers_memory" {
    type = number
    description = "Yandex Cloud memory in GB per worker"
    default = 48
}

variable "workers_boot_disk" {
    type = number
    description = "Yandex Cloud disk size in GB per worker"
    default = 20
}

variable "workers_secondary_disk" {
    type = number
    description = "Yandex Cloud disk size in GB per worker"
    default = 465
}
