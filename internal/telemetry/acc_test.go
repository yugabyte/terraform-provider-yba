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

package telemetry_test

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"

	"github.com/yugabyte/terraform-provider-yba/internal/acctest"
	"github.com/yugabyte/terraform-provider-yba/internal/api"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

const globalRuntimeScope = "00000000-0000-0000-0000-000000000000"

// enableTelemetryFlags sets the runtime flags these tests need and never resets
// them: provider CRUD needs an export flag on, and resetting metrics_export is
// rejected while ANY universe still has metrics export.
func enableTelemetryFlags(t *testing.T) {
	t.Helper()
	c := acctest.APIClient
	flags := map[string]string{
		"yb.universe.metrics_export_enabled":         "true",
		"yb.universe.audit_logging_enabled":          "true",
		"yb.telemetry.allow_otlp":                    "true",
		"yb.telemetry.skip_connectivity_validations": "true",
	}
	for key, val := range flags {
		_, resp, err := c.YugawareClient.RuntimeConfigurationAPI.
			SetKey(context.Background(), c.CustomerID, globalRuntimeScope, key).
			NewValue(val).Execute()
		if err != nil {
			t.Fatalf("enabling runtime flag %s=%s: %s", key, val,
				utils.ErrorFromHTTPResponse(resp, err, utils.TestEntity,
					"Telemetry Flags", "Set"))
		}
	}
}

// otlpProviderHCL renders an OTLP provider — the representative sink, valid for
// both metrics and logs.
func otlpProviderHCL(label, name string) string {
	return fmt.Sprintf(`
resource "yba_otlp_telemetry_provider" %q {
  name = %q

  endpoint = "http://otel-collector.acctest:4317"
}
`, label, name)
}

func otlpProviderConfig(name string) string {
	return otlpProviderHCL("test", name) + `
data "yba_telemetry_provider" "lookup" {
  name = yba_otlp_telemetry_provider.test.name
}
`
}

func TestAccTelemetryProvider_OTLP(t *testing.T) {
	name := acctest.RandomName("tp-otlp")
	resourceName := "yba_otlp_telemetry_provider.test"

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			acctest.TestAccPreCheck(t)
			enableTelemetryFlags(t)
		},
		ProviderFactories: acctest.ProviderFactories,
		CheckDestroy:      testAccCheckTelemetryProviderDestroy,
		Steps: []resource.TestStep{
			{
				Config: otlpProviderConfig(name),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckTelemetryProviderExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "name", name),
					resource.TestCheckResourceAttrPair(
						"data.yba_telemetry_provider.lookup", "id", resourceName, "id"),
					resource.TestCheckResourceAttr(
						"data.yba_telemetry_provider.lookup", "type", "OTLP"),
				),
			},
			{
				// Config fields aren't refreshed on Read (YBA masks secrets), so
				// import verifies only id/name/tags.
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{
					"endpoint", "auth_type", "protocol", "compression",
					"timeout_seconds",
				},
			},
		},
	})
}

func testAccCheckTelemetryProviderExists(n string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("telemetry provider %q not found in state", n)
		}
		if rs.Primary.ID == "" {
			return fmt.Errorf("telemetry provider %q has no id", n)
		}
		c := acctest.APIClient
		//nolint:bodyclose // body is closed inside GetTelemetryProvider
		_, _, err := c.VanillaClient.GetTelemetryProvider(
			context.Background(), c.CustomerID, rs.Primary.ID, c.APIKey)
		return err
	}
}

func testAccCheckTelemetryProviderDestroy(s *terraform.State) error {
	c := acctest.APIClient
	for _, rs := range s.RootModule().Resources {
		// Matches every per-sink resource (yba_otlp_telemetry_provider, ...)
		// but not yba_universe_telemetry_config.
		if !strings.HasSuffix(rs.Type, "_telemetry_provider") {
			continue
		}
		//nolint:bodyclose // body is closed inside GetTelemetryProvider
		_, _, err := c.VanillaClient.GetTelemetryProvider(
			context.Background(), c.CustomerID, rs.Primary.ID, c.APIKey)
		if err == nil {
			return fmt.Errorf("telemetry provider %s still exists after destroy", rs.Primary.ID)
		}
		if !errors.Is(err, api.ErrTelemetryProviderMissing) {
			return fmt.Errorf("unexpected error checking destroyed provider %s: %w",
				rs.Primary.ID, err)
		}
	}
	return nil
}

// fastUpgradeOptions keeps each reconfigure cheap; the 3-min default per-node
// sleeps would otherwise dominate every edit step.
const fastUpgradeOptions = `
  upgrade_options {
    sleep_after_master_restart_millis  = 5000
    sleep_after_tserver_restart_millis = 5000
  }
`

func universeTelemetryMetricsOnly(uniRef, provARef string) string {
	return fmt.Sprintf(`
resource "yba_universe_telemetry_config" "test" {
  universe_uuid = %s

  metrics {
    collection_level        = "NORMAL"
    scrape_interval_seconds = 30
    scrape_timeout_seconds  = 20
    scrape_config_targets   = ["MASTER_EXPORT", "TSERVER_EXPORT"]

    exporter {
      exporter_uuid  = %s
      metrics_prefix = "acc1."
    }
  }
%s
}
`, uniRef, provARef, fastUpgradeOptions)
}

func universeTelemetryMultiExporter(uniRef, provARef, provBRef string) string {
	return fmt.Sprintf(`
resource "yba_universe_telemetry_config" "test" {
  universe_uuid = %s

  metrics {
    collection_level        = "ALL"
    scrape_interval_seconds = 15
    scrape_timeout_seconds  = 10
    scrape_config_targets   = ["MASTER_EXPORT", "TSERVER_EXPORT", "NODE_EXPORT"]

    exporter {
      exporter_uuid   = %s
      metrics_prefix  = "acc2."
      send_batch_size = 200
    }
    exporter {
      exporter_uuid = %s
    }
  }

  audit_logs {
    ysql_audit_config {
      classes   = ["READ", "WRITE", "DDL"]
      log_level = "LOG"
    }
    exporter {
      exporter_uuid   = %s
      additional_tags = { env = "acc" }
    }
  }
%s
}
`, uniRef, provARef, provBRef, provARef, fastUpgradeOptions)
}

func gcpProviderAndUniverse(name string) string {
	return fmt.Sprintf(`
variable "GCP_VPC_NETWORK" {
  type = string
}

variable "GCP_REGION" {
  type = string
}

variable "GCP_CREDENTIALS" {
  type      = string
  sensitive = true
}

variable "GCP_PROJECT_ID" {
  type = string
}

variable "GCP_SUBNETWORK" {
  type = string
}

resource "yba_cloud_provider" "gcp" {
  code = "gcp"
  name = "%s-cp"
  gcp_config_settings {
    network      = var.GCP_VPC_NETWORK
    use_host_vpc = false
    project_id   = var.GCP_PROJECT_ID
    credentials  = var.GCP_CREDENTIALS
  }
  regions {
    code = var.GCP_REGION
    name = var.GCP_REGION
    zones {
      subnet = var.GCP_SUBNETWORK
    }
  }
  ssh_port        = 22
  air_gap_install = false
}

data "yba_provider_key" "key" {
  provider_id = yba_cloud_provider.gcp.id
}

data "yba_release_version" "release_version" {
  depends_on = [data.yba_provider_key.key]
}

resource "yba_universe" "gcp" {
  clusters {
    cluster_type = "PRIMARY"
    user_intent {
      universe_name      = "%s"
      provider           = yba_cloud_provider.gcp.id
      region_list        = yba_cloud_provider.gcp.regions[*].uuid
      num_nodes          = 1
      replication_factor = 1
      instance_type      = "n2-standard-2"
      device_info {
        num_volumes  = 1
        volume_size  = 375
        storage_type = "Persistent"
      }
      assign_public_ip              = true
      use_time_sync                 = true
      enable_ysql                   = true
      enable_node_to_node_encrypt   = true
      enable_client_to_node_encrypt = true
      yb_software_version           = data.yba_release_version.release_version.id
      access_key_code               = data.yba_provider_key.key.id
      instance_tags = {
        "yb_owner" = "terraform_acctest"
        "yb_task"  = "dev"
        "yb_dept"  = "dev"
      }
    }
  }
  communication_ports {}
}
`, name, name)
}

// Named *Long so the short tier's `-skip '^TestAccLong'` skips it; runs only on
// acctest-long (merge to main / manual dispatch).
func TestAccLong_UniverseTelemetryConfig_GCP(t *testing.T) {
	name := acctest.RandomName("tel-gcp")
	nameA := acctest.RandomName("tp-a")
	nameB := acctest.RandomName("tp-b")
	base := gcpProviderAndUniverse(name)
	provA := otlpProviderHCL("a", nameA)
	provB := otlpProviderHCL("b", nameB)
	uniRef := "yba_universe.gcp.id"
	resourceName := "yba_universe_telemetry_config.test"

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			acctest.TestAccPreCheck(t)
			acctest.TestAccPreCheckGCP(t)
			enableTelemetryFlags(t)
		},
		ProviderFactories: acctest.ProviderFactories,
		CheckDestroy:      testAccCheckTelemetryUniverseDestroy,
		Steps: []resource.TestStep{
			{
				Config: base + provA + universeTelemetryMetricsOnly(
					uniRef, "yba_otlp_telemetry_provider.a.id"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrPair(
						resourceName, "universe_uuid", "yba_universe.gcp", "id"),
					resource.TestCheckResourceAttr(
						resourceName, "metrics.0.collection_level", "NORMAL"),
					resource.TestCheckResourceAttr(resourceName, "metrics.0.exporter.#", "1"),
				),
			},
			{
				Config: base + provA + provB + universeTelemetryMultiExporter(
					uniRef, "yba_otlp_telemetry_provider.a.id", "yba_otlp_telemetry_provider.b.id"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(
						resourceName, "metrics.0.collection_level", "ALL"),
					resource.TestCheckResourceAttr(resourceName, "metrics.0.exporter.#", "2"),
					resource.TestCheckResourceAttr(
						resourceName, "metrics.0.scrape_config_targets.#", "3"),
					resource.TestCheckResourceAttr(resourceName, "audit_logs.#", "1"),
					resource.TestCheckResourceAttr(
						resourceName, "audit_logs.0.exporter.0.additional_tags.env", "acc"),
				),
			},
			{
				ResourceName:     resourceName,
				ImportState:      true,
				ImportStateCheck: importedMetricsExporters(2),
			},
			{
				// Remove only the config, keeping the universe: asserts the
				// disable-on-destroy path leaves the surviving universe with no exporters.
				Config: base + provA + provB,
				Check:  testAccCheckUniverseExportDisabled("yba_universe.gcp"),
			},
		},
	})
}

// importedMetricsExporters checks universe_uuid + exporter count, not full-state
// equality, so a defaulted field YBA echoes back differently can't make it flaky.
func importedMetricsExporters(want int) resource.ImportStateCheckFunc {
	return func(states []*terraform.InstanceState) error {
		if len(states) != 1 {
			return fmt.Errorf("expected 1 imported state, got %d", len(states))
		}
		attrs := states[0].Attributes
		if attrs["universe_uuid"] == "" {
			return errors.New("imported universe_uuid is empty")
		}
		if got := attrs["metrics.0.exporter.#"]; got != strconv.Itoa(want) {
			return fmt.Errorf("imported metrics exporter count = %q, want %d", got, want)
		}
		return nil
	}
}

func testAccCheckUniverseExportDisabled(uniResource string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[uniResource]
		if !ok {
			return fmt.Errorf("universe resource %s not found in state", uniResource)
		}
		uniUUID := rs.Primary.ID
		c := acctest.APIClient
		config, _, err := c.YugawareClientV2.UniverseAPI.
			GetExportTelemetryConfig(context.Background(), c.CustomerID, uniUUID).Execute()
		if err != nil {
			return fmt.Errorf("checking export disabled on universe %s: %w", uniUUID, err)
		}
		if config == nil {
			return nil
		}
		if config.Metrics != nil && len(config.Metrics.Exporters) > 0 {
			return fmt.Errorf("universe %s still has %d metrics exporter(s) after config removal",
				uniUUID, len(config.Metrics.Exporters))
		}
		if config.AuditLogs != nil && len(config.AuditLogs.Exporters) > 0 {
			return fmt.Errorf("universe %s still has %d audit exporter(s) after config removal",
				uniUUID, len(config.AuditLogs.Exporters))
		}
		if config.QueryLogs != nil && len(config.QueryLogs.Exporters) > 0 {
			return fmt.Errorf("universe %s still has %d query exporter(s) after config removal",
				uniUUID, len(config.QueryLogs.Exporters))
		}
		return nil
	}
}

func testAccCheckTelemetryUniverseDestroy(s *terraform.State) error {
	if err := testAccCheckTelemetryProviderDestroy(s); err != nil {
		return err
	}
	c := acctest.APIClient
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "yba_universe" {
			continue
		}
		_, _, err := c.YugawareClient.UniverseManagementAPI.
			GetUniverse(context.Background(), c.CustomerID, rs.Primary.ID).Execute()
		if err == nil {
			return fmt.Errorf("universe %s still exists after destroy", rs.Primary.ID)
		}
	}
	return nil
}
