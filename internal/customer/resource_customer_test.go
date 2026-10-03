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

package customer

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

	"github.com/yugabyte/terraform-provider-yba/internal/api"
)

// customerReadServer fakes the two YBA endpoints resourceCustomerRead hits:
// session_info accepts only validToken; api_login replies with loginStatus/loginBody.
func customerReadServer(
	t *testing.T,
	validToken string,
	loginStatus int,
	loginBody string) (*api.APIClient, *int) {
	t.Helper()
	loginCalls := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/session_info", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("X-AUTH-YW-API-TOKEN") != validToken {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"Invalid token"}`))
			return
		}
		_, _ = w.Write([]byte(
			`{"customerUUID":"cust-uuid","userUUID":"user-uuid","apiToken":"` +
				validToken + `"}`))
	})
	mux.HandleFunc("/api/v1/api_login", func(w http.ResponseWriter, _ *http.Request) {
		loginCalls++
		w.Header().Set("Content-Type", "application/json")
		if loginStatus != 0 {
			w.WriteHeader(loginStatus)
		}
		_, _ = w.Write([]byte(loginBody))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &api.APIClient{
		VanillaClient: &api.VanillaClient{
			Client: srv.Client(),
			Host:   strings.TrimPrefix(srv.URL, "http://"),
		},
	}, &loginCalls
}

func customerReadData(t *testing.T, email, password, token string) *schema.ResourceData {
	t.Helper()
	d := ResourceCustomer().TestResourceData()
	d.SetId("cust-uuid")
	for k, v := range map[string]string{
		"email": email, "password": password, "api_token": token,
	} {
		if err := d.Set(k, v); err != nil {
			t.Fatalf("set %s: %v", k, err)
		}
	}
	return d
}

// A rotated token must trigger re-login and land the fresh token in state.
func TestCustomerReadRefreshesRotatedToken(t *testing.T) {
	apiClient, loginCalls := customerReadServer(t, "fresh-token", 0,
		`{"apiToken":"fresh-token","customerUUID":"cust-uuid","userUUID":"user-uuid"}`)
	d := customerReadData(t, "admin@yugabyte.com", "Password123!", "stale-token")

	if diags := resourceCustomerRead(context.Background(), d, apiClient); diags.HasError() {
		t.Fatalf("read diags: %v", diags)
	}
	if got := d.Get("api_token"); got != "fresh-token" {
		t.Errorf("api_token = %q, want fresh-token", got)
	}
	if got := d.Get("cuuid"); got != "cust-uuid" {
		t.Errorf("cuuid = %q, want cust-uuid", got)
	}
	if *loginCalls != 1 {
		t.Errorf("login calls = %d, want 1", *loginCalls)
	}
}

// A valid token must pass through untouched, with no login call.
func TestCustomerReadKeepsValidToken(t *testing.T) {
	apiClient, loginCalls := customerReadServer(t, "live-token", 0, `{}`)
	d := customerReadData(t, "admin@yugabyte.com", "Password123!", "live-token")

	if diags := resourceCustomerRead(context.Background(), d, apiClient); diags.HasError() {
		t.Fatalf("read diags: %v", diags)
	}
	if got := d.Get("api_token"); got != "live-token" {
		t.Errorf("api_token = %q, want live-token", got)
	}
	if *loginCalls != 0 {
		t.Errorf("login calls = %d, want 0", *loginCalls)
	}
}

// A failed re-login (password drift) must fail the read, not mask it.
func TestCustomerReadFailsWhenReloginFails(t *testing.T) {
	apiClient, loginCalls := customerReadServer(t, "unreachable", http.StatusUnauthorized,
		`{"error":"Invalid Customer Credentials"}`)
	d := customerReadData(t, "admin@yugabyte.com", "WrongPassword!", "stale-token")

	diags := resourceCustomerRead(context.Background(), d, apiClient)
	if !diags.HasError() {
		t.Fatal("expected error when re-login fails")
	}
	if !strings.Contains(diags[0].Summary, "Re-login") {
		t.Errorf("unexpected error: %v", diags)
	}
	if *loginCalls != 1 {
		t.Errorf("login calls = %d, want 1", *loginCalls)
	}
}

// Without credentials in state (imported resource) the 401 must surface as-is.
func TestCustomerReadNoCredentialsSurfaces401(t *testing.T) {
	apiClient, loginCalls := customerReadServer(t, "unreachable", 0, `{}`)
	d := customerReadData(t, "", "", "stale-token")

	diags := resourceCustomerRead(context.Background(), d, apiClient)
	if !diags.HasError() {
		t.Fatal("expected error when no credentials are stored")
	}
	if !strings.Contains(diags[0].Summary, "401") {
		t.Errorf("unexpected error: %v", diags)
	}
	if *loginCalls != 0 {
		t.Errorf("login calls = %d, want 0", *loginCalls)
	}
}
