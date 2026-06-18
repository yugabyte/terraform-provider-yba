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
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"

	"github.com/yugabyte/terraform-provider-yba/internal/acctest"
	"github.com/yugabyte/terraform-provider-yba/internal/api"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// globalRuntimeScope is YBA's well-known global runtime-config scope UUID.
const globalRuntimeScope = "00000000-0000-0000-0000-000000000000"

// enableTelemetryFlags turns on the global runtime flags these tests depend on,
// via the API, and intentionally never resets them. It is called from each
// test's PreCheck.
//
// Why set-and-leave instead of managing them as yba_runtime_config resources
// (which DeleteKey on destroy)? Two YBA rules make a reset-on-teardown fragile
// on the shared standing fixture:
//
//   - TelemetryProviderService.throwExceptionIfRuntimeFlagDisabled: every
//     provider CRUD call (including delete) needs at least one of the three
//     export flags on.
//   - MetricsExportEnabledValidator.validateDeleteConfig: resetting
//     yb.universe.metrics_export_enabled is rejected while ANY universe on the
//     YBA still has metrics export — not just this test's universe.
//
// Leaving the flags enabled is the correct steady state for a telemetry-test
// YBA anyway, and keeps these tests independent of teardown ordering.
//
// What each flag gates (verified against YBA source, not assumed):
//   - yb.universe.metrics_export_enabled: gates provider CREATE for every type
//     (TelemetryProviderController -> throwExceptionIfRuntimeFlagDisabled needs
//     one of metrics/audit/query export enabled). The unified v2
//     export-telemetry-configs endpoint itself checks no runtime flag.
//   - yb.universe.audit_logging_enabled: kept on defensively for the audit-log
//     pipeline step (and is a second way to satisfy the create gate above).
//   - yb.telemetry.allow_otlp: type-gated — required to CREATE an OTLP-type
//     provider (throwExceptionIfOTLPExporterRuntimeFlagDisabled). It is
//     additive, NOT a substitute for the export-enabled flag above. DataDog,
//     Splunk, etc. do not need it; OTLP does.
//   - yb.telemetry.skip_connectivity_validations: every provider create runs a
//     connectivity probe; skipping it lets the tests use a placeholder OTLP
//     endpoint with no reachable collector.
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

// otlpProviderHCL renders an OTLP telemetry provider with the given Terraform
// resource label and YBA name. OTLP is used as the representative type because
// it is allowed for both metrics and logs export (ProviderType.OTLP(true,true))
// and exercises the yb.telemetry.allow_otlp gate. The placeholder endpoint is
// only accepted because skip_connectivity_validations is enabled; with the
// default NoAuth / gRPC settings no credentials are needed.
func otlpProviderHCL(label, name string) string {
	return fmt.Sprintf(`
resource "yba_telemetry_provider" %q {
  name = %q

  otlp {
    endpoint = "http://otel-collector.acctest:4317"
  }
}
`, label, name)
}

// otlpProviderConfig renders a single OTLP provider plus a data source that
// looks it up by name.
func otlpProviderConfig(name string) string {
	return otlpProviderHCL("test", name) + `
data "yba_telemetry_provider" "lookup" {
  name = yba_telemetry_provider.test.name
}
`
}

// TestAccTelemetryProvider_OTLP exercises the full yba_telemetry_provider
// lifecycle against a standing YBA — create, read-back, the data-source
// lookup-by-name, and the detach-aware delete (a no-op detach here since no
// universe references the provider) — plus import. No universe is involved, so
// it runs on the short tier on every PR.
func TestAccTelemetryProvider_OTLP(t *testing.T) {
	name := acctest.RandomName("tp-otlp")
	resourceName := "yba_telemetry_provider.test"

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
					resource.TestCheckResourceAttr(resourceName, "type", "OTLP"),
					// The data source resolves the same provider by name.
					resource.TestCheckResourceAttrPair(
						"data.yba_telemetry_provider.lookup", "id", resourceName, "id"),
					resource.TestCheckResourceAttr(
						"data.yba_telemetry_provider.lookup", "type", "OTLP"),
				),
			},
			{
				// Importer round-trips the id. The config block (endpoint/auth)
				// is intentionally not refreshed on Read — YBA masks secrets —
				// so import verification is limited to the id/name/type.
				ResourceName:            resourceName,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"otlp", "otlp.#", "otlp.0.%"},
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
		if rs.Type != "yba_telemetry_provider" {
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

// ---------------------------------------------------------------------------
// Universe telemetry config — edit-flow steps. uniRef / provRef are raw HCL
// expressions (e.g. `yba_universe.gcp.id`) injected into the config.
// ---------------------------------------------------------------------------

// fastUpgradeOptions keeps each reconfigure cheap: a 1-node universe restarts
// in one cycle, and the 3-minute default per-node sleeps would dominate every
// edit step otherwise.
const fastUpgradeOptions = `
  upgrade_options {
    sleep_after_master_restart_millis  = 5000
    sleep_after_tserver_restart_millis = 5000
  }
`

// universeTelemetryMetricsOnly: a single metrics pipeline with one exporter.
// Used as the create step.
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

// universeTelemetryMultiExporter is the heavy edit step. Relative to
// universeTelemetryMetricsOnly it: edits metrics in place (collection_level,
// scrape interval/timeout, a TypeSet target added, prefix and batch size
// changed); adds a SECOND metrics exporter (provider B) — multiple exporters in
// one pipeline; and adds an audit-logs pipeline that reuses provider A — the
// same provider shared across two pipelines of one universe.
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
      enabled   = true
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

// ---------------------------------------------------------------------------
// Long tier: self-provisions a GCP universe and runs the edit flow against it.
// Named *_GCP so make acctest-long (which `-skip '_(AWS|Azure)_'`) runs it; the
// short tier `-skip '^TestAccLong'`, so it never runs on a plain PR. The
// acctest-long workflow runs on merges to main and on manual dispatch.
// ---------------------------------------------------------------------------

// gcpProviderAndUniverse renders a GCP cloud provider and a minimal single-node
// universe (RF1) to attach telemetry to. Mirrors the universe acceptance tests'
// GCP fixture; the TF_VAR_GCP_* inputs come from the acctest env.
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
		// Destroying the config disables export, then the providers detach and
		// delete and the universe is torn down; CheckDestroy confirms providers
		// and universe are all gone.
		CheckDestroy: testAccCheckTelemetryUniverseDestroy,
		Steps: []resource.TestStep{
			{
				Config: base + provA + universeTelemetryMetricsOnly(
					uniRef, "yba_telemetry_provider.a.id"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrPair(
						resourceName, "universe_uuid", "yba_universe.gcp", "id"),
					resource.TestCheckResourceAttr(
						resourceName, "metrics.0.collection_level", "NORMAL"),
					resource.TestCheckResourceAttr(resourceName, "metrics.0.exporter.#", "1"),
				),
			},
			{
				// Heavy edit: in-place metrics change + second exporter + an
				// audit pipeline sharing the first exporter, all in one apply.
				Config: base + provA + provB + universeTelemetryMultiExporter(
					uniRef, "yba_telemetry_provider.a.id", "yba_telemetry_provider.b.id"),
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
		},
	})
}

// importedMetricsExporters asserts the imported state repopulated the metrics
// pipeline from the v2 GET API: it checks universe_uuid is set and the metrics
// exporter count, rather than full-state equality, so a defaulted
// batching/memory field that YBA echoes back in a different shape can't make
// the step flaky — the point is that import reads the pipeline back at all.
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
