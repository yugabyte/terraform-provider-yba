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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// TelemetryProvider mirrors the YBA TelemetryProvider model. The `config`
// field is intentionally a free-form map because the YBA OpenAPI v1 spec
// declares `TelemetryProviderConfig` as a polymorphic discriminator-only
// schema (the generated platform-go-client therefore only contains the
// `type` field). YBA picks a concrete config schema at runtime from the
// embedded `type` value (DATA_DOG, OTLP, AWS_CLOUD_WATCH,
// GCP_CLOUD_MONITORING, SPLUNK, LOKI, DYNATRACE), so we round-trip raw JSON
// to preserve the provider-specific fields.
type TelemetryProvider struct {
	UUID         string                 `json:"uuid,omitempty"`
	CustomerUUID string                 `json:"customerUUID,omitempty"`
	Name         string                 `json:"name"`
	Config       map[string]interface{} `json:"config"`
	Tags         map[string]string      `json:"tags,omitempty"`
}

// CreateTelemetryProvider creates a new telemetry provider.
func (vc *VanillaClient) CreateTelemetryProvider(
	ctx context.Context, cUUID string, token string, provider TelemetryProvider,
) (*TelemetryProvider, error) {
	url := fmt.Sprintf("api/v1/customers/%s/telemetry_provider", cUUID)
	body, err := json.Marshal(provider)
	if err != nil {
		return nil, err
	}
	resp, err := vc.makeRequest(http.MethodPost, url, bytes.NewBuffer(body), token)
	if err != nil {
		return nil, fmt.Errorf("create telemetry provider request failed: %w", err)
	}
	defer resp.Body.Close()
	if err := checkHTTPStatus(resp, "Create Telemetry Provider"); err != nil {
		return nil, err
	}
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	out := TelemetryProvider{}
	if err := json.Unmarshal(respBytes, &out); err != nil {
		return nil, fmt.Errorf("unmarshal telemetry provider response: %w", err)
	}
	return &out, nil
}

// GetTelemetryProvider fetches a single telemetry provider by UUID.
func (vc *VanillaClient) GetTelemetryProvider(
	ctx context.Context, cUUID, providerUUID, token string,
) (*TelemetryProvider, *http.Response, error) {
	url := fmt.Sprintf("api/v1/customers/%s/telemetry_provider/%s", cUUID, providerUUID)
	resp, err := vc.makeRequest(http.MethodGet, url, nil, token)
	if err != nil {
		return nil, nil, fmt.Errorf("get telemetry provider request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, resp, nil
	}
	if err := checkHTTPStatus(resp, "Get Telemetry Provider"); err != nil {
		return nil, resp, err
	}
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp, err
	}
	out := TelemetryProvider{}
	if err := json.Unmarshal(respBytes, &out); err != nil {
		return nil, resp, fmt.Errorf("unmarshal telemetry provider response: %w", err)
	}
	return &out, resp, nil
}

// DeleteTelemetryProvider deletes a telemetry provider by UUID.
func (vc *VanillaClient) DeleteTelemetryProvider(
	ctx context.Context, cUUID, providerUUID, token string,
) error {
	url := fmt.Sprintf("api/v1/customers/%s/telemetry_provider/%s", cUUID, providerUUID)
	resp, err := vc.makeRequest(http.MethodDelete, url, nil, token)
	if err != nil {
		return fmt.Errorf("delete telemetry provider request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil
	}
	return checkHTTPStatus(resp, "Delete Telemetry Provider")
}

// checkHTTPStatus returns an error containing the response body when the
// status code is not in the 2xx range. Used by the raw telemetry helpers
// that bypass the generated client.
func checkHTTPStatus(resp *http.Response, op string) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	body, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("%s failed (HTTP %d): %s", op, resp.StatusCode, string(body))
}
