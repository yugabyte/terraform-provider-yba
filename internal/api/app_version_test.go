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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	client "github.com/yugabyte/platform-go-client"
)

func appVersionClient(t *testing.T, handler http.HandlerFunc) (*APIClient, *int) {
	t.Helper()
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	cfg := client.NewConfiguration()
	cfg.Host = strings.TrimPrefix(srv.URL, "http://")
	cfg.Scheme = "http"
	return &APIClient{YugawareClient: client.NewAPIClient(cfg)}, &hits
}

func TestAppVersionFetchesOnceAndCaches(t *testing.T) {
	c, hits := appVersionClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/app_version" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"2.31.0.0-b395"}`))
	})
	for i := 0; i < 2; i++ {
		got, err := c.AppVersion(context.Background())
		if err != nil {
			t.Fatalf("call %d: %v", i+1, err)
		}
		if got != "2.31.0.0-b395" {
			t.Fatalf("call %d: version = %q", i+1, got)
		}
	}
	if *hits != 1 {
		t.Errorf("app_version fetched %d times, want 1", *hits)
	}
}

func TestAppVersionErrorIsNotCached(t *testing.T) {
	c, hits := appVersionClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	for i := 0; i < 2; i++ {
		if _, err := c.AppVersion(context.Background()); err == nil {
			t.Fatalf("call %d: expected an error", i+1)
		}
	}
	if *hits != 2 {
		t.Errorf("a failed fetch must retry next time, got %d hits", *hits)
	}
}

func TestAppVersionMissingFieldIsError(t *testing.T) {
	c, _ := appVersionClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"build":"x"}`))
	})
	if _, err := c.AppVersion(context.Background()); err == nil {
		t.Error("expected an error when the response has no version")
	}
}

// A client with no server (bootstrap mode, unit tests) reports "" so gates
// skip; a seeded value is served without a server.
func TestAppVersionWithoutServer(t *testing.T) {
	c := &APIClient{}
	if got, err := c.AppVersion(context.Background()); err != nil || got != "" {
		t.Fatalf("bare client: (%q, %v), want (\"\", nil)", got, err)
	}
	c.SetAppVersion("2026.1.2.0-b84")
	if got, _ := c.AppVersion(context.Background()); got != "2026.1.2.0-b84" {
		t.Errorf("seeded version = %q", got)
	}
}
