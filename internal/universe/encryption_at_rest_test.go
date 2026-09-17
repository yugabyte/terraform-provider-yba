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

package universe

import (
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	client "github.com/yugabyte/platform-go-client"

	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

func TestPlanEncryptionAtRest(t *testing.T) {
	on := func(kms string) earState { return earState{enabled: true, kmsConfigUUID: kms} }
	off := func(kms string) earState { return earState{enabled: false, kmsConfigUUID: kms} }

	cases := []struct {
		name    string
		desired earState
		trigger bool
		live    earState
		want    []earAction
		wantErr string
	}{
		{
			name:    "enable on a plain universe",
			desired: on("A"),
			live:    off(""),
			want: []earAction{
				{label: "Enable Encryption At Rest", op: earOpEnable, kmsConfigUUID: "A"},
			},
		},
		{
			// Enabling already generates a fresh universe key.
			name:    "trigger set in the same apply that enables",
			desired: on("A"),
			trigger: true,
			live:    off(""),
			want: []earAction{
				{label: "Enable Encryption At Rest", op: earOpEnable, kmsConfigUUID: "A"},
			},
		},
		{name: "already matching", desired: on("A"), live: on("A")},
		{
			name: "master key rotation", desired: on("B"), live: on("A"),
			want: []earAction{{label: "Master Key Rotation", op: earOpEnable, kmsConfigUUID: "B"}},
		},
		{
			name:    "universe key rotation",
			desired: on("A"),
			trigger: true,
			live:    on("A"),
			want: []earAction{
				{label: "Universe Key Rotation", op: earOpEnable, kmsConfigUUID: "A"},
			},
		},
		{
			name: "master key rotation then universe key rotation", desired: on("B"),
			trigger: true, live: on("A"),
			want: []earAction{
				{label: "Master Key Rotation", op: earOpEnable, kmsConfigUUID: "B"},
				{label: "Universe Key Rotation", op: earOpEnable, kmsConfigUUID: "B"},
			},
		},
		{
			name: "disable", desired: off("A"), live: on("A"),
			want: []earAction{{label: "Disable Encryption At Rest", op: earOpDisable}},
		},
		{name: "disable when already disabled", desired: off(""), live: off("A")},
		{
			// YBA keeps the old config's key history and re-sends the old key
			// before switching; one ENABLE with the new config covers it.
			name:    "re-enable with another config",
			desired: on("B"),
			live:    off("A"),
			want: []earAction{
				{label: "Enable Encryption At Rest", op: earOpEnable, kmsConfigUUID: "B"},
			},
		},
		{
			name:    "enabled without a config",
			desired: on(""),
			live:    off(""),
			wantErr: "kms_config_uuid is required",
		},
		{
			name:    "trigger while disabling",
			desired: off("A"),
			trigger: true,
			live:    on("A"),
			wantErr: "requires encryption_at_rest.enabled",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := planEncryptionAtRest(tc.desired, tc.trigger, tc.live)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("want error containing %q, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("plan: %v", err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("actions = %+v want %+v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("action %d = %+v want %+v", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func earTestSchema() map[string]*schema.Schema {
	return map[string]*schema.Schema{"encryption_at_rest": encryptionAtRestSchema()}
}

func TestFlattenEncryptionAtRestCarriesTriggerAndMirrorsServer(t *testing.T) {
	d := schema.TestResourceDataRaw(t, earTestSchema(), map[string]interface{}{
		"encryption_at_rest": []interface{}{map[string]interface{}{
			"kms_config_uuid": "A", "universe_key_rotation_trigger": "2026-09",
		}},
	})
	// After a disable YBA still reports the last config; the block mirrors that.
	got := flattenEncryptionAtRest(d, &client.EncryptionAtRestConfig{
		EncryptionAtRestEnabled: utils.GetBoolPointer(false),
		KmsConfigUUID:           utils.GetStringPointer("A"),
	})
	block := got[0].(map[string]interface{})
	if block["enabled"] != false || block["kms_config_uuid"] != "A" {
		t.Errorf("block = %v: want the server's enabled=false and config A", block)
	}
	if block["universe_key_rotation_trigger"] != "2026-09" {
		t.Errorf("trigger = %v: it has no server-side counterpart and must be carried over",
			block["universe_key_rotation_trigger"])
	}

	// A universe that never had encryption configured still gets one block, so a
	// config with enabled = false does not diff forever against an empty state.
	got = flattenEncryptionAtRest(schema.TestResourceDataRaw(t, earTestSchema(), nil), nil)
	block = got[0].(map[string]interface{})
	if block["enabled"] != false || block["kms_config_uuid"] != "" ||
		block["universe_key_rotation_trigger"] != "" {
		t.Errorf("block for an unconfigured universe = %v", block)
	}
}

func TestEncryptionAtRestSchemaSanity(t *testing.T) {
	s := encryptionAtRestSchema()
	if !s.Optional || !s.Computed {
		t.Error("the block must be Optional+Computed so omitting it leaves the universe alone")
	}
	if s.MaxItems != 1 {
		t.Error("the block must be a singleton")
	}
	elem := s.Elem.(*schema.Resource).Schema
	if elem["enabled"].Default != true {
		t.Error("a block that names a config must mean enabled")
	}
	if !elem["kms_config_uuid"].Computed {
		t.Error("kms_config_uuid must be Computed: YBA keeps it after a disable")
	}
	if elem["universe_key_rotation_trigger"].Computed {
		t.Error("the trigger is user bookkeeping and must not be Computed")
	}
}
