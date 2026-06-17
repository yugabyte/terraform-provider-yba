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

// TelemetryProvider mirrors the YBA TelemetryProvider model. The `config`
// field is intentionally a free-form map because the YBA OpenAPI v1 spec
// declares `TelemetryProviderConfig` as a polymorphic discriminator-only
// schema (the generated platform-go-client therefore only contains the
// `type` field). YBA picks a concrete config schema at runtime from the
// embedded `type` value (DATA_DOG, OTLP, AWS_CLOUDWATCH, GCP_CLOUD_MONITORING,
// SPLUNK, LOKI, DYNATRACE, S3), so we round-trip raw JSON to preserve the
// provider-specific fields.
type TelemetryProvider struct {
	UUID         string                 `json:"uuid,omitempty"`
	CustomerUUID string                 `json:"customerUUID,omitempty"`
	Name         string                 `json:"name"`
	Config       map[string]interface{} `json:"config"`
	Tags         map[string]string      `json:"tags,omitempty"`
}

// ErrTelemetryProviderMissing is the typed sentinel returned when YBA reports
// a telemetry provider as already gone. YBA delivers this signal across at
// least three different HTTP shapes (404, 400 with "does not exist", 400
// with "Invalid Telemetry Provider UUID"), all of which collapse into this
// sentinel so that callers can `errors.Is` against a single value instead
// of substring-matching response bodies themselves.
var ErrTelemetryProviderMissing = errors.New("telemetry provider does not exist")

// telemetryProviderMissingMarkers list the substrings YBA returns in the
// response body when the requested telemetry provider has already been
// removed. YBA frustratingly delivers these as HTTP 400 *and* HTTP 500 in
// different code paths instead of 404. Keep this list close to the typed
// sentinel above so callers never have to reason about the wire format.
var telemetryProviderMissingMarkers = []string{
	"does not exist",             // DELETE path
	"Invalid Telemetry Provider", // GET path: "Invalid Telemetry Provider UUID: ..."
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

// GetTelemetryProvider fetches a single telemetry provider by UUID.
//
// The lookup is idempotent against missing providers: a 404, or a 4xx/5xx
// whose body matches one of telemetryProviderMissingMarkers, returns
// (nil, resp, ErrTelemetryProviderMissing) so the resource's read flow can
// drop the resource from state by calling errors.Is.
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

// ListTelemetryProviders returns every telemetry provider configured for the
// customer. Sensitive config values are masked by YBA in this response, so it
// is only suitable for lookups by name / metadata (the data source), not for
// reconstructing a provider's secrets.
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

// DeleteTelemetryProvider deletes a telemetry provider by UUID. The delete
// is idempotent: a 404 response, or a 4xx whose body matches one of
// telemetryProviderMissingMarkers, returns nil so the destroy step can be
// retried safely. Other errors propagate verbatim with the same formatting
// the rest of the provider uses (utils.ErrorFromHTTPResponse).
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

// errorIndicatesProviderMissing reports whether err carries one of the YBA
// response bodies that mean "the requested telemetry provider is no longer
// present" — see telemetryProviderMissingMarkers for the list.
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

// vanillaHTTPError returns nil for 2xx responses and otherwise formats the
// response body with utils.ErrorFromHTTPResponse so the message matches the
// shape produced by every other resource in this provider. Pass a placeholder
// HTTP-status error rather than nil to keep the %w chain valid.
func vanillaHTTPError(resp *http.Response, entityName, operation string) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	statusErr := fmt.Errorf("HTTP %d", resp.StatusCode)
	return utils.ErrorFromHTTPResponse(
		resp, statusErr, utils.ResourceEntity, entityName, operation)
}
