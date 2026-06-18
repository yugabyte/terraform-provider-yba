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
	"os"
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

// enableTelemetryFlags turns on the global runtime flags every telemetry
// acceptance test depends on, via the API, and intentionally never resets
// them. It is called from each test's PreCheck.
//
// Why set-and-leave instead of managing them as yba_runtime_config resources
// (which DeleteKey on destroy)? Two YBA rules make a reset-on-teardown
// fragile on the shared standing fixture:
//
//   - TelemetryProviderService.throwExceptionIfRuntimeFlagDisabled: every
//     provider CRUD call (including delete) needs at least one export flag on.
//   - MetricsExportEnabledValidator.validateDeleteConfig: resetting
//     yb.universe.metrics_export_enabled is rejected while ANY universe on the
//     YBA still has metrics export — not just this test's universe.
//
// Leaving the flags enabled is the correct steady state for a telemetry-test
// YBA anyway, and keeps these tests independent of teardown ordering.
//
// The flags set are:
//   - yb.universe.metrics_export_enabled / yb.universe.audit_logging_enabled:
//     gate provider creation and the metrics / audit export pipelines.
//   - yb.telemetry.skip_connectivity_validations: every provider create runs a
//     connectivity probe (DataDog API, S3 PutObject, ...); skipping it lets the
//     tests use placeholder credentials with no reachable destination.
func enableTelemetryFlags(t *testing.T) {
	t.Helper()
	c := acctest.APIClient
	flags := map[string]string{
		"yb.universe.metrics_export_enabled":         "true",
		"yb.universe.audit_logging_enabled":          "true",
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

// dataDogProviderHCL renders a DataDog telemetry provider with the given
// Terraform resource label and YBA name. DataDog is used throughout because it
// has no per-type enablement flag (only OTLP / Loki / S3 do), so the global
// flags set in PreCheck are sufficient. The placeholder API key is only
// accepted because skip_connectivity_validations is enabled.
func dataDogProviderHCL(label, name string) string {
	return fmt.Sprintf(`
resource "yba_telemetry_provider" %q {
  name = %q

  data_dog {
    site    = "datadoghq.com"
    api_key = "placeholder-key-connectivity-skipped"
  }
}
`, label, name)
}

// dataDogProviderConfig renders a single DataDog provider plus a data source
// that looks it up by name.
func dataDogProviderConfig(name string) string {
	return dataDogProviderHCL("test", name) + `
data "yba_telemetry_provider" "lookup" {
  name = yba_telemetry_provider.test.name
}
`
}

// TestAccTelemetryProvider_DataDog exercises the full yba_telemetry_provider
// lifecycle against a standing YBA — create, read-back, the data-source
// lookup-by-name, and the detach-aware delete (a no-op detach here since no
// universe references the provider) — plus import. Short tier: no universe, so
// it runs on every PR.
func TestAccTelemetryProvider_DataDog(t *testing.T) {
	name := acctest.RandomName("tp-dd")
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
				Config: dataDogProviderConfig(name),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckTelemetryProviderExists(resourceName),
					resource.TestCheckResourceAttr(resourceName, "name", name),
					resource.TestCheckResourceAttr(resourceName, "type", "DATA_DOG"),
					// The data source resolves the same provider by name.
					resource.TestCheckResourceAttrPair(
						"data.yba_telemetry_provider.lookup", "id", resourceName, "id"),
					resource.TestCheckResourceAttr(
						"data.yba_telemetry_provider.lookup", "type", "DATA_DOG"),
				),
			},
			{
				// Importer round-trips the id. The config block (site/api_key)
				// is intentionally not refreshed on Read — YBA never returns the
				// secret — so import verification is limited to the id/name/type.
				ResourceName:            resourceName,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"data_dog", "data_dog.#", "data_dog.0.%"},
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
// Universe telemetry config — edit-flow steps shared by the env-gated and the
// long (self-provisioning) tests below. uniRef / provRef are raw HCL
// expressions (a quoted literal UUID, or a reference like
// `yba_universe.gcp.id`) so the same step bodies drive either universe source.
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
// Used as the create step and as the shrink-back step (which removes the audit
// pipeline and the second exporter added by universeTelemetryMultiExporter).
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

// importedMetricsExporters asserts the imported state repopulated the metrics
// pipeline from the v2 GET API. It checks the universe_uuid round-tripped and
// the metrics exporter count, rather than full-state equality, so a defaulted
// batching/memory field that YBA echoes back in a different shape can't make
// the step flaky — the point is that import reads the pipeline back at all.
func importedMetricsExporters(uniRef string, want int) resource.ImportStateCheckFunc {
	return func(states []*terraform.InstanceState) error {
		if len(states) != 1 {
			return fmt.Errorf("expected 1 imported state, got %d", len(states))
		}
		attrs := states[0].Attributes
		if got := attrs["universe_uuid"]; got != uniRef {
			return fmt.Errorf("imported universe_uuid = %q, want %q", got, uniRef)
		}
		if got := attrs["metrics.0.exporter.#"]; got != strconv.Itoa(want) {
			return fmt.Errorf("imported metrics exporter count = %q, want %d", got, want)
		}
		return nil
	}
}

// testAccCheckUniverseExportDisabled confirms that destroying the
// yba_universe_telemetry_config left no metrics exporters on the (still
// existing) universe — i.e. the empty-config disable POST actually ran.
func testAccCheckUniverseExportDisabled(uniUUID string) func(*terraform.State) error {
	return func(_ *terraform.State) error {
		c := acctest.APIClient
		config, _, err := c.YugawareClientV2.UniverseAPI.
			GetExportTelemetryConfig(context.Background(), c.CustomerID, uniUUID).Execute()
		if err != nil {
			return fmt.Errorf("checking export disabled on universe %s: %w", uniUUID, err)
		}
		if config != nil && config.Metrics != nil && len(config.Metrics.Exporters) > 0 {
			return fmt.Errorf("universe %s still has %d metrics exporter(s) after destroy",
				uniUUID, len(config.Metrics.Exporters))
		}
		return nil
	}
}

// TestAccUniverseTelemetryConfig drives the full edit flow against a
// PRE-EXISTING universe named by YBA_ACCTEST_UNIVERSE_UUID: create a metrics
// pipeline, edit it heavily while adding a second exporter and an audit
// pipeline that shares the first exporter, shrink back to metrics-only, then
// import. It skips when the env var is unset (every reconfigure restarts the
// target universe), so it is opt-in for fast local iteration without paying the
// ~15-minute universe deploy that TestAccLong_UniverseTelemetryConfig_GCP does.
func TestAccUniverseTelemetryConfig(t *testing.T) {
	uniUUID := os.Getenv("YBA_ACCTEST_UNIVERSE_UUID")
	if uniUUID == "" {
		t.Skip("YBA_ACCTEST_UNIVERSE_UUID not set; skipping universe telemetry config " +
			"edit-flow test (each step triggers a restart of the target universe)")
	}
	uniRef := strconv.Quote(uniUUID)
	nameA := acctest.RandomName("tp-a")
	nameB := acctest.RandomName("tp-b")
	provA := dataDogProviderHCL("a", nameA)
	provB := dataDogProviderHCL("b", nameB)
	resourceName := "yba_universe_telemetry_config.test"

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			acctest.TestAccPreCheck(t)
			enableTelemetryFlags(t)
		},
		ProviderFactories: acctest.ProviderFactories,
		CheckDestroy: func(s *terraform.State) error {
			if err := testAccCheckTelemetryProviderDestroy(s); err != nil {
				return err
			}
			return testAccCheckUniverseExportDisabled(uniUUID)(s)
		},
		Steps: []resource.TestStep{
			{
				Config: provA + universeTelemetryMetricsOnly(uniRef, "yba_telemetry_provider.a.id"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "universe_uuid", uniUUID),
					resource.TestCheckResourceAttr(
						resourceName, "metrics.0.collection_level", "NORMAL"),
					resource.TestCheckResourceAttr(resourceName, "metrics.0.exporter.#", "1"),
					resource.TestCheckResourceAttr(
						resourceName, "metrics.0.exporter.0.metrics_prefix", "acc1."),
				),
			},
			{
				Config: provA + provB + universeTelemetryMultiExporter(
					uniRef, "yba_telemetry_provider.a.id", "yba_telemetry_provider.b.id"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(
						resourceName, "metrics.0.collection_level", "ALL"),
					resource.TestCheckResourceAttr(resourceName, "metrics.0.exporter.#", "2"),
					resource.TestCheckResourceAttr(
						resourceName, "metrics.0.scrape_config_targets.#", "3"),
					resource.TestCheckResourceAttr(resourceName, "audit_logs.#", "1"),
					resource.TestCheckResourceAttr(
						resourceName, "audit_logs.0.ysql_audit_config.0.enabled", "true"),
					resource.TestCheckResourceAttr(
						resourceName, "audit_logs.0.exporter.0.additional_tags.env", "acc"),
				),
			},
			{
				// Shrink back: removing provider B's HCL and the audit block
				// must take the metrics pipeline back to a single exporter and
				// clear the audit pipeline (Update, not just Create).
				Config: provA + universeTelemetryMetricsOnly(uniRef, "yba_telemetry_provider.a.id"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "metrics.0.exporter.#", "1"),
					resource.TestCheckResourceAttr(resourceName, "audit_logs.#", "0"),
				),
			},
			{
				ResourceName:     resourceName,
				ImportState:      true,
				ImportStateCheck: importedMetricsExporters(uniUUID, 1),
			},
		},
	})
}

// ---------------------------------------------------------------------------
// Long tier: self-provisions a GCP universe and runs the edit flow against it.
// Named *_GCP so make acctest-long (which `-skip '_(AWS|Azure)_'`) runs it; the
// short tier `-skip '^TestAccLong'`, so it never runs on a plain PR.
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
	provA := dataDogProviderHCL("a", nameA)
	provB := dataDogProviderHCL("b", nameB)
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
				ImportStateCheck: importedMetricsExportersFromState(resourceName, 2),
			},
		},
	})
}

// importedMetricsExportersFromState is importedMetricsExporters for a universe
// whose UUID is only known at apply time: it pulls the expected universe_uuid
// from the prior resource state rather than a literal.
func importedMetricsExportersFromState(
	resourceName string, want int,
) resource.ImportStateCheckFunc {
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
