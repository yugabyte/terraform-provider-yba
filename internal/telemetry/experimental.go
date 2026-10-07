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

// experimentalAdmonition is its own Description paragraph. It follows the
// summary paragraph, which the doc templates publish as the Registry summary.
const experimentalAdmonition = "~> **Experimental:** Telemetry export is an " +
	"experimental feature of YugabyteDB Anywhere. A later YBA release can change " +
	"it in ways that are not backward compatible. Read the release notes before " +
	"you upgrade YBA or the provider."

func experimentalWarning(resourceName string) diag.Diagnostic {
	return diag.Diagnostic{
		Severity: diag.Warning,
		Summary: fmt.Sprintf(
			"%s wraps an experimental YBA telemetry API", resourceName),
		Detail: "The underlying YugabyteDB Anywhere export-telemetry API is " +
			"still experimental and may change in backward-incompatible ways. " +
			"Pin your provider version and review release notes before upgrading.",
	}
}
