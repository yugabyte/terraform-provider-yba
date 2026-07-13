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

// ResourceDatadogTelemetryProvider exposes a Datadog export destination that
// universes attach via yba_universe_telemetry_config.
func ResourceDatadogTelemetryProvider() *schema.Resource {
	return sinkResource(sinkSpec{
		resourceType: "yba_datadog_telemetry_provider",
		displayName:  "Datadog",
		apiType:      typeDataDog,
		description: "Datadog Telemetry Provider resource. Defines a reusable " +
			"Datadog export destination that universes can use to export audit " +
			"logs, query logs, and metrics.",
		fields: map[string]*schema.Schema{
			"site": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "Datadog site (e.g. datadoghq.com, datadoghq.eu).",
			},
			"api_key": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Sensitive:   true,
				Description: "Datadog API key.",
			},
		},
		buildConfig: func(d *schema.ResourceData) map[string]interface{} {
			return map[string]interface{}{
				"site":   d.Get("site"),
				"apiKey": d.Get("api_key"),
			}
		},
	})
}
