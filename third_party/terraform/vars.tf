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

# Worker params
variable "workers_cpu" {
    type = number
    description = "Yandex Cloud cpu in cores per worker"
    default = 4
}

variable "workers_memory" {
    type = number
    description = "Yandex Cloud memory in GB per worker"
    default = 8
}

variable "workers_disk" {
    type = number
    description = "Yandex Cloud disk size in GB per worker"
    default = 30
}

variable "workers_count" {
    type = number
    description = "Yandex Cloud count of workers"
    default = 3
}

