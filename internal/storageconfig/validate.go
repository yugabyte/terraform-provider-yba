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

package storageconfig

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// validateIAMConfigRequiresIAMProfile rejects an iam_config block when
// use_iam_instance_profile is not true. The backend only reads IAM_CONFIGURATION
// inside the isIAMInstanceProfile branch; providing it alongside static access
// keys is always a no-op and signals a misconfigured resource.
func validateIAMConfigRequiresIAMProfile(
	_ context.Context,
	d *schema.ResourceDiff,
	_ interface{},
) error {
	iamConfigs, ok := d.Get("iam_config").([]interface{})
	if !ok || len(iamConfigs) == 0 {
		return nil
	}
	useIAM, _ := d.Get("use_iam_instance_profile").(bool)
	if !useIAM {
		return fmt.Errorf(
			"iam_config block is only valid when use_iam_instance_profile is true; " +
				"remove iam_config or set use_iam_instance_profile = true",
		)
	}
	return nil
}

// validateNoDuplicateRegionLocations is the CustomizeDiff function shared by all storage config
// resources. It rejects configurations where region_locations contains two or more blocks with
// the same region value, since YBA has no server-side guard and such configs cause silent
// corruption when used in backups.
func validateNoDuplicateRegionLocations(
	_ context.Context,
	d *schema.ResourceDiff,
	_ interface{},
) error {
	rawLocs, ok := d.Get("region_locations").([]interface{})
	if !ok || len(rawLocs) == 0 {
		return nil
	}

	seen := make(map[string]bool, len(rawLocs))
	for _, raw := range rawLocs {
		if raw == nil {
			continue
		}
		loc, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		region, _ := loc["region"].(string)
		if region == "" {
			continue
		}
		if seen[region] {
			return fmt.Errorf(
				"duplicate region %q in region_locations: each region must appear at most once",
				region,
			)
		}
		seen[region] = true
	}
	return nil
}
