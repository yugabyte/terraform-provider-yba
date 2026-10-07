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
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
)

// previewAdmonition is its own Description paragraph. It follows the summary
// paragraph, which the doc templates publish as the Registry summary.
const previewAdmonition = "~> **Preview:** YugabyteDB Anywhere marks its " +
	"telemetry export APIs as preview. A later YBA release can change them in " +
	"ways that are not backward compatible. Read the release notes before you " +
	"upgrade YBA or the provider."

func previewWarning(resourceName string) diag.Diagnostic {
	return diag.Diagnostic{
		Severity: diag.Warning,
		Summary: fmt.Sprintf(
			"%s uses a preview YugabyteDB Anywhere API", resourceName),
		Detail: "YugabyteDB Anywhere marks its telemetry export APIs as preview. " +
			"A later YBA release can change them in ways that are not backward " +
			"compatible. Pin your provider version and read the release notes " +
			"before you upgrade YBA or the provider.",
	}
}
