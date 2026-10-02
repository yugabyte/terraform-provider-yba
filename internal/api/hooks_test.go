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
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

// parseHookMultipart decodes the multipart hook form a stub server received:
// the plain fields (name, executionLang, useSudo, runtimeArgs[...]) and the
// hookFile content.
func parseHookMultipart(t *testing.T, r *http.Request) (fields map[string]string, hookFile string) {
	t.Helper()
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		t.Fatalf("request is not multipart: %v", err)
	}
	fields = map[string]string{}
	for k, v := range r.MultipartForm.Value {
		if len(v) > 0 {
			fields[k] = v[0]
		}
	}
	files := r.MultipartForm.File["hookFile"]
	if len(files) != 1 {
		t.Fatalf("expected exactly one hookFile part, got %d", len(files))
	}
	f, err := files[0].Open()
	if err != nil {
		t.Fatalf("open hookFile part: %v", err)
	}
	defer func() { _ = f.Close() }()
	content, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("read hookFile part: %v", err)
	}
	return fields, string(content)
}

func TestCreateHookSendsMultipartForm(t *testing.T) {
	var (
		gotMethod string
		gotPath   string
		gotFields map[string]string
		gotFile   string
	)
	vc, _ := newStubVanillaClient(t,
		func(w http.ResponseWriter, r *http.Request) {
			gotMethod = r.Method
			gotPath = r.URL.Path
			gotFields, gotFile = parseHookMultipart(t, r)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"uuid":"h-new",
				"customerUUID":"cust-1",
				"name":"10-mount.sh",
				"executionLang":"Bash",
				"hookText":"#!/bin/bash\necho hi\n",
				"useSudo":true,
				"runtimeArgs":{"DEVICE":"/dev/sdb"}
			}`))
		})

	in := Hook{
		Name:          "10-mount.sh",
		ExecutionLang: "Bash",
		HookText:      "#!/bin/bash\necho hi\n",
		UseSudo:       true,
		RuntimeArgs:   map[string]string{"DEVICE": "/dev/sdb"},
	}
	out, err := vc.CreateHook(context.Background(), "cust-1", "token", in)
	if err != nil {
		t.Fatalf("create error: %v", err)
	}
	if out.UUID != "h-new" {
		t.Errorf("unexpected response: %+v", out)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("expected POST, got %s", gotMethod)
	}
	if !strings.HasSuffix(gotPath, "/customers/cust-1/hooks") {
		t.Errorf("unexpected path: %s", gotPath)
	}
	want := map[string]string{
		"name":                "10-mount.sh",
		"executionLang":       "Bash",
		"useSudo":             "true",
		"runtimeArgs[DEVICE]": "/dev/sdb",
	}
	for k, v := range want {
		if gotFields[k] != v {
			t.Errorf("form field %s = %q, want %q", k, gotFields[k], v)
		}
	}
	if gotFile != in.HookText {
		t.Errorf("hookFile content = %q, want %q", gotFile, in.HookText)
	}
}

// The regression trap in GET /hooks: Jackson's @JsonIdentityInfo serializes a
// hook that already appeared (nested inside a sibling's hookScope.hooks) as a
// bare UUID string at the top level. A naive []Hook unmarshal fails or loses
// the hook; the parser must recover its full body from the nested copy.
func TestListHooksResolvesIdentityReferences(t *testing.T) {
	body := `[
		{
			"uuid":"h1","name":"a.sh","executionLang":"Bash",
			"hookText":"A","useSudo":false,
			"hookScope":{
				"uuid":"s1","triggerType":"PreNodeProvision",
				"hooks":[
					"h1",
					{"uuid":"h2","name":"b.py","executionLang":"Python",
					 "hookText":"B","useSudo":true,
					 "runtimeArgs":{"K":"V"},"hookScope":"s1"}
				]
			}
		},
		"h2",
		{"uuid":"h3","name":"c.sh","executionLang":"Bash",
		 "hookText":"C","useSudo":false,"hookScope":null}
	]`
	vc, _ := newStubVanillaClient(t,
		func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(body))
		})

	hooks, err := vc.ListHooks(context.Background(), "cust-1", "token")
	if err != nil {
		t.Fatalf("list error: %v", err)
	}
	if len(hooks) != 3 {
		t.Fatalf("expected 3 hooks, got %d: %+v", len(hooks), hooks)
	}
	byUUID := map[string]Hook{}
	for _, h := range hooks {
		byUUID[h.UUID] = h
	}
	h2, ok := byUUID["h2"]
	if !ok {
		t.Fatal("hook h2 (identity reference) not resolved")
	}
	if h2.Name != "b.py" || h2.ExecutionLang != "Python" || h2.HookText != "B" ||
		!h2.UseSudo || h2.RuntimeArgs["K"] != "V" {
		t.Errorf("hook h2 body not recovered from nested copy: %+v", h2)
	}
	if byUUID["h3"].HookText != "C" {
		t.Errorf("unattached hook h3 not parsed: %+v", byUUID["h3"])
	}
}

func TestListHooksUnresolvedReferenceFails(t *testing.T) {
	vc, _ := newStubVanillaClient(t,
		func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`["h-orphan"]`))
		})

	_, err := vc.ListHooks(context.Background(), "cust-1", "token")
	if err == nil {
		t.Fatal("expected error for unresolvable hook reference")
	}
	if !strings.Contains(err.Error(), "h-orphan") {
		t.Errorf("error must name the unresolved hook: %v", err)
	}
}

func TestGetHookMissing(t *testing.T) {
	vc, _ := newStubVanillaClient(t,
		func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"uuid":"other","name":"x.sh",
				"executionLang":"Bash","hookText":"X","useSudo":false}]`))
		})

	hook, err := vc.GetHook(context.Background(), "cust-1", "h-1", "token")
	if hook != nil {
		t.Errorf("expected nil hook on missing, got %+v", hook)
	}
	if !errors.Is(err, ErrHookMissing) {
		t.Fatalf("expected ErrHookMissing, got %v", err)
	}
}

func TestDeleteHookIdempotent(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
	}{
		{"404", http.StatusNotFound, ``},
		{
			// Hook.getOrBadRequest answers a missing hook with 400, not 404.
			name:   "400 invalid hook uuid",
			status: http.StatusBadRequest,
			body:   `{"error":"Invalid Hook UUID:h-1"}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			vc, _ := newStubVanillaClient(t,
				func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(tc.status)
					_, _ = w.Write([]byte(tc.body))
				})

			if err := vc.DeleteHook(
				context.Background(), "cust-1", "h-1", "token"); err != nil {
				t.Errorf("expected nil for idempotent delete, got %v", err)
			}
		})
	}
}

func TestDeleteHookSurfacesErrors(t *testing.T) {
	vc, _ := newStubVanillaClient(t,
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(
				`{"error":"Custom hooks is not enabled on this Anywhere instance"}`))
		})

	err := vc.DeleteHook(context.Background(), "cust-1", "h-1", "token")
	if err == nil {
		t.Fatal("expected non-nil error for disabled custom hooks")
	}
	if !strings.Contains(err.Error(), "Custom hooks is not enabled") {
		t.Errorf("error did not surface body: %v", err)
	}
}

func TestCreateHookScopeBody(t *testing.T) {
	var (
		gotMethod string
		gotPath   string
		gotBody   map[string]interface{}
	)
	vc, _ := newStubVanillaClient(t,
		func(w http.ResponseWriter, r *http.Request) {
			gotMethod = r.Method
			gotPath = r.URL.Path
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &gotBody)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"uuid":"s-new","triggerType":"PreNodeProvision",
				"universeUUID":"u-1","clusterUUID":"c-1"}`))
		})

	spec := HookScopeSpec{
		TriggerType:  "PreNodeProvision",
		UniverseUUID: "u-1",
		ClusterUUID:  "c-1",
	}
	out, err := vc.CreateHookScope(context.Background(), "cust-1", "token", spec)
	if err != nil {
		t.Fatalf("create error: %v", err)
	}
	if out.UUID != "s-new" || out.UniverseUUID != "u-1" {
		t.Errorf("unexpected response: %+v", out)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("expected POST, got %s", gotMethod)
	}
	if !strings.HasSuffix(gotPath, "/customers/cust-1/hook_scopes") {
		t.Errorf("unexpected path: %s", gotPath)
	}
	if gotBody["triggerType"] != "PreNodeProvision" ||
		gotBody["universeUUID"] != "u-1" || gotBody["clusterUUID"] != "c-1" {
		t.Errorf("request body lost fields: %+v", gotBody)
	}
	if _, present := gotBody["providerUUID"]; present {
		t.Errorf("empty providerUUID must be omitted, got %+v", gotBody)
	}
}

// Scope responses embed attached hooks as full objects (first occurrence) but
// Jackson may also emit bare UUID strings; both shapes must yield the UUID.
func TestListHookScopesParsesHookRefs(t *testing.T) {
	body := `[
		{"uuid":"s1","triggerType":"PreNodeProvision","providerUUID":"p-1",
		 "hooks":[
			{"uuid":"h1","name":"a.sh","executionLang":"Bash",
			 "hookText":"A","useSudo":false,"hookScope":"s1"},
			"h2"
		 ]},
		{"uuid":"s2","triggerType":"ApiTriggered"}
	]`
	vc, _ := newStubVanillaClient(t,
		func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(body))
		})

	scopes, err := vc.ListHookScopes(context.Background(), "cust-1", "token")
	if err != nil {
		t.Fatalf("list error: %v", err)
	}
	if len(scopes) != 2 {
		t.Fatalf("expected 2 scopes, got %d", len(scopes))
	}
	if scopes[0].ProviderUUID != "p-1" || scopes[0].TriggerType != "PreNodeProvision" {
		t.Errorf("scope fields not parsed: %+v", scopes[0])
	}
	if len(scopes[0].HookUUIDs) != 2 ||
		scopes[0].HookUUIDs[0] != "h1" || scopes[0].HookUUIDs[1] != "h2" {
		t.Errorf("hook refs not parsed from mixed shapes: %+v", scopes[0].HookUUIDs)
	}
	if len(scopes[1].HookUUIDs) != 0 {
		t.Errorf("scope without hooks must have no refs: %+v", scopes[1].HookUUIDs)
	}
}

func TestDeleteHookScopeIdempotentOn404(t *testing.T) {
	vc, _ := newStubVanillaClient(t,
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"Invalid HookScope UUID:s-1"}`))
		})

	if err := vc.DeleteHookScope(
		context.Background(), "cust-1", "s-1", "token"); err != nil {
		t.Errorf("expected nil for idempotent delete, got %v", err)
	}
}
