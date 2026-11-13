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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	client "github.com/yugabyte/platform-go-client"
)

func TestIsResourceGone(t *testing.T) {
	const id = "db9d2a4e-e5fa-42d8-8c92-f5fc7bf71606"
	cases := []struct {
		name   string
		status int
		body   string
		want   bool
	}{
		{
			name:   "user gone",
			status: http.StatusBadRequest,
			body:   `{"success":false,"error":"Invalid User UUID: '` + id + `' for customer: 'c'."}`,
			want:   true,
		},
		{
			name:   "node gone",
			status: http.StatusBadRequest,
			body:   `{"success":false,"error":"Invalid node UUID: ` + id + `"}`,
			want:   true,
		},
		{
			name:   "universe gone",
			status: http.StatusBadRequest,
			body:   `{"success":false,"error":"Cannot find universe ` + id + `"}`,
			want:   true,
		},
		{
			name:   "404",
			status: http.StatusNotFound,
			body:   `{"success":false,"error":"Invalid Hook UUID: h"}`,
			want:   true,
		},
		{
			name:   "parent customer UUID",
			status: http.StatusBadRequest,
			body:   `{"success":false,"error":"Invalid Customer UUID:c"}`,
			want:   false,
		},
		{
			name:   "unrelated 400",
			status: http.StatusBadRequest,
			body:   `{"success":false,"error":"Invalid role for ` + id + `"}`,
			want:   false,
		},
		{
			name:   "marker on a non-400",
			status: http.StatusInternalServerError,
			body:   `{"success":false,"error":"Invalid User UUID: ` + id + `"}`,
			want:   false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(tc.status)
					_, _ = w.Write([]byte(tc.body))
				}),
			)
			defer srv.Close()

			cfg := client.NewConfiguration()
			cfg.Scheme = "http"
			cfg.Host = srv.Listener.Addr().String()
			c := client.NewAPIClient(cfg)

			_, resp, err := c.UserManagementAPI.GetUserDetails(
				context.Background(), "c", id).Execute()
			if err == nil {
				t.Fatal("expected an error from the stub server")
			}
			if got := IsResourceGone(resp, err, id); got != tc.want {
				t.Errorf("IsResourceGone = %v want %v", got, tc.want)
			}
			wrapped := ErrorFromHTTPResponse(resp, err, ResourceEntity, "User", "Read")
			if got := IsResourceGone(resp, wrapped, id); got != tc.want {
				t.Errorf("IsResourceGone (wrapped) = %v want %v", got, tc.want)
			}
			if !strings.Contains(wrapped.Error(), "Invalid") &&
				!strings.Contains(wrapped.Error(), "Cannot find") {
				t.Errorf("YBA message lost from the error: %v", wrapped)
			}
		})
	}
}
