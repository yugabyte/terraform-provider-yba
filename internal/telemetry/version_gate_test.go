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

// The gate always checks the resource floor, checks the server-log minimum
// only when a server-log block is present, names the failing block, and stays
// out of the way when the version is unknown or unparseable. Version
// comparison itself is TestMeetsMinimum's job.
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
		{"server-log block below its minimum", "2.31.0.0-b385", block("master_logs"),
			"master_logs requires YugabyteDB Anywhere 2.31.0.0-b386"},
		{"server-log block at its minimum", "2.31.0.0-b386", block("master_logs"), ""},
		{"no server-log block skips its minimum", "2.31.0.0-b385", block("metrics"), ""},
		{"below the unified API floor", "2026.1.1.0-b91", block("metrics"),
			"yba_universe_telemetry_config requires YugabyteDB Anywhere 2026.1.2.0-b35"},
		{"unparseable version is let through", "dev-build", block("master_logs"), ""},
		{"unknown version skips the gate", "", block("master_logs"), ""},
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
}
