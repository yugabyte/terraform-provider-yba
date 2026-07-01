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
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// TelemetryProvider mirrors the YBA TelemetryProvider model. config is a
// free-form map: YBA's v1 spec declares TelemetryProviderConfig as
// discriminator-only (generated client has type only), and resolves the concrete
// schema from type at runtime — so we round-trip raw JSON to keep type-specific fields.
type TelemetryProvider struct {
	UUID         string                 `json:"uuid,omitempty"`
	CustomerUUID string                 `json:"customerUUID,omitempty"`
	Name         string                 `json:"name"`
	Config       map[string]interface{} `json:"config"`
	Tags         map[string]string      `json:"tags,omitempty"`
}

// ErrTelemetryProviderMissing is the typed sentinel for "provider already gone".
// YBA signals this as a 404 or a 400 with a body marker below; all collapse here
// so callers errors.Is instead of substring-matching bodies.
var ErrTelemetryProviderMissing = errors.New("telemetry provider does not exist")

// telemetryProviderMissingMarkers are body substrings YBA returns (as 400, not
// 404) when a provider is already gone, so body matching is unavoidable. Verified
// against server source; neither collides with the "...as it is in use." rejection.
var telemetryProviderMissingMarkers = []string{
	"does not exist",                  // DELETE path: "Telemetry Provider '<uuid>' does not exist."
	"Invalid Telemetry Provider UUID", // GET path: "Invalid Telemetry Provider UUID: <uuid>"
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
	resp, err := vc.makeRequest(ctx, http.MethodPost, url, bytes.NewBuffer(body), token)
	if err != nil {
		return nil, fmt.Errorf("create telemetry provider request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if httpErr := vanillaHTTPError(resp, "Telemetry Provider", "Create"); httpErr != nil {
		return nil, httpErr
	}
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	out := TelemetryProvider{}
	if err = json.Unmarshal(respBytes, &out); err != nil {
		return nil, fmt.Errorf("unmarshal telemetry provider response: %w", err)
	}
	return &out, nil
}

// GetTelemetryProvider fetches a provider by UUID. Missing providers (404, or a
// 4xx/5xx body matching telemetryProviderMissingMarkers) return
// ErrTelemetryProviderMissing so Read can drop it from state.
func (vc *VanillaClient) GetTelemetryProvider(
	ctx context.Context, cUUID, providerUUID, token string,
) (*TelemetryProvider, *http.Response, error) {
	url := fmt.Sprintf("api/v1/customers/%s/telemetry_provider/%s", cUUID, providerUUID)
	resp, err := vc.makeRequest(ctx, http.MethodGet, url, nil, token)
	if err != nil {
		return nil, nil, fmt.Errorf("get telemetry provider request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if utils.IsHTTPNotFound(resp) {
		return nil, resp, ErrTelemetryProviderMissing
	}
	if httpErr := vanillaHTTPError(resp, "Telemetry Provider", "Get"); httpErr != nil {
		if errorIndicatesProviderMissing(httpErr) {
			return nil, resp, ErrTelemetryProviderMissing
		}
		return nil, resp, httpErr
	}
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp, err
	}
	out := TelemetryProvider{}
	if err = json.Unmarshal(respBytes, &out); err != nil {
		return nil, resp, fmt.Errorf("unmarshal telemetry provider response: %w", err)
	}
	return &out, resp, nil
}

// ListTelemetryProviders returns every provider for the customer. YBA masks
// sensitive config here, so it's only good for name/metadata lookups, not secrets.
func (vc *VanillaClient) ListTelemetryProviders(
	ctx context.Context, cUUID, token string,
) ([]TelemetryProvider, error) {
	url := fmt.Sprintf("api/v1/customers/%s/telemetry_provider", cUUID)
	resp, err := vc.makeRequest(ctx, http.MethodGet, url, nil, token)
	if err != nil {
		return nil, fmt.Errorf("list telemetry providers request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if httpErr := vanillaHTTPError(resp, "Telemetry Provider", "List"); httpErr != nil {
		return nil, httpErr
	}
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	out := []TelemetryProvider{}
	if err = json.Unmarshal(respBytes, &out); err != nil {
		return nil, fmt.Errorf("unmarshal telemetry provider list response: %w", err)
	}
	return out, nil
}

// DeleteTelemetryProvider deletes a provider by UUID. Idempotent: 404 or a
// missing-marker body returns nil; other errors propagate.
func (vc *VanillaClient) DeleteTelemetryProvider(
	ctx context.Context, cUUID, providerUUID, token string,
) error {
	url := fmt.Sprintf("api/v1/customers/%s/telemetry_provider/%s", cUUID, providerUUID)
	resp, err := vc.makeRequest(ctx, http.MethodDelete, url, nil, token)
	if err != nil {
		return fmt.Errorf("delete telemetry provider request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if utils.IsHTTPNotFound(resp) {
		return nil
	}
	if httpErr := vanillaHTTPError(resp, "Telemetry Provider", "Delete"); httpErr != nil {
		if errorIndicatesProviderMissing(httpErr) {
			return nil
		}
		return httpErr
	}
	return nil
}

func errorIndicatesProviderMissing(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	for _, m := range telemetryProviderMissingMarkers {
		if strings.Contains(msg, m) {
			return true
		}
	}
	return false
}

func vanillaHTTPError(resp *http.Response, entityName, operation string) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	statusErr := fmt.Errorf("HTTP %d", resp.StatusCode)
	return utils.ErrorFromHTTPResponse(
		resp, statusErr, utils.ResourceEntity, entityName, operation)
}
