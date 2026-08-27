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

// Package perfadvisor manages the YugabyteDB Anywhere Perf Advisor endpoints
// (yba_perf_advisor_endpoint), collector lookup, and universe registration.
package perfadvisor

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

	"github.com/yugabyte/terraform-provider-yba/internal/api"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// DataSourcePACollector looks up the Perf Advisor collector a universe
// registration should target.
//
// Read-only by design: the only collector YBA supports today is the embedded
// one, which YBA creates and owns itself - its API refuses create, edit and
// delete for it. So there is nothing here to declare, only a UUID to look up.
func DataSourcePACollector() *schema.Resource {
	return &schema.Resource{
		Description: "Perf Advisor Collector data source. Looks up the " +
			"collector that scrapes universes for Perf Advisor, so its UUID " +
			"can be passed to `yba_universe_perf_advisor_registration`.\n\n" +
			"~> **Note:** The embedded collector is created and managed by " +
			"YBA itself and cannot be declared in Terraform. With no filter " +
			"this data source returns the single configured collector, and " +
			"errors if there is more than one.",

		ReadContext: dataSourcePACollectorRead,

		Timeouts: &schema.ResourceTimeout{
			Read: schema.DefaultTimeout(5 * time.Minute),
		},

		Schema: map[string]*schema.Schema{
			"uuid": {
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
				Description: "UUID of the collector. Set to select a specific one.",
			},
			"pa_url": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "URL of the Perf Advisor the collector runs against.",
			},
			"yba_url": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "URL the collector uses to reach this YBA.",
			},
			"metrics_url": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "URL of the Prometheus the collector scrapes.",
			},
			"metrics_scrape_period_secs": {
				Type:        schema.TypeInt,
				Computed:    true,
				Description: "Scrape interval, in seconds.",
			},
			"embedded": {
				Type:     schema.TypeBool,
				Computed: true,
				Description: "True when this is the embedded Perf Advisor that " +
					"YBA manages itself.",
			},
			"in_use_status": {
				Type:     schema.TypeString,
				Computed: true,
				Description: "Whether any universe is registered with this " +
					"collector: IN_USE, NOT_IN_USE, or ERROR when the " +
					"collector could not be reached.",
			},
		},
	}
}

func dataSourcePACollectorRead(
	ctx context.Context, d *schema.ResourceData, meta interface{},
) diag.Diagnostics {
	c := meta.(*api.APIClient)

	collectors, response, err := c.YugawareClient.PACollectorAPI.
		ListAllPACollectors(ctx, c.CustomerID).Execute()
	if err != nil {
		return diag.FromErr(utils.ErrorFromHTTPResponse(
			response, err, "PA Collector", "Read", "List"))
	}
	if len(collectors) == 0 {
		return diag.Errorf(
			"no Perf Advisor collector is configured for this customer")
	}

	wanted, hasWanted := d.GetOk("uuid")
	if !hasWanted && len(collectors) > 1 {
		return diag.Errorf(
			"%d Perf Advisor collectors are configured; set uuid to select one",
			len(collectors))
	}

	for _, collector := range collectors {
		if hasWanted && collector.GetUuid() != wanted.(string) {
			continue
		}
		d.SetId(collector.GetUuid())
		if err := d.Set("uuid", collector.GetUuid()); err != nil {
			return diag.FromErr(err)
		}
		if err := d.Set("pa_url", collector.GetPaUrl()); err != nil {
			return diag.FromErr(err)
		}
		if err := d.Set("yba_url", collector.GetYbaUrl()); err != nil {
			return diag.FromErr(err)
		}
		if err := d.Set("metrics_url", collector.GetMetricsUrl()); err != nil {
			return diag.FromErr(err)
		}
		if err := d.Set("metrics_scrape_period_secs",
			collector.GetMetricsScrapePeriodSecs()); err != nil {
			return diag.FromErr(err)
		}
		if err := d.Set("embedded", collector.GetEmbedded()); err != nil {
			return diag.FromErr(err)
		}
		if err := d.Set("in_use_status", collector.GetInUseStatus()); err != nil {
			return diag.FromErr(err)
		}
		return nil
	}
	return diag.Errorf("Perf Advisor collector %s was not found", wanted)
}
