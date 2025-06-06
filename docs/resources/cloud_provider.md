---
page_title: "yba_cloud_provider Resource - YugabyteDB Anywhere"
description: |-
  Cloud Provider Resource.
---

# yba_cloud_provider (Resource)

Cloud Provider Resource.

The following credentials are required as environment variables (if fields are not set) to configure the corresponding Cloud Providers:

|Cloud Provider|Setting|Configuration Field|Environment Variable|
|-------|--------|---------|-------------------------------|
|[AWS](https://docs.aws.amazon.com/cli/latest/userguide/cli-configure-envvars.html)||||
||Access Key ID|`aws_config_settings.access_key_id`|`AWS_ACCESS_KEY_ID`|
||Secret Access Key|`aws_config_settings.secret_access_key`|`AWS_SECRET_ACCESS_KEY`|
|[GCP](https://cloud.google.com/docs/authentication/application-default-credentials)||||
|| GCP Service Account Credentials File Path|`gcp_config_settings.credentials`|`GOOGLE_APPLICATION_CREDENTIALS`|
|[Azure](https://learn.microsoft.com/en-us/azure/developer/go/azure-sdk-authentication?tabs=bash)||||
||Active Subscription ID|`azure_config_settings.subscription_id`|`AZURE_SUBSCRIPTION_ID`|
||Resource Group|`azure_config_settings.resource_group`|`AZURE_RG`|
||Tenant ID|`azure_config_settings.tenant_id`|`AZURE_TENANT_ID`|
||Client ID|`azure_config_settings.client_id`|`AZURE_CLIENT_ID`|
||Client Secret|`azure_config_settings.client_secret`|`AZURE_CLIENT_SECRET`|

-> **Note:** AWS Environment variables or credential fields are not required for IAM based AWS cloud providers. Please set *aws_config_settings.use_iam_instance_profile* to use host IAM configuration for AWS cloud providers.

-> **Note:** GCP Environment variables or credential fileds are not required for Host credentials based GCP cloud providers. Please set *gcp_config_settings.use_host_credentials* to use host credentials for GCP cloud providers.

## Example Usage

```terraform
resource "yba_cloud_provider" "cloud_provider" {
  code        = "<code>"
  dest_vpc_id = "<vpc-network>"
  name        = "<cloud-provider-name>"
  regions {
    code = "<region-code>"
    name = "<region-name>"
  }
  air_gap_install = false
}

resource "yba_cloud_provider" "aws_cloud_provider" {
  code = "aws"
  name = "aws-provider"
  aws_config_settings {
    access_key_id     = "<s3-access-key-id>"
    secret_access_key = "<s3-secret-access-key>"
  }
  regions {
    code              = "us-west-2"
    name              = "us-west-2"
    security_group_id = "<aws-sg-id>"
    vnet_name         = "<aws-vpc-id>"
    zones {
      code   = "us-west-2a"
      name   = "us-west-2a"
      subnet = "<subnet-id>"
    }
    zones {
      code   = "us-west-2b"
      name   = "us-west-2b"
      subnet = "<subnet-id>"
    }
    zones {
      code   = "us-west-2c"
      name   = "us-west-2c"
      subnet = "<subnet-id>"
    }
  }
  air_gap_install = false
}

resource "yba_cloud_provider" "aws_iam_cloud_provider" {
  code = "aws"
  name = "aws-provider"
  aws_config_settings {
    use_iam_instance_profile = true
  }
  regions {
    code              = "us-west-2"
    name              = "us-west-2"
    security_group_id = "<aws-sg-id>"
    vnet_name         = "<aws-vpc-id>"
    zones {
      code   = "us-west-2a"
      name   = "us-west-2a"
      subnet = "<subnet-id>"
    }
  }
  air_gap_install = false
}

resource "yba_cloud_provider" "azure_cloud_provider" {
  code = "azu"
  name = "azure-provider"
  azure_config_settings {
    subscription_id = "<azure-subscription-id>"
    tenant_id       = "<azure-tenant-id>"
    client_id       = "<azure-client-id>"
    client_secret   = "<azure-client-secret>"
    resource_group  = "<azure-rg>"
  }
  regions {
    code      = "westus2"
    name      = "westus2"
    vnet_name = "<azure-vnet-id>"
    zones {
      code   = "westus2-1"
      name   = "westus2-1"
      subnet = "<azure-subnet-id>"
    }
    zones {
      code   = "westus2-2"
      name   = "westus2-2"
      subnet = "<azure-subnet-id>"
    }
    zones {
      code   = "westus2-3"
      name   = "westus2-3"
      subnet = "<azure-subnet-id>"
    }
  }
  air_gap_install = false
}

resource "yba_cloud_provider" "gcp_cloud_provider" {
  code = "gcp"
  name = "gcp-provider"
  gcp_config_settings {
    network      = "<gcp-network>"
    use_host_vpc = false
    project_id   = "<gcp-project-id>"
    credentials  = "<GCP Service Account credentials JSON as a string>"
  }
  regions {
    code = "us-west1"
    name = "us-west1"
    zones {
      subnet = "<gcp-shared-subnet-id>"
    }
  }
  air_gap_install = false
}

resource "yba_cloud_provider" "gcp_cloud_provider_with_image_bundles" {
  code = "gcp"
  name = "gcp-provider"
  gcp_config_settings {
    network      = "<gcp-network>"
    use_host_vpc = false
    project_id   = "<gcp-project-id>"
    credentials  = "<GCP Service Account credentials JSON as a string>"
  }
  regions {
    code = "us-west1"
    name = "us-west1"
    zones {
      subnet = "<gcp-shared-subnet-id>"
    }
  }
  image_bundles {
    name           = "<gcp-image-bundle-name-1>"
    use_as_default = false
    details {
      arch            = "x86_64"
      global_yb_image = "<ami-id>"
      ssh_user        = "centos"
      ssh_port        = 22
      use_imds_v2     = false
    }
  }
  image_bundles {
    name           = "<gcp-image-bundle-name-2>"
    use_as_default = true
    details {
      arch            = "x86_64"
      global_yb_image = "<ami-id>"
      ssh_user        = "centos"
      ssh_port        = 22
      use_imds_v2     = false
    }
  }
  air_gap_install = false
}

resource "yba_cloud_provider" "aws_cloud_provider_image_bundles" {
  code = "aws"
  name = "aws-provider"
  aws_config_settings {
    access_key_id     = "<s3-access-key-id>"
    secret_access_key = "<s3-secret-access-key>"
  }
  regions {
    code              = "us-west-2"
    name              = "us-west-2"
    security_group_id = "<aws-sg-id>"
    vnet_name         = "<aws-vpc-id>"
    zones {
      code   = "us-west-2a"
      name   = "us-west-2a"
      subnet = "<subnet-id>"
    }
    zones {
      code   = "us-west-2b"
      name   = "us-west-2b"
      subnet = "<subnet-id>"
    }
  }
  image_bundles {
    name           = "<image-bundle-name-1>"
    use_as_default = false
    details {
      arch = "x86_64"
      region_overrides = {
        "us-west-2" = "<ami-id>"
      }
      ssh_user    = "ec2-user"
      ssh_port    = 22
      use_imds_v2 = false
    }
  }
  image_bundles {
    name           = "<image-bundle-name-2>"
    use_as_default = true
    details {
      arch = "x86_64"
      region_overrides = {
        "us-west-2" = "<ami-id>"
      }
      ssh_user    = "ec2-user"
      ssh_port    = 22
      use_imds_v2 = false
    }
  }
  air_gap_install = false
}
```


The details for configuration are available in the [YugabyteDB Anywhere Configure Cloud Provider Documentation](https://docs.yugabyte.com/preview/yugabyte-platform/configure-yugabyte-platform/set-up-cloud-provider/aws/).

<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `code` (String) Code of the cloud provider. Permitted values: gcp, aws, azu.
- `name` (String) Name of the provider.
- `regions` (Block List, Min: 1) Regions associated with cloud providers. (see [below for nested schema](#nestedblock--regions))

### Optional

- `air_gap_install` (Boolean) Flag indicating if the universe should use an air-gapped installation.
- `aws_config_settings` (Block List, Max: 1) Settings that can be configured for AWS. (see [below for nested schema](#nestedblock--aws_config_settings))
- `azure_config_settings` (Block List, Max: 1) Settings that can be configured for Azure. (see [below for nested schema](#nestedblock--azure_config_settings))
- `dest_vpc_id` (String, Deprecated) Destination VPC network. Deprecated since YugabyteDB Anywhere 2.17.2.0. Please use 'gcp_config_settings.network' instead.
- `gcp_config_settings` (Block List, Max: 1) Settings that can be configured for GCP. (see [below for nested schema](#nestedblock--gcp_config_settings))
- `host_vpc_id` (String, Deprecated) Host VPC Network. Deprecated since YugabyteDB Anywhere 2.17.2.0. Will be removed in the next terraform-provider-yba release.
- `host_vpc_region` (String, Deprecated) Host VPC Region. Deprecated since YugabyteDB Anywhere 2.17.2.0.Will be removed in the next terraform-provider-yba release.
- `image_bundles` (Block List) Image bundles associated with cloud providers. Supported from YugabyteDB Anywhere version: 2.20.3.0-b68 (see [below for nested schema](#nestedblock--image_bundles))
- `key_pair_name` (String) Access Key Pair name.
- `ntp_servers` (List of String) NTP servers. Set "set_up_chrony" to true to use these servers.
- `set_up_chrony` (Boolean) Set up NTP servers.
- `show_set_up_chrony` (Boolean) Show setup chrony.
- `ssh_port` (Number, Deprecated) Port to use for ssh commands. Deprecated since YugabyteDB Anywhere 2.20.3.0. Please use 'image_bundles[*].details.ssh_port' instead.
- `ssh_private_key_content` (String) Private key to use for ssh commands.
- `ssh_user` (String, Deprecated) User to use for ssh commands. Deprecated since YugabyteDB Anywhere 2.20.3.0. Please use 'image_bundles[*].details.ssh_user' instead.
- `timeouts` (Block, Optional) (see [below for nested schema](#nestedblock--timeouts))

### Read-Only

- `config` (Map of String) Configuration values to be set for the provider. AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY must be set for AWS providers. The contents of your google credentials must be included here for GCP providers. AZURE_SUBSCRIPTION_ID, AZURE_RG, AZURE_TENANT_ID, AZURE_CLIENT_ID, AZURE_CLIENT_SECRET must be set for AZURE providers.
- `id` (String) The ID of this resource.

<a id="nestedblock--regions"></a>
### Nested Schema for `regions`

Optional:

- `code` (String) Region code. Varies by cloud provider.
- `instance_template` (String) Instance template to be used in this region. Only set for GCP provider. Allowed in YugabyteDB Anywhere versions above 2.18.0.0-b65.
- `latitude` (Number) Latitude of the region.
- `longitude` (Number) Longitude of the region.
- `name` (String) Name of the region. Varies by cloud provider.
- `security_group_id` (String) Security group ID to use for this region. Only set for AWS/Azure providers.
- `vnet_name` (String) Name of the virtual network/VPC ID to use for this region. Only set for AWS/Azure providers.
- `yb_image` (String, Deprecated) AMI to be used in this region. Deprecated since YugabyteDB Anywhere 2.20.3.0. Please use image_bundles block instead.
- `zones` (Block List) Zones associated with the region. (see [below for nested schema](#nestedblock--regions--zones))

Read-Only:

- `config` (Map of String) Config details corresponding to region.
- `uuid` (String) Region UUID.

<a id="nestedblock--regions--zones"></a>
### Nested Schema for `regions.zones`

Optional:

- `code` (String) Code of the zone. Varies by cloud provider.
- `name` (String) Name of the zone. Varies by cloud provider.
- `secondary_subnet` (String) The secondary subnet in the AZ.
- `subnet` (String) Subnet to use for this zone.

Read-Only:

- `active` (Boolean) Flag indicating if the zone is active.
- `config` (Map of String) Configuration details corresponding to zone.
- `kube_config_path` (String) Path to Kubernetes configuration file.
- `uuid` (String) Zone UUID.



<a id="nestedblock--aws_config_settings"></a>
### Nested Schema for `aws_config_settings`

Optional:

- `access_key_id` (String, Sensitive) AWS Access Key ID. Can also be set using environment variable AWS_ACCESS_KEY_ID.
- `hosted_zone_id` (String) Hosted Zone ID for AWS corresponsding to Amazon Route53.
- `secret_access_key` (String, Sensitive) AWS Secret Access Key. Can also be set using environment variable AWS_SECRET_ACCESS_KEY.
- `use_iam_instance_profile` (Boolean) Use IAM Role from the YugabyteDB Anywhere Host. Provider creation will fail on insufficient permissions on the host. False by default.


<a id="nestedblock--azure_config_settings"></a>
### Nested Schema for `azure_config_settings`

Optional:

- `client_id` (String) Azure Client ID. Can also be set using environment variable AZURE_CLIENT_ID.
- `client_secret` (String, Sensitive) Azure Client Secret. Can also be set using environment variable AZURE_CLIENT_SECRET. Required with client_id.
- `hosted_zone_id` (String) Private DNS Zone for Azure.
- `network_resource_group` (String) Azure Network Resource Group.All network resources and NIC resouce of VMs will be created in this group. If left empty, the default resource group will be used.
- `network_subscription_id` (String) Azure Network Subscription ID.All network resources and NIC resouce of VMs will be created in this group. If left empty, the default subscription ID will be used.
- `resource_group` (String) Azure Resource Group. Can also be set using environment variable AZURE_RG. Required with client_id.
- `subscription_id` (String) Azure Subscription ID. Can also be set using environment variable AZURE_SUBSCRIPTION_ID. Required with client_id.
- `tenant_id` (String) Azure Tenant ID. Can also be set using environment variable AZURE_TENANT_ID. Required with client_id.


<a id="nestedblock--gcp_config_settings"></a>
### Nested Schema for `gcp_config_settings`

Optional:

- `create_vpc` (Boolean) Create VPC in GCP. gcp_config_settings.network is required if create_vpc is set.
- `credentials` (String, Sensitive) Google Service Account Credentials. Can also be set by providing the JSON file path with the environment variable GOOGLE_APPLICATION_CREDENTIALS.
- `network` (String) VPC network name in GCP.
- `project_id` (String) Project ID that hosts universe nodes in GCP.
- `shared_vpc_project_id` (String) Specify the project to use Shared VPC to connect resources from multiple GCP projects to a common VPC.
- `use_host_credentials` (Boolean) Enabling Host Credentials in GCP.
- `use_host_vpc` (Boolean) Enabling Host VPC in GCP. gcp_config_settings.network is required if use_host_vpc is not set.
- `yb_firewall_tags` (String) Tags for firewall rules in GCP.


<a id="nestedblock--image_bundles"></a>
### Nested Schema for `image_bundles`

Required:

- `details` (Block List, Min: 1, Max: 1) (see [below for nested schema](#nestedblock--image_bundles--details))
- `name` (String) Name of the image bundle.

Optional:

- `use_as_default` (Boolean) Flag indicating if the image bundle should be used as default for this archietecture.

Read-Only:

- `active` (Boolean) Is the image bundle active.
- `metadata` (List of Object) (see [below for nested schema](#nestedatt--image_bundles--metadata))
- `uuid` (String) Image bundle UUID.

<a id="nestedblock--image_bundles--details"></a>
### Nested Schema for `image_bundles.details`

Required:

- `arch` (String) Image bundle architecture.
- `ssh_user` (String) SSH user for the image.

Optional:

- `global_yb_image` (String) Global YB image for the bundle.
- `region_overrides` (Map of String) Region overrides for the bundle. Provide region code as the key and override image as the value.
- `ssh_port` (Number) SSH port for the image. Default is 22.
- `use_imds_v2` (Boolean) Use IMDS v2 for the image.


<a id="nestedatt--image_bundles--metadata"></a>
### Nested Schema for `image_bundles.metadata`

Read-Only:

- `type` (String)
- `version` (String)



<a id="nestedblock--timeouts"></a>
### Nested Schema for `timeouts`

Optional:

- `create` (String)
- `delete` (String)

## Import

Cloud Providers can be imported using `cloud provider uuid`:

```sh
terraform import yba_cloud_provider.cloud_provider <cloud-provider-uuid>
```
