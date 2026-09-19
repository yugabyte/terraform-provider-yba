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

// Hermetic CRUD tests: they drive the resource lifecycle functions against a
// stub YBA over httptest, covering the scope bookkeeping most likely to
// regress — find-or-create and adopt on attach, garbage collection of a scope
// its last hook leaves, and reads that reconstruct or drop the binding.
package hooks

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

	"github.com/yugabyte/terraform-provider-yba/internal/api"
)

func newHookTestClient(t *testing.T, handler http.HandlerFunc) *api.APIClient {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return &api.APIClient{
		VanillaClient: &api.VanillaClient{
			Client:      srv.Client(),
			Host:        strings.TrimPrefix(srv.URL, "http://"),
			EnableHTTPS: false,
		},
		CustomerID: "cust-1",
		APIKey:     "token",
	}
}

const stubHookJSON = `{
	"uuid":"h-1","name":"10-mount.sh","executionLang":"Bash",
	"hookText":"#!/bin/bash\necho hi\n","useSudo":true,
	"runtimeArgs":{"DEVICE":"/dev/sdb"}
}`

// stubHookData mirrors stubHookJSON as resource fields, plus a global
// ApiTriggered binding.
func stubHookData(t *testing.T, res *schema.Resource) *schema.ResourceData {
	t.Helper()
	d := res.TestResourceData()
	for k, v := range map[string]interface{}{
		"name":           "10-mount.sh",
		"execution_lang": "Bash",
		"hook_text":      "#!/bin/bash\necho hi\n",
		"use_sudo":       true,
		"runtime_args":   map[string]interface{}{"DEVICE": "/dev/sdb"},
		"trigger_type":   "ApiTriggered",
	} {
		if err := d.Set(k, v); err != nil {
			t.Fatalf("set %s: %v", k, err)
		}
	}
	return d
}

const (
	listHookScopesPath  = "GET /api/v1/customers/cust-1/hook_scopes"
	createHookScopePath = "POST /api/v1/customers/cust-1/hook_scopes"
	attachPath          = "POST /api/v1/customers/cust-1/hook_scopes/s-1/hooks/h-1"
	deleteScopePath     = "DELETE /api/v1/customers/cust-1/hook_scopes/s-1"
	deleteHookPath      = "DELETE /api/v1/customers/cust-1/hooks/h-1"
)

func countRequests(log []string, want string) int {
	n := 0
	for _, req := range log {
		if req == want {
			n++
		}
	}
	return n
}

func TestHookCreateCreatesScopeAndAttaches(t *testing.T) {
	var scopeCreated, attached bool
	var requests []string
	apiClient := newHookTestClient(t,
		func(w http.ResponseWriter, r *http.Request) {
			requests = append(requests, r.Method+" "+r.URL.Path)
			w.Header().Set("Content-Type", "application/json")
			switch {
			case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/hooks"):
				_, _ = w.Write([]byte(stubHookJSON))
			case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/hooks"):
				_, _ = w.Write([]byte("[" + stubHookJSON + "]"))
			case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/hook_scopes"):
				switch {
				case !scopeCreated:
					_, _ = w.Write([]byte(`[]`))
				case !attached:
					_, _ = w.Write([]byte(`[{"uuid":"s-1","triggerType":"ApiTriggered"}]`))
				default:
					_, _ = w.Write([]byte(
						`[{"uuid":"s-1","triggerType":"ApiTriggered","hooks":["h-1"]}]`))
				}
			case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/hook_scopes"):
				scopeCreated = true
				_, _ = w.Write([]byte(`{"uuid":"s-1","triggerType":"ApiTriggered"}`))
			case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/hooks/"):
				attached = true
				_, _ = w.Write([]byte(
					`{"uuid":"s-1","triggerType":"ApiTriggered","hooks":["h-1"]}`))
			default:
				t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			}
		})

	d := stubHookData(t, ResourceHook())
	if diags := resourceHookCreate(context.Background(), d, apiClient); diags.HasError() {
		t.Fatalf("create returned diags: %v", diags)
	}
	if d.Id() != "h-1" {
		t.Errorf("id = %q, want h-1", d.Id())
	}
	if got := d.Get("trigger_type"); got != "ApiTriggered" {
		t.Errorf("trigger_type not read back from scope: %v", got)
	}
	if countRequests(requests, createHookScopePath) != 1 {
		t.Errorf("expected exactly one scope create, got requests %v", requests)
	}
	if countRequests(requests, attachPath) != 1 {
		t.Errorf("expected exactly one attach, got requests %v", requests)
	}
}

func TestHookCreateAdoptsExistingScope(t *testing.T) {
	var attached bool
	var requests []string
	apiClient := newHookTestClient(t,
		func(w http.ResponseWriter, r *http.Request) {
			requests = append(requests, r.Method+" "+r.URL.Path)
			w.Header().Set("Content-Type", "application/json")
			switch {
			case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/hooks"):
				_, _ = w.Write([]byte(stubHookJSON))
			case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/hooks"):
				_, _ = w.Write([]byte("[" + stubHookJSON + "]"))
			case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/hook_scopes"):
				if attached {
					_, _ = w.Write([]byte(
						`[{"uuid":"s-1","triggerType":"ApiTriggered","hooks":["h-1"]}]`))
				} else {
					_, _ = w.Write([]byte(`[{"uuid":"s-1","triggerType":"ApiTriggered"}]`))
				}
			case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/hooks/"):
				attached = true
				_, _ = w.Write([]byte(
					`{"uuid":"s-1","triggerType":"ApiTriggered","hooks":["h-1"]}`))
			default:
				t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			}
		})

	d := stubHookData(t, ResourceHook())
	if diags := resourceHookCreate(context.Background(), d, apiClient); diags.HasError() {
		t.Fatalf("create returned diags: %v", diags)
	}
	if countRequests(requests, createHookScopePath) != 0 {
		t.Errorf("an existing scope must be adopted, not recreated: %v", requests)
	}
	if countRequests(requests, attachPath) != 1 {
		t.Errorf("expected exactly one attach, got requests %v", requests)
	}
}

func TestHookReadPopulatesBinding(t *testing.T) {
	apiClient := newHookTestClient(t,
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if strings.HasSuffix(r.URL.Path, "/hook_scopes") {
				_, _ = w.Write([]byte(`[{
					"uuid":"s-1","triggerType":"PreNodeProvision",
					"providerUUID":"p-1","hooks":["h-1"]}]`))
				return
			}
			_, _ = w.Write([]byte("[" + stubHookJSON + "]"))
		})

	d := ResourceHook().TestResourceData()
	d.SetId("h-1")
	if diags := resourceHookRead(context.Background(), d, apiClient); diags.HasError() {
		t.Fatalf("read returned diags: %v", diags)
	}
	if got := d.Get("trigger_type"); got != "PreNodeProvision" {
		t.Errorf("trigger_type = %v, want PreNodeProvision", got)
	}
	if got := d.Get("provider_uuid"); got != "p-1" {
		t.Errorf("provider_uuid = %v, want p-1", got)
	}
	if got := d.Get("hook_text"); got != "#!/bin/bash\necho hi\n" {
		t.Errorf("hook_text not refreshed: %q", got)
	}
	if got := d.Get("runtime_args.DEVICE"); got != "/dev/sdb" {
		t.Errorf("runtime_args not refreshed: %v", got)
	}
}

func TestHookReadUnattachedClearsTrigger(t *testing.T) {
	apiClient := newHookTestClient(t,
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if strings.HasSuffix(r.URL.Path, "/hook_scopes") {
				_, _ = w.Write([]byte(`[]`))
				return
			}
			_, _ = w.Write([]byte("[" + stubHookJSON + "]"))
		})

	d := stubHookData(t, ResourceHook())
	d.SetId("h-1")
	if diags := resourceHookRead(context.Background(), d, apiClient); diags.HasError() {
		t.Fatalf("read returned diags: %v", diags)
	}
	if d.Id() != "h-1" {
		t.Errorf("an unattached hook still exists and must stay in state, id = %q", d.Id())
	}
	if got := d.Get("trigger_type"); got != "" {
		t.Errorf("trigger_type must read back empty for an unattached hook, got %v", got)
	}
}

func TestHookReadDropsMissingFromState(t *testing.T) {
	apiClient := newHookTestClient(t,
		func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[]`))
		})

	d := ResourceHook().TestResourceData()
	d.SetId("h-gone")
	if diags := resourceHookRead(context.Background(), d, apiClient); diags.HasError() {
		t.Fatalf("read returned diags: %v", diags)
	}
	if d.Id() != "" {
		t.Errorf("missing hook must be dropped from state, id = %q", d.Id())
	}
}

// Deleting the only hook of a scope must remove the scope with it (via YBA's
// cascade) instead of leaving an empty scope behind.
func TestHookDeleteCascadesSingleUseScope(t *testing.T) {
	var requests []string
	apiClient := newHookTestClient(t,
		func(w http.ResponseWriter, r *http.Request) {
			requests = append(requests, r.Method+" "+r.URL.Path)
			w.Header().Set("Content-Type", "application/json")
			switch r.Method {
			case http.MethodGet:
				_, _ = w.Write([]byte(
					`[{"uuid":"s-1","triggerType":"ApiTriggered","hooks":["h-1"]}]`))
			case http.MethodDelete:
				_, _ = w.Write([]byte(`{"success":true}`))
			default:
				t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			}
		})

	d := ResourceHook().TestResourceData()
	d.SetId("h-1")
	if diags := resourceHookDelete(context.Background(), d, apiClient); diags.HasError() {
		t.Fatalf("delete returned diags: %v", diags)
	}
	if d.Id() != "" {
		t.Errorf("id must be cleared after delete, got %q", d.Id())
	}
	if countRequests(requests, deleteScopePath) != 1 {
		t.Errorf("expected the single-use scope to be deleted, got %v", requests)
	}
	if countRequests(requests, deleteHookPath) != 0 {
		t.Errorf("the scope cascade removes the hook; no direct hook delete expected: %v",
			requests)
	}
}

// Deleting one of several hooks sharing a scope must leave the scope (and its
// other hooks) alone.
func TestHookDeleteKeepsSharedScope(t *testing.T) {
	var requests []string
	apiClient := newHookTestClient(t,
		func(w http.ResponseWriter, r *http.Request) {
			requests = append(requests, r.Method+" "+r.URL.Path)
			w.Header().Set("Content-Type", "application/json")
			switch r.Method {
			case http.MethodGet:
				_, _ = w.Write([]byte(
					`[{"uuid":"s-1","triggerType":"ApiTriggered","hooks":["h-1","h-2"]}]`))
			case http.MethodDelete:
				_, _ = w.Write([]byte(`{"success":true}`))
			default:
				t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			}
		})

	d := ResourceHook().TestResourceData()
	d.SetId("h-1")
	if diags := resourceHookDelete(context.Background(), d, apiClient); diags.HasError() {
		t.Fatalf("delete returned diags: %v", diags)
	}
	if countRequests(requests, deleteHookPath) != 1 {
		t.Errorf("expected the hook itself to be deleted, got %v", requests)
	}
	if countRequests(requests, deleteScopePath) != 0 {
		t.Errorf("a scope with other hooks must not be deleted: %v", requests)
	}
}

// Changing the trigger must move the hook: attach to the (created) new scope,
// then delete the old scope the hook was alone in.
func TestHookUpdateMovesBinding(t *testing.T) {
	var scopeCreated, attached bool
	var requests []string
	apiClient := newHookTestClient(t,
		func(w http.ResponseWriter, r *http.Request) {
			requests = append(requests, r.Method+" "+r.URL.Path)
			w.Header().Set("Content-Type", "application/json")
			switch {
			case r.Method == http.MethodPut:
				_, _ = w.Write([]byte(stubHookJSON))
			case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/hooks"):
				_, _ = w.Write([]byte("[" + stubHookJSON + "]"))
			case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/hook_scopes"):
				if attached {
					_, _ = w.Write([]byte(
						`[{"uuid":"s-new","triggerType":"PreRebootUniverse","hooks":["h-1"]}]`))
				} else {
					_, _ = w.Write([]byte(
						`[{"uuid":"s-old","triggerType":"PostNodeProvision","hooks":["h-1"]}]`))
				}
			case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/hook_scopes"):
				scopeCreated = true
				_, _ = w.Write([]byte(`{"uuid":"s-new","triggerType":"PreRebootUniverse"}`))
			case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/hooks/"):
				if !scopeCreated {
					t.Errorf("attach before the new scope was created: %v", requests)
				}
				attached = true
				_, _ = w.Write([]byte(
					`{"uuid":"s-new","triggerType":"PreRebootUniverse","hooks":["h-1"]}`))
			case r.Method == http.MethodDelete &&
				strings.HasSuffix(r.URL.Path, "/hook_scopes/s-old"):
				if !attached {
					t.Errorf("old scope deleted before the hook was re-attached: %v", requests)
				}
				_, _ = w.Write([]byte(`{"success":true}`))
			default:
				t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			}
		})

	d := stubHookData(t, ResourceHook())
	d.SetId("h-1")
	if err := d.Set("trigger_type", "PreRebootUniverse"); err != nil {
		t.Fatal(err)
	}
	if diags := resourceHookUpdate(context.Background(), d, apiClient); diags.HasError() {
		t.Fatalf("update returned diags: %v", diags)
	}
	if !attached {
		t.Errorf("hook was never attached to the new scope: %v", requests)
	}
	if countRequests(requests, "DELETE /api/v1/customers/cust-1/hook_scopes/s-old") != 1 {
		t.Errorf("expected the emptied old scope to be deleted: %v", requests)
	}
	if got := d.Get("trigger_type"); got != "PreRebootUniverse" {
		t.Errorf("trigger_type = %v, want PreRebootUniverse", got)
	}
}

// An update that only touches script fields must not churn the binding.
func TestHookUpdateBindingNoopWhenCorrect(t *testing.T) {
	var requests []string
	apiClient := newHookTestClient(t,
		func(w http.ResponseWriter, r *http.Request) {
			requests = append(requests, r.Method+" "+r.URL.Path)
			w.Header().Set("Content-Type", "application/json")
			switch {
			case r.Method == http.MethodPut:
				_, _ = w.Write([]byte(stubHookJSON))
			case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/hooks"):
				_, _ = w.Write([]byte("[" + stubHookJSON + "]"))
			case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/hook_scopes"):
				_, _ = w.Write([]byte(
					`[{"uuid":"s-1","triggerType":"ApiTriggered","hooks":["h-1"]}]`))
			default:
				t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			}
		})

	d := stubHookData(t, ResourceHook())
	d.SetId("h-1")
	if diags := resourceHookUpdate(context.Background(), d, apiClient); diags.HasError() {
		t.Fatalf("update returned diags: %v", diags)
	}
	if countRequests(requests, "PUT /api/v1/customers/cust-1/hooks/h-1") != 1 {
		t.Errorf("expected exactly one hook PUT, got %v", requests)
	}
	for _, req := range requests {
		if req == createHookScopePath || req == attachPath ||
			strings.HasPrefix(req, "DELETE ") {
			t.Errorf("binding already correct; unexpected scope churn %q in %v",
				req, requests)
		}
	}
}
