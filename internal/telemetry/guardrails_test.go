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
)

// diffErr runs the resource's full diff (which executes CustomizeDiff) against
// a raw HCL-shaped config with no prior state, returning the error
// CustomizeDiff produced (nil when the config is accepted). This is the only
// way to exercise CustomizeDiff the way Terraform core does at plan time.
func diffErr(t *testing.T, res *schema.Resource, raw map[string]interface{}) error {
	t.Helper()
	_, err := res.Diff(
		context.Background(), nil, terraform.NewResourceConfigRaw(raw), nil)
	return err
}

// TestValidateTelemetryProviderAuthPlanTime drives the conditional-auth
// CustomizeDiff through the SDK exactly as `terraform plan` would. The whole
// point of this guardrail is that missing credentials are caught BEFORE the
// (sensitive) values are written to state and shipped to YBA, so a plain
// build-path test would not prove the contract.
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

// TestValidateNoDuplicateExportersPlanTime proves that the same provider
// listed twice inside one pipeline is rejected at plan time, while sharing a
// provider ACROSS pipelines (audit + metrics) on the same universe stays
// legal — a provider whose type supports both logs and metrics is a perfectly
// valid single destination for both.
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

// nestedValidate returns the ValidateFunc registered on a field inside a
// MaxItems=1 nested block, failing the test if either the block or the field
// is missing or carries no ValidateFunc.
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

// TestOTLPAuthTypeEnum codifies the bug fix: the YBA AuthCredentials.AuthType
// enum is {BasicAuth, NoAuth, BearerToken}. The original byoc code accepted
// "BearerAuth", which YBA rejects as an invalid enum, so bearer auth never
// worked. If anyone reverts the value this test fails.
func TestOTLPAuthTypeEnum(t *testing.T) {
	vf := nestedValidate(t, ResourceTelemetryProvider(), "otlp", "auth_type")
	for _, ok := range []string{"NoAuth", "BasicAuth", "BearerToken"} {
		if _, errs := vf(ok, "auth_type"); len(errs) > 0 {
			t.Errorf("auth_type %q should be valid, got %v", ok, errs)
		}
	}
	// The old, broken value must be rejected.
	if _, errs := vf("BearerAuth", "auth_type"); len(errs) == 0 {
		t.Error("auth_type \"BearerAuth\" must be rejected (YBA enum is BearerToken)")
	}
}

// TestLokiAuthTypeEnum verifies Loki rejects BearerToken — YBA's LokiConfig
// only supports NoAuth and BasicAuth.
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

// TestS3PartitionEnum codifies the second bug fix: YBA's S3Config.partition is
// a time-bucket granularity enum {hour, minute}, NOT an AWS partition. The
// original byoc docs advertised "aws"/"aws-us-gov"/"aws-cn", every one of
// which YBA rejects.
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

// TestMetricsEnums guards the universe-config enum fields: collection_level
// must accept every MetricCollectionLevel value (including TABLE_OFF, which is
// real) and scrape_config_targets must reject an unknown target.
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
}
