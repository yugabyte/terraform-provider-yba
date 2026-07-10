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

// ResourceSplunkTelemetryProvider exposes a Splunk HTTP Event Collector export
// destination that universes attach via yba_universe_telemetry_config.
func ResourceSplunkTelemetryProvider() *schema.Resource {
	return sinkResource(sinkSpec{
		resourceType: "yba_splunk_telemetry_provider",
		displayName:  "Splunk",
		apiType:      typeSplunk,
		description: "Splunk Telemetry Provider resource. Defines a reusable " +
			"Splunk HTTP Event Collector destination that universes can use to " +
			"export audit logs and query logs.",
		fields: map[string]*schema.Schema{
			"endpoint": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "Splunk HEC endpoint URL.",
			},
			"token": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Sensitive:   true,
				Description: "Splunk HEC access token.",
			},
			"source": {
				Type:        schema.TypeString,
				Optional:    true,
				ForceNew:    true,
				Description: "Optional Splunk source field.",
			},
			"source_type": {
				Type:        schema.TypeString,
				Optional:    true,
				ForceNew:    true,
				Description: "Optional Splunk source type field.",
			},
			"index": {
				Type:        schema.TypeString,
				Optional:    true,
				ForceNew:    true,
				Description: "Optional Splunk index name.",
			},
		},
		buildConfig: func(d *schema.ResourceData) map[string]interface{} {
			out := map[string]interface{}{
				"endpoint": d.Get("endpoint"),
				"token":    d.Get("token"),
			}
			setIfNonEmpty(out, "source", d.Get("source"))
			setIfNonEmpty(out, "sourceType", d.Get("source_type"))
			setIfNonEmpty(out, "index", d.Get("index"))
			return out
		},
	})
}
