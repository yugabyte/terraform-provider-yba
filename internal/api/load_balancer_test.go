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

package api

import (
	"context"
	"net/http"
	"testing"

	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// TestUpdateLoadBalancerConfigTaskConflict: a 409 from update_lb_config must
// classify as a universe task conflict so DispatchAndWait retries it instead
// of hard-failing the apply.
func TestUpdateLoadBalancerConfigTaskConflict(t *testing.T) {
	vc, _ := newStubVanillaClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"success":false,"error":"Task UpdateLoadBalancerConfig ` +
			`cannot be queued on existing task EditUniverse"}`))
	})

	_, resp, err := vc.UpdateLoadBalancerConfig(
		context.Background(), "cust", "uni", map[string]interface{}{}, "token")
	if err == nil {
		t.Fatal("expected an error from the 409 stub")
	}
	if resp == nil || resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected the 409 response back, got %v", resp)
	}
	// Wrapper already closed the body; explicit Close satisfies bodyclose.
	_ = resp.Body.Close()
	if !utils.IsUniverseTaskConflict(resp, err) {
		t.Errorf("IsUniverseTaskConflict = false for update_lb_config 409, err=%v", err)
	}
}
