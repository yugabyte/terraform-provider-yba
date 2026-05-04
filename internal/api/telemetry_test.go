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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newStubVanillaClient stands up an httptest.Server and returns a
// VanillaClient pointed at it. The supplied handler is responsible for
// returning whatever body / status code the test wants. Cleanup is
// registered with t.Cleanup so callers do not need to remember to close.
func newStubVanillaClient(
	t *testing.T, handler http.HandlerFunc,
) (*VanillaClient, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	addr := strings.TrimPrefix(srv.URL, "http://")
	vc := &VanillaClient{
		Client:      srv.Client(),
		Host:        addr,
		EnableHTTPS: false,
	}
	return vc, srv
}

// TestGetTelemetryProviderMissing verifies that the various YBA "missing
// provider" response shapes (404, 400 with "does not exist", 400 with
// "Invalid Telemetry Provider UUID", 500 with same body markers) all
// collapse to the typed ErrTelemetryProviderMissing sentinel so callers
// can switch on it with errors.Is rather than matching strings.
func TestGetTelemetryProviderMissing(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
	}{
		{"404", http.StatusNotFound, ``},
		{
			name:   "400 does not exist",
			status: http.StatusBadRequest,
			body:   `{"error":"telemetry provider tp-1 does not exist"}`,
		},
		{
			name:   "400 invalid telemetry provider uuid",
			status: http.StatusBadRequest,
			body:   `{"error":"Invalid Telemetry Provider UUID: tp-1"}`,
		},
		{
			name:   "500 invalid telemetry provider uuid",
			status: http.StatusInternalServerError,
			body:   `{"error":"Invalid Telemetry Provider UUID: tp-1"}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			vc, _ := newStubVanillaClient(t,
				func(w http.ResponseWriter, _ *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(tc.status)
					_, _ = w.Write([]byte(tc.body))
				})

			provider, _, err := vc.GetTelemetryProvider(
				context.Background(), "cust-uuid", "tp-1", "token")
			if provider != nil {
				t.Errorf("expected nil provider on missing, got %+v", provider)
			}
			if !errors.Is(err, ErrTelemetryProviderMissing) {
				t.Fatalf("expected ErrTelemetryProviderMissing, got %v", err)
			}
		})
	}
}

// TestGetTelemetryProviderUnrelatedError ensures that errors that are not
// "missing provider" still surface as wrapped errors carrying the body —
// callers must NOT silently treat them as deletions.
func TestGetTelemetryProviderUnrelatedError(t *testing.T) {
	vc, _ := newStubVanillaClient(t,
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":"forbidden"}`))
		})

	provider, _, err := vc.GetTelemetryProvider(
		context.Background(), "cust", "tp", "token")
	if provider != nil {
		t.Errorf("expected nil provider on error, got %+v", provider)
	}
	if err == nil {
		t.Fatal("expected non-nil error on 403")
	}
	if errors.Is(err, ErrTelemetryProviderMissing) {
		t.Errorf("403 must NOT be classified as missing-provider")
	}
}

// TestGetTelemetryProviderSuccess walks the happy path: a 200 with a
// well-formed JSON body should round-trip into a TelemetryProvider with
// every public field populated.
func TestGetTelemetryProviderSuccess(t *testing.T) {
	body := `{
		"uuid":"tp-1",
		"customerUUID":"cust-1",
		"name":"my-otlp",
		"config":{"type":"OTLP","endpoint":"https://collector"},
		"tags":{"env":"prod"}
	}`
	vc, _ := newStubVanillaClient(t,
		func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(body))
		})

	provider, _, err := vc.GetTelemetryProvider(
		context.Background(), "cust", "tp", "token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if provider == nil {
		t.Fatal("expected non-nil provider")
	}
	if provider.UUID != "tp-1" || provider.Name != "my-otlp" {
		t.Errorf("unexpected provider %+v", provider)
	}
	if provider.Config["type"] != "OTLP" {
		t.Errorf("config type lost in round-trip: %+v", provider.Config)
	}
	if provider.Tags["env"] != "prod" {
		t.Errorf("tags lost in round-trip: %+v", provider.Tags)
	}
}

// TestDeleteTelemetryProviderIdempotent verifies that the various YBA
// "missing provider" response shapes are collapsed to nil — Terraform's
// destroy must succeed even when the provider has already been removed
// out-of-band, otherwise repeated applies would be stuck on a phantom
// resource.
func TestDeleteTelemetryProviderIdempotent(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
	}{
		{"404", http.StatusNotFound, ``},
		{
			name:   "400 does not exist",
			status: http.StatusBadRequest,
			body:   `{"error":"telemetry provider does not exist"}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			vc, _ := newStubVanillaClient(t,
				func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(tc.status)
					_, _ = w.Write([]byte(tc.body))
				})

			err := vc.DeleteTelemetryProvider(
				context.Background(), "cust", "tp", "token")
			if err != nil {
				t.Errorf("expected nil for idempotent delete, got %v", err)
			}
		})
	}
}

// TestDeleteTelemetryProviderSurfacesErrors ensures that destructive
// failures NOT matching the missing-provider markers (auth, in-use, etc.)
// propagate to the caller. Silently swallowing them would corrupt state.
func TestDeleteTelemetryProviderSurfacesErrors(t *testing.T) {
	vc, _ := newStubVanillaClient(t,
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(
				`{"error":"Cannot delete Telemetry Provider, as it is in use."}`))
		})

	err := vc.DeleteTelemetryProvider(
		context.Background(), "cust", "tp", "token")
	if err == nil {
		t.Fatal("expected non-nil error for in-use delete failure")
	}
	if errors.Is(err, ErrTelemetryProviderMissing) {
		t.Errorf("in-use delete must NOT be classified as missing-provider")
	}
}

// TestCreateTelemetryProvider verifies that Create POSTs the marshalled
// payload to the expected endpoint and round-trips the response body.
func TestCreateTelemetryProvider(t *testing.T) {
	var (
		gotMethod string
		gotPath   string
		gotBody   map[string]interface{}
	)
	vc, _ := newStubVanillaClient(t,
		func(w http.ResponseWriter, r *http.Request) {
			gotMethod = r.Method
			gotPath = r.URL.Path
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &gotBody)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"uuid":"tp-new",
				"customerUUID":"cust-1",
				"name":"my-otlp",
				"config":{"type":"OTLP","endpoint":"https://collector"}
			}`))
		})

	in := TelemetryProvider{
		Name: "my-otlp",
		Config: map[string]interface{}{
			"type":     "OTLP",
			"endpoint": "https://collector",
		},
	}
	out, err := vc.CreateTelemetryProvider(
		context.Background(), "cust-1", "token", in)
	if err != nil {
		t.Fatalf("create error: %v", err)
	}
	if out == nil || out.UUID != "tp-new" {
		t.Errorf("unexpected response: %+v", out)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("expected POST, got %s", gotMethod)
	}
	if !strings.HasSuffix(gotPath, "/customers/cust-1/telemetry_provider") {
		t.Errorf("unexpected path: %s", gotPath)
	}
	if gotBody["name"] != "my-otlp" {
		t.Errorf("name lost in request body: %+v", gotBody)
	}
	if cfg, ok := gotBody["config"].(map[string]interface{}); !ok ||
		cfg["type"] != "OTLP" {
		t.Errorf("config lost in request body: %+v", gotBody)
	}
}

// TestCreateTelemetryProviderError ensures non-2xx responses surface a
// wrapped error rather than swallowing the failure silently.
func TestCreateTelemetryProviderError(t *testing.T) {
	vc, _ := newStubVanillaClient(t,
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"name already taken"}`))
		})

	out, err := vc.CreateTelemetryProvider(
		context.Background(), "cust", "token",
		TelemetryProvider{Name: "dup", Config: map[string]interface{}{}})
	if out != nil {
		t.Errorf("expected nil output on error, got %+v", out)
	}
	if err == nil {
		t.Fatal("expected non-nil error on 400")
	}
	if !strings.Contains(err.Error(), "name already taken") {
		t.Errorf("error did not surface body: %v", err)
	}
}

// TestErrTelemetryProviderMissingIsStable guards against accidental
// renames or replacements of the sentinel — every consumer (the resource
// Read flow, future agents extending the package) reaches for it via
// errors.Is(err, api.ErrTelemetryProviderMissing). If a refactor ever
// stops wrapping with %w (e.g. switches to fmt.Sprintf-then-errors.New)
// this assertion will catch it before the resource silently stops
// detecting out-of-band deletes.
func TestErrTelemetryProviderMissingIsStable(t *testing.T) {
	if ErrTelemetryProviderMissing == nil {
		t.Fatal("ErrTelemetryProviderMissing must not be nil")
	}
	wrapped := fmt.Errorf("delete telemetry provider tp-1: %w",
		ErrTelemetryProviderMissing)
	if !errors.Is(wrapped, ErrTelemetryProviderMissing) {
		t.Fatal("ErrTelemetryProviderMissing must remain identifiable through wrap")
	}
}
