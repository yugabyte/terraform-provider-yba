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
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

	"github.com/yugabyte/terraform-provider-yba/internal/api"
)

// DataSourceTelemetryProvider looks up a telemetry provider by name, exposing its
// UUID/type/tags — e.g. to wire an exporter_uuid to a provider created outside
// this configuration.
func DataSourceTelemetryProvider() *schema.Resource {
	return &schema.Resource{
		Description: experimentalAdmonition +
			"Telemetry Provider data source. Looks up an existing telemetry " +
			"provider by name and returns its UUID, type, and tags so it can be " +
			"referenced (e.g. as an `exporter_uuid`) without hard-coding the UUID.",

		ReadContext: dataSourceTelemetryProviderRead,

		Schema: map[string]*schema.Schema{
			"name": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Name of the telemetry provider to look up.",
			},
			"type": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "Telemetry provider type (e.g. DATA_DOG, OTLP, S3).",
			},
			"tags": {
				Type:        schema.TypeMap,
				Computed:    true,
				Elem:        &schema.Schema{Type: schema.TypeString},
				Description: "Tags associated with the telemetry provider.",
			},
		},
	}
}

func dataSourceTelemetryProviderRead(
	ctx context.Context, d *schema.ResourceData, meta interface{},
) diag.Diagnostics {
	apiClient := meta.(*api.APIClient)
	name := d.Get("name").(string)

	providers, err := apiClient.VanillaClient.ListTelemetryProviders(
		ctx, apiClient.CustomerID, apiClient.APIKey)
	if err != nil {
		return diag.FromErr(err)
	}

	var matches []api.TelemetryProvider
	for _, p := range providers {
		if p.Name == name {
			matches = append(matches, p)
		}
	}
	switch len(matches) {
	case 0:
		return diag.Errorf("no telemetry provider found with name %q", name)
	case 1:
		// ok
	default:
		// YBA enforces name uniqueness per customer; surface the anomaly
		// instead of silently picking one.
		return diag.Errorf(
			"found %d telemetry providers named %q; names are expected to be unique",
			len(matches), name)
	}

	provider := matches[0]
	d.SetId(provider.UUID)
	if t, ok := provider.Config["type"].(string); ok {
		if err := d.Set("type", t); err != nil {
			return diag.FromErr(err)
		}
	}
	if err := d.Set("tags", provider.Tags); err != nil {
		return diag.FromErr(err)
	}
	return nil
}
