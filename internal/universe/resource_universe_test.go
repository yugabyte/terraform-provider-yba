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

package universe

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	client "github.com/yugabyte/platform-go-client"

	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// YBA rejects a GFlags upgrade that changes nothing. Only when the request
// changes a cluster that YBA returned with REDACTED gflags does that rejection
// confirm that YBA holds the configured values, and the dispatch complete.
func TestDispatchGFlagsUpgradeNoChanges(t *testing.T) {
	const noChanges = `{"success":false,` +
		`"error":"No changes in gflags (modify specificGflags in cluster)"}`
	cases := []struct {
		name     string
		status   int
		body     string
		redacted bool
		wantErr  bool
	}{
		{"no changes, REDACTED gflags", http.StatusBadRequest, noChanges, true, false},
		{"no changes, nothing redacted", http.StatusBadRequest, noChanges, false, true},
		{
			"another 400", http.StatusBadRequest,
			`{"success":false,"error":"Cannot delete gFlags through non-restart upgrade option."}`,
			true, true,
		},
		{"marker on a 500", http.StatusInternalServerError, noChanges, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(tc.status)
					_, _ = w.Write([]byte(tc.body))
				}),
			)
			defer srv.Close()
			cfg := client.NewConfiguration()
			cfg.Scheme = "http"
			cfg.Host = srv.Listener.Addr().String()

			c := client.NewAPIClient(cfg)
			ctx, req := context.Background(), client.GFlagsUpgradeParams{}

			taskUUID, _, err := dispatchGFlagsUpgrade(ctx, c, "c", "u", req, tc.redacted)()

			if (err != nil) != tc.wantErr || taskUUID != "" {
				t.Errorf("task %q, err %v; want error %v and no task", taskUUID, err, tc.wantErr)
			}
		})
	}
}

// A valid on-prem cluster must pass: a check that always fails blocks every
// plan for an on-prem universe. Each on-prem rule fails on its own, and other
// provider types skip the rules.
func TestValidateOnPremUserIntent(t *testing.T) {
	intent := func(
		providerType string, tags map[string]string, mountPoints, storageType string,
	) client.UserIntent {
		di := &client.DeviceInfo{}
		if mountPoints != "" {
			di.MountPoints = utils.GetStringPointer(mountPoints)
		}
		if storageType != "" {
			di.StorageType = utils.GetStringPointer(storageType)
		}
		return client.UserIntent{
			ProviderType: utils.GetStringPointer(providerType),
			InstanceTags: &tags,
			DeviceInfo:   di,
		}
	}
	cases := []struct {
		name    string
		ui      client.UserIntent
		wantErr bool
	}{
		{"valid onprem", intent("onprem", nil, "/mnt/d0", ""), false},
		{"onprem with instance tags", intent("onprem", map[string]string{"k": "v"}, "/mnt/d0", ""),
			true},
		{"onprem without mount points", intent("onprem", nil, "", ""), true},
		{"onprem with storage type", intent("onprem", nil, "/mnt/d0", "GP3"), true},
		{
			"aws skips the onprem rules",
			intent("aws", map[string]string{"k": "v"}, "", "GP3"),
			false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateOnPremUserIntent(c.ui, "primary")
			if (err != nil) != c.wantErr {
				t.Fatalf("validateOnPremUserIntent() error = %v, wantErr %v", err, c.wantErr)
			}
		})
	}
}

// An edit of communication_ports keeps the ports that the block does not set: YBA resets a
// port that the request leaves out. A create (no current ports) sends only the set ports.
func TestBuildCommunicationPortsKeepsUnmodeledPorts(t *testing.T) {
	block := map[string]interface{}{
		"master_http_port": 7011, "master_rpc_port": 7100, "node_exporter_port": 9300,
		"redis_server_http_port": 11000, "redis_server_rpc_port": 6379,
		"tserver_http_port": 9000, "tserver_rpc_port": 9100,
		"yql_server_http_port": 12000, "yql_server_rpc_port": 9042,
		"ysql_server_http_port": 13000, "ysql_server_rpc_port": 5433,
		"yb_controller_rpc_port": 0,
	}
	current := &client.CommunicationPorts{
		MasterHttpPort:            utils.GetInt32Pointer(7000),
		OtelCollectorMetricsPort:  utils.GetInt32Pointer(8899),
		YbControllerHttpPort:      utils.GetInt32Pointer(14010),
		YbControllerrRpcPort:      utils.GetInt32Pointer(18010),
		InternalYsqlServerRpcPort: utils.GetInt32Pointer(6440),
	}

	got := buildCommunicationPorts(current, block)
	if got.GetMasterHttpPort() != 7011 || got.GetOtelCollectorMetricsPort() != 8899 ||
		got.GetYbControllerHttpPort() != 14010 || got.GetYbControllerrRpcPort() != 18010 ||
		got.GetInternalYsqlServerRpcPort() != 6440 {
		t.Errorf("edit ports = %+v", got)
	}
	if current.GetMasterHttpPort() != 7000 {
		t.Error("buildCommunicationPorts changed the current ports")
	}
	// A port the block leaves out (0) is not sent on create, so YBA uses its default.
	if got := buildCommunicationPorts(nil, block); got.OtelCollectorMetricsPort != nil ||
		got.YbControllerrRpcPort != nil || got.GetMasterHttpPort() != 7011 {
		t.Errorf("create ports = %+v", got)
	}
}
