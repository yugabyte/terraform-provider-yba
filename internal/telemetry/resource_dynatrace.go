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

// ResourceDynatraceTelemetryProvider exposes a Dynatrace OTLP export
// destination that universes attach via yba_universe_telemetry_config.
func ResourceDynatraceTelemetryProvider() *schema.Resource {
	return sinkResource(sinkSpec{
		resourceType: "yba_dynatrace_telemetry_provider",
		displayName:  "Dynatrace",
		apiType:      typeDynatrace,
		description: "Dynatrace Telemetry Provider resource. Defines a reusable " +
			"Dynatrace OTLP ingest destination that universes can use to export " +
			"metrics.\n\n" +
			"~> **Note:** YBA allows Dynatrace only as a **metrics** exporter — " +
			"it cannot be referenced from a universe's audit log or query log " +
			"exporter lists.",
		fields: map[string]*schema.Schema{
			"endpoint": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
				Description: "Dynatrace OTLP ingest base URL " +
					"(e.g. https://<env>.live.dynatrace.com/api/v2/otlp).",
			},
			"api_token": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Sensitive:   true,
				Description: "Dynatrace ingest access token.",
			},
		},
		buildConfig: func(d *schema.ResourceData) map[string]interface{} {
			return map[string]interface{}{
				"endpoint": d.Get("endpoint"),
				"apiToken": d.Get("api_token"),
			}
		},
	})
}
