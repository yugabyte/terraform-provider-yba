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
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/customdiff"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	client "github.com/yugabyte/platform-go-client"
	"github.com/yugabyte/terraform-provider-yba/internal/api"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
	"golang.org/x/exp/slices"
)

// ResourceOnPremNodeInstances creates and maintains resource for nodes for an OnPrem providers
func ResourceOnPremNodeInstances() *schema.Resource {
	return &schema.Resource{
		Description: "Resource to add node instances to an on-premises provider.",

		CreateContext: resourceOnPremNodeCreate,
		ReadContext:   resourceOnPremNodeRead,
		UpdateContext: resourceOnPremNodeUpdate,
		DeleteContext: resourceOnPremNodeDelete,

		CustomizeDiff: resourceOnPremNodeDiff(),

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(10 * time.Minute),
			Delete: schema.DefaultTimeout(5 * time.Minute),
		},

		Schema: map[string]*schema.Schema{
			"provider_uuid": {
				Type:         schema.TypeString,
				Optional:     true,
				AtLeastOneOf: []string{"provider_uuid", "provider_name"},
				Description: "UUID of the On-Premises Provider for the node. At least one of " +
					"provider_uuid or provider_name is required.",
				DiffSuppressFunc: func(k, old, new string, d *schema.ResourceData) bool {
					pName := d.Get("provider_name").(string)
					if len(old) > 0 && len(new) == 0 {
						// Check if provider name is give
						if len(pName) > 0 {
							return true
						}
					}
					return false
				},
			},
			"provider_name": {
				Type:         schema.TypeString,
				Optional:     true,
				AtLeastOneOf: []string{"provider_uuid", "provider_name"},
				Description: "Name of the On-Premises Provider for the node. At least one of " +
					"provider_uuid or provider_name is required.",
				DiffSuppressFunc: func(k, old, new string, d *schema.ResourceData) bool {
					pUUID := d.Get("provider_uuid").(string)
					if len(old) > 0 && len(new) == 0 {
						// Check if provider UUID is give
						if len(pUUID) > 0 {
							return true
						}
					}
					return false
				},
			},
			"instance_name": {
				Type:        schema.TypeString,
				Optional:    true,
				ForceNew:    true,
				Description: "Node instance name provided by the user.",
			},
			"instance_type": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "Node instance type.",
			},
			"instance_type_code": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "Node instance type code.",
			},
			"ip": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "IP address of node.",
			},
			"node_name": {
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
				Description: "Node name allocated during universe creation.",
			},
			"node_configs": {
				Type:        schema.TypeList,
				Optional:    true,
				Description: "Node Configurations.",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"type": {
							Type:     schema.TypeString,
							Required: true,
							Description: "Type of node configurations. For example: " +
								"SSH_PORT(\"SSH port is open\"), NODE_AGENT_ACCESS(\"Reachability " +
								"of node agent server\")",
						},
						"value": {
							Type:        schema.TypeString,
							Required:    true,
							Description: "Value of node configuration.",
						},
					},
				},
			},
			"region": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "Region of node.",
			},
			"zone": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "Zone of node.",
			},
			"zone_uuid": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "Zone UUID of node.",
			},
			"ssh_user": {
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
				Description: "SSH user.",
			},
			"details_json": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "Node details.",
			},
			"in_use": {
				Type:        schema.TypeBool,
				Optional:    true,
				Computed:    true,
				Description: "Is the node used in a universe.",
			},
		},
	}
}

func resourceOnPremNodeDiff() schema.CustomizeDiffFunc {
	return customdiff.All(customdiff.IfValueChange("ip",
		func(ctx context.Context, old, new, meta interface{}) bool {
			return strings.Compare(old.(string), new.(string)) != 0 && old.(string) != ""
		},
		func(ctx context.Context, d *schema.ResourceDiff, meta interface{}) error {
			// if node is in use, restrict removal
			c := meta.(*api.APIClient).YugawareClient
			cUUID := meta.(*api.APIClient).CustomerID
			pUUID := d.Get("provider_uuid").(string)
			pName := d.Get("provider_name").(string)

			pUUID, pName, err := fetchProviderUUIDAndName(ctx, c, cUUID, pUUID, pName)
			if err != nil {
				return err
			}

			ip := d.Get("ip").(string)
			existingNodeInstances, err := nodeInstancesRead(ctx, c, cUUID, pUUID)
			if err != nil {
				return err
			}
			inUseNodes := make([]string, 0)
			for _, n := range existingNodeInstances {
				if n.GetInUse() {
					details := n.GetDetails()
					ip := details.GetIp()
					inUseNodes = append(inUseNodes, ip)

				}
			}
			if len(inUseNodes) > 0 {
				if !slices.Contains(inUseNodes, ip) {
					return fmt.Errorf("Unable to remove in-use node: %v", ip)
				}
			}
			return nil
		},
	))
}

func findProviderByName(providers []client.Provider, name string) (*client.Provider, error) {
	for _, p := range providers {
		if strings.Compare(p.GetName(), name) == 0 {
			return &p, nil
		}
	}
	return nil, fmt.Errorf("Unable to find provider %s", name)
}

func fetchProviderList(ctx context.Context, c *client.APIClient, cUUID string) (
	[]client.Provider, error) {
	r, response, err := c.CloudProvidersApi.GetListOfProviders(ctx, cUUID).Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"On Prem Node Instance - Get Provider", "Read")
		return nil, errMessage
	}
	return r, nil
}

func fetchProviderUUIDAndName(ctx context.Context, c *client.APIClient, cUUID, pUUID,
	pName string) (string, string, error) {
	var err error

	r, err := fetchProviderList(ctx, c, cUUID)
	if err != nil {
		return "", "", err
	}

	if len(pUUID) == 0 && len(pName) > 0 {
		p, err := findProviderByName(r, pName)
		if err != nil {
			return "", "", err
		}
		pUUID = p.GetUuid()

	} else if len(pUUID) > 0 && len(pName) == 0 {
		p, err := findProvider(r, pUUID)
		if err != nil {
			return "", "", err
		}
		pName = p.GetName()
	}

	return pUUID, pName, nil
}

func resourceOnPremNodeCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) (
	diag.Diagnostics) {
	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID
	var err error

	pUUID := d.Get("provider_uuid").(string)
	pName := d.Get("provider_name").(string)

	pUUID, pName, err = fetchProviderUUIDAndName(ctx, c, cUUID, pUUID, pName)
	if err != nil {
		return diag.FromErr(err)
	}

	err = d.Set("provider_uuid", pUUID)
	if err != nil {
		return diag.FromErr(err)
	}
	err = d.Set("provider_name", pName)
	if err != nil {
		return diag.FromErr(err)
	}

	nodeConfigs := make([]map[string]interface{}, 0)
	for _, nc := range d.Get("node_configs").([]interface{}) {
		n := nc.(map[string]interface{})
		i := map[string]interface{}{
			"type":  n["type"],
			"value": n["value"],
		}
		nodeConfigs = append(nodeConfigs, i)
	}

	nodes := make([]interface{}, 0)
	i := map[string]interface{}{
		"instance_name": d.Get("instance_name").(string),
		"instance_type": d.Get("instance_type").(string),
		"ip":            d.Get("ip").(string),
		"node_name":     d.Get("node_name").(string),
		"node_configs":  nodeConfigs,
		"region":        d.Get("region").(string),
		"zone":          d.Get("zone").(string),
		"ssh_user":      d.Get("ssh_user").(string),
		"in_use":        d.Get("in_use").(bool),
	}
	nodes = append(nodes, i)

	nodeListReturned, err := nodeInstancesCreate(ctx, c, cUUID, pUUID, nodes)
	if err != nil {
		return diag.FromErr(err)
	}
	if len(nodeListReturned) > 0 {
		n := nodeListReturned[0]
		d.SetId(n["node_uuid"].(string))
	}

	return resourceOnPremNodeRead(ctx, d, meta)

}

func resourceOnPremNodeRead(ctx context.Context, d *schema.ResourceData, meta interface{}) (
	diag.Diagnostics) {
	var diags diag.Diagnostics
	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID
	nUUID := d.Id()

	node, err := nodeInstanceGet(ctx, c, cUUID, nUUID)
	if err != nil {
		return diag.FromErr(err)
	}
	details := node.GetDetails()

	err = d.Set("instance_name", node.GetInstanceName())
	if err != nil {
		return diag.FromErr(err)
	}

	err = d.Set("instance_type", details.GetInstanceType())
	if err != nil {
		return diag.FromErr(err)
	}

	err = d.Set("instance_type_code", node.GetInstanceTypeCode())
	if err != nil {
		return diag.FromErr(err)
	}

	err = d.Set("ip", details.GetIp())
	if err != nil {
		return diag.FromErr(err)
	}

	err = d.Set("node_name", node.GetNodeName())
	if err != nil {
		return diag.FromErr(err)
	}

	err = d.Set("region", details.GetRegion())
	if err != nil {
		return diag.FromErr(err)
	}

	err = d.Set("zone", details.GetZone())
	if err != nil {
		return diag.FromErr(err)
	}

	err = d.Set("zone_uuid", node.GetZoneUuid())
	if err != nil {
		return diag.FromErr(err)
	}

	err = d.Set("ssh_user", details.GetSshUser())
	if err != nil {
		return diag.FromErr(err)
	}

	err = d.Set("details_json", node.GetDetailsJson())
	if err != nil {
		return diag.FromErr(err)
	}

	err = d.Set("in_use", node.GetInUse())
	if err != nil {
		return diag.FromErr(err)
	}

	err = d.Set("node_configs", flattenNodeConfig(details.GetNodeConfigs()))
	if err != nil {
		return diag.FromErr(err)
	}

	pUUID := d.Get("provider_uuid").(string)
	pName := d.Get("provider_name").(string)

	pUUID, pName, err = fetchProviderUUIDAndName(ctx, c, cUUID, pUUID, pName)
	if err != nil {
		return diag.FromErr(err)
	}

	err = d.Set("provider_uuid", pUUID)
	if err != nil {
		return diag.FromErr(err)
	}
	err = d.Set("provider_name", pName)
	if err != nil {
		return diag.FromErr(err)
	}
	return diags

}

func resourceOnPremNodeUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) (
	diag.Diagnostics) {
	return diag.Diagnostics{}
}

func resourceOnPremNodeDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) (
	diag.Diagnostics) {
	var diags diag.Diagnostics

	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID

	pUUID := d.Get("provider_uuid").(string)
	nUUID := d.Id()
	ip := d.Get("ip").(string)

	node, err := nodeInstanceGet(ctx, c, cUUID, nUUID)
	if err != nil {
		return diag.FromErr(err)
	}
	inUse := node.GetInUse()
	if inUse {
		return diag.FromErr(fmt.Errorf("Unable to remove in use node: %v", ip))
	}
	err = nodeInstanceDelete(ctx, c, cUUID, pUUID, ip)
	if err != nil {
		return diag.FromErr(err)
	}
	d.SetId("")
	return diags
}
