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
// the docs templates publish the first paragraph as the Registry summary. No
// stable YBA release has the Perf Advisor API that the resources use yet. When
// one does, gate the resources on it and name that release in the docs.
const previewAdmonition = "~> **Preview:** The YugabyteDB Anywhere Perf Advisor " +
	"API that this resource uses is in preview. It can change in ways that are " +
	"not backward compatible between YBA releases.\n\n"

// collectorAPIAdmonition follows the summary paragraph of the yba_pa_collector
// Description. YBA marks its Perf Advisor collector API as internal; the docs
// call it preview, as for the Perf Advisor resources.
const collectorAPIAdmonition = "~> **Preview:** The YugabyteDB Anywhere Perf " +
	"Advisor collector API that this data source uses is in preview. It can " +
	"change in ways that are not backward compatible between YBA releases.\n\n"

// onlineModeKey is the customer runtime config key that turns on Perf Advisor
// online mode. YBA ships it set to false.
const onlineModeKey = "`yb.ui.feature_flags.enable_pa_online_mode`"

func previewWarning(resourceName string) diag.Diagnostic {
	return diag.Diagnostic{
		Severity: diag.Warning,
		Summary: fmt.Sprintf(
			"%s uses a preview YugabyteDB Anywhere API", resourceName),
		Detail: "The YugabyteDB Anywhere Perf Advisor API that this resource uses " +
			"is in preview. It can change in ways that are not backward compatible " +
			"between YBA releases. Pin your provider version and read the release " +
			"notes before you upgrade.",
	}
}
