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
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

// The upgrade options of YBA's universe upgrade APIs.
const (
	UpgradeOptionRolling    = "Rolling"
	UpgradeOptionNonRolling = "Non-Rolling"
	UpgradeOptionNonRestart = "Non-Restart"
)

// NodeRestartSettings holds a node_restart_settings block: how YBA restarts
// the nodes of a universe in an upgrade.
type NodeRestartSettings struct {
	UpgradeOption           string
	SleepAfterMasterMillis  int32
	SleepAfterTServerMillis int32
}

// DefaultNodeRestartSettings are the defaults of node_restart_settings.
var DefaultNodeRestartSettings = NodeRestartSettings{
	UpgradeOption:           UpgradeOptionRolling,
	SleepAfterMasterMillis:  180000,
	SleepAfterTServerMillis: 180000,
}

// NodeRestartSettingsSchema returns the node_restart_settings block of a
// resource whose YBA call restarts universe nodes. Every such resource uses
// it, so the block has the same fields, defaults and lifecycle everywhere.
// YBA does not store the settings: each request that restarts nodes carries
// them. Terraform keeps them in the state like any other argument, and the
// resource makes no YBA call for a change to the block alone. options lists
// the upgrade options that the resource's API accepts.
func NodeRestartSettingsSchema(
	description, optionDescription string, options ...string,
) *schema.Schema {
	return &schema.Schema{
		Type:     schema.TypeList,
		Optional: true,
		MaxItems: 1,
		Description: description + " YugabyteDB Anywhere does not store these " +
			"settings: Terraform keeps them in the state and sends them with each " +
			"request that restarts nodes. A change to this block alone makes no " +
			"change in YugabyteDB Anywhere.",
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				"upgrade_option": {
					Type:     schema.TypeString,
					Optional: true,
					Default:  DefaultNodeRestartSettings.UpgradeOption,
					ValidateDiagFunc: validation.ToDiagFunc(
						validation.StringInSlice(options, false)),
					Description: optionDescription,
				},
				"sleep_after_master_restart_millis": {
					Type:             schema.TypeInt,
					Optional:         true,
					Default:          int(DefaultNodeRestartSettings.SleepAfterMasterMillis),
					ValidateDiagFunc: validation.ToDiagFunc(validation.IntAtLeast(0)),
					Description: "Milliseconds to sleep after each master node restart. " +
						"Must be 0 or a positive integer; 0 means no wait. Defaults to 180000 " +
						"(3 minutes), matching the YugabyteDB Anywhere platform default.",
				},
				"sleep_after_tserver_restart_millis": {
					Type:             schema.TypeInt,
					Optional:         true,
					Default:          int(DefaultNodeRestartSettings.SleepAfterTServerMillis),
					ValidateDiagFunc: validation.ToDiagFunc(validation.IntAtLeast(0)),
					Description: "Milliseconds to sleep after each TServer node restart. " +
						"Must be 0 or a positive integer; 0 means no wait. Defaults to 180000 " +
						"(3 minutes), matching the YugabyteDB Anywhere platform default.",
				},
			},
		},
	}
}

// GetNodeRestartSettings returns the node_restart_settings of d, with the
// schema defaults for the values that the block leaves out. ok is false when
// the resource has no block; the settings are then the defaults.
func GetNodeRestartSettings(d *schema.ResourceData) (s NodeRestartSettings, ok bool) {
	list, _ := d.Get("node_restart_settings").([]interface{})
	if len(list) == 0 {
		return DefaultNodeRestartSettings, false
	}
	m, _ := list[0].(map[string]interface{})
	if m == nil {
		return DefaultNodeRestartSettings, true
	}
	return NodeRestartSettings{
		UpgradeOption:           m["upgrade_option"].(string),
		SleepAfterMasterMillis:  int32(m["sleep_after_master_restart_millis"].(int)),
		SleepAfterTServerMillis: int32(m["sleep_after_tserver_restart_millis"].(int)),
	}, true
}
