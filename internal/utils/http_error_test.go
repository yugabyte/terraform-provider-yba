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
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func httpResponseWithBody(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

// YBA uses 401/403 for application-level rejections too (a feature disabled by
// runtime config, an endpoint restricted to Super Admins). The translated
// error must carry YBA's own message, not blame the API token.
func TestErrorFromHTTPResponseSurfacesAuthBodies(t *testing.T) {
	cases := []struct {
		name        string
		status      int
		body        string
		wantPart    string
		notWantPart string
	}{
		{
			name:        "401 with YBA message",
			status:      http.StatusUnauthorized,
			body:        `{"error":"Custom hooks is not enabled on this Anywhere instance"}`,
			wantPart:    "Custom hooks is not enabled",
			notWantPart: "the API token is invalid",
		},
		{
			name:        "403 with YBA message",
			status:      http.StatusForbidden,
			body:        `{"error":"Only Super Admins can perform this operation."}`,
			wantPart:    "Only Super Admins",
			notWantPart: "does not have permission",
		},
		{
			name:     "401 with empty body keeps token hint",
			status:   http.StatusUnauthorized,
			body:     ``,
			wantPart: "the API token is invalid, expired, or missing",
		},
		{
			name:     "403 with non-YBA body keeps generic message",
			status:   http.StatusForbidden,
			body:     `<html>gateway error</html>`,
			wantPart: "does not have permission",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := httpResponseWithBody(tc.status, tc.body)
			defer func() { _ = resp.Body.Close() }()
			err := ErrorFromHTTPResponse(
				resp, errors.New("api error"), ResourceEntity, "Hook", "Create")
			if err == nil {
				t.Fatal("expected non-nil error")
			}
			if !strings.Contains(err.Error(), tc.wantPart) {
				t.Errorf("error %q must contain %q", err.Error(), tc.wantPart)
			}
			if tc.notWantPart != "" && strings.Contains(err.Error(), tc.notWantPart) {
				t.Errorf("error %q must not contain %q", err.Error(), tc.notWantPart)
			}
		})
	}
}
