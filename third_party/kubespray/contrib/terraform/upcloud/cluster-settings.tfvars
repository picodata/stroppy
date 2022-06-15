# See: https://developers.upcloud.com/1.3/5-zones/
zone     = "fi-hel1"
username = "ubuntu"

# Prefix to use for all resources to separate them from other resources
prefix = "kubespray"

inventory_file = "inventory.ini"

#  Set the operating system using UUID or exact name
template_name = "Ubuntu Server 20.04 LTS (Focal Fossa)"

ssh_public_keys = [
  # Put your public SSH key here
  "ssh-rsa public key 1",
  "ssh-rsa public key 2",
]

# check list of available plan https://developers.upcloud.com/1.3/7-plans/
machines = {
  "master-0" : {
    "node_type" : "master",
    # plan to use instead of custom cpu/mem
    "plan" : null,
    #number of cpu cores
    "cpu" : "2",
    #memory size in MB
    "mem" : "4096"
    # The size of the storage in GB
    "disk_size" : 250
    "additional_disks" : {}
  },
  "worker-0" : {
    "node_type" : "worker",
    # plan to use instead of custom cpu/mem
    "plan" : null,
    #number of cpu cores
    "cpu" : "2",
    #memory size in MB
    "mem" : "4096"
    # The size of the storage in GB
    "disk_size" : 250
    "additional_disks" : {
      # "some-disk-name-1": {
      #   "size": 100,
      #   "tier": "maxiops",
      # },
      # "some-disk-name-2": {
      #   "size": 100,
      #   "tier": "maxiops",
      # }
    }
  },
  "worker-1" : {
    "node_type" : "worker",
    # plan to use instead of custom cpu/mem
    "plan" : null,
    #number of cpu cores
    "cpu" : "2",
    #memory size in MB
    "mem" : "4096"
    # The size of the storage in GB
    "disk_size" : 250
    "additional_disks" : {
      # "some-disk-name-1": {
      #   "size": 100,
      #   "tier": "maxiops",
      # },
      # "some-disk-name-2": {
      #   "size": 100,
      #   "tier": "maxiops",
      # }
    }
  },
  "worker-2" : {
    "node_type" : "worker",
    # plan to use instead of custom cpu/mem
    "plan" : null,
    #number of cpu cores
    "cpu" : "2",
    #memory size in MB
    "mem" : "4096"
    # The size of the storage in GB
    "disk_size" : 250
    "additional_disks" : {
      # "some-disk-name-1": {
      #   "size": 100,
      #   "tier": "maxiops",
      # },
      # "some-disk-name-2": {
      #   "size": 100,
      #   "tier": "maxiops",
      # }
    }
  }
}

firewall_enabled = false

master_allowed_remote_ips = [
  {
    "start_address" : "0.0.0.0"
    "end_address" : "255.255.255.255"
  }
]

k8s_allowed_remote_ips = [
  {
    "start_address" : "0.0.0.0"
    "end_address" : "255.255.255.255"
  }
]

loadbalancer_enabled = false
loadbalancer_plan    = "development"
loadbalancers = {
  # "http" : {
  #   "port" : 80,
  #   "backend_servers" : [
  #     "worker-0",
  #     "worker-1",
  #     "worker-2"
  #   ]
  # }
}
