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
	"encoding/json"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	client "github.com/yugabyte/platform-go-client"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// TestBuildExportTelemetryConfigSpec verifies that the unified
// export-telemetry-configs payload produced by the resource (via the v2 SDK
// types) marshals to the snake_case JSON shape documented by YBA.
func TestBuildExportTelemetryConfigSpec(t *testing.T) {
	res := ResourceUniverseTelemetryConfig()
	d := schema.TestResourceDataRaw(t, res.Schema, map[string]interface{}{
		"universe_uuid": "abc-uuid",
		"audit_logs": []interface{}{
			map[string]interface{}{
				"ysql_audit_config": []interface{}{
					map[string]interface{}{
						"enabled":                true,
						"classes":                []interface{}{"READ", "WRITE"},
						"log_catalog":            true,
						"log_client":             true,
						"log_level":              "WARNING",
						"log_parameter":          true,
						"log_parameter_max_size": 4096,
						"log_relation":           true,
						"log_rows":               true,
						"log_statement":          true,
						"log_statement_once":     true,
					},
				},
				"exporter": []interface{}{
					map[string]interface{}{
						"exporter_uuid": "exp-1",
						"additional_tags": map[string]interface{}{
							"env": "prod",
						},
					},
				},
			},
		},
		"metrics": []interface{}{
			map[string]interface{}{
				"scrape_interval_seconds": 30,
				"scrape_timeout_seconds":  20,
				"collection_level":        "NORMAL",
				"scrape_config_targets":   []interface{}{"MASTER_EXPORT", "TSERVER_EXPORT"},
				"exporter": []interface{}{
					map[string]interface{}{
						"exporter_uuid":              "exp-1",
						"send_batch_size":            100,
						"send_batch_max_size":        1000,
						"send_batch_timeout_seconds": 60,
						"memory_limit_mib":           2048,
						"metrics_prefix":             "ybdb.",
					},
				},
			},
		},
	})

	spec := buildExportTelemetryConfigSpec(d)
	payload, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("marshal spec: %v", err)
	}

	var out struct {
		TelemetryConfig struct {
			AuditLogs *struct {
				YsqlAuditConfig map[string]interface{}   `json:"ysql_audit_config"`
				Exporters       []map[string]interface{} `json:"exporters"`
			} `json:"audit_logs"`
			Metrics *struct {
				ScrapeIntervalSeconds int                      `json:"scrape_interval_seconds"`
				CollectionLevel       string                   `json:"collection_level"`
				ScrapeConfigTargets   []string                 `json:"scrape_config_targets"`
				Exporters             []map[string]interface{} `json:"exporters"`
			} `json:"metrics"`
		} `json:"telemetry_config"`
		UpgradeOptions map[string]interface{} `json:"upgrade_options"`
	}
	if err := json.Unmarshal(payload, &out); err != nil {
		t.Fatalf("unmarshal payload: %v\n%s", err, payload)
	}
	if out.TelemetryConfig.AuditLogs == nil {
		t.Fatalf("audit_logs missing from payload: %s", payload)
	}
	if got := out.TelemetryConfig.AuditLogs.YsqlAuditConfig["log_level"]; got != "WARNING" {
		t.Errorf("ysql log_level: got %v want WARNING", got)
	}
	if len(out.TelemetryConfig.AuditLogs.Exporters) != 1 {
		t.Errorf("expected exactly 1 audit exporter, got %d", len(out.TelemetryConfig.AuditLogs.Exporters))
	}
	if out.TelemetryConfig.Metrics == nil {
		t.Fatalf("metrics missing from payload: %s", payload)
	}
	if out.TelemetryConfig.Metrics.CollectionLevel != "NORMAL" {
		t.Errorf("metrics collection_level: got %q want NORMAL",
			out.TelemetryConfig.Metrics.CollectionLevel)
	}
	if len(out.TelemetryConfig.Metrics.ScrapeConfigTargets) != 2 {
		t.Errorf("expected 2 scrape targets, got %d",
			len(out.TelemetryConfig.Metrics.ScrapeConfigTargets))
	}
	if len(out.TelemetryConfig.Metrics.Exporters) != 1 {
		t.Errorf("expected exactly 1 metrics exporter, got %d",
			len(out.TelemetryConfig.Metrics.Exporters))
	}
	mexp := out.TelemetryConfig.Metrics.Exporters[0]
	if mexp["metrics_prefix"] != "ybdb." {
		t.Errorf("metrics_prefix: got %v want ybdb.", mexp["metrics_prefix"])
	}
	if got, ok := out.UpgradeOptions["rolling_upgrade"].(bool); !ok || !got {
		t.Errorf("upgrade_options.rolling_upgrade: got %v want true", out.UpgradeOptions["rolling_upgrade"])
	}
}

// TestBuildDisableSpec verifies the empty `telemetry_config: {}` body used
// when deleting a `yba_universe_telemetry_config` resource.
func TestBuildDisableSpec(t *testing.T) {
	res := ResourceUniverseTelemetryConfig()
	d := schema.TestResourceDataRaw(t, res.Schema, map[string]interface{}{
		"universe_uuid": "abc-uuid",
	})
	spec := buildDisableSpec(d)
	payload, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("marshal disable spec: %v", err)
	}
	var out map[string]interface{}
	if err := json.Unmarshal(payload, &out); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, payload)
	}
	tc, ok := out["telemetry_config"].(map[string]interface{})
	if !ok {
		t.Fatalf("telemetry_config not an object: %s", payload)
	}
	if len(tc) != 0 {
		t.Errorf("expected empty telemetry_config, got %v", tc)
	}
}

// TestBuildDetachSpec verifies that buildDetachSpec rewrites a universe's
// telemetry configuration with the target provider UUID filtered out of
// every exporter list, and drops the enclosing section (audit_logs /
// query_logs / metrics) entirely when it has no remaining exporters.
func TestBuildDetachSpec(t *testing.T) {
	keep := "keep-uuid"
	drop := "drop-uuid"
	u := client.UniverseResp{
		UniverseUUID: utils.GetStringPointer("uni-1"),
		Name:         utils.GetStringPointer("uni-1"),
		UniverseDetails: &client.UniverseDefinitionTaskParamsResp{
			Clusters: []client.Cluster{
				{
					UserIntent: client.UserIntent{
						AuditLogConfig: &client.AuditLogConfig{
							UniverseLogsExporterConfig: []client.UniverseLogsExporterConfig{
								{ExporterUuid: keep},
								{ExporterUuid: drop},
							},
						},
						QueryLogConfig: &client.QueryLogConfig{
							UniverseLogsExporterConfig: []client.UniverseQueryLogsExporterConfig{
								{ExporterUuid: drop},
							},
						},
						MetricsExportConfig: &client.MetricsExportConfig{
							UniverseMetricsExporterConfig: []client.UniverseMetricsExporterConfig{
								{ExporterUuid: keep},
								{ExporterUuid: drop},
							},
							ScrapeConfigTargets: []string{"MASTER_EXPORT"},
						},
					},
				},
			},
		},
	}

	spec := buildDetachSpec(&u, drop)

	if spec.TelemetryConfig == nil {
		t.Fatalf("telemetry_config nil in detach spec")
	}
	a := spec.TelemetryConfig.AuditLogs
	if a == nil {
		t.Fatalf("audit_logs dropped even though a non-target exporter remains")
	}
	if len(a.Exporters) != 1 || a.Exporters[0].ExporterUuid != keep {
		t.Errorf("audit exporters after detach = %+v; want [%s]", a.Exporters, keep)
	}
	if spec.TelemetryConfig.QueryLogs != nil {
		t.Errorf("query_logs should be dropped when only exporter is the detached provider")
	}
	m := spec.TelemetryConfig.Metrics
	if m == nil {
		t.Fatalf("metrics dropped even though a non-target exporter remains")
	}
	if len(m.Exporters) != 1 || m.Exporters[0].ExporterUuid != keep {
		t.Errorf("metrics exporters after detach = %+v; want [%s]", m.Exporters, keep)
	}
	if len(m.ScrapeConfigTargets) != 1 || string(m.ScrapeConfigTargets[0]) != "MASTER_EXPORT" {
		t.Errorf("metrics scrape targets not preserved: %+v", m.ScrapeConfigTargets)
	}
}

// TestUniverseReferencesProvider ensures the in-use detector inspects
// every telemetry sub-config (audit / query / metrics).
func TestUniverseReferencesProvider(t *testing.T) {
	target := "target-uuid"
	mk := func(audit, query, metrics string) *client.UniverseResp {
		u := &client.UniverseResp{
			UniverseDetails: &client.UniverseDefinitionTaskParamsResp{
				Clusters: []client.Cluster{{UserIntent: client.UserIntent{}}},
			},
		}
		intent := &u.UniverseDetails.Clusters[0].UserIntent
		if audit != "" {
			intent.AuditLogConfig = &client.AuditLogConfig{
				UniverseLogsExporterConfig: []client.UniverseLogsExporterConfig{
					{ExporterUuid: audit},
				},
			}
		}
		if query != "" {
			intent.QueryLogConfig = &client.QueryLogConfig{
				UniverseLogsExporterConfig: []client.UniverseQueryLogsExporterConfig{
					{ExporterUuid: query},
				},
			}
		}
		if metrics != "" {
			intent.MetricsExportConfig = &client.MetricsExportConfig{
				UniverseMetricsExporterConfig: []client.UniverseMetricsExporterConfig{
					{ExporterUuid: metrics},
				},
			}
		}
		return u
	}
	cases := []struct {
		name string
		u    *client.UniverseResp
		want bool
	}{
		{"none", mk("other", "other", "other"), false},
		{"audit", mk(target, "other", "other"), true},
		{"query", mk("other", target, "other"), true},
		{"metrics", mk("other", "other", target), true},
		{"empty", &client.UniverseResp{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := universeReferencesProvider(tc.u, target); got != tc.want {
				t.Errorf("universeReferencesProvider = %v want %v", got, tc.want)
			}
		})
	}
}

// TestTelemetryProviderType ensures the polymorphic block selector returns
// the correct YBA ProviderType enum.
func TestTelemetryProviderType(t *testing.T) {
	cases := []struct {
		block string
		want  string
	}{
		{"data_dog", typeDataDog},
		{"otlp", typeOTLP},
		{"aws_cloud_watch", typeAWSCloudWatch},
		{"gcp_cloud_monitoring", typeGCPCloudMonitor},
		{"splunk", typeSplunk},
		{"loki", typeLoki},
		{"dynatrace", typeDynatrace},
		{"s3", typeS3},
	}
	for _, tc := range cases {
		t.Run(tc.block, func(t *testing.T) {
			res := ResourceTelemetryProvider()
			raw := map[string]interface{}{
				"name": "test",
				tc.block: []interface{}{
					map[string]interface{}{},
				},
			}
			d := schema.TestResourceDataRaw(t, res.Schema, raw)
			got, err := telemetryProviderType(d)
			if err != nil {
				t.Fatalf("type: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q want %q", got, tc.want)
			}
		})
	}
}

// TestBuildTelemetryProviderConfigS3 verifies that the S3 block (the most
// recently added provider type and the one most likely to drift from the
// YBA OpenAPI shape) lays out every field with the camelCase key YBA
// expects, and that boolean defaults that map to false are omitted rather
// than transmitted as zero values.
func TestBuildTelemetryProviderConfigS3(t *testing.T) {
	res := ResourceTelemetryProvider()
	d := schema.TestResourceDataRaw(t, res.Schema, map[string]interface{}{
		"name": "my-s3",
		"s3": []interface{}{
			map[string]interface{}{
				"bucket":                              "logs-bucket",
				"region":                              "us-east-1",
				"access_key":                          "AKIA...",
				"secret_key":                          "secret",
				"directory_prefix":                    "yba/audit",
				"file_prefix":                         "uni-",
				"endpoint":                            "https://s3.us-east-1.amazonaws.com",
				"role_arn":                            "arn:aws:iam::1:role/x",
				"partition":                           "aws",
				"marshaler":                           "json",
				"disable_ssl":                         true,
				"force_path_style":                    true,
				"include_universe_and_node_in_prefix": true,
			},
		},
	})

	cfg, err := buildTelemetryProviderConfig(d)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	wantKeys := map[string]interface{}{
		"type":                           typeS3,
		"bucket":                         "logs-bucket",
		"region":                         "us-east-1",
		"accessKey":                      "AKIA...",
		"secretKey":                      "secret",
		"directoryPrefix":                "yba/audit",
		"filePrefix":                     "uni-",
		"endpoint":                       "https://s3.us-east-1.amazonaws.com",
		"roleArn":                        "arn:aws:iam::1:role/x",
		"partition":                      "aws",
		"marshaler":                      "json",
		"disableSSL":                     true,
		"forcePathStyle":                 true,
		"includeUniverseAndNodeInPrefix": true,
	}
	for k, want := range wantKeys {
		if got := cfg[k]; got != want {
			t.Errorf("config[%q] = %v want %v", k, got, want)
		}
	}
}

// TestBuildTelemetryProviderConfigS3OmitsFalse verifies that boolean
// fields default to omission when set to false. YBA treats a missing key
// as "use the YBA default" which is what we want; sending false would
// pin the field and could surprise users on YBA upgrades that change the
// default.
func TestBuildTelemetryProviderConfigS3OmitsFalse(t *testing.T) {
	res := ResourceTelemetryProvider()
	d := schema.TestResourceDataRaw(t, res.Schema, map[string]interface{}{
		"name": "minimal-s3",
		"s3": []interface{}{
			map[string]interface{}{
				"bucket":     "b",
				"region":     "us-east-1",
				"access_key": "A",
				"secret_key": "S",
			},
		},
	})
	cfg, err := buildTelemetryProviderConfig(d)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	for _, k := range []string{
		"disableSSL", "forcePathStyle", "includeUniverseAndNodeInPrefix",
		"directoryPrefix", "filePrefix", "endpoint", "roleArn", "partition",
		"marshaler",
	} {
		if _, set := cfg[k]; set {
			t.Errorf("optional key %q must be omitted when unset, got %v",
				k, cfg[k])
		}
	}
}

// TestBuildTelemetryProviderConfigOTLPBasicAuth covers the auth_type
// branch logic: OTLP renders BasicAuth and BearerAuth into distinct
// nested objects, NoAuth into neither. A regression in the switch
// (e.g. always emitting basicAuth) is the kind of change that easily
// slips past a code review of an unrelated change.
func TestBuildTelemetryProviderConfigOTLPBasicAuth(t *testing.T) {
	res := ResourceTelemetryProvider()
	d := schema.TestResourceDataRaw(t, res.Schema, map[string]interface{}{
		"name": "otlp",
		"otlp": []interface{}{
			map[string]interface{}{
				"endpoint":            "https://collector",
				"auth_type":           "BasicAuth",
				"basic_auth_username": "user",
				"basic_auth_password": "pass",
			},
		},
	})
	cfg, err := buildTelemetryProviderConfig(d)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if cfg["type"] != typeOTLP {
		t.Errorf("type = %v want %v", cfg["type"], typeOTLP)
	}
	auth, ok := cfg["basicAuth"].(map[string]string)
	if !ok {
		t.Fatalf("basicAuth missing or wrong shape: %T %v",
			cfg["basicAuth"], cfg["basicAuth"])
	}
	if auth["username"] != "user" || auth["password"] != "pass" {
		t.Errorf("basicAuth = %+v", auth)
	}
	if _, set := cfg["bearerToken"]; set {
		t.Errorf("bearerToken must NOT be emitted with BasicAuth")
	}
}

// TestTelemetryConfigBlocksMatchSwitch protects against the "added a new
// provider type but forgot to wire it everywhere" bug. The list,
// `ExactlyOneOf` resolution, and the type/build switch must all stay in
// sync; if any one drifts the resource silently rejects the new type at
// plan time without a useful error.
func TestTelemetryConfigBlocksMatchSwitch(t *testing.T) {
	for _, block := range telemetryConfigBlocks {
		t.Run(block, func(t *testing.T) {
			res := ResourceTelemetryProvider()
			s, ok := res.Schema[block]
			if !ok {
				t.Fatalf("schema is missing block %q", block)
			}
			if !s.ForceNew {
				t.Errorf("block %q must be ForceNew "+
					"(YBA does not allow editing in place)", block)
			}
			if s.MaxItems != 1 {
				t.Errorf("block %q must have MaxItems=1, got %d",
					block, s.MaxItems)
			}
			if len(s.ExactlyOneOf) == 0 {
				t.Errorf("block %q must declare ExactlyOneOf", block)
			}
			d := schema.TestResourceDataRaw(t, res.Schema,
				map[string]interface{}{
					"name": "x",
					block:  []interface{}{map[string]interface{}{}},
				})
			if _, err := telemetryProviderType(d); err != nil {
				t.Errorf("type lookup for block %q failed: %v", block, err)
			}
		})
	}
}

// TestResourceTelemetryProviderSchema sanity-checks the resource shape
// against the design choices we made in the audit pass: tags forces
// destroy-recreate (YBA has no PUT for telemetry providers), there is no
// customer_uuid field (it's the same for every resource managed by a
// given provider instance and just adds noise to plans), and the long
// Delete timeout is wired through.
func TestResourceTelemetryProviderSchema(t *testing.T) {
	res := ResourceTelemetryProvider()

	if _, present := res.Schema["customer_uuid"]; present {
		t.Error("customer_uuid must not be exposed on yba_telemetry_provider " +
			"(it duplicates apiClient.CustomerID)")
	}

	tags, ok := res.Schema["tags"]
	if !ok {
		t.Fatal("tags field missing from schema")
	}
	if !tags.ForceNew {
		t.Error("tags must be ForceNew until YBA exposes a PUT endpoint " +
			"for telemetry providers")
	}

	name, ok := res.Schema["name"]
	if !ok || !name.Required || !name.ForceNew {
		t.Errorf("name must be Required+ForceNew, got %+v", name)
	}

	if res.Timeouts == nil || res.Timeouts.Delete == nil {
		t.Fatal("Delete timeout must be set so destroy can wait for " +
			"per-universe rolling-upgrade detach tasks")
	}
	if *res.Timeouts.Delete != telemetryUpgradeTimeout {
		t.Errorf("Delete timeout = %s want %s",
			*res.Timeouts.Delete, telemetryUpgradeTimeout)
	}

	if res.Importer == nil {
		t.Error("Importer must be set so existing providers can be imported")
	}
}

// TestResourceUniverseTelemetryConfigSchema sanity-checks the universe
// telemetry config resource: universe_uuid must be ForceNew (a different
// universe is a new resource entirely), and Create/Update/Delete must
// share the same long timeout so a rolling restart does not hit the
// SDK's 20-minute default mid-upgrade.
func TestResourceUniverseTelemetryConfigSchema(t *testing.T) {
	res := ResourceUniverseTelemetryConfig()

	uu, ok := res.Schema["universe_uuid"]
	if !ok || !uu.Required || !uu.ForceNew {
		t.Errorf("universe_uuid must be Required+ForceNew, got %+v", uu)
	}

	if res.Timeouts == nil {
		t.Fatal("Timeouts must be set on yba_universe_telemetry_config")
	}
	for name, got := range map[string]*time.Duration{
		"Create": res.Timeouts.Create,
		"Update": res.Timeouts.Update,
		"Delete": res.Timeouts.Delete,
	} {
		if got == nil {
			t.Errorf("%s timeout must be set", name)
			continue
		}
		if *got != telemetryUpgradeTimeout {
			t.Errorf("%s timeout = %s want %s",
				name, *got, telemetryUpgradeTimeout)
		}
	}
}

// TestBuildUpgradeOptionsDefault verifies that an absent upgrade_options
// block produces a payload that requests a rolling upgrade but lets YBA
// pick its own per-restart sleep defaults. Hard-coding sleeps here would
// silently extend every reconfigure on multi-node universes by minutes.
func TestBuildUpgradeOptionsDefault(t *testing.T) {
	out := buildUpgradeOptions(nil)
	if out.RollingUpgrade == nil || !*out.RollingUpgrade {
		t.Errorf("RollingUpgrade must default to true, got %v",
			out.RollingUpgrade)
	}
	if out.SleepAfterMasterRestartMillis != nil {
		t.Errorf("SleepAfterMasterRestartMillis must be nil when block "+
			"absent, got %d", *out.SleepAfterMasterRestartMillis)
	}
	if out.SleepAfterTserverRestartMillis != nil {
		t.Errorf("SleepAfterTserverRestartMillis must be nil when block "+
			"absent, got %d", *out.SleepAfterTserverRestartMillis)
	}
}

// TestBuildExportTelemetryConfigSpecMetricsOnly is a defensive smoke test
// for the buildExportTelemetryConfigSpec / build* helpers: when a resource
// configures only the metrics block, audit_logs and query_logs must
// remain nil (so the unified endpoint disables them) instead of being
// populated with empty structs (which would then trip YBA's enum
// validation on missing required fields).
func TestBuildExportTelemetryConfigSpecMetricsOnly(t *testing.T) {
	res := ResourceUniverseTelemetryConfig()
	d := schema.TestResourceDataRaw(t, res.Schema, map[string]interface{}{
		"universe_uuid": "uni-1",
		"metrics": []interface{}{
			map[string]interface{}{
				"exporter": []interface{}{
					map[string]interface{}{"exporter_uuid": "exp-1"},
				},
			},
		},
	})
	spec := buildExportTelemetryConfigSpec(d)
	if spec.TelemetryConfig == nil {
		t.Fatal("telemetry_config must be set")
	}
	if spec.TelemetryConfig.AuditLogs != nil {
		t.Error("audit_logs must be nil when not configured")
	}
	if spec.TelemetryConfig.QueryLogs != nil {
		t.Error("query_logs must be nil when not configured")
	}
	if spec.TelemetryConfig.Metrics == nil {
		t.Fatal("metrics must be set when configured")
	}
	if got := spec.TelemetryConfig.Metrics.Exporters; len(got) != 1 ||
		got[0].ExporterUuid != "exp-1" {
		t.Errorf("metrics exporters = %+v want [exp-1]", got)
	}
}
