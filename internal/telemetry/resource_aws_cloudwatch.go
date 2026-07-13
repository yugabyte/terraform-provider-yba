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
)

// ResourceAWSCloudWatchTelemetryProvider exposes an AWS CloudWatch Logs export
// destination that universes attach via yba_universe_telemetry_config.
func ResourceAWSCloudWatchTelemetryProvider() *schema.Resource {
	return sinkResource(sinkSpec{
		resourceType: "yba_aws_cloudwatch_telemetry_provider",
		displayName:  "AWS CloudWatch",
		apiType:      typeAWSCloudWatch,
		description: "AWS CloudWatch Telemetry Provider resource. Defines a " +
			"reusable CloudWatch Logs destination that universes can use to " +
			"export audit logs and query logs.",
		fields: map[string]*schema.Schema{
			"log_group": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "CloudWatch log group.",
			},
			"log_stream": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "CloudWatch log stream.",
			},
			"region": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "AWS region.",
			},
			"access_key": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Sensitive:   true,
				Description: "AWS access key with CloudWatch permissions.",
			},
			"secret_key": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Sensitive:   true,
				Description: "AWS secret key for the access key.",
			},
			"role_arn": {
				Type:        schema.TypeString,
				Optional:    true,
				ForceNew:    true,
				Description: "Optional IAM role ARN to assume.",
			},
			"endpoint": {
				Type:        schema.TypeString,
				Optional:    true,
				ForceNew:    true,
				Description: "Optional override endpoint URL (e.g. for VPC endpoints).",
			},
		},
		buildConfig: func(d *schema.ResourceData) map[string]interface{} {
			out := map[string]interface{}{
				"logGroup":  d.Get("log_group"),
				"logStream": d.Get("log_stream"),
				"region":    d.Get("region"),
				"accessKey": d.Get("access_key"),
				"secretKey": d.Get("secret_key"),
			}
			setIfNonEmpty(out, "roleARN", d.Get("role_arn"))
			setIfNonEmpty(out, "endpoint", d.Get("endpoint"))
			return out
		},
	})
}
