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
	"os"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	client "github.com/yugabyte/platform-go-client"

	"github.com/yugabyte/terraform-provider-yba/internal/acctest"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// TestAccGCPProvider_WithCredentials tests GCP provider creation with credentials
func TestAccGCPProvider_WithCredentials(t *testing.T) {
	var provider client.Provider

	rName := acctest.RandomName("gcp")
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

	rName := acctest.RandomName("gcp-host")
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

// TestAccGCPProvider_WithSharedVPC tests GCP provider with shared VPC configuration.
//
// Requires a GCP Shared VPC host project (a separate project from GCP_PROJECT_ID),
// surfaced as TF_VAR_GCP_SHARED_VPC_PROJECT_ID. The acctest/gcp fixture is a plain
// single-project VPC and doesn't provision one, so this skips unless that var is
// set. To enable: pre-create a Shared VPC host project, grant the yba SA
// networkUser on its subnet, and export GCP_SHARED_VPC_PROJECT_ID.
func TestAccGCPProvider_WithSharedVPC(t *testing.T) {
	var provider client.Provider

	rName := acctest.RandomName("gcp-vpc")
	resource.ParallelTest(t, resource.TestCase{
		PreCheck: func() {
			acctest.TestAccPreCheck(t)
			acctest.TestAccPreCheckGCP(t)
			if os.Getenv("TF_VAR_GCP_SHARED_VPC_PROJECT_ID") == "" {
				t.Skip("TF_VAR_GCP_SHARED_VPC_PROJECT_ID not set; skipping Shared VPC test")
			}
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

	rName := acctest.RandomName("gcp-upd")
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

	rName := acctest.RandomName("gcp-fw")
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
	cUUID := acctest.APIClient.CustomerID

	for _, r := range s.RootModule().Resources {
		if r.Type != "yba_gcp_provider" {
			continue
		}
		// Provider deletion is async: YBA runs a delete task and the provider
		// lingers in the list until it finishes. Poll until it's gone instead of
		// a blind sleep, so this returns as soon as deletion completes (and fails
		// fast with a clear error rather than hanging the suite).
		const timeout = 90 * time.Second
		const interval = 5 * time.Second
		deadline := time.Now().Add(timeout)
		for {
			res, response, err := conn.CloudProvidersAPI.GetListOfProviders(context.Background(),
				cUUID).Execute()
			if err != nil {
				return utils.ErrorFromHTTPResponse(response, err, utils.TestEntity,
					"GCP Provider Test", "Read")
			}
			found := false
			for _, p := range res {
				if p.GetUuid() == r.Primary.ID {
					found = true
					break
				}
			}
			if !found {
				break
			}
			if time.Now().After(deadline) {
				return fmt.Errorf(
					"GCP provider %s is not destroyed after %s", r.Primary.ID, timeout)
			}
			time.Sleep(interval)
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

variable "GCP_REGION" {
  type = string
}

variable "GCP_SUBNETWORK" {
  type = string
}

resource "yba_gcp_provider" "test" {
  name        = "%s"
  credentials = var.GCP_CREDENTIALS
  project_id  = var.GCP_PROJECT_ID
  network     = var.GCP_VPC_NETWORK
  regions {
    code = var.GCP_REGION
    shared_subnet = var.GCP_SUBNETWORK
  }
  yba_managed_image_bundles {
    arch = "x86_64"
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

variable "GCP_REGION" {
  type = string
}

variable "GCP_SUBNETWORK" {
  type = string
}

resource "yba_gcp_provider" "test" {
  name                 = "%s"
  credentials          = var.GCP_CREDENTIALS
  project_id           = var.GCP_PROJECT_ID
  network              = var.GCP_VPC_NETWORK
  regions {
    code          = var.GCP_REGION
    shared_subnet = var.GCP_SUBNETWORK
  }
  yba_managed_image_bundles {
    arch = "x86_64"
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

variable "GCP_REGION" {
  type = string
}

variable "GCP_SUBNETWORK" {
  type = string
}

resource "yba_gcp_provider" "test" {
  name                 = "%s"
  use_host_credentials = true
  project_id           = var.GCP_PROJECT_ID
  network              = var.GCP_VPC_NETWORK
  regions {
    code = var.GCP_REGION
    shared_subnet = var.GCP_SUBNETWORK
  }
  yba_managed_image_bundles {
    arch = "x86_64"
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

variable "GCP_REGION" {
  type = string
}

variable "GCP_SUBNETWORK" {
  type = string
}

resource "yba_gcp_provider" "test" {
  name                  = "%s"
  credentials           = var.GCP_CREDENTIALS
  project_id            = var.GCP_PROJECT_ID
  shared_vpc_project_id = var.GCP_SHARED_VPC_PROJECT_ID
  network               = var.GCP_VPC_NETWORK
  regions {
    code = var.GCP_REGION
    shared_subnet = var.GCP_SUBNETWORK
  }
  yba_managed_image_bundles {
    arch = "x86_64"
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

variable "GCP_REGION" {
  type = string
}

variable "GCP_SUBNETWORK" {
  type = string
}

resource "yba_gcp_provider" "test" {
  name             = "%s"
  credentials      = var.GCP_CREDENTIALS
  project_id       = var.GCP_PROJECT_ID
  network          = var.GCP_VPC_NETWORK
  yb_firewall_tags = "yb-db-node"
  regions {
    code = var.GCP_REGION
    shared_subnet = var.GCP_SUBNETWORK
  }
  yba_managed_image_bundles {
    arch = "x86_64"
  }
  air_gap_install = false
}
`, name)
}
