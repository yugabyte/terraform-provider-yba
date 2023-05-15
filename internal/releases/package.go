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
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func PackageSchema() *schema.Schema {
	return &schema.Schema{
		Type:     schema.TypeList,
		ForceNew: true,
		Optional: true,
		Computed: true,
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				"path": {
					Type:        schema.TypeString,
					Computed:    true,
					Optional:    true,
					Description: "Path",
				},
				"arch": {
					Type:        schema.TypeString,
					Computed:    true,
					Optional:    true,
					Description: "Architecture",
				},
			},
		},
	}

}
