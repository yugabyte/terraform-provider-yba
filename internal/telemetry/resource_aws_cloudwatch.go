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
		description: "Manages an AWS CloudWatch telemetry provider in YugabyteDB " +
			"Anywhere. Universes send logs to it, in CloudWatch Logs, through " +
			"`yba_universe_telemetry_config`.",
		notes: "~> **Note:** YBA accepts an AWS CloudWatch telemetry provider only in " +
			"a log pipeline, not in `metrics`. All AWS CloudWatch and S3 " +
			"telemetry providers that one universe uses must have the same " +
			"`access_key` and `secret_key`.\n\n",
		fields: map[string]*schema.Schema{
			"log_group": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "CloudWatch Logs log group.",
			},
			"log_stream": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "CloudWatch Logs log stream.",
			},
			"region": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "AWS region of the log group.",
			},
			"access_key": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Sensitive:   true,
				Description: "AWS access key ID with permission to write to CloudWatch Logs.",
			},
			"secret_key": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Sensitive:   true,
				Description: "AWS secret access key of `access_key`.",
			},
			"role_arn": {
				Type:        schema.TypeString,
				Optional:    true,
				ForceNew:    true,
				Description: "ARN of an IAM role to assume to write the logs.",
			},
			"endpoint": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
				Description: "CloudWatch Logs endpoint URL to use instead of the " +
					"default, for example a VPC endpoint.",
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
