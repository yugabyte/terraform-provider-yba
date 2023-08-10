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

package onprem

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	client "github.com/yugabyte/platform-go-client"
	"github.com/yugabyte/terraform-provider-yba/internal/api"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// NodeInstanceFilter filters node instances of a given onprem provider
func NodeInstanceFilter() *schema.Resource {
	return &schema.Resource{
		Description: "Filter list of nodes handled in the onprem provider.",

		ReadContext: dataSourceNodeInstanceFilterRead,

		Schema: map[string]*schema.Schema{
			"provider_id": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "UUID of the onprem provider.",
			},
			"ip": {
				Type:     schema.TypeString,
				Optional: true,
				Description: "Nodes with IP addresses containing given filter string. " +
					"For example, setting ip = \"10.1.2\" may return nodes " +
					"with IPs \"10.1.20.3\" and \"10.10.1.23\".",
			},
			"instance_type": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Nodes with instance types containing given filter string.",
			},
			"instance_name": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Nodes with instance names containing given filter string.",
			},
			"in_use": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "Nodes of the on premises provider used in a universe.",
			},
			"region": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Nodes in a particular region.",
			},
			"zone": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Nodes in a particular zone.",
			},
			"nodes": {
				Type:        schema.TypeList,
				Computed:    true,
				Elem:        NodeInstanceSchema().Elem,
				Description: "Node instances associated with the provider and given filters.",
			},
		},
	}
}

func dataSourceNodeInstanceFilterRead(ctx context.Context, d *schema.ResourceData,
	meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID
	pUUID := d.Get("provider_id").(string)

	// list all nodes in the provider
	r, response, err := c.NodeInstancesApi.ListByProvider(ctx, cUUID, pUUID).Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.DataSourceEntity,
			"Filter Node Instances", "List Nodes")
		return diag.FromErr(errMessage)
	}

	if d.Get("ip") != nil {
		ip := d.Get("ip").(string)
		result := make([]client.NodeInstance, 0)
		for _, n := range r {
			details := n.GetDetails()
			if strings.Contains(details.GetIp(), ip) {
				result = append(result, n)
			}
		}
		r = result
	}

	if d.Get("instance_type") != nil {
		instanceType := d.Get("instance_type").(string)
		result := make([]client.NodeInstance, 0)
		for _, n := range r {
			if strings.Contains(n.GetInstanceTypeCode(), instanceType) {
				result = append(result, n)
			}
		}
		r = result
	}

	if d.Get("instance_name") != nil {
		instanceName := d.Get("instance_name").(string)
		result := make([]client.NodeInstance, 0)
		for _, n := range r {
			if strings.Contains(n.GetInstanceName(), instanceName) {
				result = append(result, n)
			}
		}
		r = result
	}

	if d.Get("region") != nil {
		region := d.Get("region").(string)
		result := make([]client.NodeInstance, 0)
		for _, n := range r {
			details := n.GetDetails()
			if strings.Contains(details.GetRegion(), region) {
				result = append(result, n)
			}
		}
		r = result
	}

	if d.Get("zone") != nil {
		zone := d.Get("zone").(string)
		result := make([]client.NodeInstance, 0)
		for _, n := range r {
			details := n.GetDetails()
			if strings.Contains(details.GetZone(), zone) {
				result = append(result, n)
			}
		}
		r = result
	}

	if d.Get("in_use") != nil {
		inUse := d.Get("in_use").(bool)
		result := make([]client.NodeInstance, 0)
		for _, n := range r {
			if inUse == n.GetInUse() {
				result = append(result, n)
			}
		}
		r = result
	}

	d.SetId(pUUID)
	d.Set("nodes", flattenNodeInstances(r, nil))
	return diags
}
