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
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func buildConfig(
	t *testing.T, block string, fields map[string]interface{},
) map[string]interface{} {
	t.Helper()
	res := ResourceTelemetryProvider()
	d := schema.TestResourceDataRaw(t, res.Schema, map[string]interface{}{
		"name": "tp",
		block:  []interface{}{fields},
	})
	cfg, err := buildTelemetryProviderConfig(d)
	if err != nil {
		t.Fatalf("build %s config: %v", block, err)
	}
	return cfg
}

// assertConfig checks every want key is present with the exact value and every
// absent key is missing. A mistyped camelCase key (e.g. roleArn vs roleARN)
// silently drops a field server-side.
func assertConfig(
	t *testing.T,
	got map[string]interface{},
	want map[string]interface{},
	absent ...string,
) {
	t.Helper()
	for k, v := range want {
		gv, ok := got[k]
		if !ok {
			t.Errorf("config missing key %q (want %v)", k, v)
			continue
		}
		if gv != v {
			t.Errorf("config[%q] = %v (%T), want %v (%T)", k, gv, gv, v, v)
		}
	}
	for _, k := range absent {
		if v, ok := got[k]; ok {
			t.Errorf("config must NOT contain key %q, got %v", k, v)
		}
	}
}

func TestBuildDataDogConfig(t *testing.T) {
	cfg := buildConfig(t, "data_dog", map[string]interface{}{
		"site":    "datadoghq.eu",
		"api_key": "dd-key",
	})
	assertConfig(t, cfg, map[string]interface{}{
		"type":   typeDataDog,
		"site":   "datadoghq.eu",
		"apiKey": "dd-key",
	})
}

// TestBuildAWSCloudWatchConfig pins the all-caps roleARN key (distinct from S3's
// roleArn).
func TestBuildAWSCloudWatchConfig(t *testing.T) {
	cfg := buildConfig(t, "aws_cloud_watch", map[string]interface{}{
		"log_group":  "yba/audit",
		"log_stream": "primary",
		"region":     "us-west-2",
		"access_key": "AKIA",
		"secret_key": "secret",
		"role_arn":   "arn:aws:iam::1:role/cw",
		"endpoint":   "https://logs.vpce",
	})
	assertConfig(t, cfg, map[string]interface{}{
		"type":      typeAWSCloudWatch,
		"logGroup":  "yba/audit",
		"logStream": "primary",
		"region":    "us-west-2",
		"accessKey": "AKIA",
		"secretKey": "secret",
		"roleARN":   "arn:aws:iam::1:role/cw",
		"endpoint":  "https://logs.vpce",
	})

	minimal := buildConfig(t, "aws_cloud_watch", map[string]interface{}{
		"log_group":  "g",
		"log_stream": "s",
		"region":     "r",
		"access_key": "a",
		"secret_key": "s",
	})
	assertConfig(t, minimal, map[string]interface{}{"type": typeAWSCloudWatch},
		"roleARN", "endpoint")
}

// TestBuildGCPConfig: project is omitted when empty (YBA derives it from the
// credentials JSON).
func TestBuildGCPConfig(t *testing.T) {
	cfg := buildConfig(t, "gcp_cloud_monitoring", map[string]interface{}{
		"project":          "my-proj",
		"credentials_json": `{"type":"service_account"}`,
	})
	assertConfig(t, cfg, map[string]interface{}{
		"type":              typeGCPCloudMonitor,
		"project":           "my-proj",
		"credentialsString": `{"type":"service_account"}`,
	})

	noProj := buildConfig(t, "gcp_cloud_monitoring", map[string]interface{}{
		"credentials_json": `{"project_id":"x"}`,
	})
	assertConfig(t, noProj,
		map[string]interface{}{"credentialsString": `{"project_id":"x"}`},
		"project")
}

func TestBuildSplunkConfig(t *testing.T) {
	cfg := buildConfig(t, "splunk", map[string]interface{}{
		"endpoint":    "https://hec:8088",
		"token":       "hec-token",
		"source":      "yba",
		"source_type": "_json",
		"index":       "main",
	})
	assertConfig(t, cfg, map[string]interface{}{
		"type":       typeSplunk,
		"endpoint":   "https://hec:8088",
		"token":      "hec-token",
		"source":     "yba",
		"sourceType": "_json",
		"index":      "main",
	})

	minimal := buildConfig(t, "splunk", map[string]interface{}{
		"endpoint": "https://hec:8088",
		"token":    "t",
	})
	assertConfig(t, minimal, map[string]interface{}{"type": typeSplunk},
		"source", "sourceType", "index")
}

func TestBuildDynatraceConfig(t *testing.T) {
	cfg := buildConfig(t, "dynatrace", map[string]interface{}{
		"endpoint":  "https://env.live.dynatrace.com/api/v2/otlp",
		"api_token": "dt-token",
	})
	assertConfig(t, cfg, map[string]interface{}{
		"type":     typeDynatrace,
		"endpoint": "https://env.live.dynatrace.com/api/v2/otlp",
		"apiToken": "dt-token",
	})
}

func TestBuildLokiConfigBasicAuth(t *testing.T) {
	cfg := buildConfig(t, "loki", map[string]interface{}{
		"endpoint":            "https://loki",
		"auth_type":           "BasicAuth",
		"organization_id":     "tenant-1",
		"basic_auth_username": "u",
		"basic_auth_password": "p",
	})
	assertConfig(t, cfg, map[string]interface{}{
		"type":           typeLoki,
		"endpoint":       "https://loki",
		"authType":       "BasicAuth",
		"organizationID": "tenant-1",
	})
	auth, ok := cfg["basicAuth"].(map[string]string)
	if !ok || auth["username"] != "u" || auth["password"] != "p" {
		t.Errorf("loki basicAuth = %#v", cfg["basicAuth"])
	}

	noAuth := buildConfig(t, "loki", map[string]interface{}{
		"endpoint":  "https://loki",
		"auth_type": "NoAuth",
	})
	assertConfig(t, noAuth, map[string]interface{}{"authType": "NoAuth"},
		"basicAuth", "organizationID")
}

// BearerToken emits a bearerToken object, not basicAuth.
func TestBuildOTLPConfigBearerToken(t *testing.T) {
	cfg := buildConfig(t, "otlp", map[string]interface{}{
		"endpoint":     "https://collector",
		"auth_type":    "BearerToken",
		"bearer_token": "tok",
	})
	if cfg["authType"] != "BearerToken" {
		t.Errorf("authType = %v want BearerToken", cfg["authType"])
	}
	bt, ok := cfg["bearerToken"].(map[string]string)
	if !ok || bt["token"] != "tok" {
		t.Errorf("bearerToken = %#v", cfg["bearerToken"])
	}
	if _, set := cfg["basicAuth"]; set {
		t.Error("basicAuth must NOT be emitted under BearerToken")
	}
}

func TestBuildOTLPConfigNoAuthOmitsCredentials(t *testing.T) {
	cfg := buildConfig(t, "otlp", map[string]interface{}{
		"endpoint":  "https://collector",
		"auth_type": "NoAuth",
	})
	assertConfig(t, cfg,
		map[string]interface{}{"type": typeOTLP, "authType": "NoAuth"},
		"basicAuth", "bearerToken")
}

func TestBuildOTLPConfigHeadersAndEndpoints(t *testing.T) {
	cfg := buildConfig(t, "otlp", map[string]interface{}{
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
		"type":            typeOTLP,
		"endpoint":        "https://collector",
		"protocol":        "HTTP",
		"compression":     "none",
		"timeoutSeconds":  12,
		"logsEndpoint":    "https://collector/logs",
		"metricsEndpoint": "https://collector/metrics",
	})
	h, ok := cfg["headers"].(map[string]interface{})
	if !ok || h["X-Scope-OrgID"] != "1" {
		t.Errorf("headers = %#v", cfg["headers"])
	}

	minimal := buildConfig(t, "otlp", map[string]interface{}{
		"endpoint":  "https://collector",
		"auth_type": "NoAuth",
	})
	assertConfig(t, minimal, map[string]interface{}{"type": typeOTLP},
		"logsEndpoint", "metricsEndpoint", "headers")
}

func TestTelemetryProviderTypeNoBlock(t *testing.T) {
	res := ResourceTelemetryProvider()
	d := schema.TestResourceDataRaw(t, res.Schema, map[string]interface{}{
		"name": "tp",
	})
	if _, err := telemetryProviderType(d); err == nil {
		t.Error("expected error when no provider block is configured")
	}
	if _, err := buildTelemetryProviderConfig(d); err == nil {
		t.Error("buildTelemetryProviderConfig must error with no block set")
	}
}

// TestHelpersTolerateWeirdInput: the extraction helpers must not panic on
// nil/wrong-type/empty input; all degrade to a zero value.
func TestHelpersTolerateWeirdInput(t *testing.T) {
	if m := firstMap(nil); len(m) != 0 {
		t.Errorf("firstMap(nil) = %v", m)
	}
	if m := firstMap([]interface{}{}); len(m) != 0 {
		t.Errorf("firstMap(empty) = %v", m)
	}
	if m := firstMap([]interface{}{nil}); len(m) != 0 {
		t.Errorf("firstMap([nil]) = %v", m)
	}
	if m := firstMap("not-a-list"); len(m) != 0 {
		t.Errorf("firstMap(string) = %v", m)
	}
	if m := firstMap([]interface{}{map[string]interface{}{"k": "v"}}); m["k"] != "v" {
		t.Errorf("firstMap valid = %v", m)
	}

	if stringValue(nil) != "" {
		t.Error("stringValue(nil) must be empty")
	}
	if got := stringValue(42); got != "42" {
		t.Errorf("stringValue(42) = %q", got)
	}
	if got := stringValue("s"); got != "s" {
		t.Errorf("stringValue(s) = %q", got)
	}

	if intValue(nil) != 0 || intValue("x") != 0 {
		t.Error("intValue must default to 0 on nil/non-int")
	}
	if intValue(7) != 7 {
		t.Error("intValue(7) != 7")
	}
	if int32Value("x") != 0 || int32Value(5) != 5 {
		t.Error("int32Value mishandled")
	}
	if boolValue(nil) || boolValue("true") {
		t.Error("boolValue must default false on nil/non-bool")
	}
	if !boolValue(true) {
		t.Error("boolValue(true) != true")
	}

	// stringList accepts both a TypeList ([]interface{}) and a TypeSet (*schema.Set).
	if got := stringList(nil); len(got) != 0 {
		t.Errorf("stringList(nil) = %v", got)
	}
	if got := stringList([]interface{}{"a", "b"}); len(got) != 2 {
		t.Errorf("stringList(list) = %v", got)
	}
	set := schema.NewSet(schema.HashString, []interface{}{"a", "b", "c"})
	if got := stringList(set); len(got) != 3 {
		t.Errorf("stringList(set) = %v", got)
	}

	if got := stringMap(nil); len(got) != 0 {
		t.Errorf("stringMap(nil) = %v", got)
	}
	if got := stringMap(map[string]interface{}{"k": 1}); got["k"] != "1" {
		t.Errorf("stringMap coercion = %v", got)
	}
}

// TestBuildUpgradeOptionsVariants: a zero sleep is omitted so YBA keeps its own
// default rather than restarting with no pause.
func TestBuildUpgradeOptionsVariants(t *testing.T) {
	nonRolling := buildUpgradeOptions([]interface{}{map[string]interface{}{
		"rolling_upgrade":                    false,
		"sleep_after_master_restart_millis":  1000,
		"sleep_after_tserver_restart_millis": 2000,
	}})
	if nonRolling.RollingUpgrade == nil || *nonRolling.RollingUpgrade {
		t.Error("rolling_upgrade=false must propagate as false")
	}
	if nonRolling.SleepAfterMasterRestartMillis == nil ||
		*nonRolling.SleepAfterMasterRestartMillis != 1000 {
		t.Errorf("master sleep = %v", nonRolling.SleepAfterMasterRestartMillis)
	}
	if nonRolling.SleepAfterTserverRestartMillis == nil ||
		*nonRolling.SleepAfterTserverRestartMillis != 2000 {
		t.Errorf("tserver sleep = %v", nonRolling.SleepAfterTserverRestartMillis)
	}

	zeroSleep := buildUpgradeOptions([]interface{}{map[string]interface{}{
		"rolling_upgrade":                   true,
		"sleep_after_master_restart_millis": 0,
	}})
	if zeroSleep.SleepAfterMasterRestartMillis != nil {
		t.Error("a zero sleep must be omitted (let YBA pick its default)")
	}
}
