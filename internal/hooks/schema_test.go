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

// Schema sanity tests: they protect the API contract another change is most
// likely to break — every field updatable in place (script fields via PUT,
// binding fields via re-attach), the scope-target exclusivity rules, and
// importability.
package hooks

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func TestHookSchemaContract(t *testing.T) {
	res := ResourceHook()
	if err := res.InternalValidate(nil, true); err != nil {
		t.Fatalf("schema fails InternalValidate: %v", err)
	}
	if res.UpdateContext == nil {
		t.Error("yba_hook must support in-place update")
	}
	if res.Importer == nil {
		t.Error("yba_hook must be importable")
	}
	// Script fields update through the PUT endpoint and binding fields through
	// re-attachment; a ForceNew flag would needlessly destroy and recreate.
	for name, s := range res.Schema {
		if s.ForceNew {
			t.Errorf("field %q must not be ForceNew: every field updates in place", name)
		}
		if s.Description == "" {
			t.Errorf("field %q needs a Description: it renders into the docs", name)
		}
	}
	// The field groups drive the update flow's error reverts: together they
	// must cover the whole schema, so a new field cannot silently skip both.
	grouped := map[string]bool{}
	for _, f := range append(append([]string{}, hookScriptFields...), hookScopeFields...) {
		if res.Schema[f] == nil {
			t.Errorf("field group names unknown schema field %q", f)
		}
		grouped[f] = true
	}
	for name := range res.Schema {
		if !grouped[name] {
			t.Errorf("schema field %q missing from hookScriptFields/hookScopeFields", name)
		}
	}

	if res.Schema["trigger_type"].Required != true {
		t.Error("trigger_type must be Required: an unbound hook never runs")
	}
	if res.Schema["execution_lang"].ValidateFunc == nil {
		t.Fatal("execution_lang must validate against hookExecutionLangs")
	}
	for _, lang := range hookExecutionLangs {
		if _, errs := res.Schema["execution_lang"].ValidateFunc(
			lang, "execution_lang"); len(errs) > 0 {
			t.Errorf("execution_lang %q must validate, got %v", lang, errs)
		}
	}
	if _, errs := res.Schema["execution_lang"].ValidateFunc(
		"Perl", "execution_lang"); len(errs) == 0 {
		t.Error("execution_lang must reject unsupported languages")
	}
	if elem, ok := res.Schema["runtime_args"].Elem.(*schema.Schema); !ok ||
		elem.Type != schema.TypeString {
		t.Error("runtime_args must be a map of strings")
	}

	// The scope target is polymorphic: universe and provider are mutually
	// exclusive, and cluster only qualifies a universe.
	assertStringSliceEqual(t, "universe_uuid.ConflictsWith",
		res.Schema["universe_uuid"].ConflictsWith, []string{"provider_uuid"})
	assertStringSliceEqual(t, "provider_uuid.ConflictsWith",
		res.Schema["provider_uuid"].ConflictsWith, []string{"universe_uuid"})
	assertStringSliceEqual(t, "cluster_uuid.RequiredWith",
		res.Schema["cluster_uuid"].RequiredWith, []string{"universe_uuid"})
}

func assertStringSliceEqual(t *testing.T, label string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s = %v, want %v", label, got, want)
		return
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%s = %v, want %v", label, got, want)
			return
		}
	}
}
