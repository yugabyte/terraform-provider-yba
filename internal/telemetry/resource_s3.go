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
		apiType:      typeS3,
		description: "Manages an Amazon S3 telemetry provider in YugabyteDB " +
			"Anywhere. Universes send logs to an S3 bucket through " +
			"`yba_universe_telemetry_config`, for example to archive audit logs.",
		notes: "~> **Note:** Requires YugabyteDB Anywhere 2026.1.0.0 or later. YBA " +
			"creates, reads and deletes an S3 telemetry provider only when the " +
			"global runtime config `yb.telemetry.allow_s3` is `true`. The default is `false`. " +
			"To set it, use the `yba_runtime_config` resource. YBA accepts an S3 " +
			"telemetry provider only in a log pipeline, not in `metrics`. All AWS " +
			"CloudWatch and S3 telemetry providers that one universe uses must " +
			"have the same `access_key` and `secret_key`.\n\n",
		fields: map[string]*schema.Schema{
			"bucket": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
				Description: "Name of the S3 bucket. When YBA creates the telemetry " +
					"provider, it writes a test object to the bucket to check access.",
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
				Description: "AWS access key ID with permission to write to the bucket.",
			},
			"secret_key": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Sensitive:   true,
				Description: "AWS secret access key of `access_key`.",
			},
			"directory_prefix": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
				Description: "Root directory in the bucket for the objects. " +
					"YBA uses `yb-logs/` when this is not set.",
			},
			"file_prefix": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
				Description: "Prefix of each object name. YBA uses `yb-otel-` " +
					"when this is not set.",
			},
			"endpoint": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
				Description: "S3 endpoint URL to use instead of the default, for " +
					"example a VPC endpoint or an S3-compatible store.",
			},
			"role_arn": {
				Type:        schema.TypeString,
				Optional:    true,
				ForceNew:    true,
				Description: "ARN of an IAM role to assume to write the objects.",
			},
			"partition": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
				ValidateFunc: validation.StringInSlice(
					[]string{"hour", "minute"},
					false,
				),
				Description: "Time unit of the directory layout in the bucket: " +
					"`hour` or `minute`. YBA uses `minute` when this is not set.",
			},
			"marshaler": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
				Description: "Format of the objects: `OTLP_JSON` or `SUMO_IC`. " +
					"YBA uses `OTLP_JSON` when this is not set.",
			},
			"disable_ssl": {
				Type:        schema.TypeBool,
				Optional:    true,
				ForceNew:    true,
				Default:     false,
				Description: "Connect to the S3 endpoint without TLS. Defaults to `false`.",
			},
			"force_path_style": {
				Type:     schema.TypeBool,
				Optional: true,
				ForceNew: true,
				Default:  false,
				Description: "Use path-style addressing instead of " +
					"virtual-hosted-style addressing. Defaults to `false`.",
			},
			"include_universe_and_node_in_prefix": {
				Type:     schema.TypeBool,
				Optional: true,
				ForceNew: true,
				Default:  false,
				Description: "Add `<universe-uuid>/<node-name>` to the directory " +
					"of each object. Defaults to `false`.",
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
