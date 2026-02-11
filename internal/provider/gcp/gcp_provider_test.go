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

package gcp_test

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

// TestAccGCPProvider_WithCredentials tests GCP provider creation with credentials
func TestAccGCPProvider_WithCredentials(t *testing.T) {
	var provider client.Provider

	rName := fmt.Sprintf("tf-acctest-gcp-%s", sdkacctest.RandString(12))
	resource.ParallelTest(t, resource.TestCase{
		PreCheck: func() {
			acctest.TestAccPreCheck(t)
			acctest.TestAccPreCheckGCP(t)
		},
		ProviderFactories: acctest.ProviderFactories,
		CheckDestroy:      testAccCheckDestroyGCPProvider,
		Steps: []resource.TestStep{
			{
				Config: gcpProviderConfigWithCredentials(rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckGCPProviderExists("yba_gcp_provider.test", &provider),
					resource.TestCheckResourceAttr("yba_gcp_provider.test", "name", rName),
					resource.TestCheckResourceAttr(
						"yba_gcp_provider.test", "use_host_credentials", "false"),
					resource.TestCheckResourceAttrSet("yba_gcp_provider.test", "version"),
					resource.TestCheckResourceAttrSet(
						"yba_gcp_provider.test", "access_key_code"),
				),
			},
		},
	})
}

// TestAccGCPProvider_WithHostCredentials tests GCP provider with host credentials
func TestAccGCPProvider_WithHostCredentials(t *testing.T) {
	var provider client.Provider

	rName := fmt.Sprintf("tf-acctest-gcp-host-%s", sdkacctest.RandString(12))
	resource.ParallelTest(t, resource.TestCase{
		PreCheck: func() {
			acctest.TestAccPreCheck(t)
			// No GCP credential check - using host credentials
		},
		ProviderFactories: acctest.ProviderFactories,
		CheckDestroy:      testAccCheckDestroyGCPProvider,
		Steps: []resource.TestStep{
			{
				Config: gcpProviderConfigWithHostCredentials(rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckGCPProviderExists("yba_gcp_provider.test", &provider),
					resource.TestCheckResourceAttr("yba_gcp_provider.test", "name", rName),
					resource.TestCheckResourceAttr(
						"yba_gcp_provider.test", "use_host_credentials", "true"),
				),
			},
		},
	})
}

// TestAccGCPProvider_WithSharedVPC tests GCP provider with shared VPC configuration
func TestAccGCPProvider_WithSharedVPC(t *testing.T) {
	var provider client.Provider

	rName := fmt.Sprintf("tf-acctest-gcp-vpc-%s", sdkacctest.RandString(12))
	resource.ParallelTest(t, resource.TestCase{
		PreCheck: func() {
			acctest.TestAccPreCheck(t)
			acctest.TestAccPreCheckGCP(t)
		},
		ProviderFactories: acctest.ProviderFactories,
		CheckDestroy:      testAccCheckDestroyGCPProvider,
		Steps: []resource.TestStep{
			{
				Config: gcpProviderConfigWithSharedVPC(rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckGCPProviderExists("yba_gcp_provider.test", &provider),
					resource.TestCheckResourceAttr("yba_gcp_provider.test", "name", rName),
					resource.TestCheckResourceAttrSet(
						"yba_gcp_provider.test", "shared_vpc_project_id"),
				),
			},
		},
	})
}

// TestAccGCPProvider_Update tests updating GCP provider fields
func TestAccGCPProvider_Update(t *testing.T) {
	var provider client.Provider

	rName := fmt.Sprintf("tf-acctest-gcp-upd-%s", sdkacctest.RandString(12))
	rNameUpdated := rName + "-updated"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck: func() {
			acctest.TestAccPreCheck(t)
			acctest.TestAccPreCheckGCP(t)
		},
		ProviderFactories: acctest.ProviderFactories,
		CheckDestroy:      testAccCheckDestroyGCPProvider,
		Steps: []resource.TestStep{
			{
				Config: gcpProviderConfigBasic(rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckGCPProviderExists("yba_gcp_provider.test", &provider),
					resource.TestCheckResourceAttr("yba_gcp_provider.test", "name", rName),
				),
			},
			{
				Config: gcpProviderConfigBasic(rNameUpdated),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckGCPProviderExists("yba_gcp_provider.test", &provider),
					resource.TestCheckResourceAttr(
						"yba_gcp_provider.test", "name", rNameUpdated),
				),
			},
		},
	})
}

// TestAccGCPProvider_WithFirewallTags tests GCP provider with firewall tags
func TestAccGCPProvider_WithFirewallTags(t *testing.T) {
	var provider client.Provider

	rName := fmt.Sprintf("tf-acctest-gcp-fw-%s", sdkacctest.RandString(12))
	resource.ParallelTest(t, resource.TestCase{
		PreCheck: func() {
			acctest.TestAccPreCheck(t)
			acctest.TestAccPreCheckGCP(t)
		},
		ProviderFactories: acctest.ProviderFactories,
		CheckDestroy:      testAccCheckDestroyGCPProvider,
		Steps: []resource.TestStep{
			{
				Config: gcpProviderConfigWithFirewallTags(rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckGCPProviderExists("yba_gcp_provider.test", &provider),
					resource.TestCheckResourceAttr(
						"yba_gcp_provider.test", "yb_firewall_tags", "yb-db-node"),
				),
			},
		},
	})
}

func testAccCheckDestroyGCPProvider(s *terraform.State) error {
	conn := acctest.APIClient.YugawareClient

	for _, r := range s.RootModule().Resources {
		if r.Type != "yba_gcp_provider" {
			continue
		}
		time.Sleep(60 * time.Second)
		cUUID := acctest.APIClient.CustomerID
		res, response, err := conn.CloudProvidersAPI.GetListOfProviders(context.Background(),
			cUUID).Execute()
		if err != nil {
			errMessage := utils.ErrorFromHTTPResponse(response, err, utils.TestEntity,
				"GCP Provider Test", "Read")
			return errMessage
		}
		for _, p := range res {
			if p.GetUuid() == r.Primary.ID {
				return errors.New("GCP provider is not destroyed")
			}
		}
	}

	return nil
}

func testAccCheckGCPProviderExists(
	name string,
	provider *client.Provider) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		r, ok := s.RootModule().Resources[name]
		if !ok {
			return fmt.Errorf("resource not found: %s", name)
		}
		if r.Primary.ID == "" {
			return errors.New("no ID is set for GCP provider resource")
		}

		conn := acctest.APIClient.YugawareClient
		cUUID := acctest.APIClient.CustomerID
		res, response, err := conn.CloudProvidersAPI.GetListOfProviders(context.Background(),
			cUUID).Execute()
		if err != nil {
			errMessage := utils.ErrorFromHTTPResponse(response, err, utils.TestEntity,
				"GCP Provider", "Read")
			return errMessage
		}
		for _, p := range res {
			if *p.Uuid == r.Primary.ID {
				*provider = p
				return nil
			}
		}
		return errors.New("GCP provider does not exist")
	}
}

func gcpProviderConfigBasic(name string) string {
	return fmt.Sprintf(`
variable "GCP_VPC_NETWORK" {
  type        = string
  description = "GCP VPC network to run acceptance testing"
}

variable "GCP_CREDENTIALS" {
  type        = string
  sensitive   = true
  description = "GCP service account credentials JSON"
}

variable "GCP_PROJECT_ID" {
  type        = string
  description = "GCP project ID"
}

resource "yba_gcp_provider" "test" {
  name        = "%s"
  credentials = var.GCP_CREDENTIALS
  project_id  = var.GCP_PROJECT_ID
  network     = var.GCP_VPC_NETWORK
  regions {
    name = "us-west1"
    shared_subnet = "default"
  }
  air_gap_install = false
}
`, name)
}

func gcpProviderConfigWithCredentials(name string) string {
	return fmt.Sprintf(`
variable "GCP_VPC_NETWORK" {
  type = string
}

variable "GCP_CREDENTIALS" {
  type      = string
  sensitive = true
}

variable "GCP_PROJECT_ID" {
  type = string
}

resource "yba_gcp_provider" "test" {
  name                 = "%s"
  credentials          = var.GCP_CREDENTIALS
  use_host_credentials = false
  project_id           = var.GCP_PROJECT_ID
  network              = var.GCP_VPC_NETWORK
  ssh_keypair_name     = "test-keypair"
  regions {
    name          = "us-west1"
    shared_subnet = "default"
  }
  air_gap_install = false
}
`, name)
}

func gcpProviderConfigWithHostCredentials(name string) string {
	return fmt.Sprintf(`
variable "GCP_VPC_NETWORK" {
  type = string
}

variable "GCP_PROJECT_ID" {
  type = string
}

resource "yba_gcp_provider" "test" {
  name                 = "%s"
  use_host_credentials = true
  project_id           = var.GCP_PROJECT_ID
  network              = var.GCP_VPC_NETWORK
  ssh_keypair_name     = "test-keypair"
  regions {
    name = "us-west1"
    shared_subnet = "default"
  }
  air_gap_install = false
}
`, name)
}

func gcpProviderConfigWithSharedVPC(name string) string {
	return fmt.Sprintf(`
variable "GCP_VPC_NETWORK" {
  type = string
}

variable "GCP_CREDENTIALS" {
  type      = string
  sensitive = true
}

variable "GCP_PROJECT_ID" {
  type = string
}

variable "GCP_SHARED_VPC_PROJECT_ID" {
  type        = string
  description = "GCP shared VPC host project ID"
}

resource "yba_gcp_provider" "test" {
  name                  = "%s"
  credentials           = var.GCP_CREDENTIALS
  project_id            = var.GCP_PROJECT_ID
  shared_vpc_project_id = var.GCP_SHARED_VPC_PROJECT_ID
  network               = var.GCP_VPC_NETWORK
  ssh_keypair_name      = "test-keypair"
  regions {
    name = "us-west1"
    shared_subnet = "default"
  }
  air_gap_install = false
}
`, name)
}

func gcpProviderConfigWithFirewallTags(name string) string {
	return fmt.Sprintf(`
variable "GCP_VPC_NETWORK" {
  type = string
}

variable "GCP_CREDENTIALS" {
  type      = string
  sensitive = true
}

variable "GCP_PROJECT_ID" {
  type = string
}

resource "yba_gcp_provider" "test" {
  name             = "%s"
  credentials      = var.GCP_CREDENTIALS
  project_id       = var.GCP_PROJECT_ID
  network          = var.GCP_VPC_NETWORK
  yb_firewall_tags = "yb-db-node"
  ssh_keypair_name = "test-keypair"
  regions {
    name = "us-west1"
    shared_subnet = "default"
  }
  air_gap_install = false
}
`, name)
}
