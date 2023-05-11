// Licensed to Yugabyte, Inc. under one or more contributor license
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
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/utils"
)

func S3Schema() *schema.Resource {
	return &schema.Resource{
		Schema: map[string]*schema.Schema{
			"access_key_id": {
				Type:        schema.TypeString,
				Computed:    true,
				Sensitive:   true,
				Description: "S3 Access Key ID",
			},
			"secret_access_key": {
				Type:        schema.TypeString,
				Computed:    true,
				Sensitive:   true,
				Description: "S3 Secret Access Key",
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

func formatInputS3(ctx context.Context, data []interface{}) (map[string]interface{}, error) {

	s3 := make(map[string]interface{})
	awsCreds, err := utils.AwsCredentialsFromEnv()
	if err != nil {
		return nil, err
	}
	for _, v := range data {
		s3 = v.(map[string]interface{})
		s3["accessKeyId"] = awsCreds.AccessKeyID
		s3["secretAccessKey"] = awsCreds.SecretAccessKey
		s3["paths"] = formatInputPaths(ctx, s3["paths"])

	}
	return s3, nil
}

func formatOutputS3(ctx context.Context, s3 map[string]interface{}) []map[string]interface{} {

	s3["access_key_id"] = s3["accessKeyId"]
	delete(s3, "accessKeyId")
	s3["secret_access_key"] = s3["secretAccessKey"]
	delete(s3, "secretAccessKey")
	mapSlice := []map[string]interface{}{}
	paths_formatted := formatOutputPaths(ctx, s3["paths"].(map[string]interface{}))
	s3["paths"] = append(mapSlice, paths_formatted)

	s3_formatted := []map[string]interface{}{}
	s3_formatted = append(s3_formatted, s3)
	return s3_formatted
}
