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
