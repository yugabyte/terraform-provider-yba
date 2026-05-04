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

package telemetry

import "time"

// telemetryUpgradeTimeout is the canonical default timeout for any operation
// that triggers per-universe rolling restart tasks via the unified
// `export-telemetry-configs` API.
//
// Each universe rolling upgrade serializes through master and tserver
// processes with a default 3-minute sleep between restarts (see
// upgrade_options.sleep_after_*_restart_millis), so a 9-node universe takes
// roughly 30+ minutes per upgrade. The destroy path of a
// `yba_telemetry_provider` may have to drive multiple universes through this
// flow back-to-back, which is why we default to two hours. Operators can
// override per-resource via the standard `timeouts {}` block.
const telemetryUpgradeTimeout = 2 * time.Hour
