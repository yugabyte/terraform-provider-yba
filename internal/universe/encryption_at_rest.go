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
	"context"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	client "github.com/yugabyte/platform-go-client"

	"github.com/yugabyte/terraform-provider-yba/internal/api"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// Values of YBA's EncryptionAtRestConfig.opType and .type. There is no rotate
// op: ENABLE with the active configuration rotates the universe key, ENABLE
// with another configuration rotates the master key.
const (
	earOpEnable       = "ENABLE"
	earOpDisable      = "DISABLE"
	earKeyTypeDataKey = "DATA_KEY"

	earTriggerKey = "encryption_at_rest.0.universe_key_rotation_trigger"
)

// encryptionAtRestSchema is the universe's encryption_at_rest block. It is
// Optional+Computed like root_ca: omitting it leaves the universe as it is,
// and disabling is an explicit enabled = false.
func encryptionAtRestSchema() *schema.Schema {
	return &schema.Schema{
		Type:     schema.TypeList,
		Optional: true,
		Computed: true,
		MaxItems: 1,
		Description: "Encryption at rest for the universe's data, keyed through an " +
			"encryption-at-rest configuration (`yba_gcp_ear_config`, or one found with " +
			"`yba_ear_config`). Set at creation to encrypt from the first write, or add to an " +
			"existing universe to enable it in place. Changing `kms_config_uuid` on an " +
			"enabled universe rotates the master key: YugabyteDB Anywhere re-wraps every " +
			"universe key with the new configuration in one task, without restarting nodes. " +
			"Changing `universe_key_rotation_trigger` rotates the universe key itself under " +
			"the current master key. The block is read from the universe when omitted, so " +
			"removing it changes nothing; set `enabled = false` to turn encryption off.\n\n" +
			"~> **Note:** A configuration that a universe has used keeps that universe's key " +
			"history until the universe is deleted, and cannot be deleted before then, even " +
			"after encryption is disabled or the universe moves to another configuration. " +
			"YugabyteDB Anywhere needs the history to restore backups taken under earlier " +
			"keys.",
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				"enabled": {
					Type:     schema.TypeBool,
					Optional: true,
					Default:  true,
					Description: "Whether encryption at rest is enabled. `false` disables it " +
						"in place through a YugabyteDB Anywhere task; data written afterwards " +
						"is stored in clear.",
				},
				"kms_config_uuid": {
					Type:     schema.TypeString,
					Optional: true,
					Computed: true,
					Description: "UUID of the encryption-at-rest configuration whose master " +
						"key wraps the universe keys. Required when `enabled` is true. Changing " +
						"it on an enabled universe rotates the master key. After disabling, " +
						"YugabyteDB Anywhere keeps reporting the last configuration here.",
				},
				"universe_key_rotation_trigger": {
					Type:     schema.TypeString,
					Optional: true,
					Description: "Opaque trigger for universe key rotation: changing it to any " +
						"new non-empty value generates a fresh universe key under the current " +
						"master key on the next apply (setting it for the first time counts). " +
						"Removing it never fires. A date reads well in diffs; pair it with " +
						"`time_rotating` for automated rotation. It applies to an enabled " +
						"universe only, and when it changes in the same apply that enables " +
						"encryption the enable itself already generates a fresh key. When it " +
						"changes together with `kms_config_uuid`, the master key rotation runs " +
						"first.",
				},
			},
		},
	}
}

// earState is the encryption-at-rest slice of a universe, desired or live.
type earState struct {
	enabled       bool
	kmsConfigUUID string
}

// earAction is one set_key dispatch.
type earAction struct {
	label         string // task label for logs and errors
	op            string // earOpEnable or earOpDisable
	kmsConfigUUID string // configuration for ENABLE; empty for DISABLE
}

// desiredEncryptionAtRest reads the block. ok is false when neither the
// config nor the state carries one (a universe created before the block
// existed and never refreshed).
func desiredEncryptionAtRest(d *schema.ResourceData) (earState, bool) {
	list, _ := d.Get("encryption_at_rest").([]interface{})
	if len(list) == 0 {
		return earState{}, false
	}
	m, ok := list[0].(map[string]interface{})
	if !ok {
		return earState{}, false
	}
	enabled, _ := m["enabled"].(bool)
	kms, _ := m["kms_config_uuid"].(string)
	return earState{enabled: enabled, kmsConfigUUID: kms}, true
}

// encryptionAtRestCreateConfig is the create-time config: ENABLE with the
// configuration when the block asks for encryption, nil otherwise (the
// server default leaves encryption off).
func encryptionAtRestCreateConfig(d *schema.ResourceData) (*client.EncryptionAtRestConfig, error) {
	desired, ok := desiredEncryptionAtRest(d)
	if !ok || !desired.enabled {
		return nil, nil
	}
	if desired.kmsConfigUUID == "" {
		return nil, fmt.Errorf(
			"encryption_at_rest.kms_config_uuid is required when encryption_at_rest.enabled is true",
		)
	}
	return earEnableRequest(desired.kmsConfigUUID), nil
}

func earEnableRequest(kmsConfigUUID string) *client.EncryptionAtRestConfig {
	return &client.EncryptionAtRestConfig{
		EncryptionAtRestEnabled: utils.GetBoolPointer(true),
		KmsConfigUUID:           utils.GetStringPointer(kmsConfigUUID),
		OpType:                  utils.GetStringPointer(earOpEnable),
		Type:                    utils.GetStringPointer(earKeyTypeDataKey),
	}
}

func earDisableRequest() *client.EncryptionAtRestConfig {
	return &client.EncryptionAtRestConfig{
		EncryptionAtRestEnabled: utils.GetBoolPointer(false),
		OpType:                  utils.GetStringPointer(earOpDisable),
		Type:                    utils.GetStringPointer(earKeyTypeDataKey),
	}
}

// flattenEncryptionAtRest builds the state block from the live universe. The
// rotation trigger has no server-side counterpart, so it is carried over from
// the prior value (the config during apply, the state during refresh). YBA
// keeps kmsConfigUUID after a disable, and the block mirrors that.
func flattenEncryptionAtRest(
	d *schema.ResourceData, cfg *client.EncryptionAtRestConfig,
) []interface{} {
	trigger := ""
	if prior, _ := d.Get("encryption_at_rest").([]interface{}); len(prior) > 0 {
		if m, ok := prior[0].(map[string]interface{}); ok {
			trigger, _ = m["universe_key_rotation_trigger"].(string)
		}
	}
	state := earState{}
	if cfg != nil {
		state.enabled = cfg.GetEncryptionAtRestEnabled()
		state.kmsConfigUUID = cfg.GetKmsConfigUUID()
	}
	return []interface{}{map[string]interface{}{
		"enabled":                       state.enabled,
		"kms_config_uuid":               state.kmsConfigUUID,
		"universe_key_rotation_trigger": trigger,
	}}
}

// validateEncryptionAtRestDiff rejects enabled = true without a configuration
// at plan time, when the UUID is known. An unknown UUID (a configuration
// created in the same apply) is checked again at apply time.
func validateEncryptionAtRestDiff(_ context.Context, d *schema.ResourceDiff, _ interface{}) error {
	list, _ := d.Get("encryption_at_rest").([]interface{})
	if len(list) == 0 {
		return nil
	}
	m, ok := list[0].(map[string]interface{})
	if !ok {
		return nil
	}
	if enabled, _ := m["enabled"].(bool); !enabled {
		return nil
	}
	if !d.NewValueKnown("encryption_at_rest.0.kms_config_uuid") {
		return nil
	}
	if kms, _ := m["kms_config_uuid"].(string); kms == "" {
		return fmt.Errorf(
			"encryption_at_rest.kms_config_uuid is required when encryption_at_rest.enabled is true",
		)
	}
	return nil
}

// planEncryptionAtRest derives the set_key dispatches from the desired block,
// whether the rotation trigger changed, and the live universe.
//
//   - Enable, master key rotation (a different configuration on an enabled
//     universe) and disable are each one dispatch; a universe that already
//     matches gets none.
//   - A fired trigger adds a universe key rotation (ENABLE with the current
//     configuration) after any master key rotation. It is skipped when this
//     apply enables encryption, because enabling already generates a fresh
//     universe key, and rejected when the desired state is disabled.
func planEncryptionAtRest(desired earState, triggerFired bool, live earState) ([]earAction, error) {
	if desired.enabled && desired.kmsConfigUUID == "" {
		return nil, fmt.Errorf(
			"encryption_at_rest.kms_config_uuid is required when encryption_at_rest.enabled is true",
		)
	}
	if !desired.enabled && triggerFired {
		return nil, fmt.Errorf(
			"encryption_at_rest.universe_key_rotation_trigger requires encryption_at_rest.enabled " +
				"to be true: a disabled universe has no universe key to rotate",
		)
	}

	var actions []earAction
	enabling := false
	switch {
	case desired.enabled && !live.enabled:
		enabling = true
		actions = append(actions, earAction{
			label: "Enable Encryption At Rest", op: earOpEnable,
			kmsConfigUUID: desired.kmsConfigUUID,
		})
	case desired.enabled && desired.kmsConfigUUID != live.kmsConfigUUID:
		actions = append(actions, earAction{
			label: "Master Key Rotation", op: earOpEnable,
			kmsConfigUUID: desired.kmsConfigUUID,
		})
	case !desired.enabled && live.enabled:
		actions = append(actions, earAction{label: "Disable Encryption At Rest", op: earOpDisable})
	}
	if desired.enabled && triggerFired && !enabling {
		actions = append(actions, earAction{
			label: "Universe Key Rotation", op: earOpEnable,
			kmsConfigUUID: desired.kmsConfigUUID,
		})
	}
	return actions, nil
}

// performEncryptionAtRest runs at the tail of resourceUniverseUpdate and
// dispatches the set_key tasks the block change calls for. Each task takes
// the universe lock, so it goes through DispatchAndWait for the 409 retry.
func performEncryptionAtRest(
	ctx context.Context, d *schema.ResourceData, meta interface{},
) diag.Diagnostics {
	if !d.HasChange("encryption_at_rest") {
		return nil
	}
	desired, ok := desiredEncryptionAtRest(d)
	if !ok {
		return nil
	}
	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID

	liveUni, response, err := c.UniverseManagementAPI.GetUniverse(ctx, cUUID, d.Id()).Execute()
	if err != nil {
		return diag.FromErr(utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"Universe", "Update - Fetch universe for encryption at rest"))
	}
	live := earState{}
	if cfg := liveUni.UniverseDetails.EncryptionAtRestConfig; cfg != nil {
		live.enabled = cfg.GetEncryptionAtRestEnabled()
		live.kmsConfigUUID = cfg.GetKmsConfigUUID()
	}

	actions, err := planEncryptionAtRest(desired, triggerFired(d, earTriggerKey), live)
	if err != nil {
		return diag.FromErr(err)
	}
	for _, action := range actions {
		req := earDisableRequest()
		if action.op == earOpEnable {
			req = earEnableRequest(action.kmsConfigUUID)
		}
		if diags := utils.DispatchAndWait(ctx, action.label, cUUID, c,
			d.Timeout(schema.TimeoutUpdate),
			utils.ResourceEntity, "Universe", fmt.Sprintf("Update - %s", action.label),
			func() (string, *http.Response, error) {
				r, resp, e := c.UniverseManagementAPI.SetUniverseKey(ctx, cUUID, d.Id()).
					SetUniverseKeyRequest(*req).Execute()
				if e != nil {
					return "", resp, e
				}
				return r.GetTaskUUID(), resp, nil
			},
		); diags != nil {
			return diags
		}
	}
	return nil
}
