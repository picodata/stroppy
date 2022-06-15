locals {
  # Create a list of all disks to create
  disks = flatten([
    for node_name, machine in var.machines : [
      for disk_name, disk in machine.additional_disks : {
        disk = disk
        disk_name = disk_name
        node_name = node_name
      }
    ]
  ])

  lb_backend_servers = flatten([
    for lb_name, loadbalancer in var.loadbalancers : [
      for backend_server in loadbalancer.backend_servers : {
        port = loadbalancer.port
        lb_name = lb_name
        server_name = backend_server
      }
    ]
  ])

  # If prefix is set, all resources will be prefixed with "${var.prefix}-"
  # Else don't prefix with anything
  resource-prefix = "%{ if var.prefix != ""}${var.prefix}-%{ endif }"
}

resource "upcloud_network" "private" {
  name = "${local.resource-prefix}k8s-network"
  zone = var.zone

  ip_network {
    address = var.private_network_cidr
    dhcp    = true
    family  = "IPv4"
  }
}

resource "upcloud_storage" "additional_disks" {
  for_each = {
    for disk in local.disks: "${disk.node_name}_${disk.disk_name}" => disk.disk
  }

  size  = each.value.size
  tier  = each.value.tier
  title = "${local.resource-prefix}${each.key}"
  zone  = var.zone
}

resource "upcloud_server" "master" {
  for_each = {
    for name, machine in var.machines :
    name => machine
    if machine.node_type == "master"
  }

  hostname = "${local.resource-prefix}${each.key}"
  plan     = each.value.plan
  cpu      = each.value.plan == null ? each.value.cpu : null
  mem      = each.value.plan == null ? each.value.mem : null
  zone     = var.zone

  template {
  storage = var.template_name
  size    = each.value.disk_size
  }

  # Public network interface
  network_interface {
    type = "public"
  }

  # Private network interface
  network_interface {
    type    = "private"
    network = upcloud_network.private.id
  }

  # Ignore volumes created by csi-driver
  lifecycle {
    ignore_changes = [storage_devices]
  }
  
  firewall  = var.firewall_enabled

  dynamic "storage_devices" {
    for_each = {
      for disk_key_name, disk in upcloud_storage.additional_disks :
        disk_key_name => disk
        # Only add the disk if it matches the node name in the start of its name
        if length(regexall("^${each.key}_.+", disk_key_name)) > 0
    }

    content {
      storage = storage_devices.value.id
    }
  }

  # Include at least one public SSH key
  login {
    user            = var.username
    keys            = var.ssh_public_keys
    create_password = false
  }
}

resource "upcloud_server" "worker" {
  for_each = {
    for name, machine in var.machines :
    name => machine
    if machine.node_type == "worker"
  }

  hostname = "${local.resource-prefix}${each.key}"
  plan     = each.value.plan
  cpu      = each.value.plan == null ? each.value.cpu : null
  mem      = each.value.plan == null ? each.value.mem : null
  zone     = var.zone

  template {
    storage = var.template_name
    size    = each.value.disk_size
  }

  # Public network interface
  network_interface {
    type = "public"
  }

  # Private network interface
  network_interface {
    type    = "private"
    network = upcloud_network.private.id
  }

  # Ignore volumes created by csi-driver
  lifecycle {
    ignore_changes = [storage_devices]
  }

  firewall  = var.firewall_enabled

  dynamic "storage_devices" {
    for_each = {
      for disk_key_name, disk in upcloud_storage.additional_disks :
        disk_key_name => disk
        # Only add the disk if it matches the node name in the start of its name
        if length(regexall("^${each.key}_.+", disk_key_name)) > 0
    }

    content {
      storage = storage_devices.value.id
    }
  }

  # Include at least one public SSH key
  login {
    user            = var.username
    keys            = var.ssh_public_keys
    create_password = false
  }
}

resource "upcloud_firewall_rules" "master" {
  for_each = upcloud_server.master
  server_id = each.value.id

  dynamic firewall_rule {
    for_each = var.master_allowed_remote_ips

    content {
      action                 = "accept"
      comment                = "Allow master API access from this network"
      destination_port_end   = "6443"
      destination_port_start = "6443"
      direction              = "in"
      family                 = "IPv4"
      protocol               = "tcp"
      source_address_end     = firewall_rule.value.end_address
      source_address_start   = firewall_rule.value.start_address
    }
  }

  dynamic firewall_rule {
    for_each = length(var.master_allowed_remote_ips) > 0 ? [1] : []

    content {
      action                 = "drop"
      comment                = "Deny master API access from other networks"
      destination_port_end   = "6443"
      destination_port_start = "6443"
      direction              = "in"
      family                 = "IPv4"
      protocol               = "tcp"
      source_address_end     = "255.255.255.255"
      source_address_start   = "0.0.0.0"
    }
  }

  dynamic firewall_rule {
    for_each = var.k8s_allowed_remote_ips

    content {
      action                 = "accept"
      comment                = "Allow SSH from this network"
      destination_port_end   = "22"
      destination_port_start = "22"
      direction              = "in"
      family                 = "IPv4"
      protocol               = "tcp"
      source_address_end     = firewall_rule.value.end_address
      source_address_start   = firewall_rule.value.start_address
    }
  }

  dynamic firewall_rule {
    for_each = length(var.k8s_allowed_remote_ips) > 0 ? [1] : []

    content {
      action                 = "drop"
      comment                = "Deny SSH from other networks"
      destination_port_end   = "22"
      destination_port_start = "22"
      direction              = "in"
      family                 = "IPv4"
      protocol               = "tcp"
      source_address_end     = "255.255.255.255"
      source_address_start   = "0.0.0.0"
    }
  }
}

resource "upcloud_firewall_rules" "k8s" {
  for_each = upcloud_server.worker
  server_id = each.value.id

  dynamic firewall_rule {
    for_each = var.k8s_allowed_remote_ips

    content {
      action                 = "accept"
      comment                = "Allow SSH from this network"
      destination_port_end   = "22"
      destination_port_start = "22"
      direction              = "in"
      family                 = "IPv4"
      protocol               = "tcp"
      source_address_end     = firewall_rule.value.end_address
      source_address_start   = firewall_rule.value.start_address
    }
  }

  dynamic firewall_rule {
    for_each = length(var.k8s_allowed_remote_ips) > 0 ? [1] : []

    content {
      action                 = "drop"
      comment                = "Deny SSH from other networks"
      destination_port_end   = "22"
      destination_port_start = "22"
      direction              = "in"
      family                 = "IPv4"
      protocol               = "tcp"
      source_address_end     = "255.255.255.255"
      source_address_start   = "0.0.0.0"
    }
  }
}

resource "upcloud_loadbalancer" "lb" {
  count             = var.loadbalancer_enabled ? 1 : 0
  configured_status = "started"
  name              = "${local.resource-prefix}lb"
  plan              = var.loadbalancer_plan
  zone              = var.zone
  network           = upcloud_network.private.id
}

resource "upcloud_loadbalancer_backend" "lb_backend" {
  for_each = var.loadbalancer_enabled ? var.loadbalancers : {}

  loadbalancer = upcloud_loadbalancer.lb[0].id
  name         = "lb-backend-${each.key}"
}

resource "upcloud_loadbalancer_frontend" "lb_frontend" {
  for_each = var.loadbalancer_enabled ? var.loadbalancers : {}
  
  loadbalancer         = upcloud_loadbalancer.lb[0].id
  name                 = "lb-frontend-${each.key}"
  mode                 = "tcp"
  port                 = each.value.port
  default_backend_name = upcloud_loadbalancer_backend.lb_backend[each.key].name
}

resource "upcloud_loadbalancer_static_backend_member" "lb_backend_member" {
  for_each = {
    for be_server in local.lb_backend_servers: 
      "${be_server.server_name}-lb-backend-${be_server.lb_name}" => be_server
      if var.loadbalancer_enabled
  }

  backend      = upcloud_loadbalancer_backend.lb_backend[each.value.lb_name].id
  name         = "${local.resource-prefix}${each.key}"
  ip           = merge(upcloud_server.master, upcloud_server.worker)[each.value.server_name].network_interface[1].ip_address
  port         = each.value.port
  weight       = 100
  max_sessions = var.loadbalancer_plan == "production-small" ? 50000 : 1000
  enabled      = true
}
