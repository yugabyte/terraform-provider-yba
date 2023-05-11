// Licensed to Yugabyte, Inc. under one or more contributor license
// agreements. See the NOTICE file distributed with this work for
// additional information regarding copyright ownership. Yugabyte
// licenses this file to you under the Apache License, Version 2.0
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

func HttpSchema() *schema.Resource {
	return &schema.Resource{
		Schema: map[string]*schema.Schema{
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

func formatInputHttp(ctx context.Context, data []interface{}) map[string]interface{} {

	http := make(map[string]interface{})
	for _, v := range data {
		http = v.(map[string]interface{})
		http["paths"] = formatInputPaths(ctx, http["paths"])

	}
	return http
}

func formatOutputHttp(ctx context.Context, http map[string]interface{}) []map[string]interface{} {

	mapSlice := []map[string]interface{}{}
	paths_formatted := formatOutputPaths(ctx, http["paths"].(map[string]interface{}))
	http["paths"] = append(mapSlice, paths_formatted)

	http_formatted := []map[string]interface{}{}
	http_formatted = append(http_formatted, http)
	return http_formatted
}
