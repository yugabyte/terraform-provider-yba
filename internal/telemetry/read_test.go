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
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	clientv2 "github.com/yugabyte/platform-go-client/v2"

	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// TestUniverseTelemetryConfigReadFromGetAPI drives the rewritten Read end to
// end through the v2 GetExportTelemetryConfig endpoint: the fake YBA returns a
// populated TelemetryConfig and Read must flatten it into state. This is what
// makes import work and replaces the old universeDetails spelunking.
func TestUniverseTelemetryConfigReadFromGetAPI(t *testing.T) {
	tags := map[string]string{"team": "obs"}
	f := &fakeYBA{
		getConfig: &clientv2.TelemetryConfig{
			AuditLogs: &clientv2.AuditLogsTelemetrySpec{
				YsqlAuditConfig: &clientv2.YSQLAuditConfig{
					Enabled:  true,
					Classes:  []string{"READ", "WRITE"},
					LogLevel: "WARNING",
				},
				Exporters: []clientv2.UniverseLogsExporterConfig{
					{ExporterUuid: "exp-1", AdditionalTags: &tags},
				},
			},
			Metrics: &clientv2.MetricsTelemetrySpec{
				ScrapeIntervalSeconds: utils.GetInt32Pointer(30),
				CollectionLevel:       utils.GetStringPointer("NORMAL"),
				ScrapeConfigTargets:   []clientv2.ScrapeConfigTargetType{"MASTER_EXPORT"},
				Exporters: []clientv2.UniverseMetricsExporterConfig{
					{ExporterUuid: "exp-1", MetricsPrefix: utils.GetStringPointer("yb.")},
				},
			},
		},
	}
	apiClient := newDetachTestClient(t, f)

	res := ResourceUniverseTelemetryConfig()
	d := res.TestResourceData()
	d.SetId("uni-1")

	if diags := resourceUniverseTelemetryConfigRead(
		context.Background(), d, apiClient); diags.HasError() {
		t.Fatalf("read returned diags: %v", diags)
	}

	if d.Id() != "uni-1" || d.Get("universe_uuid") != "uni-1" {
		t.Errorf("universe_uuid not set: id=%q uuid=%v", d.Id(), d.Get("universe_uuid"))
	}
	if got := d.Get("audit_logs.0.ysql_audit_config.0.log_level"); got != "WARNING" {
		t.Errorf("audit log_level = %v want WARNING", got)
	}
	classes := d.Get("audit_logs.0.ysql_audit_config.0.classes").(*schema.Set)
	if classes.Len() != 2 {
		t.Errorf("audit classes = %v want 2 elements", classes.List())
	}
	if got := d.Get("audit_logs.0.exporter.0.exporter_uuid"); got != "exp-1" {
		t.Errorf("audit exporter uuid = %v", got)
	}
	if got := d.Get("audit_logs.0.exporter.0.additional_tags.team"); got != "obs" {
		t.Errorf("audit exporter additional_tags = %v", got)
	}
	if got := d.Get("metrics.0.exporter.0.metrics_prefix"); got != "yb." {
		t.Errorf("metrics_prefix = %v", got)
	}
	// Query logs were not configured server-side, so the block must be empty.
	if n := len(d.Get("query_logs").([]interface{})); n != 0 {
		t.Errorf("query_logs must be empty when unset server-side, got %d", n)
	}
}

// TestUniverseTelemetryConfigReadEmpty verifies that an empty TelemetryConfig
// (YBA returns {} when nothing is configured) clears all blocks rather than
// erroring — this is the drift signal when exporters are disabled out-of-band.
func TestUniverseTelemetryConfigReadEmpty(t *testing.T) {
	f := &fakeYBA{getConfig: &clientv2.TelemetryConfig{}}
	apiClient := newDetachTestClient(t, f)

	res := ResourceUniverseTelemetryConfig()
	d := res.TestResourceData()
	d.SetId("uni-1")

	if diags := resourceUniverseTelemetryConfigRead(
		context.Background(), d, apiClient); diags.HasError() {
		t.Fatalf("read returned diags: %v", diags)
	}
	if d.Id() != "uni-1" {
		t.Errorf("id must be preserved on empty config, got %q", d.Id())
	}
	for _, block := range []string{"audit_logs", "query_logs", "metrics"} {
		if n := len(d.Get(block).([]interface{})); n != 0 {
			t.Errorf("%s must be empty for an empty config, got %d", block, n)
		}
	}
}

// TestUniverseTelemetryConfigReadUniverseGone verifies that a deleted universe
// (404, or YBA's non-404 "Cannot find universe" body) removes the resource
// from state so Terraform plans a recreate instead of erroring forever.
func TestUniverseTelemetryConfigReadUniverseGone(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
	}{
		{"404", http.StatusNotFound, `{"error":"not found"}`},
		{"400 cannot find universe", http.StatusBadRequest,
			`{"error":"Cannot find universe uni-1"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeYBA{getStatus: tc.status, getBody: tc.body}
			apiClient := newDetachTestClient(t, f)

			res := ResourceUniverseTelemetryConfig()
			d := res.TestResourceData()
			d.SetId("uni-1")

			diags := resourceUniverseTelemetryConfigRead(
				context.Background(), d, apiClient)
			if diags.HasError() {
				t.Fatalf("read should not error for a gone universe: %v", diags)
			}
			if d.Id() != "" {
				t.Errorf("resource must be removed from state, id=%q", d.Id())
			}
		})
	}
}
