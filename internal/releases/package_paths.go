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

package releases

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// PackagePathsSchema is used to hold the path and checksum to the YBDB x86_64 package path
func PackagePathsSchema() *schema.Resource {
	return &schema.Resource{
		Schema: map[string]*schema.Schema{
			"x86_64": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "Path to x86_64 package.",
			},
			"x86_64_checksum": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Checksum for x86_64 package.",
			},
		},
	}
}

func formatInputPaths(ctx context.Context, paths interface{}) map[string]interface{} {

	path := make(map[string]interface{})
	for _, p := range paths.([]interface{}) {
		path = p.(map[string]interface{})
	}
	return path
}

func formatOutputPaths(ctx context.Context, paths map[string]interface{}) map[string]interface{} {

	if checksum, exists := paths["x86_64Checksum"]; exists {
		paths["x86_64_checksum"] = checksum
		delete(paths, "x86_64Checksum")
	}
	return paths

}
