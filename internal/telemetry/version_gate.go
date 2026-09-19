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

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

	"github.com/yugabyte/terraform-provider-yba/internal/api"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// YBA ships the unified export-telemetry-configs API and its server-log
// pipelines in different builds on each release line, so every gate carries
// one minimum per line and utils.YBAMinimumVersion picks the line from the
// build the target YBA reports. Each value is the first build on its line
// that contains the yugabyte-db commit making the API public:
//
//   - unifiedTelemetryAPIMin: the v2 export-telemetry-configs API exists at
//     all (d84b20c52c, 2026-03-17, and its 2026.1 backport). Below it the
//     server answers 404 and the provider would misread that as a deleted
//     resource.
//   - serverLogPipelinesMin: the six server-log pipelines are public
//     (f56b4bf608, 2026-08-24; backported to 2026.1 as 14027f1cc0). Below it
//     the server rejects the spec with an unrecognized-field error.
var (
	unifiedTelemetryAPIMin = utils.YBAMinimumVersion{
		Stable:  "2026.1.0.0-b61",
		Preview: "2.29.0.0-b622",
	}
	serverLogPipelinesMin = utils.YBAMinimumVersion{
		Stable:  "2026.1.2.0-b84",
		Preview: "2.31.0.0-b386",
	}
)

// versionNote renders the docs sentence for a minimum. subject carries its own
// verb ("This resource requires", "Requires"). Resource-level callers prefix a
// "~> **Note:**" callout; nested-block descriptions stay one paragraph because
// tfplugindocs appends the nested-schema link to the description's last line.
func versionNote(subject string, minimum utils.YBAMinimumVersion) string {
	return fmt.Sprintf("%s YugabyteDB Anywhere `%s` (stable) or "+
		"`%s` (preview) or later; `terraform plan` fails against an older build.",
		subject, minimum.Stable, minimum.Preview)
}

// validateYBAVersion fails the plan when the target YBA predates the unified
// API, or predates the server-log pipelines while the config uses one. It is
// skipped without a server to ask (nil meta, a bootstrap client). A version
// string CompareYbVersions cannot read is logged and let through, so the
// server's own error stays the backstop for builds outside YBA's scheme.
func validateYBAVersion(
	ctx context.Context, d *schema.ResourceDiff, meta interface{},
) error {
	c, ok := meta.(*api.APIClient)
	if !ok || c == nil {
		return nil
	}
	version, err := c.AppVersion(ctx)
	if err != nil {
		return err
	}
	if version == "" {
		return nil
	}
	if err := requireMinimum(
		ctx, version, "yba_universe_telemetry_config", unifiedTelemetryAPIMin); err != nil {
		return err
	}
	for _, p := range telemetryPipelines {
		if p.min == nil {
			continue
		}
		if blocks, _ := d.Get(p.label).([]interface{}); len(blocks) == 0 {
			continue
		}
		if err := requireMinimum(ctx, version, p.label, *p.min); err != nil {
			return err
		}
	}
	return nil
}

func requireMinimum(
	ctx context.Context, version, what string, minimum utils.YBAMinimumVersion,
) error {
	ok, applied, err := utils.MeetsMinimum(version, minimum)
	switch {
	case err != nil:
		// Outside YBA's version scheme: log it and let the server decide.
		tflog.Warn(ctx, "cannot parse the YBA version; skipping the minimum-version check",
			map[string]interface{}{"version": version, "check": what})
	case !ok:
		return fmt.Errorf(
			"%s requires YugabyteDB Anywhere %s or later (stable: %s, preview: %s); "+
				"the target YBA reports %s",
			what, applied, minimum.Stable, minimum.Preview, version)
	}
	return nil
}
