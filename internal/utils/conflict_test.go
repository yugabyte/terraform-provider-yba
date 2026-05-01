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

package utils

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	client "github.com/yugabyte/platform-go-client"
)

// TestIsUniverseTaskConflictResponse covers the cheap response-only paths of
// IsUniverseTaskConflict that do not require a real *GenericOpenAPIError
// (the generated client error type has unexported fields and cannot be
// constructed from outside the client package).
func TestIsUniverseTaskConflictResponse(t *testing.T) {
	cases := []struct {
		name string
		resp *http.Response
		err  error
		want bool
	}{
		{"nil response", nil, errors.New("boom"), false},
		{"non-409 response", &http.Response{StatusCode: http.StatusInternalServerError}, errors.New("boom"), false},
		{"409 plain error has no body", &http.Response{StatusCode: http.StatusConflict}, errors.New("409 Conflict"), false},
		{"409 nil error", &http.Response{StatusCode: http.StatusConflict}, nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsUniverseTaskConflict(tc.resp, tc.err); got != tc.want {
				t.Errorf("IsUniverseTaskConflict = %v want %v", got, tc.want)
			}
		})
	}
}

// TestIsUniverseTaskConflictRealClient drives the helper end-to-end through
// the generated client so that body extraction is exercised against the real
// *GenericOpenAPIError type that appears at runtime. Each case stands up a
// throwaway HTTP server that returns a YBA-shaped 409 response and verifies
// the helper either matches or rejects it appropriately.
func TestIsUniverseTaskConflictRealClient(t *testing.T) {
	cases := []struct {
		name string
		body string
		want bool
	}{
		{
			name: "cannot be queued",
			body: `{"error":"Task ConfigureExportTelemetryConfig cannot be queued on existing task ConfigureExportTelemetryConfig"}`,
			want: true,
		},
		{
			name: "locked state",
			body: `{"error":"Cannot run Backup task since the universe U is currently in a locked state."}`,
			want: true,
		},
		{
			name: "unrelated 409",
			body: `{"error":"Some other 4xx that is not a universe task conflict"}`,
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusConflict)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			cfg := client.NewConfiguration()
			cfg.Scheme = "http"
			cfg.Host = srv.Listener.Addr().String()
			c := client.NewAPIClient(cfg)

			_, resp, err := c.SessionManagementAPI.AppVersion(context.Background()).Execute()
			if err == nil {
				t.Fatalf("expected an error from 409 stub server")
			}
			if resp == nil || resp.StatusCode != http.StatusConflict {
				t.Fatalf("expected HTTP 409, got resp=%v", resp)
			}
			if got := IsUniverseTaskConflict(resp, err); got != tc.want {
				t.Errorf("IsUniverseTaskConflict = %v want %v\nbody=%s", got, tc.want, tc.body)
			}
			// Also verify wrapped errors still match (DispatchAndWait wraps
			// errors via fmt.Errorf("...: %w", err) before returning).
			wrapped := fmt.Errorf("dispatch failed: %w", err)
			if got := IsUniverseTaskConflict(resp, wrapped); got != tc.want {
				t.Errorf("IsUniverseTaskConflict (wrapped) = %v want %v", got, tc.want)
			}
		})
	}
}
