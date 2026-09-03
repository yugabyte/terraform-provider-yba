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
	"strings"
	"testing"

	"github.com/yugabyte/terraform-provider-yba/internal/api"
)

// The gate must pick the bound from the target's release line, treat custom
// and suffixed builds the way YBA does, and stay out of the way when it has
// no version to check.
func TestValidateYBAVersionPlanTime(t *testing.T) {
	res := ResourceUniverseTelemetryConfig()
	block := func(name string) map[string]interface{} {
		return map[string]interface{}{
			"universe_uuid": "u",
			name: []interface{}{map[string]interface{}{
				"exporter": []interface{}{map[string]interface{}{"exporter_uuid": "e"}},
			}},
		}
	}
	cases := []struct {
		name    string
		version string
		raw     map[string]interface{}
		wantErr string
	}{
		{"preview below pipeline min", "2.31.0.0-b164", block("master_logs"),
			"master_logs requires YugabyteDB Anywhere 2.31.0.0-b386"},
		{"preview at pipeline min", "2.31.0.0-b386", block("master_logs"), ""},
		{"preview later build", "2.31.0.0-b395", block("controller_logs"), ""},
		{"preview old build, no server-log block", "2.31.0.0-b164", block("metrics"), ""},
		{"stable below pipeline min", "2026.1.1.0-b91", block("tserver_logs"),
			"tserver_logs requires YugabyteDB Anywhere 2026.1.2.0-b84"},
		{"stable at pipeline min", "2026.1.2.0-b84", block("tserver_logs"), ""},
		{"stable later release", "2026.2.0.0-b1", block("ynp_logs"), ""},
		{"stable below unified API floor", "2025.2.0.0-b131", block("metrics"),
			"yba_universe_telemetry_config requires YugabyteDB Anywhere 2026.1.0.0-b61"},
		{"preview below unified API floor", "2.29.0.0-b621", block("metrics"),
			"yba_universe_telemetry_config requires YugabyteDB Anywhere 2.29.0.0-b622"},
		{"preview at unified API floor", "2.29.0.0-b622", block("audit_logs"), ""},
		{"custom build of a new enough release passes", "2.31.0.0-custom",
			block("master_logs"), ""},
		{"custom build of an old release fails", "2.29.0.0-custom", block("master_logs"),
			"master_logs requires"},
		{"downstream suffix keeps the build number", "2.31.0.0-b395-ybm7",
			block("master_logs"), ""},
		{"downstream suffix on an old build fails", "2.31.0.0-b100-ybm7",
			block("master_logs"), "master_logs requires"},
		{"stable downstream suffix", "2026.1.2.0-b90-ybm", block("node_agent_logs"), ""},
		{"experimental patch build is assumed compatible", "2.31.0.4263-b4",
			block("master_logs"), ""},
		{"experimental patch build on an older-looking line still passes",
			"2.29.0.4263-b1", block("master_logs"), ""},
		{"unparseable version is let through", "dev-build", block("master_logs"), ""},
		{"no version seeded skips the gate", "", block("master_logs"), ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			meta := &api.APIClient{}
			if c.version != "" {
				meta.SetAppVersion(c.version)
			}
			err := diffErrMeta(t, res, c.raw, meta)
			switch {
			case c.wantErr == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case c.wantErr != "" && err == nil:
				t.Fatalf("expected error containing %q", c.wantErr)
			case c.wantErr != "" && !strings.Contains(err.Error(), c.wantErr):
				t.Fatalf("error %q does not contain %q", err, c.wantErr)
			}
		})
	}
	t.Run("nil meta skips the gate", func(t *testing.T) {
		if err := diffErr(t, res, block("master_logs")); err != nil {
			t.Fatalf("nil meta must skip the version gate, got: %v", err)
		}
	})
}

// Every server-log pipeline in the registry carries the same minimum, and the
// three older pipelines carry none — so adding a block without deciding its
// minimum is caught here.
func TestPipelineRegistryMinimums(t *testing.T) {
	serverLogs := map[string]bool{
		"master_logs": true, "tserver_logs": true, "ysql_conn_mgr_logs": true,
		"node_agent_logs": true, "ynp_logs": true, "controller_logs": true,
	}
	for _, p := range telemetryPipelines {
		if serverLogs[p.label] {
			if p.min != &serverLogPipelinesMin {
				t.Errorf("%s must be gated on serverLogPipelinesMin", p.label)
			}
			continue
		}
		if p.min != nil {
			t.Errorf(
				"%s predates the server-log pipelines and must not carry their minimum",
				p.label,
			)
		}
	}
	if got := versionNote(
		"X requires",
		serverLogPipelinesMin,
	); !strings.Contains(
		got,
		"2.31.0.0-b386",
	) ||
		!strings.Contains(got, "2026.1.2.0-b84") {
		t.Errorf("docs note must name both minimums: %s", got)
	}
}
