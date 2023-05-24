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
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// GcsSchema is used to describe path and credentials of YBDB releases imported from GCS buckets
func GcsSchema() *schema.Resource {
	return &schema.Resource{
		Schema: map[string]*schema.Schema{
			"credentials_json": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "GCS Credentials in from json file",
			},
			"paths": {
				Type:        schema.TypeList,
				MaxItems:    1,
				Required:    true,
				Elem:        PackagePathsSchema(),
				Description: "Package path and checksum",
			},
		},
	}

}

func formatInputGcs(ctx context.Context, data []interface{}) (map[string]interface{}, error) {

	gcs := make(map[string]interface{})
	for _, v := range data {
		gcs = v.(map[string]interface{})
		var err error
		gcs["credentialsJson"], err = utils.GcpGetCredentialsAsString()
		if err != nil {
			return nil, err
		}
		gcs["paths"] = formatInputPaths(ctx, gcs["paths"])

	}
	return gcs, nil
}

func formatOutputGcs(ctx context.Context, gcs map[string]interface{}) []map[string]interface{} {

	gcs["credentials_json"] = gcs["credentialsJson"]
	delete(gcs, "credentialsJson")
	mapSlice := []map[string]interface{}{}
	paths_formatted := formatOutputPaths(ctx, gcs["paths"].(map[string]interface{}))
	gcs["paths"] = append(mapSlice, paths_formatted)

	gcs_formatted := []map[string]interface{}{}
	gcs_formatted = append(gcs_formatted, gcs)
	return gcs_formatted
}
