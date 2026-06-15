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

package runtimeconfig_test

import (
	"context"
	"testing"

	"github.com/yugabyte/terraform-provider-yba/internal/runtimeconfig"
)

const importGlobalScope = "00000000-0000-0000-0000-000000000000"

// TestRuntimeConfigImportIDValid covers the supported import ID shapes:
// "<key>" (global scope assumed) and "<scope-uuid>/<key>".
func TestRuntimeConfigImportIDValid(t *testing.T) {
	cases := []struct {
		name, id, wantScope, wantKey, wantID string
	}{
		{
			name:      "key only assumes global scope",
			id:        "yb.telemetry.allow_s3",
			wantScope: importGlobalScope,
			wantKey:   "yb.telemetry.allow_s3",
			wantID:    importGlobalScope + "/yb.telemetry.allow_s3",
		},
		{
			name:      "explicit global scope and key",
			id:        importGlobalScope + "/yb.telemetry.allow_s3",
			wantScope: importGlobalScope,
			wantKey:   "yb.telemetry.allow_s3",
			wantID:    importGlobalScope + "/yb.telemetry.allow_s3",
		},
		{
			name:      "non-global scope and key",
			id:        "11111111-1111-1111-1111-111111111111/yb.some.key",
			wantScope: "11111111-1111-1111-1111-111111111111",
			wantKey:   "yb.some.key",
			wantID:    "11111111-1111-1111-1111-111111111111/yb.some.key",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := runtimeconfig.ResourceRuntimeConfig()
			d := r.TestResourceData()
			d.SetId(tc.id)

			out, err := r.Importer.StateContext(context.Background(), d, nil)
			if err != nil {
				t.Fatalf("unexpected error importing %q: %v", tc.id, err)
			}
			if len(out) != 1 {
				t.Fatalf("got %d resource data, want 1", len(out))
			}
			rd := out[0]
			if got := rd.Get("scope").(string); got != tc.wantScope {
				t.Errorf("scope = %q, want %q", got, tc.wantScope)
			}
			if got := rd.Get("key").(string); got != tc.wantKey {
				t.Errorf("key = %q, want %q", got, tc.wantKey)
			}
			if got := rd.Id(); got != tc.wantID {
				t.Errorf("id = %q, want %q", got, tc.wantID)
			}
		})
	}
}

// TestRuntimeConfigImportIDInvalid covers malformed IDs that must be rejected
// up front rather than producing a resource with an empty scope or key that
// only fails on the next refresh.
func TestRuntimeConfigImportIDInvalid(t *testing.T) {
	cases := []struct{ name, id string }{
		{"empty id", ""},
		{"trailing slash leaves empty key", "11111111-1111-1111-1111-111111111111/"},
		{"leading slash leaves empty scope", "/yb.telemetry.allow_s3"},
		{"only a slash", "/"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := runtimeconfig.ResourceRuntimeConfig()
			d := r.TestResourceData()
			d.SetId(tc.id)

			if _, err := r.Importer.StateContext(context.Background(), d, nil); err == nil {
				t.Errorf("import of %q succeeded, want an error", tc.id)
			}
		})
	}
}
