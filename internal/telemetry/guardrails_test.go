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

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"

	"github.com/yugabyte/terraform-provider-yba/internal/api"
)

// diffErr runs the full diff (executing CustomizeDiff) the way plan does,
// returning CustomizeDiff's error (nil when accepted).
func diffErr(t *testing.T, res *schema.Resource, raw map[string]interface{}) error {
	t.Helper()
	return diffErrMeta(t, res, raw, nil)
}

// diffErrMeta is diffErr with explicit provider meta, for the cross-resource checks.
func diffErrMeta(
	t *testing.T, res *schema.Resource, raw map[string]interface{}, meta interface{},
) error {
	t.Helper()
	_, err := res.Diff(
		context.Background(), nil, terraform.NewResourceConfigRaw(raw), meta)
	return err
}

// Same provider twice in one pipeline is rejected; shared across pipelines stays legal.
func TestValidateNoDuplicateExportersPlanTime(t *testing.T) {
	res := ResourceUniverseTelemetryConfig()
	exp := func(uuids ...string) []interface{} {
		out := make([]interface{}, 0, len(uuids))
		for _, u := range uuids {
			out = append(out, map[string]interface{}{"exporter_uuid": u})
		}
		return out
	}
	cases := []struct {
		name    string
		raw     map[string]interface{}
		wantErr string
	}{
		{
			name: "audit duplicate rejected",
			raw: map[string]interface{}{
				"universe_uuid": "u",
				"audit_logs": []interface{}{map[string]interface{}{
					"exporter": exp("a", "a"),
				}},
			},
			wantErr: "audit_logs: exporter_uuid \"a\" is listed more than once",
		},
		{
			name: "metrics duplicate rejected",
			raw: map[string]interface{}{
				"universe_uuid": "u",
				"metrics": []interface{}{map[string]interface{}{
					"exporter": exp("m", "m"),
				}},
			},
			wantErr: "metrics: exporter_uuid \"m\" is listed more than once",
		},
		{
			name: "query duplicate rejected",
			raw: map[string]interface{}{
				"universe_uuid": "u",
				"query_logs": []interface{}{map[string]interface{}{
					"exporter": exp("q", "q"),
				}},
			},
			wantErr: "query_logs: exporter_uuid \"q\" is listed more than once",
		},
		{
			name: "distinct exporters in one pipeline accepted",
			raw: map[string]interface{}{
				"universe_uuid": "u",
				"audit_logs": []interface{}{map[string]interface{}{
					"exporter": exp("a", "b", "c"),
				}},
			},
		},
		{
			name: "same provider shared across pipelines accepted",
			raw: map[string]interface{}{
				"universe_uuid": "u",
				"audit_logs": []interface{}{map[string]interface{}{
					"exporter": exp("shared"),
				}},
				"metrics": []interface{}{map[string]interface{}{
					"exporter": exp("shared"),
				}},
			},
		},
		{
			name: "empty exporter_uuid rejected",
			raw: map[string]interface{}{
				"universe_uuid": "u",
				"audit_logs": []interface{}{map[string]interface{}{
					"exporter": exp(""),
				}},
			},
			wantErr: "audit_logs: exporter #1 has an empty exporter_uuid",
		},
		{
			name: "master_logs duplicate rejected",
			raw: map[string]interface{}{
				"universe_uuid": "u",
				"master_logs": []interface{}{map[string]interface{}{
					"exporter": exp("s", "s"),
				}},
			},
			wantErr: "master_logs: exporter_uuid \"s\" is listed more than once",
		},
		{
			name: "same provider shared across server-log pipelines accepted",
			raw: map[string]interface{}{
				"universe_uuid": "u",
				"master_logs": []interface{}{map[string]interface{}{
					"exporter": exp("shared"),
				}},
				"tserver_logs": []interface{}{map[string]interface{}{
					"exporter": exp("shared"),
				}},
				"controller_logs": []interface{}{map[string]interface{}{
					"exporter": exp("shared"),
				}},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := diffErr(t, res, tc.raw)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("expected config to be accepted, got: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

// Two resources claiming one universe are rejected; re-planning the same one is not.
func TestSingleManagerPerUniversePlanTime(t *testing.T) {
	res := ResourceUniverseTelemetryConfig()
	audit := func(uuid string) map[string]interface{} {
		return map[string]interface{}{
			"universe_uuid": "uni-1",
			"audit_logs": []interface{}{map[string]interface{}{
				"exporter": []interface{}{
					map[string]interface{}{"exporter_uuid": uuid},
				},
			}},
		}
	}
	metrics := func(uuid string) map[string]interface{} {
		return map[string]interface{}{
			"universe_uuid": "uni-1",
			"metrics": []interface{}{map[string]interface{}{
				"exporter": []interface{}{
					map[string]interface{}{"exporter_uuid": uuid},
				},
			}},
		}
	}

	t.Run("two resources for the same universe rejected", func(t *testing.T) {
		meta := &api.APIClient{} // shared per-run registry key
		if err := diffErrMeta(t, res, audit("a"), meta); err != nil {
			t.Fatalf("first resource should be accepted, got: %v", err)
		}
		// Second, differently-configured resource for the same universe: the foot-gun.
		err := diffErrMeta(t, res, metrics("m"), meta)
		if err == nil {
			t.Fatal("expected the second resource for uni-1 to be rejected")
		}
		if !strings.Contains(err.Error(), "already managed by another") {
			t.Errorf("error %q does not explain the duplicate", err.Error())
		}
	})

	t.Run("re-planning the identical resource is accepted", func(t *testing.T) {
		meta := &api.APIClient{}
		if err := diffErrMeta(t, res, audit("a"), meta); err != nil {
			t.Fatalf("first plan should be accepted, got: %v", err)
		}
		if err := diffErrMeta(t, res, audit("a"), meta); err != nil {
			t.Fatalf("re-planning the same resource must not be flagged, got: %v", err)
		}
	})

	t.Run("different universes accepted", func(t *testing.T) {
		meta := &api.APIClient{}
		if err := diffErrMeta(t, res, audit("a"), meta); err != nil {
			t.Fatalf("uni-1 should be accepted, got: %v", err)
		}
		other := metrics("m")
		other["universe_uuid"] = "uni-2"
		if err := diffErrMeta(t, res, other, meta); err != nil {
			t.Fatalf("a different universe must be accepted, got: %v", err)
		}
	})

	t.Run("no meta skips the cross-resource check", func(t *testing.T) {
		// Unit tests that exercise other rules pass nil meta; the registry must
		// not panic or falsely reject there.
		if err := diffErrMeta(t, res, audit("a"), nil); err != nil {
			t.Fatalf("nil meta must skip the claim check, got: %v", err)
		}
		if err := diffErrMeta(t, res, audit("a"), nil); err != nil {
			t.Fatalf("nil meta must skip the claim check, got: %v", err)
		}
	})
}

func TestMetricsEnums(t *testing.T) {
	res := ResourceUniverseTelemetryConfig()
	metricsElem := res.Schema["metrics"].Elem.(*schema.Resource)

	cl := metricsElem.Schema["collection_level"].ValidateFunc
	if cl == nil {
		t.Fatal("collection_level must have a ValidateFunc")
	}
	for _, ok := range []string{"ALL", "NORMAL", "MINIMAL", "TABLE_OFF", "OFF"} {
		if _, errs := cl(ok, "collection_level"); len(errs) > 0 {
			t.Errorf("collection_level %q should be valid, got %v", ok, errs)
		}
	}
	if _, errs := cl("EVERYTHING", "collection_level"); len(errs) == 0 {
		t.Error("collection_level \"EVERYTHING\" must be rejected")
	}

	target := metricsElem.Schema["scrape_config_targets"].Elem.(*schema.Schema).ValidateFunc
	if target == nil {
		t.Fatal("scrape_config_targets elem must have a ValidateFunc")
	}
	if _, errs := target("MASTER_EXPORT", "scrape_config_targets"); len(errs) > 0 {
		t.Errorf("MASTER_EXPORT should be valid, got %v", errs)
	}
	if _, errs := target("BOGUS_EXPORT", "scrape_config_targets"); len(errs) == 0 {
		t.Error("scrape_config_targets \"BOGUS_EXPORT\" must be rejected")
	}

	// Computed: YBA fills an empty set with all targets, so unset must absorb it, not diff.
	if !metricsElem.Schema["scrape_config_targets"].Computed {
		t.Error("scrape_config_targets must be Computed so the YBA-filled " +
			"\"all targets\" default does not perpetually diff")
	}
}

func TestAuditAndQueryLogEnums(t *testing.T) {
	res := ResourceUniverseTelemetryConfig()
	auditElem := res.Schema["audit_logs"].Elem.(*schema.Resource)
	ysqlAudit := auditElem.Schema["ysql_audit_config"].Elem.(*schema.Resource)
	ycqlAudit := auditElem.Schema["ycql_audit_config"].Elem.(*schema.Resource)
	ysqlQuery := res.Schema["query_logs"].Elem.(*schema.Resource).
		Schema["ysql_query_log_config"].Elem.(*schema.Resource)

	cases := []struct {
		name string
		// SchemaValidateFunc is the (deprecated) type this provider's ValidateFuncs return.
		vf   schema.SchemaValidateFunc //nolint:staticcheck // SA1019: matches the schema's ValidateFunc type
		good string
		bad  string
	}{
		{"ysql audit log_level", ysqlAudit.Schema["log_level"].ValidateFunc, "LOG", "TRACE"},
		{
			"ysql audit classes",
			ysqlAudit.Schema["classes"].Elem.(*schema.Schema).ValidateFunc,
			"DDL",
			"EVERYTHING",
		},
		{"ycql audit log_level", ycqlAudit.Schema["log_level"].ValidateFunc, "ERROR", "LOG"},
		{
			"ycql included_categories",
			ycqlAudit.Schema["included_categories"].Elem.(*schema.Schema).ValidateFunc,
			"DML",
			"NONSENSE",
		},
		{"query log_statement", ysqlQuery.Schema["log_statement"].ValidateFunc, "DDL", "SOME"},
		{
			"query log_error_verbosity",
			ysqlQuery.Schema["log_error_verbosity"].ValidateFunc,
			"TERSE",
			"LOUD",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.vf == nil {
				t.Fatalf("%s must have a ValidateFunc", tc.name)
			}
			if _, errs := tc.vf(tc.good, tc.name); len(errs) > 0 {
				t.Errorf("%s %q should be valid, got %v", tc.name, tc.good, errs)
			}
			if _, errs := tc.vf(tc.bad, tc.name); len(errs) == 0 {
				t.Errorf("%s %q must be rejected", tc.name, tc.bad)
			}
		})
	}
}

// Every telemetryPipelines entry must exist in the schema, and vice versa every
// pipeline block must be registered there — the guardrails and the claim
// fingerprint only see pipelines listed in that slice.
func TestTelemetryPipelinesMatchSchema(t *testing.T) {
	res := ResourceUniverseTelemetryConfig()
	registered := map[string]bool{}
	for _, p := range telemetryPipelines {
		registered[p.label] = true
		s, ok := res.Schema[p.label]
		if !ok {
			t.Errorf("telemetryPipelines entry %q has no schema block", p.label)
			continue
		}
		elem, ok := s.Elem.(*schema.Resource)
		if !ok || elem.Schema["exporter"] == nil {
			t.Errorf("pipeline %q must carry an exporter block", p.label)
		}
	}
	for name, s := range res.Schema {
		if name == "universe_uuid" || name == "upgrade_options" {
			continue
		}
		if s.MaxItems != 1 {
			t.Errorf("pipeline %q must set MaxItems=1", name)
		}
		if !registered[name] {
			t.Errorf("schema block %q is missing from telemetryPipelines — the "+
				"duplicate-exporter guardrail and claim fingerprint skip it", name)
		}
	}
}

func TestServerLogsEnumsAndDefaults(t *testing.T) {
	res := ResourceUniverseTelemetryConfig()
	masterElem := res.Schema["master_logs"].Elem.(*schema.Resource)
	tserverElem := res.Schema["tserver_logs"].Elem.(*schema.Resource)

	for name, s := range map[string]*schema.Schema{
		"master min_level":  masterElem.Schema["min_level"],
		"tserver min_level": tserverElem.Schema["min_level"],
	} {
		if s.ValidateFunc == nil {
			t.Fatalf("%s must have a ValidateFunc", name)
		}
		for _, ok := range []string{"INFO", "WARNING", "ERROR", "FATAL"} {
			if _, errs := s.ValidateFunc(ok, name); len(errs) > 0 {
				t.Errorf("%s %q should be valid, got %v", name, ok, errs)
			}
		}
		if _, errs := s.ValidateFunc("DEBUG", name); len(errs) == 0 {
			t.Errorf("%s \"DEBUG\" must be rejected", name)
		}
	}

	// Defaults track the YBA API via the generated client: master exports INFO+,
	// tserver defaults to WARNING+ (INFO is very high volume).
	if got := masterElem.Schema["min_level"].Default; got != "INFO" {
		t.Errorf("master_logs min_level default = %v want INFO", got)
	}
	if got := tserverElem.Schema["min_level"].Default; got != "WARNING" {
		t.Errorf("tserver_logs min_level default = %v want WARNING", got)
	}

	noise := masterElem.Schema["noise_sample_drop_ratio"]
	if noise == nil || noise.ValidateFunc == nil {
		t.Fatal("master_logs noise_sample_drop_ratio must exist with a ValidateFunc")
	}
	if got := noise.Default; got != 0.99 {
		t.Errorf("noise_sample_drop_ratio default = %v want 0.99", got)
	}
	if _, errs := noise.ValidateFunc(1.5, "noise_sample_drop_ratio"); len(errs) == 0 {
		t.Error("noise_sample_drop_ratio 1.5 must be rejected")
	}
	if _, errs := noise.ValidateFunc(0.0, "noise_sample_drop_ratio"); len(errs) > 0 {
		t.Errorf("noise_sample_drop_ratio 0.0 must be accepted, got %v", errs)
	}
	if tserverElem.Schema["noise_sample_drop_ratio"] != nil {
		t.Error("noise_sample_drop_ratio is master-only; tserver_logs must not have it")
	}

	// Server-log exporters carry the batching fields with client-sourced defaults.
	exporterElem := masterElem.Schema["exporter"].Elem.(*schema.Resource)
	for field, want := range map[string]interface{}{
		"send_batch_max_size":                 1000,
		"send_batch_size":                     100,
		"send_batch_timeout_seconds":          10,
		"memory_limit_mib":                    2048,
		"memory_limit_check_interval_seconds": 10,
	} {
		s, ok := exporterElem.Schema[field]
		if !ok {
			t.Errorf("server-log exporter must carry %q", field)
			continue
		}
		if s.Default != want {
			t.Errorf("server-log exporter %s default = %v want %v", field, s.Default, want)
		}
	}
}
