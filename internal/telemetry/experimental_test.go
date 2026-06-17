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

package telemetry

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	clientv2 "github.com/yugabyte/platform-go-client/v2"
)

// TestExperimentalMarking verifies both telemetry resources carry the
// experimental docs admonition and that the apply-time warning is a non-fatal
// Warning (not an Error that would block applies).
func TestExperimentalMarking(t *testing.T) {
	for name, res := range map[string]*schema.Resource{
		"yba_telemetry_provider":        ResourceTelemetryProvider(),
		"yba_universe_telemetry_config": ResourceUniverseTelemetryConfig(),
	} {
		if !strings.Contains(res.Description, "**Experimental:**") {
			t.Errorf("%s Description must carry the Experimental admonition", name)
		}
	}
	w := experimentalWarning("x")
	if w.Severity != diag.Warning {
		t.Errorf("experimental notice must be a Warning, got severity %v", w.Severity)
	}
}

// TestUniverseTelemetryConfigCreateEmitsWarning drives Create end to end
// against the fake YBA and asserts it succeeds AND returns the experimental
// Warning (not just on Read). This also exercises the create -> dispatch ->
// wait-for-task -> read happy path.
func TestUniverseTelemetryConfigCreateEmitsWarning(t *testing.T) {
	f := &fakeYBA{
		getConfig: &clientv2.TelemetryConfig{
			Metrics: &clientv2.MetricsTelemetrySpec{
				Exporters: []clientv2.UniverseMetricsExporterConfig{
					{ExporterUuid: "exp-1"},
				},
			},
		},
	}
	apiClient := newDetachTestClient(t, f)

	res := ResourceUniverseTelemetryConfig()
	d := schema.TestResourceDataRaw(t, res.Schema, map[string]interface{}{
		"universe_uuid": "uni-1",
		"metrics": []interface{}{map[string]interface{}{
			"exporter": []interface{}{
				map[string]interface{}{"exporter_uuid": "exp-1"},
			},
		}},
	})

	diags := resourceUniverseTelemetryConfigCreate(context.Background(), d, apiClient)
	if diags.HasError() {
		t.Fatalf("create returned errors: %v", diags)
	}
	if d.Id() != "uni-1" {
		t.Errorf("id = %q want uni-1", d.Id())
	}
	var warned bool
	for _, dg := range diags {
		if dg.Severity == diag.Warning && strings.Contains(dg.Summary, "experimental") {
			warned = true
		}
	}
	if !warned {
		t.Errorf("create must surface the experimental Warning, got %+v", diags)
	}
}
