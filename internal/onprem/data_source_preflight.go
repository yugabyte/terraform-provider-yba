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
	"fmt"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	client "github.com/yugabyte/platform-go-client"
	"github.com/yugabyte/terraform-provider-yba/internal/api"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// PreflightCheck triggers preflight check for all nodes in an onprem provider
func PreflightCheck() *schema.Resource {
	return &schema.Resource{
		Description: "Trigger pre-flight check for list of nodes of the onprem provider.",

		ReadContext: dataSourcePreflightCheckRead,

		Schema: map[string]*schema.Schema{
			"provider_id": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "UUID of the onprem provider.",
			},
			"nodes": {
				Type:     schema.TypeList,
				Elem:     &schema.Schema{Type: schema.TypeString},
				Optional: true,
				Description: "Preflight checks will be triggered for nodes listed. " +
					"If empty, check is triggered for all nodes not in use in a universe.",
			},
		},
	}
}

func dataSourcePreflightCheckRead(ctx context.Context, d *schema.ResourceData, meta interface{}) (
	diag.Diagnostics) {
	var diags diag.Diagnostics

	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID
	pUUID := d.Get("provider_id").(string)
	var nodeList []string
	if d.Get("nodes") != nil {
		nodeListInterface := d.Get("nodes").([]interface{})
		nodeList = *utils.StringSlice(nodeListInterface)
	}
	if len(nodeList) == 0 {
		// list all nodes in the provider
		nodeList = make([]string, 0)
		r, response, err := c.NodeInstancesApi.ListByProvider(ctx, cUUID, pUUID).Execute()
		if err != nil {
			errMessage := utils.ErrorFromHTTPResponse(response, err, utils.DataSourceEntity,
				"Preflight Check", "List Nodes")
			return diag.FromErr(errMessage)
		}
		for _, i := range r {
			details := i.GetDetails()
			nodeList = append(nodeList, details.GetIp())
		}
	}
	for _, nodeIP := range nodeList {
		r, response, err := c.NodeInstancesApi.DetachedNodeAction(ctx, cUUID, pUUID, nodeIP).
			NodeAction(
				client.NodeActionFormData{
					NodeAction: "PRECHECK_DETACHED",
				}).Execute()
		if err != nil {
			errMessage := utils.ErrorFromHTTPResponse(response, err, utils.DataSourceEntity,
				"Preflight Check", "Read")
			return diag.FromErr(errMessage)
		}
		if r.TaskUUID != nil {
			tflog.Debug(ctx, fmt.Sprintf(
				"Waiting for preflight check of node %s in on prem provider %s", nodeIP, pUUID))
			err = utils.WaitForTask(ctx, *r.TaskUUID, cUUID, c, d.Timeout(schema.TimeoutCreate))
			if err != nil {
				return diag.FromErr(err)
			}
		}
	}
	d.SetId("Success")
	d.Set("nodes", nodeList)
	return diags
}
