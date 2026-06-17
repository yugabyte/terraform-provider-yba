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
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yugabyte/terraform-provider-yba/internal/api"
)

// telemetryListClient stands up an httptest server whose telemetry_provider
// list endpoint returns the supplied JSON body, and returns an APIClient whose
// VanillaClient points at it.
func telemetryListClient(t *testing.T, status int, body string) *api.APIClient {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if status != 0 {
				w.WriteHeader(status)
			}
			_, _ = w.Write([]byte(body))
		}))
	t.Cleanup(srv.Close)
	return &api.APIClient{
		VanillaClient: &api.VanillaClient{
			Client: srv.Client(), Host: strings.TrimPrefix(srv.URL, "http://"),
		},
		CustomerID: "cust",
		APIKey:     "token",
	}
}

// TestDataSourceTelemetryProviderByName looks up a provider by name and
// exposes its UUID/type/tags.
func TestDataSourceTelemetryProviderByName(t *testing.T) {
	apiClient := telemetryListClient(t, 0, `[
		{"uuid":"tp-1","name":"dd","config":{"type":"DATA_DOG"},"tags":{"env":"prod"}},
		{"uuid":"tp-2","name":"otlp","config":{"type":"OTLP"}}
	]`)

	res := DataSourceTelemetryProvider()
	d := res.TestResourceData()
	_ = d.Set("name", "otlp")

	if diags := dataSourceTelemetryProviderRead(
		context.Background(), d, apiClient); diags.HasError() {
		t.Fatalf("read diags: %v", diags)
	}
	if d.Id() != "tp-2" {
		t.Errorf("id = %q want tp-2", d.Id())
	}
	if got := d.Get("type"); got != "OTLP" {
		t.Errorf("type = %v want OTLP", got)
	}
}

// TestDataSourceTelemetryProviderTags verifies tags round-trip for the matched
// provider.
func TestDataSourceTelemetryProviderTags(t *testing.T) {
	apiClient := telemetryListClient(t, 0,
		`[{"uuid":"tp-1","name":"dd","config":{"type":"DATA_DOG"},"tags":{"env":"prod"}}]`)
	res := DataSourceTelemetryProvider()
	d := res.TestResourceData()
	_ = d.Set("name", "dd")
	if diags := dataSourceTelemetryProviderRead(
		context.Background(), d, apiClient); diags.HasError() {
		t.Fatalf("read diags: %v", diags)
	}
	if got := d.Get("tags.env"); got != "prod" {
		t.Errorf("tags.env = %v want prod", got)
	}
}

// TestDataSourceTelemetryProviderNotFound errors cleanly when no provider
// matches the requested name.
func TestDataSourceTelemetryProviderNotFound(t *testing.T) {
	apiClient := telemetryListClient(t, 0,
		`[{"uuid":"tp-1","name":"dd","config":{"type":"DATA_DOG"}}]`)
	res := DataSourceTelemetryProvider()
	d := res.TestResourceData()
	_ = d.Set("name", "missing")
	diags := dataSourceTelemetryProviderRead(context.Background(), d, apiClient)
	if !diags.HasError() {
		t.Fatal("expected error when no provider matches the name")
	}
	if !strings.Contains(diags[0].Summary, "no telemetry provider found") {
		t.Errorf("unexpected error: %v", diags)
	}
}

// TestDataSourceTelemetryProviderListError surfaces an underlying list failure
// instead of reporting "not found".
func TestDataSourceTelemetryProviderListError(t *testing.T) {
	apiClient := telemetryListClient(t, http.StatusInternalServerError,
		`{"error":"boom"}`)
	res := DataSourceTelemetryProvider()
	d := res.TestResourceData()
	_ = d.Set("name", "dd")
	if diags := dataSourceTelemetryProviderRead(
		context.Background(), d, apiClient); !diags.HasError() {
		t.Fatal("expected error when the list call fails")
	}
}
