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
	client "github.com/yugabyte/platform-go-client"
)

// The upgrade endpoints receive whole clusters, so the intent they carry must
// be the live YBA intent with only the field the endpoint owns overwritten —
// the same field-whitelist overlay editUniverseParameters applies for
// EditUniverse. Substituting the config-built intent wholesale would nil out
// UserIntent fields managed outside yba_universe (enableLB, the telemetry
// audit/query/metrics log configs) because buildUserIntent never sets them.

// softwareUpgradeIntent returns the live intent carrying only the config-built
// software version.
func softwareUpgradeIntent(live, built client.UserIntent) client.UserIntent {
	live.YbSoftwareVersion = built.YbSoftwareVersion
	return live
}

// systemdUpgradeIntent returns the live intent carrying only the config-built
// systemd toggle.
func systemdUpgradeIntent(live, built client.UserIntent) client.UserIntent {
	live.UseSystemd = built.UseSystemd
	return live
}
