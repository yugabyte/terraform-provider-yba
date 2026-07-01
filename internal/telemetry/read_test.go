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
	if n := len(d.Get("query_logs").([]interface{})); n != 0 {
		t.Errorf("query_logs must be empty when unset server-side, got %d", n)
	}
}

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

func TestUniverseTelemetryConfigCreate(t *testing.T) {
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

	if diags := resourceUniverseTelemetryConfigCreate(
		context.Background(), d, apiClient); diags.HasError() {
		t.Fatalf("create returned errors: %v", diags)
	}
	if d.Id() != "uni-1" {
		t.Errorf("id = %q want uni-1", d.Id())
	}
	if got := d.Get("metrics.0.exporter.0.exporter_uuid"); got != "exp-1" {
		t.Errorf("metrics exporter not populated from read-back: %v", got)
	}
}

func TestUniverseTelemetryConfigReadUniverseGone(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
	}{
		{"404", http.StatusNotFound, `{"error":"not found"}`},
		{"400 cannot find universe", http.StatusBadRequest,
			`{"error":"Cannot find universe uni-1"}`},
		{"400 does not exist", http.StatusBadRequest,
			`{"error":"universe uni-1 does not exist"}`},
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

// Replace-not-merge regression: state has exporter A, server reports B; Read must
// replace (exactly one exporter, = B), else a plan never converges.
func TestUniverseTelemetryConfigReadReplacesDrift(t *testing.T) {
	f := &fakeYBA{
		getConfig: &clientv2.TelemetryConfig{
			Metrics: &clientv2.MetricsTelemetrySpec{
				Exporters: []clientv2.UniverseMetricsExporterConfig{
					{ExporterUuid: "B"},
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
				map[string]interface{}{"exporter_uuid": "A"},
			},
		}},
	})
	d.SetId("uni-1")

	if diags := resourceUniverseTelemetryConfigRead(
		context.Background(), d, apiClient); diags.HasError() {
		t.Fatalf("read returned diags: %v", diags)
	}

	exporters := d.Get("metrics.0.exporter").([]interface{})
	if len(exporters) != 1 {
		t.Fatalf("expected exactly one metrics exporter after read (replace, "+
			"not merge), got %d: %v", len(exporters), exporters)
	}
	if got := d.Get("metrics.0.exporter.0.exporter_uuid"); got != "B" {
		t.Errorf("read must replace state with the server value: "+
			"exporter_uuid = %v want B", got)
	}
}

func TestUniverseTelemetryConfigDelete(t *testing.T) {
	t.Run("disables when a config exists", func(t *testing.T) {
		f := &fakeYBA{getConfig: &clientv2.TelemetryConfig{
			Metrics: &clientv2.MetricsTelemetrySpec{
				Exporters: []clientv2.UniverseMetricsExporterConfig{
					{ExporterUuid: "exp-1"},
				},
			},
		}}
		apiClient := newDetachTestClient(t, f)

		res := ResourceUniverseTelemetryConfig()
		d := res.TestResourceData()
		d.SetId("uni-1")

		if diags := resourceUniverseTelemetryConfigDelete(
			context.Background(), d, apiClient); diags.HasError() {
			t.Fatalf("delete returned diags: %v", diags)
		}
		if d.Id() != "" {
			t.Errorf("id must be cleared after delete, got %q", d.Id())
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		if len(f.configuredUnis) != 1 || f.configuredUnis[0] != "uni-1" {
			t.Errorf("expected one disable POST to uni-1, got %v", f.configuredUnis)
		}
	})

	t.Run("no-op when the universe has no config", func(t *testing.T) {
		f := &fakeYBA{getConfig: &clientv2.TelemetryConfig{}}
		apiClient := newDetachTestClient(t, f)

		res := ResourceUniverseTelemetryConfig()
		d := res.TestResourceData()
		d.SetId("uni-1")

		if diags := resourceUniverseTelemetryConfigDelete(
			context.Background(), d, apiClient); diags.HasError() {
			t.Fatalf("delete returned diags: %v", diags)
		}
		if d.Id() != "" {
			t.Errorf("id must be cleared, got %q", d.Id())
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		if len(f.configuredUnis) != 0 {
			t.Errorf("an already-empty universe must not be reconfigured on "+
				"delete, got %v", f.configuredUnis)
		}
	})

	t.Run("universe gone removes state without a POST", func(t *testing.T) {
		f := &fakeYBA{
			getStatus: http.StatusBadRequest,
			getBody:   `{"error":"Cannot find universe uni-1"}`,
		}
		apiClient := newDetachTestClient(t, f)

		res := ResourceUniverseTelemetryConfig()
		d := res.TestResourceData()
		d.SetId("uni-1")

		if diags := resourceUniverseTelemetryConfigDelete(
			context.Background(), d, apiClient); diags.HasError() {
			t.Fatalf("delete should not error for a gone universe: %v", diags)
		}
		if d.Id() != "" {
			t.Errorf("id must be cleared, got %q", d.Id())
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		if len(f.configuredUnis) != 0 {
			t.Errorf("a gone universe must not be reconfigured, got %v",
				f.configuredUnis)
		}
	})
}

// Missing-provider recovery: YBA's non-404 shapes (400/500 body markers) must drop
// the resource from state via errors.Is, so a YBA wording change fails here not in prod.
func TestTelemetryProviderReadMissingMarkers(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
	}{
		{"400 does not exist", http.StatusBadRequest,
			`{"error":"telemetry provider P does not exist"}`},
		{"400 invalid uuid", http.StatusBadRequest,
			`{"error":"Invalid Telemetry Provider UUID: P"}`},
		{"500 invalid uuid", http.StatusInternalServerError,
			`{"error":"Invalid Telemetry Provider UUID: P"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeYBA{getProviderStatus: tc.status, getProviderBody: tc.body}
			apiClient := newDetachTestClient(t, f)

			res := ResourceTelemetryProvider()
			d := res.TestResourceData()
			d.SetId("P")

			diags := resourceTelemetryProviderRead(
				context.Background(), d, apiClient)
			if diags.HasError() {
				t.Fatalf("read must treat a missing provider as drift, not an "+
					"error: %v", diags)
			}
			if d.Id() != "" {
				t.Errorf("missing provider must be removed from state, id=%q",
					d.Id())
			}
		})
	}
}
