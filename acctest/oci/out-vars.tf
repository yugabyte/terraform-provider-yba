# Copyright 2026 YugabyteDB, Inc.
# SPDX-License-Identifier: MPL-2.0

locals {
  test_env = <<-EOT
    TF_VAR_OCI_TENANCY_ID='${var.tenancy_ocid}'
    TF_VAR_OCI_USER_ID='${oci_identity_user.yba.id}'
    TF_VAR_OCI_FINGERPRINT='${oci_identity_api_key.yba.fingerprint}'
    TF_VAR_OCI_PRIVATE_KEY_B64='${base64encode(tls_private_key.api.private_key_pem)}'
    TF_VAR_OCI_COMPARTMENT_ID='${var.compartment_ocid}'
    TF_VAR_OCI_REGION='${var.home_region}'
    TF_VAR_OCI_REGION_2='${var.region_2}'
    TF_VAR_OCI_VCN_ID='${oci_core_vcn.main.id}'
    TF_VAR_OCI_VCN_ID_2='${oci_core_vcn.region2.id}'
    TF_VAR_OCI_SUBNET_ID='${oci_core_subnet.ybdb[0].id}'
    TF_VAR_OCI_SUBNET_ID_2='${oci_core_subnet.ybdb[1].id}'
    TF_VAR_OCI_SUBNET_REGION_2='${oci_core_subnet.region2.id}'
    TF_VAR_OCI_IMAGE_ID='${data.oci_core_images.ol9.images[0].id}'
    TF_VAR_OCI_IMAGE_ID_2='${data.oci_core_images.region2.images[0].id}'
    TF_VAR_OCI_INSTANCE_CONFIGURATION_ID='${oci_core_instance_configuration.ybdb.id}'
    TF_VAR_OCI_YBA_HOST='${oci_core_instance.yba.public_ip}'
    TF_VAR_OCI_YBA_API_KEY='${yba_customer_resource.customer.api_token}'
  EOT
}

output "test_env" {
  description = "Acceptance-test env (TF_VAR_OCI_*) as KEY='value' lines."
  value       = local.test_env
  sensitive   = true
}
