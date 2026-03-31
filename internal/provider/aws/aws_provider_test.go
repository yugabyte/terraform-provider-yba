// Licensed to YugabyteDB, Inc. under one or more contributor license
// agreements. See the NOTICE file distributed with this work for
// additional information regarding copyright ownership. Yugabyte
// licenses this file to you under the Mozilla License, Version 2.0
// (the "License"); you may not use this file except in compliance
// with the License.  You may obtain a copy of the License at
// http://mozilla.org/MPL/2.0/.
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package aws_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	sdkacctest "github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	client "github.com/yugabyte/platform-go-client"
	"github.com/yugabyte/terraform-provider-yba/internal/acctest"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// TestAccAWSProvider_WithCredentials tests AWS provider creation with access keys
func TestAccAWSProvider_WithCredentials(t *testing.T) {
	var provider client.Provider

	rName := fmt.Sprintf("tf-acctest-aws-%s", sdkacctest.RandString(12))
	resource.ParallelTest(t, resource.TestCase{
		PreCheck: func() {
			acctest.TestAccPreCheck(t)
			acctest.TestAccPreCheckAWS(t)
		},
		ProviderFactories: acctest.ProviderFactories,
		CheckDestroy:      testAccCheckDestroyAWSProvider,
		Steps: []resource.TestStep{
			{
				Config: awsProviderConfigWithCredentials(rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckAWSProviderExists("yba_aws_provider.test", &provider),
					resource.TestCheckResourceAttr("yba_aws_provider.test", "name", rName),
					resource.TestCheckResourceAttr(
						"yba_aws_provider.test", "use_iam_instance_profile", "false"),
					resource.TestCheckResourceAttrSet("yba_aws_provider.test", "version"),
					resource.TestCheckResourceAttrSet(
						"yba_aws_provider.test", "access_key_code"),
				),
			},
		},
	})
}

// TestAccAWSProvider_WithIAM tests AWS provider creation with IAM instance profile
func TestAccAWSProvider_WithIAM(t *testing.T) {
	var provider client.Provider

	rName := fmt.Sprintf("tf-acctest-aws-iam-%s", sdkacctest.RandString(12))
	resource.ParallelTest(t, resource.TestCase{
		PreCheck: func() {
			acctest.TestAccPreCheck(t)
			// No AWS credential check - using IAM
		},
		ProviderFactories: acctest.ProviderFactories,
		CheckDestroy:      testAccCheckDestroyAWSProvider,
		Steps: []resource.TestStep{
			{
				Config: awsProviderConfigWithIAM(rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckAWSProviderExists("yba_aws_provider.test", &provider),
					resource.TestCheckResourceAttr("yba_aws_provider.test", "name", rName),
					resource.TestCheckResourceAttr(
						"yba_aws_provider.test", "use_iam_instance_profile", "true"),
				),
			},
		},
	})
}

// TestAccAWSProvider_WithImageBundles tests AWS provider with custom image bundles
func TestAccAWSProvider_WithImageBundles(t *testing.T) {
	var provider client.Provider

	rName := fmt.Sprintf("tf-acctest-aws-ib-%s", sdkacctest.RandString(12))
	resource.ParallelTest(t, resource.TestCase{
		PreCheck: func() {
			acctest.TestAccPreCheck(t)
			acctest.TestAccPreCheckAWS(t)
		},
		ProviderFactories: acctest.ProviderFactories,
		CheckDestroy:      testAccCheckDestroyAWSProvider,
		Steps: []resource.TestStep{
			{
				Config: awsProviderConfigWithImageBundles(rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckAWSProviderExists("yba_aws_provider.test", &provider),
					resource.TestCheckResourceAttr(
						"yba_aws_provider.test", "image_bundles.#", "1"),
					resource.TestCheckResourceAttr(
						"yba_aws_provider.test", "image_bundles.0.name", "custom-bundle"),
					resource.TestCheckResourceAttr(
						"yba_aws_provider.test", "image_bundles.0.use_as_default", "true"),
				),
			},
		},
	})
}

// TestAccAWSProvider_Update tests updating AWS provider fields
func TestAccAWSProvider_Update(t *testing.T) {
	var provider client.Provider

	rName := fmt.Sprintf("tf-acctest-aws-upd-%s", sdkacctest.RandString(12))
	rNameUpdated := rName + "-updated"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck: func() {
			acctest.TestAccPreCheck(t)
			acctest.TestAccPreCheckAWS(t)
		},
		ProviderFactories: acctest.ProviderFactories,
		CheckDestroy:      testAccCheckDestroyAWSProvider,
		Steps: []resource.TestStep{
			{
				Config: awsProviderConfigBasic(rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckAWSProviderExists("yba_aws_provider.test", &provider),
					resource.TestCheckResourceAttr("yba_aws_provider.test", "name", rName),
				),
			},
			{
				Config: awsProviderConfigBasic(rNameUpdated),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckAWSProviderExists("yba_aws_provider.test", &provider),
					resource.TestCheckResourceAttr(
						"yba_aws_provider.test", "name", rNameUpdated),
				),
			},
		},
	})
}

func testAccCheckDestroyAWSProvider(s *terraform.State) error {
	conn := acctest.APIClient.YugawareClient

	for _, r := range s.RootModule().Resources {
		if r.Type != "yba_aws_provider" {
			continue
		}
		time.Sleep(60 * time.Second)
		cUUID := acctest.APIClient.CustomerID
		res, response, err := conn.CloudProvidersAPI.GetListOfProviders(context.Background(),
			cUUID).Execute()
		if err != nil {
			errMessage := utils.ErrorFromHTTPResponse(response, err, utils.TestEntity,
				"AWS Provider Test", "Read")
			return errMessage
		}
		for _, p := range res {
			if p.GetUuid() == r.Primary.ID {
				return errors.New("AWS provider is not destroyed")
			}
		}
	}

	return nil
}

func testAccCheckAWSProviderExists(
	name string,
	provider *client.Provider) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		r, ok := s.RootModule().Resources[name]
		if !ok {
			return fmt.Errorf("resource not found: %s", name)
		}
		if r.Primary.ID == "" {
			return errors.New("no ID is set for AWS provider resource")
		}

		conn := acctest.APIClient.YugawareClient
		cUUID := acctest.APIClient.CustomerID
		res, response, err := conn.CloudProvidersAPI.GetListOfProviders(context.Background(),
			cUUID).Execute()
		if err != nil {
			errMessage := utils.ErrorFromHTTPResponse(response, err, utils.TestEntity,
				"AWS Provider", "Read")
			return errMessage
		}
		for _, p := range res {
			if *p.Uuid == r.Primary.ID {
				*provider = p
				return nil
			}
		}
		return errors.New("AWS provider does not exist")
	}
}

func awsProviderConfigBasic(name string) string {
	return fmt.Sprintf(`
variable "AWS_SG_ID" {
  type        = string
  description = "AWS sg-id to run acceptance testing"
}

variable "AWS_VPC_ID" {
  type        = string
  description = "AWS VPC ID to run acceptance testing"
}

variable "AWS_ZONE_SUBNET_ID" {
  type        = string
  description = "AWS zonal subnet ID to run acceptance testing"
}

variable "AWS_ACCESS_KEY_ID" {
  type        = string
  description = "AWS access key ID"
}

variable "AWS_SECRET_ACCESS_KEY" {
  type        = string
  sensitive   = true
  description = "AWS secret access key"
}

resource "yba_aws_provider" "test" {
  name              = "%s"
  access_key_id     = var.AWS_ACCESS_KEY_ID
  secret_access_key = var.AWS_SECRET_ACCESS_KEY
  ssh_keypair_name  = "test-key"
  regions {
    code              = "us-west-2"
    security_group_id = var.AWS_SG_ID
    vpc_id            = var.AWS_VPC_ID
    zones {
      code   = "us-west-2a"
      subnet = var.AWS_ZONE_SUBNET_ID
    }
  }
  air_gap_install = false
}
`, name)
}

func awsProviderConfigWithCredentials(name string) string {
	return fmt.Sprintf(`
variable "AWS_SG_ID" {
  type = string
}

variable "AWS_VPC_ID" {
  type = string
}

variable "AWS_ZONE_SUBNET_ID" {
  type = string
}

variable "AWS_ACCESS_KEY_ID" {
  type = string
}

variable "AWS_SECRET_ACCESS_KEY" {
  type      = string
  sensitive = true
}

resource "yba_aws_provider" "test" {
  name                     = "%s"
  access_key_id            = var.AWS_ACCESS_KEY_ID
  secret_access_key        = var.AWS_SECRET_ACCESS_KEY
  use_iam_instance_profile = false
  ssh_keypair_name         = "test-keypair"
  regions {
    code              = "us-west-2"
    security_group_id = var.AWS_SG_ID
    vpc_id            = var.AWS_VPC_ID
    zones {
      code   = "us-west-2a"
      subnet = var.AWS_ZONE_SUBNET_ID
    }
  }
  air_gap_install = false
}
`, name)
}

func awsProviderConfigWithIAM(name string) string {
	return fmt.Sprintf(`
variable "AWS_SG_ID" {
  type = string
}

variable "AWS_VPC_ID" {
  type = string
}

variable "AWS_ZONE_SUBNET_ID" {
  type = string
}

resource "yba_aws_provider" "test" {
  name                     = "%s"
  use_iam_instance_profile = true
  ssh_keypair_name         = "test-keypair"
  regions {
    code              = "us-west-2"
    security_group_id = var.AWS_SG_ID
    vpc_id            = var.AWS_VPC_ID
    zones {
      code   = "us-west-2a"
      subnet = var.AWS_ZONE_SUBNET_ID
    }
  }
  air_gap_install = false
}
`, name)
}

func awsProviderConfigWithImageBundles(name string) string {
	return fmt.Sprintf(`
variable "AWS_SG_ID" {
  type = string
}

variable "AWS_VPC_ID" {
  type = string
}

variable "AWS_ZONE_SUBNET_ID" {
  type = string
}

variable "AWS_ACCESS_KEY_ID" {
  type = string
}

variable "AWS_SECRET_ACCESS_KEY" {
  type      = string
  sensitive = true
}

variable "AWS_AMI_ID" {
  type        = string
  description = "AMI ID for the custom image bundle"
}

resource "yba_aws_provider" "test" {
  name              = "%s"
  access_key_id     = var.AWS_ACCESS_KEY_ID
  secret_access_key = var.AWS_SECRET_ACCESS_KEY
  ssh_keypair_name  = "test-keypair"
  regions {
    code              = "us-west-2"
    security_group_id = var.AWS_SG_ID
    vpc_id            = var.AWS_VPC_ID
    zones {
      code   = "us-west-2a"
      subnet = var.AWS_ZONE_SUBNET_ID
    }
  }
  image_bundles {
    name           = "custom-bundle"
    use_as_default = true
    details {
      arch            = "x86_64"
      ssh_user     = "ec2-user"
      ssh_port     = 22
      # use_imds_v2 defaults to true; omitted here to test the default behaviour
    }
  }
  air_gap_install = false
}
`, name)
}

// TestAccAWSProvider_MultiZoneWithRegionOverrides tests AWS provider with
// multiple zones and image bundle region overrides (mirrors yba-cli create command)
func TestAccAWSProvider_MultiZoneWithRegionOverrides(t *testing.T) {
	var provider client.Provider

	rName := fmt.Sprintf("tf-acctest-aws-mz-%s", sdkacctest.RandString(12))
	resource.ParallelTest(t, resource.TestCase{
		PreCheck: func() {
			acctest.TestAccPreCheck(t)
			acctest.TestAccPreCheckAWS(t)
			acctest.TestAccPreCheckAWSMultiZone(t)
		},
		ProviderFactories: acctest.ProviderFactories,
		CheckDestroy:      testAccCheckDestroyAWSProvider,
		Steps: []resource.TestStep{
			{
				Config: awsProviderConfigMultiZoneWithRegionOverrides(rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckAWSProviderExists("yba_aws_provider.test", &provider),
					resource.TestCheckResourceAttr("yba_aws_provider.test", "name", rName),
					resource.TestCheckResourceAttr(
						"yba_aws_provider.test", "use_iam_instance_profile", "false"),
					// Verify multiple zones
					resource.TestCheckResourceAttr(
						"yba_aws_provider.test", "regions.0.zones.#", "2"),
					// Verify image bundle
					resource.TestCheckResourceAttr(
						"yba_aws_provider.test", "image_bundles.#", "1"),
					resource.TestCheckResourceAttr(
						"yba_aws_provider.test", "image_bundles.0.name", "test-cli"),
					resource.TestCheckResourceAttr(
						"yba_aws_provider.test", "image_bundles.0.use_as_default", "true"),
					resource.TestCheckResourceAttr(
						"yba_aws_provider.test", "image_bundles.0.details.0.arch", "x86_64"),
					resource.TestCheckResourceAttr(
						"yba_aws_provider.test", "image_bundles.0.details.0.ssh_user", "ec2-user"),
					resource.TestCheckResourceAttr(
						"yba_aws_provider.test", "image_bundles.0.details.0.ssh_port", "22"),
					resource.TestCheckResourceAttr(
						"yba_aws_provider.test", "image_bundles.0.details.0.use_imds_v2", "true"),
					resource.TestCheckResourceAttrSet("yba_aws_provider.test", "version"),
					resource.TestCheckResourceAttrSet("yba_aws_provider.test", "access_key_code"),
				),
			},
		},
	})
}

// awsProviderConfigMultiZoneWithRegionOverrides generates config for multi-zone test
// Mirrors: yba provider aws create -n dkumar-cli \
//
//	--region region-name=us-west-2::vpc-id=vpc-0fe36f6b::sg-id=sg-139dde6c \
//	--zone zone-name=us-west-2a::region-name=us-west-2::subnet=subnet-6553f513 \
//	--zone zone-name=us-west-2b::region-name=us-west-2::subnet=subnet-f840ce9c \
//	--image-bundle image-bundle-name=test-cli::arch=x86_64::ssh-user=... \
//	--image-bundle-region-override image-bundle-name=test-cli::region=... \
//	--image-bundle-region-override image-bundle-name=test-cli::region=...
func awsProviderConfigMultiZoneWithRegionOverrides(name string) string {
	return fmt.Sprintf(`
variable "AWS_SG_ID" {
  type        = string
  description = "AWS Security Group ID"
}

variable "AWS_VPC_ID" {
  type        = string
  description = "AWS VPC ID"
}

variable "AWS_ZONE_SUBNET_ID" {
  type        = string
  description = "AWS subnet ID for first zone (us-west-2a)"
}

variable "AWS_ZONE_SUBNET_ID_2" {
  type        = string
  description = "AWS subnet ID for second zone (us-west-2b)"
}

variable "AWS_ACCESS_KEY_ID" {
  type        = string
  description = "AWS Access Key ID"
}

variable "AWS_SECRET_ACCESS_KEY" {
  type        = string
  sensitive   = true
  description = "AWS Secret Access Key"
}

variable "AWS_AMI_ID" {
  type        = string
  description = "AMI ID for region override"
}

resource "yba_aws_provider" "test" {
  name                    = "%s"
  access_key_id           = var.AWS_ACCESS_KEY_ID
  secret_access_key       = var.AWS_SECRET_ACCESS_KEY
  skip_ssh_keypair_validation = true

  regions {
    code              = "us-west-2"
    security_group_id = var.AWS_SG_ID
    vpc_id            = var.AWS_VPC_ID
    zones {
      code   = "us-west-2a"
      subnet = var.AWS_ZONE_SUBNET_ID
    }
    zones {
      code   = "us-west-2b"
      subnet = var.AWS_ZONE_SUBNET_ID_2
    }
  }

  image_bundles {
    name           = "test-cli"
    use_as_default = true
    details {
      arch     = "x86_64"
      ssh_user = "ec2-user"
      ssh_port = 22
      # use_imds_v2 defaults to true; omitted here to test the default behaviour
      region_overrides = {
        "us-west-2" = var.AWS_AMI_ID
        "us-west-1" = var.AWS_AMI_ID
      }
    }
  }

  air_gap_install = false
}
`, name)
}
