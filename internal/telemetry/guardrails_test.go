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

// Missing credentials must be caught at plan time, before sensitive values reach state.
func TestValidateTelemetryProviderAuthPlanTime(t *testing.T) {
	res := ResourceTelemetryProvider()
	cases := []struct {
		name    string
		raw     map[string]interface{}
		wantErr string // substring; "" means the config must be accepted
	}{
		{
			name: "otlp NoAuth needs no credentials",
			raw: map[string]interface{}{
				"name": "x",
				"otlp": []interface{}{map[string]interface{}{
					"endpoint":  "https://collector",
					"auth_type": "NoAuth",
				}},
			},
		},
		{
			name: "otlp BasicAuth missing username",
			raw: map[string]interface{}{
				"name": "x",
				"otlp": []interface{}{map[string]interface{}{
					"endpoint":            "https://collector",
					"auth_type":           "BasicAuth",
					"basic_auth_password": "pw",
				}},
			},
			wantErr: "basic_auth_username",
		},
		{
			name: "otlp BasicAuth missing password",
			raw: map[string]interface{}{
				"name": "x",
				"otlp": []interface{}{map[string]interface{}{
					"endpoint":            "https://collector",
					"auth_type":           "BasicAuth",
					"basic_auth_username": "u",
				}},
			},
			wantErr: "basic_auth_username and basic_auth_password",
		},
		{
			name: "otlp BasicAuth with both is accepted",
			raw: map[string]interface{}{
				"name": "x",
				"otlp": []interface{}{map[string]interface{}{
					"endpoint":            "https://collector",
					"auth_type":           "BasicAuth",
					"basic_auth_username": "u",
					"basic_auth_password": "pw",
				}},
			},
		},
		{
			name: "otlp BearerToken missing token",
			raw: map[string]interface{}{
				"name": "x",
				"otlp": []interface{}{map[string]interface{}{
					"endpoint":  "https://collector",
					"auth_type": "BearerToken",
				}},
			},
			wantErr: "bearer_token is required",
		},
		{
			name: "otlp BearerToken with token is accepted",
			raw: map[string]interface{}{
				"name": "x",
				"otlp": []interface{}{map[string]interface{}{
					"endpoint":     "https://collector",
					"auth_type":    "BearerToken",
					"bearer_token": "tok",
				}},
			},
		},
		{
			name: "loki BasicAuth missing credentials",
			raw: map[string]interface{}{
				"name": "x",
				"loki": []interface{}{map[string]interface{}{
					"endpoint":  "https://loki",
					"auth_type": "BasicAuth",
				}},
			},
			wantErr: "basic_auth_username and basic_auth_password",
		},
		{
			name: "loki NoAuth is accepted",
			raw: map[string]interface{}{
				"name": "x",
				"loki": []interface{}{map[string]interface{}{
					"endpoint":  "https://loki",
					"auth_type": "NoAuth",
				}},
			},
		},
		{
			name: "otlp logs_endpoint under gRPC rejected",
			raw: map[string]interface{}{
				"name": "x",
				"otlp": []interface{}{map[string]interface{}{
					"endpoint":      "https://collector",
					"logs_endpoint": "https://collector/logs",
					// protocol defaults to gRPC
				}},
			},
			wantErr: "logs_endpoint is only honoured when protocol = \"HTTP\"",
		},
		{
			name: "otlp metrics_endpoint under gRPC rejected",
			raw: map[string]interface{}{
				"name": "x",
				"otlp": []interface{}{map[string]interface{}{
					"endpoint":         "https://collector",
					"protocol":         "gRPC",
					"metrics_endpoint": "https://collector/metrics",
				}},
			},
			wantErr: "metrics_endpoint is only honoured when protocol = \"HTTP\"",
		},
		{
			name: "otlp endpoint overrides under HTTP accepted",
			raw: map[string]interface{}{
				"name": "x",
				"otlp": []interface{}{map[string]interface{}{
					"endpoint":         "https://collector",
					"protocol":         "HTTP",
					"logs_endpoint":    "https://collector/logs",
					"metrics_endpoint": "https://collector/metrics",
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

func nestedValidate(
	t *testing.T, res *schema.Resource, block, field string,
) schema.SchemaValidateFunc { //nolint:staticcheck // SDKv2 ValidateFunc type
	t.Helper()
	b, ok := res.Schema[block]
	if !ok {
		t.Fatalf("schema missing block %q", block)
	}
	elem, ok := b.Elem.(*schema.Resource)
	if !ok {
		t.Fatalf("block %q has no nested resource", block)
	}
	f, ok := elem.Schema[field]
	if !ok {
		t.Fatalf("block %q missing field %q", block, field)
	}
	if f.ValidateFunc == nil {
		t.Fatalf("block %q field %q has no ValidateFunc", block, field)
	}
	return f.ValidateFunc
}

// OTLP auth_type accepts BearerToken; the similar-looking "BearerAuth" is rejected.
func TestOTLPAuthTypeEnum(t *testing.T) {
	vf := nestedValidate(t, ResourceTelemetryProvider(), "otlp", "auth_type")
	for _, ok := range []string{"NoAuth", "BasicAuth", "BearerToken"} {
		if _, errs := vf(ok, "auth_type"); len(errs) > 0 {
			t.Errorf("auth_type %q should be valid, got %v", ok, errs)
		}
	}
	if _, errs := vf("BearerAuth", "auth_type"); len(errs) == 0 {
		t.Error("auth_type \"BearerAuth\" must be rejected (YBA enum is BearerToken)")
	}
}

// Loki supports only NoAuth and BasicAuth — BearerToken must be rejected.
func TestLokiAuthTypeEnum(t *testing.T) {
	vf := nestedValidate(t, ResourceTelemetryProvider(), "loki", "auth_type")
	for _, ok := range []string{"NoAuth", "BasicAuth"} {
		if _, errs := vf(ok, "auth_type"); len(errs) > 0 {
			t.Errorf("loki auth_type %q should be valid, got %v", ok, errs)
		}
	}
	if _, errs := vf("BearerToken", "auth_type"); len(errs) == 0 {
		t.Error("loki auth_type \"BearerToken\" must be rejected (Loki has no bearer auth)")
	}
}

// S3 partition is a time bucket (hour/minute), not an AWS partition
// ("aws"/"aws-us-gov"/"aws-cn" are rejected).
func TestS3PartitionEnum(t *testing.T) {
	vf := nestedValidate(t, ResourceTelemetryProvider(), "s3", "partition")
	for _, ok := range []string{"hour", "minute"} {
		if _, errs := vf(ok, "partition"); len(errs) > 0 {
			t.Errorf("partition %q should be valid, got %v", ok, errs)
		}
	}
	for _, bad := range []string{"aws", "aws-us-gov", "aws-cn", "day", ""} {
		if _, errs := vf(bad, "partition"); len(errs) == 0 {
			t.Errorf("partition %q must be rejected", bad)
		}
	}
}

func TestOTLPTimeoutPositive(t *testing.T) {
	vf := nestedValidate(t, ResourceTelemetryProvider(), "otlp", "timeout_seconds")
	for _, bad := range []int{0, -1} {
		if _, errs := vf(bad, "timeout_seconds"); len(errs) == 0 {
			t.Errorf("timeout_seconds %d must be rejected", bad)
		}
	}
	if _, errs := vf(5, "timeout_seconds"); len(errs) > 0 {
		t.Errorf("timeout_seconds 5 should be valid, got %v", errs)
	}
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
