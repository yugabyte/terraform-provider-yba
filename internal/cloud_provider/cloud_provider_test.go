// Licensed to Yugabyte, Inc. under one or more contributor license
// agreements. See the NOTICE file distributed with this work for
// additional information regarding copyright ownership. Yugabyte
// licenses this file to you under the Apache License, Version 2.0
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

package cloud_provider_test

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
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/acctest"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/utils"
)

func TestAccCloudProvider_GCP(t *testing.T) {
	var provider client.Provider

	rName := fmt.Sprintf("tf-acctest-gcp-provider-%s", sdkacctest.RandString(12))
	resource.ParallelTest(t, resource.TestCase{
		PreCheck: func() {
			acctest.TestAccPreCheck(t)
			acctest.TestAccPreCheckGCP(t)
		},
		ProviderFactories: acctest.ProviderFactories,
		CheckDestroy:      testAccCheckDestroyCloudProvider,
		Steps: []resource.TestStep{
			{
				Config: cloudProviderGCPConfig(rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckCloudProviderExists("yb_cloud_provider.gcp", &provider),
				),
			},
		},
	})
}

func TestAccCloudProvider_AWS(t *testing.T) {
	var provider client.Provider

	rName := fmt.Sprintf("tf-acctest-aws-provider-%s", sdkacctest.RandString(12))
	resource.ParallelTest(t, resource.TestCase{
		PreCheck: func() {
			acctest.TestAccPreCheck(t)
			acctest.TestAccPreCheckAWS(t)
		},
		ProviderFactories: acctest.ProviderFactories,
		CheckDestroy:      testAccCheckDestroyCloudProvider,
		Steps: []resource.TestStep{
			{
				Config: cloudProviderAWSConfig(rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckCloudProviderExists("yb_cloud_provider.aws", &provider),
				),
			},
		},
	})
}

func TestAccCloudProvider_Azure(t *testing.T) {
	var provider client.Provider

	rName := fmt.Sprintf("tf-acctest-azure-provider-%s", sdkacctest.RandString(12))
	resource.ParallelTest(t, resource.TestCase{
		PreCheck: func() {
			acctest.TestAccPreCheck(t)
			acctest.TestAccPreCheckAzure(t)
		},
		ProviderFactories: acctest.ProviderFactories,
		CheckDestroy:      testAccCheckDestroyCloudProvider,
		Steps: []resource.TestStep{
			{
				Config: cloudProviderAzureConfig(rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckCloudProviderExists("yb_cloud_provider.azure", &provider),
				),
			},
		},
	})
}

func testAccCheckDestroyCloudProvider(s *terraform.State) error {
	conn := acctest.APIClient.YugawareClient

	for _, r := range s.RootModule().Resources {
		if r.Type != "yb_cloud_provider" {
			continue
		}
		// Since delete API of cloud provider does not track the task status after the API call,
		// Function allows time to complete the operation before checking list of available providers
		time.Sleep(60 * time.Second)
		cUUID := acctest.APIClient.CustomerID
		res, response, err := conn.CloudProvidersApi.GetListOfProviders(context.Background(),
			cUUID).Execute()
		if err != nil {
			errMessage := utils.ErrorFromHTTPResponse(response, err, utils.TestEntity,
				"Cloud Provider Test", "Read")
			return errMessage
		}
		for _, p := range res {
			if p.GetUuid() == r.Primary.ID {
				return errors.New("Cloud provider is not destroyed")
			}
		}
	}

	return nil
}

func testAccCheckCloudProviderExists(name string, provider *client.Provider) (
	resource.TestCheckFunc) {
	return func(s *terraform.State) error {
		r, ok := s.RootModule().Resources[name]
		if !ok {
			return fmt.Errorf("resource not found: %s", name)
		}
		if r.Primary.ID == "" {
			return errors.New("no ID is set for cloud provider resource")
		}

		conn := acctest.APIClient.YugawareClient
		cUUID := acctest.APIClient.CustomerID
		res, response, err := conn.CloudProvidersApi.GetListOfProviders(context.Background(),
			cUUID).Execute()
		if err != nil {
			errMessage := utils.ErrorFromHTTPResponse(response, err, utils.TestEntity,
				"Cloud Provider", "Read")
			return errMessage
		}
		for _, p := range res {
			if *p.Uuid == r.Primary.ID {
				*provider = p
				return nil
			}
		}
		return errors.New("cloud provider does not exist")
	}
}

func cloudProviderGCPConfig(name string) string {
	return fmt.Sprintf(`
resource "yb_cloud_provider" "gcp" {
  code = "gcp"
  dest_vpc_id = "default"
  name        = "%s"
  regions {
    code = "us-west1"
    name = "us-west1"
  }
  ssh_port        = 22
  air_gap_install = false
}
`, name)
}

func cloudProviderAWSConfig(name string) string {
	// TODO: remove the lifecycle ignore_changes block.
	// This is needed because the current API is not returning vnet_name
	return fmt.Sprintf(`
	resource "yb_cloud_provider" "aws" {
		code = "aws"
		name = "%s"
		regions {
		  code              = "us-west-2"
		  name              = "us-west-2"
		  security_group_id = "sg-139dde6c"
		  vnet_name         = "vpc-0fe36f6b"
		  zones {
			code   = "us-west-2a"
			name   = "us-west-2a"
			subnet = "subnet-6553f513"
		  }
		}

		ssh_port        = 22
		air_gap_install = false
	  }
`, name)
}

func cloudProviderAzureConfig(name string) string {
	return fmt.Sprintf(`
resource "yb_cloud_provider" "azure" {
  code = "azu"
  name        = "%s"
  regions {
    code = "westus2"
    name = "westus2"
	vnet_name = "terraform-acctest-vnet-westus2"
	zones {
      name = "westus2-1"
	  subnet = "terraform-acctest-subnet-westus2"
	}
  }
}
`, name)
}
