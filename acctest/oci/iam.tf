# Copyright 2026 YugabyteDB, Inc.
# SPDX-License-Identifier: MPL-2.0

# The test identities live in an identity domain in the stand's compartment,
# and the policies attach to that compartment, so the operator needs no
# tenancy-level IAM rights.
data "oci_identity_domains" "yba" {
  provider       = oci.home
  compartment_id = var.compartment_ocid
  display_name   = var.identity_domain

  lifecycle {
    postcondition {
      condition = anytrue([
        for d in self.domains : d.home_region == var.test_region ||
        contains([for r in d.replica_regions : r.region], var.test_region)
      ])
      error_message = "identity_domain must be replicated to test_region; run oci iam domain enable-replication-to-region."
    }
  }
}

locals {
  idcs_endpoint = one(data.oci_identity_domains.yba.domains).url
}

resource "tls_private_key" "api" {
  algorithm = "RSA"
  rsa_bits  = 2048
}

resource "oci_identity_domains_user" "yba" {
  provider      = oci.home
  idcs_endpoint = local.idcs_endpoint
  schemas       = ["urn:ietf:params:scim:schemas:core:2.0:User"]
  user_name     = "${var.prefix}-yba"
  description   = "API key user for yba_oci_provider acceptance tests"

  name {
    family_name = "${var.prefix}-yba"
  }

  emails {
    type    = "work"
    value   = var.yba_username
    primary = true
  }

  # The domain adds extension schemas and a recovery email on create.
  lifecycle {
    ignore_changes = [schemas, emails]
  }
}

resource "oci_identity_domains_api_key" "yba" {
  provider      = oci.home
  idcs_endpoint = local.idcs_endpoint
  schemas       = ["urn:ietf:params:scim:schemas:oracle:idcs:apikey"]
  key           = chomp(tls_private_key.api.public_key_pem)

  user {
    value = oci_identity_domains_user.yba.id
  }
}

resource "oci_identity_domains_group" "yba" {
  provider      = oci.home
  idcs_endpoint = local.idcs_endpoint
  schemas       = ["urn:ietf:params:scim:schemas:core:2.0:Group"]
  display_name  = "${var.prefix}-yba"

  members {
    type  = "User"
    value = oci_identity_domains_user.yba.id
  }

  # The domain adds extension schemas, and its group read omits members.
  lifecycle {
    ignore_changes = [schemas, members]
  }
}

resource "oci_identity_policy" "api" {
  provider       = oci.home
  compartment_id = var.compartment_ocid
  name           = "${var.prefix}-api"
  description    = "Let the API-key user manage the acceptance compartment"
  statements = [
    "Allow group ${var.identity_domain}/${oci_identity_domains_group.yba.display_name} to manage all-resources in compartment id ${var.compartment_ocid}",
  ]
}

resource "oci_identity_domains_dynamic_resource_group" "yba" {
  provider      = oci.home
  idcs_endpoint = local.idcs_endpoint
  schemas       = ["urn:ietf:params:scim:schemas:oracle:idcs:DynamicResourceGroup"]
  display_name  = "${var.prefix}-yba-instance"
  description   = "YBA acceptance-test instance principal"
  matching_rule = "instance.id = '${oci_core_instance.yba.id}'"

  # The domain adds extension schemas on create.
  lifecycle {
    ignore_changes = [schemas]
  }
}

resource "oci_identity_policy" "instance" {
  provider       = oci.home
  compartment_id = var.compartment_ocid
  name           = "${var.prefix}-instance"
  description    = "Let the YBA VM manage the acceptance compartment"
  statements = [
    "Allow dynamic-group ${var.identity_domain}/${oci_identity_domains_dynamic_resource_group.yba.display_name} to manage all-resources in compartment id ${var.compartment_ocid}",
  ]
}

data "oci_core_images" "ol9" {
  provider                 = oci.home
  compartment_id           = var.compartment_ocid
  operating_system         = "Oracle Linux"
  operating_system_version = "9"
  shape                    = var.yba_shape
  sort_by                  = "TIMECREATED"
  sort_order               = "DESC"
  state                    = "AVAILABLE"
}

data "oci_core_images" "test" {
  provider                 = oci.test
  compartment_id           = var.compartment_ocid
  operating_system         = "Oracle Linux"
  operating_system_version = "9"
  shape                    = var.yba_shape
  sort_by                  = "TIMECREATED"
  sort_order               = "DESC"
  state                    = "AVAILABLE"
}

# Used by the test region (OCI_REGION), so it lives there.
resource "oci_core_instance_configuration" "ybdb" {
  provider       = oci.test
  compartment_id = var.compartment_ocid
  display_name   = "${var.prefix}-ybdb"

  instance_details {
    instance_type = "compute"
    launch_details {
      compartment_id = var.compartment_ocid
      display_name   = "${var.prefix}-ybdb"
      shape          = var.yba_shape

      shape_config {
        ocpus         = var.yba_ocpus
        memory_in_gbs = var.yba_memory_in_gbs
      }

      source_details {
        source_type = "image"
        image_id    = data.oci_core_images.test.images[0].id
      }

      create_vnic_details {
        subnet_id        = oci_core_subnet.ybdb[0].id
        assign_public_ip = "true"
      }
    }
  }

  # instance_details is ForceNew: a newer image would replace this
  # configuration and change the OCID in the test env.
  lifecycle {
    ignore_changes = [instance_details[0].launch_details[0].source_details[0].image_id]
  }
}
