variable "prefix" {
  type = string
}

variable "zone" {
  type = string
}

variable "template_name" {}

variable "username" {}

variable "private_network_cidr" {}

variable "machines" {
  description = "Cluster machines"
  type = map(object({
    node_type       = string
    plan            = string
    cpu             = string
    mem             = string
    disk_size       =  number
    additional_disks = map(object({
      size = number
      tier = string
    }))
  }))
}

variable "ssh_public_keys" {
  type = list(string)
}

variable "firewall_enabled" {
  type = bool
}

variable "master_allowed_remote_ips" {
  type = list(object({
    start_address = string
    end_address   = string
  }))
}

variable "k8s_allowed_remote_ips" {
  type = list(object({
    start_address = string
    end_address   = string
  }))
}

variable "loadbalancer_enabled" {
  type = bool
}

variable "loadbalancer_plan" {
  type = string
}

variable "loadbalancers" {
  description = "Load balancers"

  type = map(object({
    port            = number
    backend_servers = list(string)
  }))
}
