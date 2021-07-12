package terraform

import (
	"fmt"
	"io/ioutil"
	"path/filepath"

	"github.com/ansel1/merry"
	hcl "github.com/hashicorp/hcl/v2"
	hcl2 "github.com/hashicorp/hcl/v2/hclwrite"
	llog "github.com/sirupsen/logrus"
	"github.com/zclconf/go-cty/cty"
)

const instanceFileName = "main_oracle-derived.tf"

func setVariableBlock(instanceFileBody *hcl2.Body, cpu int,
	ram int, diskString string, nodesString string) {
	tenancyOcidBlock := instanceFileBody.AppendNewBlock("variable", []string{"tenancy_ocid"})
	tenancyOcidBody := tenancyOcidBlock.Body()
	tenancyOcidBody.SetAttributeValue("default",
		cty.StringVal("ocid1.tenancy.oc1..aaaaaaaa57fvsimy5ma5gs7e6yzmhbypafasi2v3huvbcgrv3sxmos4tvawa"))

	instanceFileBody.AppendNewline()

	userOsidBlock := instanceFileBody.AppendNewBlock("variable", []string{"user_ocid"})
	userOsidBody := userOsidBlock.Body()
	userOsidBody.SetAttributeValue("default",
		cty.StringVal("ocid1.user.oc1..aaaaaaaa4nnnmsokuq5emtptzzaf5pxgrv3qovuytokfpfej35l3ni5ukijq"))

	instanceFileBody.AppendNewline()

	fingerPrintBlock := instanceFileBody.AppendNewBlock("variable", []string{"fingerprint"})
	fingerPrintBody := fingerPrintBlock.Body()
	fingerPrintBody.SetAttributeValue("default", cty.StringVal("ec:c8:5e:42:9c:71:a8:73:e5:c5:df:15:62:ee:f0:3c"))

	instanceFileBody.AppendNewline()

	privateKeyPathBlock := instanceFileBody.AppendNewBlock("variable", []string{"private_key_path"})
	privateKeyPathBody := privateKeyPathBlock.Body()
	privateKeyPathBody.SetAttributeTraversal("type", hcl.Traversal{
		//nolint:exhaustivestruct
		hcl.TraverseRoot{
			Name: "string",
		},
	})
	privateKeyPathBody.SetAttributeValue("default", cty.StringVal("private_key.pem"))

	instanceFileBody.AppendNewline()

	regionBlock := instanceFileBody.AppendNewBlock("variable", []string{"region"})
	regionBody := regionBlock.Body()
	regionBody.SetAttributeValue("default", cty.StringVal("eu-frankfurt-1"))

	instanceFileBody.AppendNewline()

	compartmentOcidBlock := instanceFileBody.AppendNewBlock("variable", []string{"compartment_ocid"})
	compartmentBody := compartmentOcidBlock.Body()
	compartmentBody.SetAttributeValue("default",
		cty.StringVal("ocid1.compartment.oc1..aaaaaaaahdkwtglvmc7yi37ohu5n7fcbfneufqxxurmj3njokujp5nfoxjpa"))

	instanceFileBody.AppendNewline()

	sshPublicKeyBlock := instanceFileBody.AppendNewBlock("variable", []string{"ssh_public_key"})
	sshPublickeyBody := sshPublicKeyBlock.Body()
	//nolint:lll
	sshPublickeyBody.SetAttributeValue("default",
		cty.StringVal(`ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQCnPYxOLBLVVzRrWlw96AOzavA034a2tV1G5rtM6b7yUc5J9Vi2g3uvAj2idlRWnumEMrm1E6Pr6LHRr1oChDSCrcfIxl8oJZQW5eQsPPtRKj9fE8v6J3Nr8hMIAflG/SBqpGQmxhRqvcuuf7RHxs8EqsnOaXxUtbtZNDSo+VZj45rVh3BSg0TxSKfDrRNRw3/HO0KtYYlH8J1VIYl9t0tlrXZEndShS9LCat/EmBjSG1dtUdzz3jo3L67cJ7Qigcg1U2drzQ78yCJHRM6oTFEQkfO+WnjDm97+zxGordWhejaVzwARP4TjDBWAZVdxHUl3yAb02nHnRkHmtliuLBcX`))

	instanceFileBody.AppendNewline()

	providerOCIblock := instanceFileBody.AppendNewBlock("provider", []string{"oci"})
	providerOCIbody := providerOCIblock.Body()
	providerOCIbody.SetAttributeTraversal("tenancy_ocid", hcl.Traversal{
		//nolint:exhaustivestruct
		hcl.TraverseRoot{
			Name: "var",
		},
		//nolint:exhaustivestruct
		hcl.TraverseAttr{
			Name: "tenancy_ocid",
		},
	})
	providerOCIbody.SetAttributeTraversal("user_ocid", hcl.Traversal{
		//nolint:exhaustivestruct
		hcl.TraverseRoot{
			Name: "var",
		},
		//nolint:exhaustivestruct
		hcl.TraverseAttr{
			Name: "user_ocid",
		},
	})
	providerOCIbody.SetAttributeTraversal("fingerprint", hcl.Traversal{
		//nolint:exhaustivestruct
		hcl.TraverseRoot{
			Name: "var",
		},
		//nolint:exhaustivestruct
		hcl.TraverseAttr{
			Name: "fingerprint",
		},
	})
	providerOCIbody.SetAttributeTraversal("private_key_path", hcl.Traversal{
		//nolint:exhaustivestruct
		hcl.TraverseRoot{
			Name: "var",
		},
		//nolint:exhaustivestruct
		hcl.TraverseAttr{
			Name: "private_key_path",
		},
	})
	providerOCIbody.SetAttributeTraversal("region", hcl.Traversal{
		//nolint:exhaustivestruct
		hcl.TraverseRoot{
			Name: "var",
		},
		//nolint:exhaustivestruct
		hcl.TraverseAttr{
			Name: "region",
		},
	})

	numInstancesBlock := instanceFileBody.AppendNewBlock("variable", []string{"num_instances"})
	numInstancesBody := numInstancesBlock.Body()
	numInstancesBody.SetAttributeValue("default", cty.StringVal(nodesString))

	instanceFileBody.AppendNewline()

	numIsciVolumesPerInstanceBlock := instanceFileBody.AppendNewBlock("variable",
		[]string{"num_iscsi_volumes_per_instance"})
	numIsciVolumesPerInstanceBody := numIsciVolumesPerInstanceBlock.Body()
	numIsciVolumesPerInstanceBody.SetAttributeValue("default", cty.StringVal("1"))

	instanceFileBody.AppendNewline()

	instanceShapeBlock := instanceFileBody.AppendNewBlock("variable", []string{"instance_shape"})
	instanceShapeBody := instanceShapeBlock.Body()
	instanceShapeBody.SetAttributeValue("default", cty.StringVal("VM.Standard.E3.Flex"))

	instanceFileBody.AppendNewline()

	instanceOcpusBlock := instanceFileBody.AppendNewBlock("variable", []string{"instance_ocpus"})
	instanceOcpusBody := instanceOcpusBlock.Body()
	instanceOcpusBody.SetAttributeValue("default", cty.NumberIntVal(int64(cpu)))

	instanceFileBody.AppendNewline()

	configMemoryInGBSBlock := instanceFileBody.AppendNewBlock("variable", []string{"instance_shape_config_memory_in_gbs"})
	configMemoryInGBSBody := configMemoryInGBSBlock.Body()
	configMemoryInGBSBody.SetAttributeValue("default", cty.NumberIntVal(int64(ram)))

	instanceFileBody.AppendNewline()

	flexInstamceImageOcidBlock := instanceFileBody.AppendNewBlock("variable", []string{"flex_instance_image_ocid"})
	flexInstamceImageOcidBody := flexInstamceImageOcidBlock.Body()
	flexInstamceImageOcidBody.SetAttributeTraversal("type", hcl.Traversal{
		//nolint:exhaustivestruct
		hcl.TraverseRoot{
			Name: "map(string)",
		},
	})
	euFrankfurt1 := hcl.Traversal{
		//nolint:exhaustivestruct
		hcl.TraverseRoot{
			Name: `{
eu-frankfurt-1  = "ocid1.image.oc1.eu-frankfurt-1.aaaaaaaaw4ap4pklk3lo5pls5rppt2vfhjvuukpi2fltc74ycmaz3w7bz2aq" 
	}`,
		},
	}
	flexInstamceImageOcidBody.SetAttributeTraversal("default", euFrankfurt1)

	instanceFileBody.AppendNewline()

	dbSizeBlock := instanceFileBody.AppendNewBlock("variable", []string{"db_size"})
	dbSizeBody := dbSizeBlock.Body()
	dbSizeBody.SetAttributeValue("default", cty.StringVal(diskString))

	instanceFileBody.AppendNewline()

	ociCoreInstanceBlock := instanceFileBody.AppendNewBlock("resource", []string{"oci_core_instance", "k8s_instance"})
	ociCoreInstanceBody := ociCoreInstanceBlock.Body()
	ociCoreInstanceBody.SetAttributeTraversal("count", hcl.Traversal{
		//nolint:exhaustivestruct
		hcl.TraverseRoot{
			Name: "var",
		},
		//nolint:exhaustivestruct
		hcl.TraverseAttr{
			Name: "num_instances",
		},
	})
	ociCoreInstanceBody.SetAttributeTraversal("availability_domain", hcl.Traversal{
		//nolint:exhaustivestruct
		hcl.TraverseRoot{
			Name: "data",
		},
		//nolint:exhaustivestruct
		hcl.TraverseAttr{
			Name: "oci_identity_availability_domain.ad.name",
		},
	})
	ociCoreInstanceBody.SetAttributeTraversal("compartment_id", hcl.Traversal{
		//nolint:exhaustivestruct
		hcl.TraverseRoot{
			Name: "var",
		},
		//nolint:exhaustivestruct
		hcl.TraverseAttr{
			Name: "compartment_ocid",
		},
	})
	ociCoreInstanceBody.SetAttributeTraversal("display_name", hcl.Traversal{
		//nolint:exhaustivestruct
		hcl.TraverseRoot{
			Name: "\"K8sNode${count.index}\"",
		},
	})
	ociCoreInstanceBody.SetAttributeTraversal("shape", hcl.Traversal{
		//nolint:exhaustivestruct
		hcl.TraverseRoot{
			Name: "var",
		},
		//nolint:exhaustivestruct
		hcl.TraverseAttr{
			Name: "instance_shape",
		},
	})

	instanceFileBody.AppendNewline()

	shareConfigBlock := ociCoreInstanceBody.AppendNewBlock("shape_config", nil)
	shareConfigBody := shareConfigBlock.Body()
	shareConfigBody.SetAttributeTraversal("ocpus", hcl.Traversal{
		//nolint:exhaustivestruct
		hcl.TraverseRoot{
			Name: "var.instance_ocpus",
		},
	},
	)
	shareConfigBody.SetAttributeTraversal("memory_in_gbs", hcl.Traversal{
		//nolint:exhaustivestruct
		hcl.TraverseRoot{
			Name: "var.instance_shape_config_memory_in_gbs",
		},
	},
	)

	createVnicDetailsBlock := ociCoreInstanceBody.AppendNewBlock("create_vnic_details", nil)
	createVnicDetailsBody := createVnicDetailsBlock.Body()
	createVnicDetailsBody.SetAttributeTraversal("subnet_id", hcl.Traversal{
		//nolint:exhaustivestruct
		hcl.TraverseRoot{
			Name: "oci_core_subnet.k8s_cluster_subnet",
		},
		//nolint:exhaustivestruct
		hcl.TraverseAttr{
			Name: "id",
		},
	})
	createVnicDetailsBody.SetAttributeValue("display_name", cty.StringVal("Primaryvnic"))
	createVnicDetailsBody.SetAttributeTraversal("assign_public_ip", hcl.Traversal{
		//nolint:exhaustivestruct
		hcl.TraverseRoot{
			Name: "true",
		},
	})
	createVnicDetailsBody.SetAttributeTraversal("assign_private_dns_record", hcl.Traversal{
		//nolint:exhaustivestruct
		hcl.TraverseRoot{
			Name: "true",
		},
	})
	createVnicDetailsBody.SetAttributeTraversal("hostname_label", hcl.Traversal{
		//nolint:exhaustivestruct
		hcl.TraverseRoot{
			Name: "\"k8s-node${count.index}\"",
		},
	})

	instanceFileBody.AppendNewline()

	sourceDetailsBlock := ociCoreInstanceBody.AppendNewBlock("source_details", nil)
	sourceDetailsBody := sourceDetailsBlock.Body()
	sourceDetailsBody.SetAttributeValue("source_type", cty.StringVal("image"))
	sourceDetailsBody.SetAttributeTraversal("source_id", hcl.Traversal{
		//nolint:exhaustivestruct
		hcl.TraverseRoot{
			Name: "var",
		},
		//nolint:exhaustivestruct
		hcl.TraverseAttr{
			Name: "flex_instance_image_ocid[var.region]",
		},
	})

	metadata := hcl.Traversal{
		//nolint:exhaustivestruct
		hcl.TraverseRoot{
			Name: "{\nssh_authorized_keys = var.ssh_public_key\n }",
		},
	}

	ociCoreInstanceBody.SetAttributeTraversal("metadata", metadata)

	freedomTags := hcl.Traversal{
		//nolint:exhaustivestruct
		hcl.TraverseRoot{
			Name: "{\n\"freeformkey${count.index}\" = \"freeformvalue${count.index}\"\n}",
		},
	}
	ociCoreInstanceBody.SetAttributeTraversal("freeform_tags", freedomTags)

	instanceFileBody.AppendNewline()

	timeoutBlock := ociCoreInstanceBody.AppendNewBlock("timeouts", nil)
	timeBody := timeoutBlock.Body()
	timeBody.SetAttributeValue("create", cty.StringVal("60m"))

	instanceFileBody.AppendNewline()

	ociCoreVolumeBlock := instanceFileBody.AppendNewBlock("resource", []string{"oci_core_volume", "block_volume"})
	ociCoreVolumeBody := ociCoreVolumeBlock.Body()
	ociCoreVolumeBody.SetAttributeTraversal("count", hcl.Traversal{
		//nolint:exhaustivestruct
		hcl.TraverseRoot{
			Name: "var.num_instances * var.num_iscsi_volumes_per_instance",
		},
	})
	ociCoreVolumeBody.SetAttributeTraversal("availability_domain", hcl.Traversal{
		//nolint:exhaustivestruct
		hcl.TraverseRoot{
			Name: "data.oci_identity_availability_domain.ad.name",
		},
	})
	ociCoreVolumeBody.SetAttributeTraversal("compartment_id", hcl.Traversal{
		//nolint:exhaustivestruct
		hcl.TraverseRoot{
			Name: "var.compartment_ocid",
		},
	})
	ociCoreVolumeBody.SetAttributeTraversal("display_name", hcl.Traversal{
		//nolint:exhaustivestruct
		hcl.TraverseRoot{
			Name: "\"BlockVolume${count.index}\"",
		},
	})
	ociCoreVolumeBody.SetAttributeTraversal("size_in_gbs", hcl.Traversal{
		//nolint:exhaustivestruct
		hcl.TraverseRoot{
			Name: "var.db_size",
		},
	})

	ociCOreVolumeAttachmentBlock := instanceFileBody.AppendNewBlock("resource",
		[]string{"oci_core_volume_attachment", "test_block_attach"})
	ociCOreVolumeAttachmentBody := ociCOreVolumeAttachmentBlock.Body()
	ociCOreVolumeAttachmentBody.SetAttributeTraversal("count", hcl.Traversal{
		//nolint:exhaustivestruct
		hcl.TraverseRoot{
			Name: "var.num_instances * var.num_iscsi_volumes_per_instance",
		},
	})
	ociCOreVolumeAttachmentBody.SetAttributeValue("attachment_type", cty.StringVal("iscsi"))
	ociCOreVolumeAttachmentBody.SetAttributeTraversal("instance_id", hcl.Traversal{
		//nolint:exhaustivestruct
		hcl.TraverseRoot{
			Name: "oci_core_instance.k8s_instance[floor(count.index / var.num_iscsi_volumes_per_instance)].id",
		},
	})
	ociCOreVolumeAttachmentBody.SetAttributeTraversal("volume_id", hcl.Traversal{
		//nolint:exhaustivestruct
		hcl.TraverseRoot{
			Name: "oci_core_volume.block_volume[count.index].id",
		},
	})
	ociCOreVolumeAttachmentBody.SetAttributeTraversal("device", hcl.Traversal{
		//nolint:exhaustivestruct
		hcl.TraverseRoot{
			Name: "count.index == 0 ? \"/dev/oracleoci/oraclevdb\" : \"\"",
		},
	})
	ociCOreVolumeAttachmentBody.SetAttributeTraversal("use_chap", hcl.Traversal{
		//nolint:exhaustivestruct
		hcl.TraverseRoot{
			Name: "false",
		},
	})
	instanceFileBody.AppendNewline()

	ociCoreInstanceDevicesBlock := instanceFileBody.AppendNewBlock("data",
		[]string{"oci_core_instance_devices", "k8s_instance_devices"})
	ociCoreInstanceDevicesBody := ociCoreInstanceDevicesBlock.Body()
	ociCoreInstanceDevicesBody.SetAttributeTraversal("count", hcl.Traversal{
		//nolint:exhaustivestruct
		hcl.TraverseRoot{
			Name: "var.num_instances",
		},
	})
	ociCoreInstanceDevicesBody.SetAttributeTraversal("instance_id", hcl.Traversal{
		//nolint:exhaustivestruct
		hcl.TraverseRoot{
			Name: "oci_core_instance.k8s_instance[count.index].id",
		},
	})

	instanceFileBody.AppendNewline()

	instancePrivateIpsBlock := instanceFileBody.AppendNewBlock("output", []string{"instance_private_ips"})
	instancePrivateIpsBody := instancePrivateIpsBlock.Body()
	instancePrivateIpsBody.SetAttributeTraversal("value", hcl.Traversal{
		//nolint:exhaustivestruct
		hcl.TraverseRoot{
			Name: "[oci_core_instance.k8s_instance.*.private_ip]",
		},
	})

	instancePublicIpsBlock := instanceFileBody.AppendNewBlock("output", []string{"instance_public_ips"})
	instancePublicIpsBody := instancePublicIpsBlock.Body()
	instancePublicIpsBody.SetAttributeTraversal("value", hcl.Traversal{
		//nolint:exhaustivestruct
		hcl.TraverseRoot{
			Name: "[oci_core_instance.k8s_instance.*.public_ip]",
		},
	})

	instanceFileBody.AppendNewline()

	instanceDevicesBlock := instanceFileBody.AppendNewBlock("output", []string{"instance_devices"})
	instanceDevicesBody := instanceDevicesBlock.Body()
	instanceDevicesBody.SetAttributeTraversal("value", hcl.Traversal{
		//nolint:exhaustivestruct
		hcl.TraverseRoot{
			Name: "[data.oci_core_instance_devices.k8s_instance_devices.*.devices]",
		},
	})

	instanceFileBody.AppendNewline()

	ociCoreVcnBlock := instanceFileBody.AppendNewBlock("resource", []string{"oci_core_vcn", "k8s_cluster_vcn"})
	ociCoreVcnBody := ociCoreVcnBlock.Body()
	ociCoreVcnBody.SetAttributeValue("cidr_block", cty.StringVal("10.1.0.0/16"))
	ociCoreVcnBody.SetAttributeTraversal("compartment_id", hcl.Traversal{
		//nolint:exhaustivestruct
		hcl.TraverseRoot{
			Name: "var.compartment_ocid",
		},
	})
	ociCoreVcnBody.SetAttributeValue("display_name", cty.StringVal("K8sVcn"))
	ociCoreVcnBody.SetAttributeValue("dns_label", cty.StringVal("K8sVcn"))

	instanceFileBody.AppendNewline()

	icoCoreVcnBlock := instanceFileBody.AppendNewBlock("resource",
		[]string{"oci_core_internet_gateway", "k8s_cluster_internet_gateway"})
	icoCoreVcnBody := icoCoreVcnBlock.Body()
	icoCoreVcnBody.SetAttributeTraversal("compartment_id", hcl.Traversal{
		//nolint:exhaustivestruct
		hcl.TraverseRoot{
			Name: "var.compartment_ocid",
		},
	})
	icoCoreVcnBody.SetAttributeValue("display_name", cty.StringVal("K8sInternetGateway"))
	icoCoreVcnBody.SetAttributeTraversal("vcn_id", hcl.Traversal{
		//nolint:exhaustivestruct
		hcl.TraverseRoot{
			Name: "oci_core_vcn.k8s_cluster_vcn.id",
		},
	})

	instanceFileBody.AppendNewline()

	icoCoreDefaultRouteTableBlock := instanceFileBody.AppendNewBlock("resource",
		[]string{"oci_core_default_route_table", "default_route_table"})
	icoCoreDefaultRouteTableBody := icoCoreDefaultRouteTableBlock.Body()
	icoCoreDefaultRouteTableBody.SetAttributeTraversal("manage_default_resource_id", hcl.Traversal{
		//nolint:exhaustivestruct
		hcl.TraverseRoot{
			Name: "oci_core_vcn.k8s_cluster_vcn.default_route_table_id",
		},
	})
	icoCoreDefaultRouteTableBody.SetAttributeValue("display_name", cty.StringVal("DefaultRouteTable"))

	instanceFileBody.AppendNewline()

	routeRulesBlock := icoCoreDefaultRouteTableBody.AppendNewBlock("route_rules", nil)
	routeRulesBody := routeRulesBlock.Body()
	routeRulesBody.SetAttributeValue("destination", cty.StringVal("0.0.0.0/0"))
	routeRulesBody.SetAttributeValue("destination_type", cty.StringVal("CIDR_BLOCK"))
	routeRulesBody.SetAttributeTraversal("network_entity_id", hcl.Traversal{
		//nolint:exhaustivestruct
		hcl.TraverseRoot{
			Name: "oci_core_internet_gateway.k8s_cluster_internet_gateway.id",
		},
	})

	instanceFileBody.AppendNewline()

	ociCoreSubnetBlock := instanceFileBody.AppendNewBlock("resource",
		[]string{"oci_core_subnet", "k8s_cluster_subnet"})
	ociCoreSubnetBody := ociCoreSubnetBlock.Body()
	ociCoreSubnetBody.SetAttributeTraversal("availability_domain", hcl.Traversal{
		//nolint:exhaustivestruct
		hcl.TraverseRoot{
			Name: "data.oci_identity_availability_domain.ad.name",
		},
	})
	ociCoreSubnetBody.SetAttributeValue("cidr_block", cty.StringVal("10.1.20.0/24"))
	ociCoreSubnetBody.SetAttributeValue("display_name", cty.StringVal("K8sSubnet"))
	ociCoreSubnetBody.SetAttributeValue("dns_label", cty.StringVal("K8sSubnet"))
	ociCoreSubnetBody.SetAttributeTraversal("security_list_ids", hcl.Traversal{
		//nolint:exhaustivestruct
		hcl.TraverseRoot{
			Name: "[oci_core_security_list.etcd2_security_list.id]",
		},
	})
	ociCoreSubnetBody.SetAttributeTraversal("compartment_id", hcl.Traversal{
		//nolint:exhaustivestruct
		hcl.TraverseRoot{
			Name: "var.compartment_ocid",
		},
	})
	ociCoreSubnetBody.SetAttributeTraversal("vcn_id", hcl.Traversal{
		//nolint:exhaustivestruct
		hcl.TraverseRoot{
			Name: "oci_core_vcn.k8s_cluster_vcn.id",
		},
	})
	ociCoreSubnetBody.SetAttributeTraversal("route_table_id", hcl.Traversal{
		//nolint:exhaustivestruct
		hcl.TraverseRoot{
			Name: "oci_core_vcn.k8s_cluster_vcn.default_route_table_id",
		},
	})
	ociCoreSubnetBody.SetAttributeTraversal("dhcp_options_id", hcl.Traversal{
		//nolint:exhaustivestruct
		hcl.TraverseRoot{
			Name: "oci_core_vcn.k8s_cluster_vcn.default_dhcp_options_id",
		},
	})

	instanceFileBody.AppendNewline()

	ociIdentityAvailableDomainBlock := instanceFileBody.AppendNewBlock("data",
		[]string{"oci_identity_availability_domain", "ad"})
	ociIdentityAvailableDomainBody := ociIdentityAvailableDomainBlock.Body()
	ociIdentityAvailableDomainBody.SetAttributeTraversal("compartment_id", hcl.Traversal{
		//nolint:exhaustivestruct
		hcl.TraverseRoot{
			Name: "var.compartment_ocid",
		},
	})
	ociIdentityAvailableDomainBody.SetAttributeValue("ad_number", cty.NumberIntVal(1))

	instanceFileBody.AppendNewline()

	ociCoreSecurityListBlock := instanceFileBody.AppendNewBlock("resource",
		[]string{"oci_core_security_list", "etcd2_security_list"})
	ociCoreSecurityListBody := ociCoreSecurityListBlock.Body()
	ociCoreSecurityListBody.SetAttributeTraversal("compartment_id", hcl.Traversal{
		//nolint:exhaustivestruct
		hcl.TraverseRoot{
			Name: "var.compartment_ocid",
		},
	})
	ociCoreSecurityListBody.SetAttributeTraversal("vcn_id", hcl.Traversal{
		//nolint:exhaustivestruct
		hcl.TraverseRoot{
			Name: "oci_core_vcn.k8s_cluster_vcn.id",
		},
	})
	ociCoreSecurityListBody.SetAttributeValue("display_name", cty.StringVal("Etcd2SecurityList"))

	egressSecurityRulesBlock6 := ociCoreSecurityListBody.AppendNewBlock("egress_security_rules", nil)
	egressSecurityRulesBody6 := egressSecurityRulesBlock6.Body()
	egressSecurityRulesBody6.SetAttributeValue("protocol", cty.StringVal("6"))
	egressSecurityRulesBody6.SetAttributeValue("destination", cty.StringVal("0.0.0.0/0"))

	egressSecurityRulesBlock17 := ociCoreSecurityListBody.AppendNewBlock("egress_security_rules", nil)
	egressSecurityRulesBody17 := egressSecurityRulesBlock17.Body()
	egressSecurityRulesBody17.SetAttributeValue("protocol", cty.StringVal("17"))
	egressSecurityRulesBody17.SetAttributeValue("destination", cty.StringVal("0.0.0.0/0"))

	ingressSecurityRulesBlock6 := ociCoreSecurityListBody.AppendNewBlock("ingress_security_rules", nil)
	ingressSecurityRulesBody6 := ingressSecurityRulesBlock6.Body()
	ingressSecurityRulesBody6.SetAttributeValue("protocol", cty.StringVal("6"))
	ingressSecurityRulesBody6.SetAttributeValue("source", cty.StringVal("0.0.0.0/0"))

	ingressSecurityRulesBlock17 := ociCoreSecurityListBody.AppendNewBlock("ingress_security_rules", nil)
	ingressSecurityRulesBody17 := ingressSecurityRulesBlock17.Body()
	ingressSecurityRulesBody17.SetAttributeValue("protocol", cty.StringVal("17"))
	ingressSecurityRulesBody17.SetAttributeValue("source", cty.StringVal("0.0.0.0/0"))
}

func PrepareOracle(cpu, ram, disk, nodes int, wd string) (err error) {
	instanceFile := hcl2.NewEmptyFile()
	instanceFileBody := instanceFile.Body()
	instanceFileBody.AppendNewline()

	nodesString := fmt.Sprintf("%d", nodes)
	diskString := fmt.Sprintf("%d", disk)
	setVariableBlock(instanceFileBody, cpu, ram, diskString, nodesString)

	providerConfigFilePath := filepath.Join(wd, instanceFileName)
	if err = ioutil.WriteFile(providerConfigFilePath, instanceFile.Bytes(), 0o600); err != nil {
		return merry.Prepend(err, "failed to write provider configuration file")
	}

	llog.Infoln("Generation provider configuration file: success")
	return
}
