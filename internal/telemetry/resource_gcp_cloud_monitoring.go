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

// ResourceGCPCloudMonitoringTelemetryProvider exposes a Google Cloud
// Monitoring/Logging export destination that universes attach via
// yba_universe_telemetry_config.
func ResourceGCPCloudMonitoringTelemetryProvider() *schema.Resource {
	return sinkResource(sinkSpec{
		resourceType: "yba_gcp_cloud_monitoring_telemetry_provider",
		displayName:  "GCP Cloud Monitoring",
		apiType:      typeGCPCloudMonitor,
		description: "GCP Cloud Monitoring Telemetry Provider resource. Defines " +
			"a reusable Google Cloud Monitoring/Logging destination that " +
			"universes can use to export audit logs and query logs.",
		fields: map[string]*schema.Schema{
			"project": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
				Description: "GCP project ID. If empty, the project_id from the " +
					"service-account credentials is used.",
			},
			"credentials_json": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Sensitive:   true,
				Description: "GCP service account credentials as a JSON string.",
			},
		},
		buildConfig: func(d *schema.ResourceData) map[string]interface{} {
			out := map[string]interface{}{
				"credentialsString": d.Get("credentials_json"),
			}
			setIfNonEmpty(out, "project", d.Get("project"))
			return out
		},
	})
}
