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
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"

	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// Hook mirrors the YBA Hook model (a custom hook script). The scope a hook is
// attached to is deliberately not a field here: attachments are read through
// ListHookScopes, whose response nests each hook exactly once.
type Hook struct {
	UUID          string            `json:"uuid,omitempty"`
	CustomerUUID  string            `json:"customerUUID,omitempty"`
	Name          string            `json:"name"`
	ExecutionLang string            `json:"executionLang"`
	HookText      string            `json:"hookText"`
	UseSudo       bool              `json:"useSudo"`
	RuntimeArgs   map[string]string `json:"runtimeArgs,omitempty"`
}

// HookScope mirrors the YBA HookScope model. HookUUIDs lists the attached
// hooks; at most one of UniverseUUID / ProviderUUID is set (neither = global
// scope), and ClusterUUID is only ever set together with UniverseUUID.
type HookScope struct {
	UUID         string   `json:"uuid,omitempty"`
	CustomerUUID string   `json:"customerUUID,omitempty"`
	TriggerType  string   `json:"triggerType"`
	UniverseUUID string   `json:"universeUUID,omitempty"`
	ProviderUUID string   `json:"providerUUID,omitempty"`
	ClusterUUID  string   `json:"clusterUUID,omitempty"`
	HookUUIDs    hookRefs `json:"hooks,omitempty"`
}

// HookScopeSpec is the create-hook-scope request body (YBA HookScopeFormData).
type HookScopeSpec struct {
	TriggerType  string `json:"triggerType"`
	UniverseUUID string `json:"universeUUID,omitempty"`
	ProviderUUID string `json:"providerUUID,omitempty"`
	ClusterUUID  string `json:"clusterUUID,omitempty"`
}

// ErrHookMissing is the typed sentinel for "already gone" hooks, so callers
// errors.Is instead of substring-matching YBA's error bodies.
var ErrHookMissing = errors.New("hook does not exist")

// YBA answers a missing hook with HTTP 400 (not 404) and this body text
// (Hook.getOrBadRequest: "Invalid Hook UUID:<uuid>"), so body matching is
// unavoidable — it happens only here, never in resource code.
const hookMissingMarker = "Invalid Hook UUID"

// hookRefs is a list of hook UUIDs parsed from a JSON "hooks" array whose
// elements Jackson may emit either as full hook objects or — via
// @JsonIdentityInfo — as bare UUID strings for hooks serialized earlier in the
// same response.
type hookRefs []string

// UnmarshalJSON accepts both element shapes and keeps only the UUIDs.
func (r *hookRefs) UnmarshalJSON(data []byte) error {
	if string(bytes.TrimSpace(data)) == "null" {
		*r = nil
		return nil
	}
	var raw []json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	out := make([]string, 0, len(raw))
	for _, el := range raw {
		el = bytes.TrimSpace(el)
		if len(el) > 0 && el[0] == '"' {
			var id string
			if err := json.Unmarshal(el, &id); err != nil {
				return err
			}
			out = append(out, id)
			continue
		}
		var obj struct {
			UUID string `json:"uuid"`
		}
		if err := json.Unmarshal(el, &obj); err != nil {
			return err
		}
		out = append(out, obj.UUID)
	}
	*r = out
	return nil
}

// hookWithScope carries the hookScope field alongside the hook body. The raw
// scope is needed by parseHookList to recover sibling hooks that Jackson's
// @JsonIdentityInfo collapsed to bare UUID strings at the top level.
type hookWithScope struct {
	Hook
	HookScope json.RawMessage `json:"hookScope"`
}

// parseHookList decodes a GET /hooks response. When two hooks share a scope,
// YBA serializes the second hook's full body nested inside the first hook's
// hookScope.hooks array and emits only its UUID string at the top level, so a
// naive []Hook unmarshal loses it. This resolves those references.
func parseHookList(data []byte) ([]Hook, error) {
	var raw []json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("unmarshal hook list response: %w", err)
	}
	hooksByUUID := map[string]Hook{}
	order := make([]string, 0, len(raw))
	for _, el := range raw {
		el = bytes.TrimSpace(el)
		if len(el) > 0 && el[0] == '"' {
			var id string
			if err := json.Unmarshal(el, &id); err != nil {
				return nil, fmt.Errorf("unmarshal hook list response: %w", err)
			}
			order = append(order, id)
			continue
		}
		var hw hookWithScope
		if err := json.Unmarshal(el, &hw); err != nil {
			return nil, fmt.Errorf("unmarshal hook list response: %w", err)
		}
		hooksByUUID[hw.UUID] = hw.Hook
		order = append(order, hw.UUID)
		if err := collectNestedHooks(hw.HookScope, hooksByUUID); err != nil {
			return nil, err
		}
	}
	out := make([]Hook, 0, len(order))
	for _, id := range order {
		h, ok := hooksByUUID[id]
		if !ok {
			return nil, fmt.Errorf(
				"hook list response references hook %q without its definition", id)
		}
		out = append(out, h)
	}
	return out, nil
}

// collectNestedHooks pulls full hook bodies out of a hook's serialized
// hookScope.hooks array into the map; string elements (identity back-
// references) are skipped, as are string/null scopes.
func collectNestedHooks(scopeRaw json.RawMessage, into map[string]Hook) error {
	scopeRaw = bytes.TrimSpace(scopeRaw)
	if len(scopeRaw) == 0 || scopeRaw[0] != '{' {
		return nil
	}
	var scope struct {
		Hooks []json.RawMessage `json:"hooks"`
	}
	if err := json.Unmarshal(scopeRaw, &scope); err != nil {
		return fmt.Errorf("unmarshal nested hook scope: %w", err)
	}
	for _, el := range scope.Hooks {
		el = bytes.TrimSpace(el)
		if len(el) == 0 || el[0] != '{' {
			continue
		}
		var hw hookWithScope
		if err := json.Unmarshal(el, &hw); err != nil {
			return fmt.Errorf("unmarshal nested hook: %w", err)
		}
		into[hw.UUID] = hw.Hook
	}
	return nil
}

// hookMultipartBody encodes a hook as the multipart form the create/update
// endpoints expect: name / executionLang / useSudo data parts, one
// runtimeArgs[KEY] part per runtime argument, and the script as the hookFile
// file part.
func hookMultipartBody(hook Hook) (io.Reader, string, error) {
	buf := &bytes.Buffer{}
	w := multipart.NewWriter(buf)
	fields := map[string]string{
		"name":          hook.Name,
		"executionLang": hook.ExecutionLang,
		"useSudo":       strconv.FormatBool(hook.UseSudo),
	}
	for k, v := range hook.RuntimeArgs {
		fields[fmt.Sprintf("runtimeArgs[%s]", k)] = v
	}
	for k, v := range fields {
		if err := w.WriteField(k, v); err != nil {
			return nil, "", fmt.Errorf("encode hook form field %s: %w", k, err)
		}
	}
	fw, err := w.CreateFormFile("hookFile", hook.Name)
	if err != nil {
		return nil, "", fmt.Errorf("encode hook file part: %w", err)
	}
	if _, err := io.WriteString(fw, hook.HookText); err != nil {
		return nil, "", fmt.Errorf("encode hook text: %w", err)
	}
	if err := w.Close(); err != nil {
		return nil, "", fmt.Errorf("finalize hook multipart body: %w", err)
	}
	return buf, w.FormDataContentType(), nil
}

// writeHook POSTs (create) or PUTs (update) the multipart hook form and
// decodes the returned hook.
func (vc *VanillaClient) writeHook(
	ctx context.Context, method, url, token string, hook Hook, operation string,
) (*Hook, error) {
	body, contentType, err := hookMultipartBody(hook)
	if err != nil {
		return nil, err
	}
	resp, err := vc.makeRequestWithContentType(ctx, method, url, contentType, body, token)
	if err != nil {
		return nil, fmt.Errorf("%s hook request failed: %w", strings.ToLower(operation), err)
	}
	defer func() { _ = resp.Body.Close() }()
	if httpErr := vanillaHTTPError(resp, "Hook", operation); httpErr != nil {
		return nil, httpErr
	}
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var hw hookWithScope
	if err = json.Unmarshal(respBytes, &hw); err != nil {
		return nil, fmt.Errorf("unmarshal hook response: %w", err)
	}
	out := hw.Hook
	return &out, nil
}

// CreateHook creates a custom hook from Name, ExecutionLang, HookText,
// UseSudo, and RuntimeArgs.
func (vc *VanillaClient) CreateHook(
	ctx context.Context, cUUID, token string, hook Hook,
) (*Hook, error) {
	url := fmt.Sprintf("api/v1/customers/%s/hooks", cUUID)
	return vc.writeHook(ctx, http.MethodPost, url, token, hook, "Create")
}

// UpdateHook replaces every field of the hook (YBA's update is a full
// replace, including clearing runtime args that are no longer sent).
func (vc *VanillaClient) UpdateHook(
	ctx context.Context, cUUID, hookUUID, token string, hook Hook,
) (*Hook, error) {
	url := fmt.Sprintf("api/v1/customers/%s/hooks/%s", cUUID, hookUUID)
	return vc.writeHook(ctx, http.MethodPut, url, token, hook, "Update")
}

// ListHooks returns every custom hook for the customer.
func (vc *VanillaClient) ListHooks(
	ctx context.Context, cUUID, token string,
) ([]Hook, error) {
	url := fmt.Sprintf("api/v1/customers/%s/hooks", cUUID)
	resp, err := vc.makeRequest(ctx, http.MethodGet, url, nil, token)
	if err != nil {
		return nil, fmt.Errorf("list hooks request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if httpErr := vanillaHTTPError(resp, "Hook", "List"); httpErr != nil {
		return nil, httpErr
	}
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return parseHookList(respBytes)
}

// GetHook fetches one hook by UUID. YBA has no per-hook GET endpoint, so this
// lists and filters; a missing hook returns ErrHookMissing so Read can drop it
// from state.
func (vc *VanillaClient) GetHook(
	ctx context.Context, cUUID, hookUUID, token string,
) (*Hook, error) {
	hooks, err := vc.ListHooks(ctx, cUUID, token)
	if err != nil {
		return nil, err
	}
	for i := range hooks {
		if hooks[i].UUID == hookUUID {
			return &hooks[i], nil
		}
	}
	return nil, fmt.Errorf("hook %s: %w", hookUUID, ErrHookMissing)
}

// DeleteHook deletes a hook by UUID. Idempotent: a 404 or YBA's 400
// "Invalid Hook UUID" body returns nil; every other error propagates.
func (vc *VanillaClient) DeleteHook(
	ctx context.Context, cUUID, hookUUID, token string,
) error {
	url := fmt.Sprintf("api/v1/customers/%s/hooks/%s", cUUID, hookUUID)
	resp, err := vc.makeRequest(ctx, http.MethodDelete, url, nil, token)
	if err != nil {
		return fmt.Errorf("delete hook request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if utils.IsHTTPNotFound(resp) {
		return nil
	}
	if httpErr := vanillaHTTPError(resp, "Hook", "Delete"); httpErr != nil {
		if strings.Contains(httpErr.Error(), hookMissingMarker) {
			return nil
		}
		return httpErr
	}
	return nil
}

// CreateHookScope creates a hook scope binding a trigger to a target (global,
// provider, universe, or cluster).
func (vc *VanillaClient) CreateHookScope(
	ctx context.Context, cUUID, token string, spec HookScopeSpec,
) (*HookScope, error) {
	url := fmt.Sprintf("api/v1/customers/%s/hook_scopes", cUUID)
	body, err := json.Marshal(spec)
	if err != nil {
		return nil, err
	}
	resp, err := vc.makeRequest(ctx, http.MethodPost, url, bytes.NewBuffer(body), token)
	if err != nil {
		return nil, fmt.Errorf("create hook scope request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if httpErr := vanillaHTTPError(resp, "Hook Scope", "Create"); httpErr != nil {
		return nil, httpErr
	}
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	out := HookScope{}
	if err = json.Unmarshal(respBytes, &out); err != nil {
		return nil, fmt.Errorf("unmarshal hook scope response: %w", err)
	}
	return &out, nil
}

// ListHookScopes returns every hook scope for the customer, including the
// UUIDs of the hooks attached to each.
func (vc *VanillaClient) ListHookScopes(
	ctx context.Context, cUUID, token string,
) ([]HookScope, error) {
	url := fmt.Sprintf("api/v1/customers/%s/hook_scopes", cUUID)
	resp, err := vc.makeRequest(ctx, http.MethodGet, url, nil, token)
	if err != nil {
		return nil, fmt.Errorf("list hook scopes request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if httpErr := vanillaHTTPError(resp, "Hook Scope", "List"); httpErr != nil {
		return nil, httpErr
	}
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	out := []HookScope{}
	if err = json.Unmarshal(respBytes, &out); err != nil {
		return nil, fmt.Errorf("unmarshal hook scope list response: %w", err)
	}
	return out, nil
}

// DeleteHookScope deletes a hook scope by UUID. Idempotent: YBA answers a
// missing scope with a real 404, which returns nil; every other error
// propagates.
//
// YBA's hook table cascades on scope delete: any hook still attached is
// deleted with the scope. Callers must only delete a scope whose hooks are
// meant to go with it.
func (vc *VanillaClient) DeleteHookScope(
	ctx context.Context, cUUID, scopeUUID, token string,
) error {
	url := fmt.Sprintf("api/v1/customers/%s/hook_scopes/%s", cUUID, scopeUUID)
	resp, err := vc.makeRequest(ctx, http.MethodDelete, url, nil, token)
	if err != nil {
		return fmt.Errorf("delete hook scope request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if utils.IsHTTPNotFound(resp) {
		return nil
	}
	return vanillaHTTPError(resp, "Hook Scope", "Delete")
}

// AttachHookToScope attaches a hook to a hook scope. A hook can be attached to
// at most one scope: attaching an already-attached hook re-points it.
func (vc *VanillaClient) AttachHookToScope(
	ctx context.Context, cUUID, scopeUUID, hookUUID, token string,
) error {
	url := fmt.Sprintf("api/v1/customers/%s/hook_scopes/%s/hooks/%s", cUUID, scopeUUID, hookUUID)
	resp, err := vc.makeRequest(ctx, http.MethodPost, url, nil, token)
	if err != nil {
		return fmt.Errorf("attach hook request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	return vanillaHTTPError(resp, "Hook Attachment", "Create")
}
