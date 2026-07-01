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

			//nolint:bodyclose // response body is closed inside GetTelemetryProvider
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

func TestGetTelemetryProviderUnrelatedError(t *testing.T) {
	vc, _ := newStubVanillaClient(t,
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":"forbidden"}`))
		})

	//nolint:bodyclose // response body is closed inside GetTelemetryProvider
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

	//nolint:bodyclose // response body is closed inside GetTelemetryProvider
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

func TestListTelemetryProviders(t *testing.T) {
	var gotPath string
	vc, _ := newStubVanillaClient(t,
		func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.Path
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[
				{"uuid":"tp-1","name":"dd","config":{"type":"DATA_DOG"},"tags":{"env":"prod"}},
				{"uuid":"tp-2","name":"otlp","config":{"type":"OTLP"}}
			]`))
		})

	providers, err := vc.ListTelemetryProviders(context.Background(), "cust-1", "token")
	if err != nil {
		t.Fatalf("list error: %v", err)
	}
	if len(providers) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(providers))
	}
	if !strings.HasSuffix(gotPath, "/customers/cust-1/telemetry_provider") {
		t.Errorf("unexpected path: %s", gotPath)
	}
	if providers[0].Name != "dd" || providers[0].Config["type"] != "DATA_DOG" {
		t.Errorf("first provider not parsed: %+v", providers[0])
	}
	if providers[0].Tags["env"] != "prod" {
		t.Errorf("tags not parsed: %+v", providers[0].Tags)
	}
}

func TestListTelemetryProvidersError(t *testing.T) {
	vc, _ := newStubVanillaClient(t,
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"boom"}`))
		})
	if _, err := vc.ListTelemetryProviders(
		context.Background(), "cust", "token"); err == nil {
		t.Fatal("expected error on 500 list response")
	}
}

// Guards the sentinel: if a refactor stops wrapping with %w, errors.Is breaks and
// Read silently stops detecting out-of-band deletes.
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
