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

// Acceptance tests for yba_universe_load_balancer_config. In universe_test to
// reuse the per-cloud universe fixtures; the load balancer itself is created
// in-test through the cloud's own provider (ExternalProviders).

package universe_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"

	"github.com/yugabyte/terraform-provider-yba/internal/acctest"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

func lbAttachConfig(uniLabel, region, lbNameRef, lbFQDNRef string) string {
	fqdn := ""
	if lbFQDNRef != "" {
		fqdn = fmt.Sprintf("lb_fqdn = %s", lbFQDNRef)
	}
	return fmt.Sprintf(`
	resource "yba_universe_load_balancer_config" "test" {
		universe_uuid = yba_universe.%s.id

		load_balancer {
			region  = %s
			lb_name = %s
			%s
		}
	}
`, uniLabel, region, lbNameRef, fqdn)
}

// awsLBConfig creates a bare NLB — YBA manages target groups and listeners
// itself. name_prefix keeps the generated name inside AWS's 32-char limit.
func awsLBConfig() string {
	return `
	variable "AWS_ACCESS_KEY_ID" {
		type      = string
		sensitive = true
	}

	variable "AWS_SECRET_ACCESS_KEY" {
		type      = string
		sensitive = true
	}

	provider "aws" {
		region     = "us-west-2"
		access_key = var.AWS_ACCESS_KEY_ID
		secret_key = var.AWS_SECRET_ACCESS_KEY
	}

	resource "aws_lb" "test" {
		name_prefix        = "ybalb-"
		load_balancer_type = "network"
		internal           = true
		subnets            = [var.AWS_ZONE_SUBNET_ID]
	}
`
}

// gcpLBConfig creates the regional backend service (plus its TCP health
// check) whose NAME is what YBA treats as the load balancer identifier on GCP.
func gcpLBConfig(name string) string {
	return fmt.Sprintf(`
	provider "google" {
		project     = var.GCP_PROJECT_ID
		credentials = var.GCP_CREDENTIALS
		region      = var.GCP_REGION
	}

	resource "google_compute_region_health_check" "test" {
		name   = "%s-hc"
		region = var.GCP_REGION
		tcp_health_check {
			port = 5433
		}
	}

	resource "google_compute_region_backend_service" "test" {
		name                  = "%s-bs"
		region                = var.GCP_REGION
		protocol              = "TCP"
		load_balancing_scheme = "INTERNAL"
		health_checks         = [google_compute_region_health_check.test.id]
	}
`, name, name)
}

// azureLBConfig creates a Standard load balancer with the frontend IP
// configuration YBA requires to already exist before attach.
func azureLBConfig(name string) string {
	return fmt.Sprintf(`
	variable "AZURE_SUBSCRIPTION_ID" {
		type = string
	}

	variable "AZURE_TENANT_ID" {
		type = string
	}

	variable "AZURE_CLIENT_ID" {
		type      = string
		sensitive = true
	}

	variable "AZURE_CLIENT_SECRET" {
		type      = string
		sensitive = true
	}

	variable "AZURE_RG" {
		type = string
	}

	provider "azurerm" {
		features {}
		subscription_id = var.AZURE_SUBSCRIPTION_ID
		tenant_id       = var.AZURE_TENANT_ID
		client_id       = var.AZURE_CLIENT_ID
		client_secret   = var.AZURE_CLIENT_SECRET
	}

	resource "azurerm_lb" "test" {
		name                = "%s-lb"
		location            = "westus2"
		resource_group_name = var.AZURE_RG
		sku                 = "Standard"

		frontend_ip_configuration {
			name                          = "%s-fe"
			subnet_id                     = var.AZURE_SUBNET_ID
			private_ip_address_allocation = "Dynamic"
		}
	}
`, name, name)
}

// testAccCheckUniverseLBDisabled asserts no cluster on the universe still has
// load balancer management enabled — the destroy semantics of the resource.
func testAccCheckUniverseLBDisabled(cloud, resName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		r, ok := s.RootModule().Resources[resName]
		if !ok {
			return fmt.Errorf("resource not found: %s", resName)
		}
		apiClient, err := acctest.APIClientForCloud(cloud)
		if err != nil {
			return err
		}
		uni, response, err := apiClient.YugawareClient.UniverseManagementAPI.
			GetUniverse(context.Background(), apiClient.CustomerID, r.Primary.ID).Execute()
		if err != nil {
			return utils.ErrorFromHTTPResponse(response, err, utils.TestEntity,
				"Universe", "Read - LB disabled check")
		}
		for _, cl := range uni.UniverseDetails.Clusters {
			if cl.UserIntent.GetEnableLB() {
				return fmt.Errorf("cluster %s still has enableLB set after destroy",
					cl.ClusterType)
			}
		}
		return nil
	}
}

func importCheckSingleLB(states []*terraform.InstanceState) error {
	if len(states) != 1 {
		return fmt.Errorf("expected 1 imported state, got %d", len(states))
	}
	st := states[0]
	if st.Attributes["universe_uuid"] != st.ID {
		return fmt.Errorf("imported universe_uuid = %q, want ID %q",
			st.Attributes["universe_uuid"], st.ID)
	}
	if st.Attributes["load_balancer.#"] != "1" {
		return fmt.Errorf("imported load_balancer count = %q, want 1",
			st.Attributes["load_balancer.#"])
	}
	return nil
}

// lbTestSteps: attach with checks, import, then drop only the LB config to
// prove destroy detaches without touching the surviving universe.
func lbTestSteps(cloud, uniLabel, base, lbHCL, attach string) []resource.TestStep {
	uniRes := "yba_universe." + uniLabel
	resName := "yba_universe_load_balancer_config.test"
	return []resource.TestStep{
		{
			Config: base + lbHCL + attach,
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttrPair(resName, "universe_uuid", uniRes, "id"),
				resource.TestCheckResourceAttr(resName, "load_balancer.#", "1"),
			),
		},
		{
			ResourceName:     resName,
			ImportState:      true,
			ImportStateCheck: importCheckSingleLB,
		},
		{
			Config: base + lbHCL,
			Check:  testAccCheckUniverseLBDisabled(cloud, uniRes),
		},
	}
}

// Named *Long so the short tier's `-skip '^TestAccLong'` skips these; they
// run on acctest-long only. The three clouds run in parallel, each against
// its own standing YBA.

func TestAccLong_UniverseLoadBalancerConfig_AWS(t *testing.T) {
	rName := acctest.RandomName("lb-aws")
	base := universeAwsConfigWithNodes(rName, 3)
	attach := lbAttachConfig("aws", `"us-west-2"`, "aws_lb.test.name", "aws_lb.test.dns_name")

	resource.ParallelTest(t, resource.TestCase{
		PreCheck: func() {
			acctest.TestAccPreCheckAWS(t)
			acctest.TestAccPreCheckCloudYBA(t, "AWS")
		},
		ProviderFactories: acctest.ProviderFactories,
		ExternalProviders: map[string]resource.ExternalProvider{
			"aws": {Source: "hashicorp/aws"},
		},
		CheckDestroy: testAccCheckDestroyProviderAndUniverse("AWS"),
		Steps:        lbTestSteps("AWS", "aws", base, awsLBConfig(), attach),
	})
}

func TestAccLong_UniverseLoadBalancerConfig_GCP(t *testing.T) {
	rName := acctest.RandomName("lb-gcp")
	base := universeGcpConfigWithNodes(rName, 3)
	attach := lbAttachConfig("gcp", "var.GCP_REGION",
		"google_compute_region_backend_service.test.name", "")

	resource.ParallelTest(t, resource.TestCase{
		PreCheck: func() {
			acctest.TestAccPreCheckGCP(t)
			acctest.TestAccPreCheckCloudYBA(t, "GCP")
		},
		ProviderFactories: acctest.ProviderFactories,
		ExternalProviders: map[string]resource.ExternalProvider{
			"google": {Source: "hashicorp/google"},
		},
		CheckDestroy: testAccCheckDestroyProviderAndUniverse("GCP"),
		Steps:        lbTestSteps("GCP", "gcp", base, gcpLBConfig(rName), attach),
	})
}

func TestAccLong_UniverseLoadBalancerConfig_Azure(t *testing.T) {
	rName := acctest.RandomName("lb-azu")
	base := universeAzureConfigWithNodes(rName, 3)
	attach := lbAttachConfig("azu", `"westus2"`, "azurerm_lb.test.name", "")

	resource.ParallelTest(t, resource.TestCase{
		PreCheck: func() {
			acctest.TestAccPreCheckAzure(t)
			acctest.TestAccPreCheckCloudYBA(t, "AZURE")
		},
		ProviderFactories: acctest.ProviderFactories,
		ExternalProviders: map[string]resource.ExternalProvider{
			"azurerm": {Source: "hashicorp/azurerm"},
		},
		CheckDestroy: testAccCheckDestroyProviderAndUniverse("AZURE"),
		Steps:        lbTestSteps("AZURE", "azu", base, azureLBConfig(rName), attach),
	})
}
