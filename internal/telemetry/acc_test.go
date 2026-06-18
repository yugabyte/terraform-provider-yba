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
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"

	"github.com/yugabyte/terraform-provider-yba/internal/acctest"
	"github.com/yugabyte/terraform-provider-yba/internal/api"
)

// telemetryFlagsConfig sets the two global runtime flags every telemetry
// provider acceptance test depends on, as managed resources so they are reset
// on destroy:
//
//   - yb.universe.metrics_export_enabled: at least one of the universe export
//     flags must be true or YBA rejects provider creation outright
//     (TelemetryProviderService.throwExceptionIfRuntimeFlagDisabled).
//   - yb.telemetry.skip_connectivity_validations: every provider type runs a
//     connectivity check on create (DataDog API, S3 PutObject, ...). Skipping
//     it lets the test use placeholder credentials without a reachable
//     destination.
//
// They are global singletons on the shared standing YBA, so these tests are
// serial (no resource.ParallelTest).
const telemetryFlagsConfig = `
resource "yba_runtime_config" "metrics_export" {
  key   = "yb.universe.metrics_export_enabled"
  value = "true"
}

resource "yba_runtime_config" "skip_connectivity" {
  key   = "yb.telemetry.skip_connectivity_validations"
  value = "true"
}
`

// dataDogProviderConfig renders a DataDog telemetry provider plus a data
// source that looks it up by name. DataDog is used because it has no
// per-type enablement flag (only OTLP / Loki / S3 do), so the two flags above
// are sufficient.
func dataDogProviderConfig(name string) string {
	return telemetryFlagsConfig + fmt.Sprintf(`
resource "yba_telemetry_provider" "test" {
  name = %q

  data_dog {
    site    = "datadoghq.com"
    api_key = "placeholder-key-connectivity-skipped"
  }

  depends_on = [
    yba_runtime_config.metrics_export,
    yba_runtime_config.skip_connectivity,
  ]
}

data "yba_telemetry_provider" "lookup" {
  name = yba_telemetry_provider.test.name
}
`, name)
}

// TestAccTelemetryProvider_DataDog exercises the full yba_telemetry_provider
// lifecycle against a standing YBA — create, read-back, the data-source
// lookup-by-name, and the detach-aware delete (a no-op detach here since no
// universe references the provider) — plus import.
func TestAccTelemetryProvider_DataDog(t *testing.T) {
	name := acctest.RandomName("tp-dd")
	resourceName := "yba_telemetry_provider.test"

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { acctest.TestAccPreCheck(t) },
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

// universeTelemetryConfig wires a metrics exporter (DataDog, which is allowed
// for metrics) onto an existing universe via a provider created in the same
// config.
func universeTelemetryConfig(uniUUID, providerName string) string {
	return telemetryFlagsConfig + fmt.Sprintf(`
resource "yba_telemetry_provider" "test" {
  name = %q

  data_dog {
    site    = "datadoghq.com"
    api_key = "placeholder-key-connectivity-skipped"
  }

  depends_on = [
    yba_runtime_config.metrics_export,
    yba_runtime_config.skip_connectivity,
  ]
}

resource "yba_universe_telemetry_config" "test" {
  universe_uuid = %q

  metrics {
    collection_level      = "NORMAL"
    scrape_config_targets = ["MASTER_EXPORT", "TSERVER_EXPORT"]

    exporter {
      exporter_uuid = yba_telemetry_provider.test.id
    }
  }
}
`, providerName, uniUUID)
}

// TestAccUniverseTelemetryConfig attaches a metrics export pipeline to a real
// universe and verifies it, then tears it down (which disables the exporter).
//
// It is gated behind YBA_ACCTEST_UNIVERSE_UUID and skips when unset: applying
// it drives a rolling restart of the named universe (minutes), so it only runs
// when an operator explicitly points it at a disposable universe rather than on
// every CI run.
func TestAccUniverseTelemetryConfig(t *testing.T) {
	uniUUID := os.Getenv("YBA_ACCTEST_UNIVERSE_UUID")
	if uniUUID == "" {
		t.Skip("YBA_ACCTEST_UNIVERSE_UUID not set; skipping universe telemetry config " +
			"acceptance test (it triggers a rolling restart of the target universe)")
	}
	name := acctest.RandomName("tp-uni")
	resourceName := "yba_universe_telemetry_config.test"

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { acctest.TestAccPreCheck(t) },
		ProviderFactories: acctest.ProviderFactories,
		// On destroy the provider is detached from the universe and deleted;
		// confirming the provider is gone transitively confirms the detach ran.
		CheckDestroy: testAccCheckTelemetryProviderDestroy,
		Steps: []resource.TestStep{
			{
				Config: universeTelemetryConfig(uniUUID, name),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "universe_uuid", uniUUID),
					resource.TestCheckResourceAttr(
						resourceName,
						"metrics.0.collection_level",
						"NORMAL",
					),
					resource.TestCheckResourceAttr(resourceName, "metrics.0.exporter.#", "1"),
				),
			},
			{
				// Import by universe UUID and confirm Read (via the v2 GET API)
				// repopulates the metrics pipeline with no drift.
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateId:     uniUUID,
				ImportStateVerify: true,
			},
		},
	})
}
