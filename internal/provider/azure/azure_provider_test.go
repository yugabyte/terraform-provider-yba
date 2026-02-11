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

package azure_test

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

// TestAccAzureProvider_WithCredentials tests Azure provider creation with credentials
func TestAccAzureProvider_WithCredentials(t *testing.T) {
	var provider client.Provider

	rName := fmt.Sprintf("tf-acctest-azure-%s", sdkacctest.RandString(12))
	resource.ParallelTest(t, resource.TestCase{
		PreCheck: func() {
			acctest.TestAccPreCheck(t)
			acctest.TestAccPreCheckAzure(t)
		},
		ProviderFactories: acctest.ProviderFactories,
		CheckDestroy:      testAccCheckDestroyAzureProvider,
		Steps: []resource.TestStep{
			{
				Config: azureProviderConfigWithCredentials(rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckAzureProviderExists("yba_azure_provider.test", &provider),
					resource.TestCheckResourceAttr("yba_azure_provider.test", "name", rName),
					resource.TestCheckResourceAttrSet("yba_azure_provider.test", "version"),
					resource.TestCheckResourceAttrSet(
						"yba_azure_provider.test", "access_key_code"),
				),
			},
		},
	})
}

// TestAccAzureProvider_WithNetworkConfig tests Azure provider with network configuration
func TestAccAzureProvider_WithNetworkConfig(t *testing.T) {
	var provider client.Provider

	rName := fmt.Sprintf("tf-acctest-azure-net-%s", sdkacctest.RandString(12))
	resource.ParallelTest(t, resource.TestCase{
		PreCheck: func() {
			acctest.TestAccPreCheck(t)
			acctest.TestAccPreCheckAzure(t)
		},
		ProviderFactories: acctest.ProviderFactories,
		CheckDestroy:      testAccCheckDestroyAzureProvider,
		Steps: []resource.TestStep{
			{
				Config: azureProviderConfigWithNetworkConfig(rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckAzureProviderExists("yba_azure_provider.test", &provider),
					resource.TestCheckResourceAttr("yba_azure_provider.test", "name", rName),
					resource.TestCheckResourceAttrSet(
						"yba_azure_provider.test", "network_subscription_id"),
					resource.TestCheckResourceAttrSet(
						"yba_azure_provider.test", "network_resource_group"),
				),
			},
		},
	})
}

// TestAccAzureProvider_Update tests updating Azure provider fields
func TestAccAzureProvider_Update(t *testing.T) {
	var provider client.Provider

	rName := fmt.Sprintf("tf-acctest-azure-upd-%s", sdkacctest.RandString(12))
	rNameUpdated := rName + "-updated"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck: func() {
			acctest.TestAccPreCheck(t)
			acctest.TestAccPreCheckAzure(t)
		},
		ProviderFactories: acctest.ProviderFactories,
		CheckDestroy:      testAccCheckDestroyAzureProvider,
		Steps: []resource.TestStep{
			{
				Config: azureProviderConfigBasic(rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckAzureProviderExists("yba_azure_provider.test", &provider),
					resource.TestCheckResourceAttr("yba_azure_provider.test", "name", rName),
				),
			},
			{
				Config: azureProviderConfigBasic(rNameUpdated),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckAzureProviderExists("yba_azure_provider.test", &provider),
					resource.TestCheckResourceAttr(
						"yba_azure_provider.test", "name", rNameUpdated),
				),
			},
		},
	})
}

// TestAccAzureProvider_MultipleZones tests Azure provider with multiple zones
func TestAccAzureProvider_MultipleZones(t *testing.T) {
	var provider client.Provider

	rName := fmt.Sprintf("tf-acctest-azure-mz-%s", sdkacctest.RandString(12))
	resource.ParallelTest(t, resource.TestCase{
		PreCheck: func() {
			acctest.TestAccPreCheck(t)
			acctest.TestAccPreCheckAzure(t)
		},
		ProviderFactories: acctest.ProviderFactories,
		CheckDestroy:      testAccCheckDestroyAzureProvider,
		Steps: []resource.TestStep{
			{
				Config: azureProviderConfigMultipleZones(rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckAzureProviderExists("yba_azure_provider.test", &provider),
					resource.TestCheckResourceAttr(
						"yba_azure_provider.test", "regions.0.zones.#", "3"),
				),
			},
		},
	})
}

func testAccCheckDestroyAzureProvider(s *terraform.State) error {
	conn := acctest.APIClient.YugawareClient

	for _, r := range s.RootModule().Resources {
		if r.Type != "yba_azure_provider" {
			continue
		}
		time.Sleep(60 * time.Second)
		cUUID := acctest.APIClient.CustomerID
		res, response, err := conn.CloudProvidersAPI.GetListOfProviders(context.Background(),
			cUUID).Execute()
		if err != nil {
			errMessage := utils.ErrorFromHTTPResponse(response, err, utils.TestEntity,
				"Azure Provider Test", "Read")
			return errMessage
		}
		for _, p := range res {
			if p.GetUuid() == r.Primary.ID {
				return errors.New("Azure provider is not destroyed")
			}
		}
	}

	return nil
}

func testAccCheckAzureProviderExists(
	name string,
	provider *client.Provider) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		r, ok := s.RootModule().Resources[name]
		if !ok {
			return fmt.Errorf("resource not found: %s", name)
		}
		if r.Primary.ID == "" {
			return errors.New("no ID is set for Azure provider resource")
		}

		conn := acctest.APIClient.YugawareClient
		cUUID := acctest.APIClient.CustomerID
		res, response, err := conn.CloudProvidersAPI.GetListOfProviders(context.Background(),
			cUUID).Execute()
		if err != nil {
			errMessage := utils.ErrorFromHTTPResponse(response, err, utils.TestEntity,
				"Azure Provider", "Read")
			return errMessage
		}
		for _, p := range res {
			if *p.Uuid == r.Primary.ID {
				*provider = p
				return nil
			}
		}
		return errors.New("Azure provider does not exist")
	}
}

func azureProviderConfigBasic(name string) string {
	return fmt.Sprintf(`
variable "AZURE_SUBSCRIPTION_ID" {
  type        = string
  description = "Azure subscription ID"
}

variable "AZURE_TENANT_ID" {
  type        = string
  description = "Azure tenant ID"
}

variable "AZURE_CLIENT_ID" {
  type        = string
  description = "Azure client/application ID"
}

variable "AZURE_CLIENT_SECRET" {
  type        = string
  sensitive   = true
  description = "Azure client secret"
}

variable "AZURE_RG" {
  type        = string
  description = "Azure resource group"
}

variable "AZURE_VNET_ID" {
  type        = string
  description = "Azure vnet ID"
}

variable "AZURE_SUBNET_ID" {
  type        = string
  description = "Azure subnet ID"
}

resource "yba_azure_provider" "test" {
  name            = "%s"
  subscription_id = var.AZURE_SUBSCRIPTION_ID
  tenant_id       = var.AZURE_TENANT_ID
  client_id       = var.AZURE_CLIENT_ID
  client_secret   = var.AZURE_CLIENT_SECRET
  resource_group  = var.AZURE_RG
  regions {
    name = "westus2"
    vnet = var.AZURE_VNET_ID
    zones {
      name   = "westus2-1"
      subnet = var.AZURE_SUBNET_ID
    }
  }
  air_gap_install = false
}
`, name)
}

func azureProviderConfigWithCredentials(name string) string {
	return fmt.Sprintf(`
variable "AZURE_SUBSCRIPTION_ID" {
  type = string
}

variable "AZURE_TENANT_ID" {
  type = string
}

variable "AZURE_CLIENT_ID" {
  type = string
}

variable "AZURE_CLIENT_SECRET" {
  type      = string
  sensitive = true
}

variable "AZURE_RG" {
  type = string
}

variable "AZURE_VNET_ID" {
  type = string
}

variable "AZURE_SUBNET_ID" {
  type = string
}

resource "yba_azure_provider" "test" {
  name             = "%s"
  subscription_id  = var.AZURE_SUBSCRIPTION_ID
  tenant_id        = var.AZURE_TENANT_ID
  client_id        = var.AZURE_CLIENT_ID
  client_secret    = var.AZURE_CLIENT_SECRET
  resource_group   = var.AZURE_RG
  ssh_keypair_name = "test-keypair"
  regions {
    name = "westus2"
    vnet = var.AZURE_VNET_ID
    zones {
      name   = "westus2-1"
      subnet = var.AZURE_SUBNET_ID
    }
  }
  air_gap_install = false
}
`, name)
}

func azureProviderConfigWithNetworkConfig(name string) string {
	return fmt.Sprintf(`
variable "AZURE_SUBSCRIPTION_ID" {
  type = string
}

variable "AZURE_TENANT_ID" {
  type = string
}

variable "AZURE_CLIENT_ID" {
  type = string
}

variable "AZURE_CLIENT_SECRET" {
  type      = string
  sensitive = true
}

variable "AZURE_RG" {
  type = string
}

variable "AZURE_VNET_ID" {
  type = string
}

variable "AZURE_SUBNET_ID" {
  type = string
}

variable "AZURE_NETWORK_SUBSCRIPTION_ID" {
  type        = string
  description = "Azure network subscription ID (for cross-subscription networking)"
}

variable "AZURE_NETWORK_RG" {
  type        = string
  description = "Azure network resource group"
}

resource "yba_azure_provider" "test" {
  name                    = "%s"
  subscription_id         = var.AZURE_SUBSCRIPTION_ID
  tenant_id               = var.AZURE_TENANT_ID
  client_id               = var.AZURE_CLIENT_ID
  client_secret           = var.AZURE_CLIENT_SECRET
  resource_group          = var.AZURE_RG
  network_subscription_id = var.AZURE_NETWORK_SUBSCRIPTION_ID
  network_resource_group  = var.AZURE_NETWORK_RG
  ssh_keypair_name        = "test-keypair"
  regions {
    name = "westus2"
    vnet = var.AZURE_VNET_ID
    zones {
      name   = "westus2-1"
      subnet = var.AZURE_SUBNET_ID
    }
  }
  air_gap_install = false
}
`, name)
}

func azureProviderConfigMultipleZones(name string) string {
	return fmt.Sprintf(`
variable "AZURE_SUBSCRIPTION_ID" {
  type = string
}

variable "AZURE_TENANT_ID" {
  type = string
}

variable "AZURE_CLIENT_ID" {
  type = string
}

variable "AZURE_CLIENT_SECRET" {
  type      = string
  sensitive = true
}

variable "AZURE_RG" {
  type = string
}

variable "AZURE_VNET_ID" {
  type = string
}

variable "AZURE_SUBNET_ID_1" {
  type = string
}

variable "AZURE_SUBNET_ID_2" {
  type = string
}

variable "AZURE_SUBNET_ID_3" {
  type = string
}

resource "yba_azure_provider" "test" {
  name             = "%s"
  subscription_id  = var.AZURE_SUBSCRIPTION_ID
  tenant_id        = var.AZURE_TENANT_ID
  client_id        = var.AZURE_CLIENT_ID
  client_secret    = var.AZURE_CLIENT_SECRET
  resource_group   = var.AZURE_RG
  ssh_keypair_name = "test-keypair"
  regions {
    name = "westus2"
    vnet = var.AZURE_VNET_ID
    zones {
      name   = "westus2-1"
      subnet = var.AZURE_SUBNET_ID_1
    }
    zones {
      name   = "westus2-2"
      subnet = var.AZURE_SUBNET_ID_2
    }
    zones {
      name   = "westus2-3"
      subnet = var.AZURE_SUBNET_ID_3
    }
  }
  air_gap_install = false
}
`, name)
}
