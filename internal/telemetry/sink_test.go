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
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// createProvider runs the full CreateContext of a per-sink resource against the
// fake YBA and returns the decoded create request body.
func createProvider(
	t *testing.T, f *fakeYBA, res *schema.Resource, raw map[string]interface{},
) (d *schema.ResourceData, name string, cfg map[string]interface{}, tags map[string]string) {
	t.Helper()
	apiClient := newDetachTestClient(t, f)
	d = schema.TestResourceDataRaw(t, res.Schema, raw)
	if diags := res.CreateContext(context.Background(), d, apiClient); diags.HasError() {
		t.Fatalf("create returned errors: %v", diags)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.createdProviders) != 1 {
		t.Fatalf("expected exactly 1 create POST, got %d", len(f.createdProviders))
	}
	var req struct {
		Name   string                 `json:"name"`
		Config map[string]interface{} `json:"config"`
		Tags   map[string]string      `json:"tags"`
	}
	if err := json.Unmarshal(f.createdProviders[0], &req); err != nil {
		t.Fatalf("unmarshal create body: %v\n%s", err, f.createdProviders[0])
	}
	return d, req.Name, req.Config, req.Tags
}

var sinkConstructors = map[string]func() *schema.Resource{
	"yba_datadog_telemetry_provider":              ResourceDatadogTelemetryProvider,
	"yba_otlp_telemetry_provider":                 ResourceOTLPTelemetryProvider,
	"yba_aws_cloudwatch_telemetry_provider":       ResourceAWSCloudWatchTelemetryProvider,
	"yba_gcp_cloud_monitoring_telemetry_provider": ResourceGCPCloudMonitoringTelemetryProvider,
	"yba_splunk_telemetry_provider":               ResourceSplunkTelemetryProvider,
	"yba_dynatrace_telemetry_provider":            ResourceDynatraceTelemetryProvider,
	"yba_s3_telemetry_provider":                   ResourceS3TelemetryProvider,
}

// sinkSensitiveFields lists the credential fields per sink that must be marked
// Sensitive (their values land in state).
var sinkSensitiveFields = map[string][]string{
	"yba_datadog_telemetry_provider":              {"api_key"},
	"yba_otlp_telemetry_provider":                 {"basic_auth_password", "bearer_token"},
	"yba_aws_cloudwatch_telemetry_provider":       {"access_key", "secret_key"},
	"yba_gcp_cloud_monitoring_telemetry_provider": {"credentials_json"},
	"yba_splunk_telemetry_provider":               {"token"},
	"yba_dynatrace_telemetry_provider":            {"api_token"},
	"yba_s3_telemetry_provider":                   {"access_key", "secret_key"},
}

// Shared guardrails for every sink resource: everything is ForceNew (YBA has
// no PUT endpoint), credentials are Sensitive, no noise/computed type field,
// import is wired, and destroy waits long enough for per-universe detach tasks.
func TestSinkResourceGuardrails(t *testing.T) {
	for name, mk := range sinkConstructors {
		t.Run(name, func(t *testing.T) {
			res := mk()

			for _, banned := range []string{"customer_uuid", "type"} {
				if _, present := res.Schema[banned]; present {
					t.Errorf("%s must not be exposed on %s (customer_uuid "+
						"duplicates apiClient.CustomerID; type is implied by "+
						"the resource)", banned, name)
				}
			}

			nameField, ok := res.Schema["name"]
			if !ok || !nameField.Required || !nameField.ForceNew {
				t.Errorf("name must be Required+ForceNew, got %+v", nameField)
			}

			for field, s := range res.Schema {
				if !s.ForceNew {
					t.Errorf("field %q must be ForceNew: YBA has no PUT "+
						"endpoint for telemetry providers", field)
				}
			}

			for _, field := range sinkSensitiveFields[name] {
				s, ok := res.Schema[field]
				if !ok {
					t.Errorf("expected credential field %q is missing", field)
					continue
				}
				if !s.Sensitive {
					t.Errorf("credential field %q must be Sensitive", field)
				}
			}

			if res.Importer == nil {
				t.Error("Importer must be set so existing providers can be imported")
			}
			if res.Timeouts == nil || res.Timeouts.Delete == nil {
				t.Fatal("Delete timeout must be set so destroy can wait for " +
					"per-universe rolling-upgrade detach tasks")
			}
			if *res.Timeouts.Delete != telemetryUpgradeTimeout {
				t.Errorf("Delete timeout = %s want %s",
					*res.Timeouts.Delete, telemetryUpgradeTimeout)
			}
		})
	}
}

// Import guardrail: reading a provider whose YBA type is not this resource's
// sink must fail loudly (wrong-resource import), not adopt the provider.
func TestSinkReadRejectsTypeMismatch(t *testing.T) {
	f := &fakeYBA{
		getProviderStatus: 200,
		getProviderBody:   `{"uuid":"P","name":"sp","config":{"type":"SPLUNK"}}`,
	}
	apiClient := newDetachTestClient(t, f)

	res := ResourceDatadogTelemetryProvider()
	d := res.TestResourceData()
	d.SetId("P")

	diags := res.ReadContext(context.Background(), d, apiClient)
	if !diags.HasError() {
		t.Fatal("read must reject a SPLUNK provider on the Datadog resource")
	}
	msg := diags[0].Summary
	if !strings.Contains(msg, "SPLUNK") || !strings.Contains(msg, "DATA_DOG") {
		t.Errorf("error %q must name both the actual and expected type", msg)
	}
	if d.Id() == "" {
		t.Error("a type mismatch must not clear the id (the provider exists)")
	}
}

// Out-of-band deletes: YBA's 404 and non-404 "missing provider" shapes must
// drop the resource from state, not error.
func TestSinkReadMissingRemovesFromState(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
	}{
		{"404", http.StatusNotFound, `{"error":"not found"}`},
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

			res := ResourceDatadogTelemetryProvider()
			d := res.TestResourceData()
			d.SetId("P")

			diags := res.ReadContext(context.Background(), d, apiClient)
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

// Tracer bullet for the per-sink factory: the Datadog resource creates a
// provider whose config carries the DATA_DOG discriminator and camelCase keys,
// and adopts the UUID the API returns.
func TestDatadogCreateSendsTypedConfig(t *testing.T) {
	f := &fakeYBA{}
	d, name, cfg, tags := createProvider(t, f, ResourceDatadogTelemetryProvider(),
		map[string]interface{}{
			"name":    "dd",
			"tags":    map[string]interface{}{"env": "prod"},
			"site":    "datadoghq.eu",
			"api_key": "dd-key",
		})

	if d.Id() != "P" {
		t.Errorf("id = %q, want the uuid returned by the create API (P)", d.Id())
	}
	if name != "dd" {
		t.Errorf("request name = %q want dd", name)
	}
	assertConfig(t, cfg, map[string]interface{}{
		"type":   "DATA_DOG",
		"site":   "datadoghq.eu",
		"apiKey": "dd-key",
	})
	if tags["env"] != "prod" {
		t.Errorf("request tags = %v, want env=prod", tags)
	}
}

func TestSplunkCreateSendsTypedConfig(t *testing.T) {
	_, _, cfg, _ := createProvider(t, &fakeYBA{}, ResourceSplunkTelemetryProvider(),
		map[string]interface{}{
			"name":        "sp",
			"endpoint":    "https://hec:8088",
			"token":       "hec-token",
			"source":      "yba",
			"source_type": "_json",
			"index":       "main",
		})
	assertConfig(t, cfg, map[string]interface{}{
		"type":       "SPLUNK",
		"endpoint":   "https://hec:8088",
		"token":      "hec-token",
		"source":     "yba",
		"sourceType": "_json",
		"index":      "main",
	})
}

// Optional Splunk fields left unset must be omitted from the payload, not sent
// as empty strings (YBA reads a missing key as "use default").
func TestSplunkCreateOmitsEmptyOptionals(t *testing.T) {
	_, _, cfg, _ := createProvider(t, &fakeYBA{}, ResourceSplunkTelemetryProvider(),
		map[string]interface{}{
			"name":     "sp",
			"endpoint": "https://hec:8088",
			"token":    "t",
		})
	assertConfig(t, cfg, map[string]interface{}{"type": "SPLUNK"},
		"source", "sourceType", "index")
}

func TestDynatraceCreateSendsTypedConfig(t *testing.T) {
	_, _, cfg, _ := createProvider(t, &fakeYBA{}, ResourceDynatraceTelemetryProvider(),
		map[string]interface{}{
			"name":      "dt",
			"endpoint":  "https://env.live.dynatrace.com/api/v2/otlp",
			"api_token": "dt-token",
		})
	assertConfig(t, cfg, map[string]interface{}{
		"type":     "DYNATRACE",
		"endpoint": "https://env.live.dynatrace.com/api/v2/otlp",
		"apiToken": "dt-token",
	})
}

// Pins the all-caps roleARN key (distinct from S3's roleArn): a mistyped
// camelCase key silently drops the field server-side.
func TestAWSCloudWatchCreateSendsTypedConfig(t *testing.T) {
	_, _, cfg, _ := createProvider(t, &fakeYBA{},
		ResourceAWSCloudWatchTelemetryProvider(),
		map[string]interface{}{
			"name":       "cw",
			"log_group":  "yba/audit",
			"log_stream": "primary",
			"region":     "us-west-2",
			"access_key": "AKIA",
			"secret_key": "secret",
			"role_arn":   "arn:aws:iam::1:role/cw",
			"endpoint":   "https://logs.vpce",
		})
	assertConfig(t, cfg, map[string]interface{}{
		"type":      "AWS_CLOUDWATCH",
		"logGroup":  "yba/audit",
		"logStream": "primary",
		"region":    "us-west-2",
		"accessKey": "AKIA",
		"secretKey": "secret",
		"roleARN":   "arn:aws:iam::1:role/cw",
		"endpoint":  "https://logs.vpce",
	})
}

func TestAWSCloudWatchCreateOmitsEmptyOptionals(t *testing.T) {
	_, _, cfg, _ := createProvider(t, &fakeYBA{},
		ResourceAWSCloudWatchTelemetryProvider(),
		map[string]interface{}{
			"name":       "cw",
			"log_group":  "g",
			"log_stream": "s",
			"region":     "r",
			"access_key": "a",
			"secret_key": "s",
		})
	assertConfig(t, cfg, map[string]interface{}{"type": "AWS_CLOUDWATCH"},
		"roleARN", "endpoint")
}

// Pins the credentialsString key (YBA ≥ 2026.1.0; the legacy credentials
// JsonNode field is deprecated) and that an empty project is omitted so YBA
// derives it from the service-account JSON.
func TestGCPCloudMonitoringCreateSendsTypedConfig(t *testing.T) {
	_, _, cfg, _ := createProvider(t, &fakeYBA{},
		ResourceGCPCloudMonitoringTelemetryProvider(),
		map[string]interface{}{
			"name":             "gcm",
			"project":          "my-proj",
			"credentials_json": `{"type":"service_account"}`,
		})
	assertConfig(t, cfg, map[string]interface{}{
		"type":              "GCP_CLOUD_MONITORING",
		"project":           "my-proj",
		"credentialsString": `{"type":"service_account"}`,
	})

	_, _, noProj, _ := createProvider(t, &fakeYBA{},
		ResourceGCPCloudMonitoringTelemetryProvider(),
		map[string]interface{}{
			"name":             "gcm",
			"credentials_json": `{"project_id":"x"}`,
		})
	assertConfig(t, noProj,
		map[string]interface{}{"credentialsString": `{"project_id":"x"}`},
		"project")
}

// Pins every S3 field to the camelCase key YBA expects (catches drift).
func TestS3CreateSendsTypedConfig(t *testing.T) {
	_, _, cfg, _ := createProvider(t, &fakeYBA{}, ResourceS3TelemetryProvider(),
		map[string]interface{}{
			"name":                                "my-s3",
			"bucket":                              "logs-bucket",
			"region":                              "us-east-1",
			"access_key":                          "AKIA...",
			"secret_key":                          "secret",
			"directory_prefix":                    "yba/audit",
			"file_prefix":                         "uni-",
			"endpoint":                            "https://s3.us-east-1.amazonaws.com",
			"role_arn":                            "arn:aws:iam::1:role/x",
			"partition":                           "minute",
			"marshaler":                           "json",
			"disable_ssl":                         true,
			"force_path_style":                    true,
			"include_universe_and_node_in_prefix": true,
		})
	assertConfig(t, cfg, map[string]interface{}{
		"type":                           "S3",
		"bucket":                         "logs-bucket",
		"region":                         "us-east-1",
		"accessKey":                      "AKIA...",
		"secretKey":                      "secret",
		"directoryPrefix":                "yba/audit",
		"filePrefix":                     "uni-",
		"endpoint":                       "https://s3.us-east-1.amazonaws.com",
		"roleArn":                        "arn:aws:iam::1:role/x",
		"partition":                      "minute",
		"marshaler":                      "json",
		"disableSSL":                     true,
		"forcePathStyle":                 true,
		"includeUniverseAndNodeInPrefix": true,
	})
}

// false must be omitted, not sent: YBA reads a missing key as "use default",
// whereas an explicit false pins the field across YBA-default changes.
func TestS3CreateOmitsFalseAndEmpty(t *testing.T) {
	_, _, cfg, _ := createProvider(t, &fakeYBA{}, ResourceS3TelemetryProvider(),
		map[string]interface{}{
			"name":       "minimal-s3",
			"bucket":     "b",
			"region":     "us-east-1",
			"access_key": "A",
			"secret_key": "S",
		})
	assertConfig(t, cfg, map[string]interface{}{"type": "S3"},
		"disableSSL", "forcePathStyle", "includeUniverseAndNodeInPrefix",
		"directoryPrefix", "filePrefix", "endpoint", "roleArn", "partition",
		"marshaler")
}

func TestOTLPCreateSendsTypedConfig(t *testing.T) {
	_, _, cfg, _ := createProvider(t, &fakeYBA{}, ResourceOTLPTelemetryProvider(),
		map[string]interface{}{
			"name":             "otlp",
			"endpoint":         "https://collector",
			"auth_type":        "NoAuth",
			"protocol":         "HTTP",
			"compression":      "none",
			"timeout_seconds":  12,
			"logs_endpoint":    "https://collector/logs",
			"metrics_endpoint": "https://collector/metrics",
			"headers": map[string]interface{}{
				"X-Scope-OrgID": "1",
			},
		})
	assertConfig(t, cfg, map[string]interface{}{
		"type":     "OTLP",
		"endpoint": "https://collector",
		"authType": "NoAuth",
		"protocol": "HTTP",
		// timeoutSeconds arrives as float64 after the JSON round-trip.
		"timeoutSeconds":  float64(12),
		"compression":     "none",
		"logsEndpoint":    "https://collector/logs",
		"metricsEndpoint": "https://collector/metrics",
	}, "basicAuth", "bearerToken")
	h, ok := cfg["headers"].(map[string]interface{})
	if !ok || h["X-Scope-OrgID"] != "1" {
		t.Errorf("headers = %#v", cfg["headers"])
	}
}

// Schema defaults must reach the payload, and unset optionals must be omitted.
func TestOTLPCreateDefaultsAndOmissions(t *testing.T) {
	_, _, cfg, _ := createProvider(t, &fakeYBA{}, ResourceOTLPTelemetryProvider(),
		map[string]interface{}{
			"name":     "otlp",
			"endpoint": "https://collector",
		})
	assertConfig(t, cfg, map[string]interface{}{
		"type":     "OTLP",
		"authType": "NoAuth",
		"protocol": "gRPC",
	}, "logsEndpoint", "metricsEndpoint", "headers", "basicAuth", "bearerToken")
}

// BearerToken emits a bearerToken object, not basicAuth.
func TestOTLPCreateBearerToken(t *testing.T) {
	_, _, cfg, _ := createProvider(t, &fakeYBA{}, ResourceOTLPTelemetryProvider(),
		map[string]interface{}{
			"name":         "otlp",
			"endpoint":     "https://collector",
			"auth_type":    "BearerToken",
			"bearer_token": "tok",
		})
	if cfg["authType"] != "BearerToken" {
		t.Errorf("authType = %v want BearerToken", cfg["authType"])
	}
	bt, ok := cfg["bearerToken"].(map[string]interface{})
	if !ok || bt["token"] != "tok" {
		t.Errorf("bearerToken = %#v", cfg["bearerToken"])
	}
	if _, set := cfg["basicAuth"]; set {
		t.Error("basicAuth must NOT be emitted under BearerToken")
	}
}

func TestOTLPCreateBasicAuth(t *testing.T) {
	_, _, cfg, _ := createProvider(t, &fakeYBA{}, ResourceOTLPTelemetryProvider(),
		map[string]interface{}{
			"name":                "otlp",
			"endpoint":            "https://collector",
			"auth_type":           "BasicAuth",
			"basic_auth_username": "user",
			"basic_auth_password": "pass",
		})
	auth, ok := cfg["basicAuth"].(map[string]interface{})
	if !ok {
		t.Fatalf("basicAuth missing or wrong shape: %T %v",
			cfg["basicAuth"], cfg["basicAuth"])
	}
	if auth["username"] != "user" || auth["password"] != "pass" {
		t.Errorf("basicAuth = %+v", auth)
	}
	if _, set := cfg["bearerToken"]; set {
		t.Error("bearerToken must NOT be emitted with BasicAuth")
	}
}

// Missing credentials must be caught at plan time, before sensitive values
// reach state; endpoint overrides are HTTP-only.
func TestOTLPValidationPlanTime(t *testing.T) {
	res := ResourceOTLPTelemetryProvider()
	cases := []struct {
		name    string
		raw     map[string]interface{}
		wantErr string // substring; "" means the config must be accepted
	}{
		{
			name: "NoAuth needs no credentials",
			raw: map[string]interface{}{
				"name":      "x",
				"endpoint":  "https://collector",
				"auth_type": "NoAuth",
			},
		},
		{
			name: "BasicAuth missing username",
			raw: map[string]interface{}{
				"name":                "x",
				"endpoint":            "https://collector",
				"auth_type":           "BasicAuth",
				"basic_auth_password": "pw",
			},
			wantErr: "basic_auth_username",
		},
		{
			name: "BasicAuth missing password",
			raw: map[string]interface{}{
				"name":                "x",
				"endpoint":            "https://collector",
				"auth_type":           "BasicAuth",
				"basic_auth_username": "u",
			},
			wantErr: "basic_auth_username and basic_auth_password",
		},
		{
			name: "BasicAuth with both is accepted",
			raw: map[string]interface{}{
				"name":                "x",
				"endpoint":            "https://collector",
				"auth_type":           "BasicAuth",
				"basic_auth_username": "u",
				"basic_auth_password": "pw",
			},
		},
		{
			name: "BearerToken missing token",
			raw: map[string]interface{}{
				"name":      "x",
				"endpoint":  "https://collector",
				"auth_type": "BearerToken",
			},
			wantErr: "bearer_token is required",
		},
		{
			name: "BearerToken with token is accepted",
			raw: map[string]interface{}{
				"name":         "x",
				"endpoint":     "https://collector",
				"auth_type":    "BearerToken",
				"bearer_token": "tok",
			},
		},
		{
			name: "logs_endpoint under gRPC rejected",
			raw: map[string]interface{}{
				"name":          "x",
				"endpoint":      "https://collector",
				"logs_endpoint": "https://collector/logs",
				// protocol defaults to gRPC
			},
			wantErr: "logs_endpoint is only honoured when protocol = \"HTTP\"",
		},
		{
			name: "metrics_endpoint under gRPC rejected",
			raw: map[string]interface{}{
				"name":             "x",
				"endpoint":         "https://collector",
				"protocol":         "gRPC",
				"metrics_endpoint": "https://collector/metrics",
			},
			wantErr: "metrics_endpoint is only honoured when protocol = \"HTTP\"",
		},
		{
			name: "endpoint overrides under HTTP accepted",
			raw: map[string]interface{}{
				"name":             "x",
				"endpoint":         "https://collector",
				"protocol":         "HTTP",
				"logs_endpoint":    "https://collector/logs",
				"metrics_endpoint": "https://collector/metrics",
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

// OTLP auth_type accepts BearerToken; the similar-looking "BearerAuth" is
// rejected. compression is YBA's CompressionType enum; timeout must be positive.
func TestOTLPFieldEnums(t *testing.T) {
	res := ResourceOTLPTelemetryProvider()

	authVF := res.Schema["auth_type"].ValidateFunc
	if authVF == nil {
		t.Fatal("auth_type must have a ValidateFunc")
	}
	for _, ok := range []string{"NoAuth", "BasicAuth", "BearerToken"} {
		if _, errs := authVF(ok, "auth_type"); len(errs) > 0 {
			t.Errorf("auth_type %q should be valid, got %v", ok, errs)
		}
	}
	if _, errs := authVF("BearerAuth", "auth_type"); len(errs) == 0 {
		t.Error("auth_type \"BearerAuth\" must be rejected (YBA enum is BearerToken)")
	}

	compVF := res.Schema["compression"].ValidateFunc
	if compVF == nil {
		t.Fatal("compression must have a ValidateFunc (YBA CompressionType enum)")
	}
	for _, ok := range []string{"gzip", "none", "snappy", "zstd"} {
		if _, errs := compVF(ok, "compression"); len(errs) > 0 {
			t.Errorf("compression %q should be valid, got %v", ok, errs)
		}
	}
	if _, errs := compVF("brotli", "compression"); len(errs) == 0 {
		t.Error("compression \"brotli\" must be rejected")
	}

	toVF := res.Schema["timeout_seconds"].ValidateFunc
	if toVF == nil {
		t.Fatal("timeout_seconds must have a ValidateFunc")
	}
	for _, bad := range []int{0, -1} {
		if _, errs := toVF(bad, "timeout_seconds"); len(errs) == 0 {
			t.Errorf("timeout_seconds %d must be rejected", bad)
		}
	}
	if _, errs := toVF(5, "timeout_seconds"); len(errs) > 0 {
		t.Errorf("timeout_seconds 5 should be valid, got %v", errs)
	}
}

// S3 partition is a time bucket (hour/minute), not an AWS partition
// ("aws"/"aws-us-gov"/"aws-cn" are rejected).
func TestS3PartitionEnum(t *testing.T) {
	vf := ResourceS3TelemetryProvider().Schema["partition"].ValidateFunc
	if vf == nil {
		t.Fatal("partition must have a ValidateFunc")
	}
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
