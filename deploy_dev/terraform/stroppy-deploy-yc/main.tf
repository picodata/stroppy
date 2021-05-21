variable "yc_token" {
  type = string
  description = "Yandex Cloud API key"
  default = "AgAAAAAiwSBdAATuwf72YMk12Un9reJndvsVFag"
}
variable "yc_region" {
  type = string
  description = "Yandex Cloud Region (i.e. ru-central1-a)"
  default = "ru-central1-c"
}
variable "yc_cloud_id" {
  type = string
  description = "Yandex Cloud id"
  default = "b1guec6d2vbn5eceh1c5"
}
variable "yc_folder_id" {
  type = string
  description = "Yandex Cloud folder id"
  default = "b1gj1dh66oosnfvon61a"
}

provider "yandex" {
  token = var.yc_token
  cloud_id  = var.yc_cloud_id
  folder_id = var.yc_folder_id
  zone      = var.yc_region
}

