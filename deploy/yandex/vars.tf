# ---
# Main configuration parameters
# ---
# export TF_VAR_sel_account=
# export TF_VAR_sel_token=
# export TF_VAR_user_name=
# export TF_VAR_user_password=
variable "sel_account" {
  type = string
}

variable "sel_token" {
  type = string
}

variable "user_name" {
  type = string
}

variable "user_password" {
  type = string
}

variable "os_auth_url" {
  default = "https://api.selvpc.ru/identity/v3"
}

variable "project_id" {
  type = string
}

variable "os_region" {
  default = "ru-7"
}

variable "server_zone" {
  default = "ru-7a"
}

# ---
# Target stroppy_hosts config
# ---
variable "subnet_cidr" {
  default = "192.168.16.0/24"
}

variable "stroppy_hosts_vcpus" {
  default = 4
}

variable "stroppy_hosts_ram_mb" {
  default = 8192
}

variable "stroppy_hosts_root_disk_gb" {
  default = 30
}
