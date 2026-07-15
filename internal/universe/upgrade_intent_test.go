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
	"testing"

	client "github.com/yugabyte/platform-go-client"

	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// The DB-version and systemd upgrade paths send whole clusters to YBA. The
// intent they carry must be the LIVE intent with only the upgraded field
// changed — a config-built intent would nil out server-managed fields set
// outside this resource (enableLB from yba_universe_load_balancer_config,
// telemetry log configs from yba_universe_telemetry_config).

func liveIntentWithOutOfBandFields() client.UserIntent {
	return client.UserIntent{
		YbSoftwareVersion: utils.GetStringPointer("2.20.0.0"),
		UseSystemd:        utils.GetBoolPointer(false),
		EnableLB:          utils.GetBoolPointer(true),
		AuditLogConfig:    &client.AuditLogConfig{ExportActive: utils.GetBoolPointer(true)},
	}
}

func TestSoftwareUpgradeIntentPreservesOutOfBandFields(t *testing.T) {
	live := liveIntentWithOutOfBandFields()
	built := client.UserIntent{
		YbSoftwareVersion: utils.GetStringPointer("2.21.0.0"),
		UseSystemd:        utils.GetBoolPointer(true),
	}

	got := softwareUpgradeIntent(live, built)

	if got.GetYbSoftwareVersion() != "2.21.0.0" {
		t.Errorf("YbSoftwareVersion = %q, want 2.21.0.0", got.GetYbSoftwareVersion())
	}
	if !got.GetEnableLB() {
		t.Error("EnableLB clobbered by software upgrade intent")
	}
	if got.AuditLogConfig == nil {
		t.Error("AuditLogConfig clobbered by software upgrade intent")
	}
	if got.GetUseSystemd() {
		t.Error("software upgrade must not change UseSystemd")
	}
}

func TestSystemdUpgradeIntentPreservesOutOfBandFields(t *testing.T) {
	live := liveIntentWithOutOfBandFields()
	built := client.UserIntent{
		UseSystemd:        utils.GetBoolPointer(true),
		YbSoftwareVersion: utils.GetStringPointer("2.21.0.0"),
	}

	got := systemdUpgradeIntent(live, built)

	if !got.GetUseSystemd() {
		t.Error("UseSystemd not taken from the config-built intent")
	}
	if !got.GetEnableLB() {
		t.Error("EnableLB clobbered by systemd upgrade intent")
	}
	if got.AuditLogConfig == nil {
		t.Error("AuditLogConfig clobbered by systemd upgrade intent")
	}
	if got.GetYbSoftwareVersion() != "2.20.0.0" {
		t.Error("systemd upgrade must not change YbSoftwareVersion")
	}
}
