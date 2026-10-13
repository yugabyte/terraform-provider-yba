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

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	clientv2 "github.com/yugabyte/platform-go-client/v2"
)

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

// node_restart_settings reaches YBA as written, with the schema defaults for
// the values that the block leaves out, on apply and on destroy, which reads
// the block from the state. Without a block, Terraform sends no upgrade
// options, so YBA applies its own defaults and can skip the restart.
func TestUpgradeOptionsFollowNodeRestartSettings(t *testing.T) {
	cases := []struct {
		name  string
		block map[string]interface{} // nil: no node_restart_settings block
		want  string                 // the upgrade_options that YBA receives
	}{
		{"no block", nil, "null"},
		{"empty block", map[string]interface{}{},
			`{"rolling_upgrade":true,"sleep_after_master_restart_millis":180000,` +
				`"sleep_after_tserver_restart_millis":180000}`},
		{"non-rolling with no waits", map[string]interface{}{
			"upgrade_option":                     "Non-Rolling",
			"sleep_after_master_restart_millis":  0,
			"sleep_after_tserver_restart_millis": 0,
		}, `{"rolling_upgrade":false,"sleep_after_master_restart_millis":0,` +
			`"sleep_after_tserver_restart_millis":0}`},
	}
	res := ResourceUniverseTelemetryConfig()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := map[string]interface{}{"universe_uuid": "uni-1"}
			if tc.block != nil {
				raw["node_restart_settings"] = []interface{}{tc.block}
			}
			applied := schema.TestResourceDataRaw(t, res.Schema, raw)
			applied.SetId("uni-1")
			destroyed, err := schema.InternalMap(res.Schema).Data(applied.State(), nil)
			if err != nil {
				t.Fatal(err)
			}

			for step, spec := range map[string]clientv2.ExportTelemetryConfigSpec{
				"apply":   buildExportTelemetryConfigSpec(applied),
				"destroy": buildDisableSpec(destroyed),
			} {
				got, err := json.Marshal(spec.UpgradeOptions)
				if err != nil {
					t.Fatal(err)
				}
				if string(got) != tc.want {
					t.Errorf("%s sends upgrade_options %s, want %s", step, got, tc.want)
				}
			}
		})
	}
}
