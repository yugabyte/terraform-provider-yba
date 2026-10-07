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

package perfadvisor

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
)

// previewAdmonition follows the summary paragraph of each resource Description:
// the docs templates publish the first paragraph as the Registry summary.
const previewAdmonition = "~> **Preview:** YugabyteDB Anywhere marks the Perf " +
	"Advisor API that this resource uses as preview. The API can change in ways " +
	"that are not backward compatible between YBA releases.\n\n"

// onlineModeKey is the customer runtime config key that turns on Perf Advisor
// online mode. YBA ships it set to false.
const onlineModeKey = "`yb.ui.feature_flags.enable_pa_online_mode`"

func previewWarning(resourceName string) diag.Diagnostic {
	return diag.Diagnostic{
		Severity: diag.Warning,
		Summary: fmt.Sprintf(
			"%s wraps a preview YBA Perf Advisor API", resourceName),
		Detail: "The underlying YugabyteDB Anywhere Perf Advisor endpoint API " +
			"is marked preview and may change in backward-incompatible ways. " +
			"Pin your provider version and review release notes before upgrading.",
	}
}
