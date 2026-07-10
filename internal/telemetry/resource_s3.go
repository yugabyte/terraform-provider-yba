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
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

// ResourceS3TelemetryProvider exposes an Amazon S3 export destination that
// universes attach via yba_universe_telemetry_config.
func ResourceS3TelemetryProvider() *schema.Resource {
	return sinkResource(sinkSpec{
		resourceType: "yba_s3_telemetry_provider",
		displayName:  "Amazon S3",
		apiType:      typeS3,
		description: "Amazon S3 Telemetry Provider resource. Defines a reusable " +
			"S3 destination that universes can use to export audit logs and " +
			"query logs — useful for long-term archival.",
		fields: map[string]*schema.Schema{
			"bucket": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "S3 bucket name.",
			},
			"region": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "AWS region of the bucket.",
			},
			"access_key": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Sensitive:   true,
				Description: "AWS access key with bucket write permissions.",
			},
			"secret_key": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Sensitive:   true,
				Description: "AWS secret key for the access key.",
			},
			"directory_prefix": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
				Description: "S3 prefix (root directory inside the bucket) " +
					"to write objects under.",
			},
			"file_prefix": {
				Type:        schema.TypeString,
				Optional:    true,
				ForceNew:    true,
				Description: "Optional file-name prefix prepended to every object.",
			},
			"endpoint": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
				Description: "Optional override endpoint URL " +
					"(e.g. for VPC endpoints or S3-compatible stores).",
			},
			"role_arn": {
				Type:        schema.TypeString,
				Optional:    true,
				ForceNew:    true,
				Description: "Optional IAM role ARN to assume.",
			},
			"partition": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
				ValidateFunc: validation.StringInSlice(
					[]string{"hour", "minute"},
					false,
				),
				Description: "Time granularity of the S3 object directory " +
					"layout. One of `hour` or `minute` (YBA default: `minute`).",
			},
			"marshaler": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
				Description: "Optional marshaler used to serialize " +
					"records (defaults to YBA's choice).",
			},
			"disable_ssl": {
				Type:        schema.TypeBool,
				Optional:    true,
				ForceNew:    true,
				Default:     false,
				Description: "Disable SSL when talking to the S3 endpoint.",
			},
			"force_path_style": {
				Type:     schema.TypeBool,
				Optional: true,
				ForceNew: true,
				Default:  false,
				Description: "Force path-style addressing instead of the " +
					"default virtual-hosted style.",
			},
			"include_universe_and_node_in_prefix": {
				Type:     schema.TypeBool,
				Optional: true,
				ForceNew: true,
				Default:  false,
				Description: "Append `<universe-uuid>/<node-name>` to the " +
					"directory prefix when writing objects.",
			},
		},
		buildConfig: func(d *schema.ResourceData) map[string]interface{} {
			out := map[string]interface{}{
				"bucket":    d.Get("bucket"),
				"region":    d.Get("region"),
				"accessKey": d.Get("access_key"),
				"secretKey": d.Get("secret_key"),
			}
			setIfNonEmpty(out, "directoryPrefix", d.Get("directory_prefix"))
			setIfNonEmpty(out, "filePrefix", d.Get("file_prefix"))
			setIfNonEmpty(out, "endpoint", d.Get("endpoint"))
			setIfNonEmpty(out, "roleArn", d.Get("role_arn"))
			setIfNonEmpty(out, "partition", d.Get("partition"))
			setIfNonEmpty(out, "marshaler", d.Get("marshaler"))
			setIfTrue(out, "disableSSL", d.Get("disable_ssl"))
			setIfTrue(out, "forcePathStyle", d.Get("force_path_style"))
			setIfTrue(out, "includeUniverseAndNodeInPrefix",
				d.Get("include_universe_and_node_in_prefix"))
			return out
		},
	})
}
