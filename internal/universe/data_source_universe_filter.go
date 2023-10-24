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

package universe

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	client "github.com/yugabyte/platform-go-client"
	"github.com/yugabyte/terraform-provider-yba/internal/api"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
	"golang.org/x/exp/slices"
)

// UniverseFilter keeps track of the filtered universes
func UniverseFilter() *schema.Resource {
	return &schema.Resource{
		Description: "List of universes matching all the given filters.",

		ReadContext: dataSourceUniverseFilterRead,

		Schema: map[string]*schema.Schema{
			"universes": {
				Type:        schema.TypeMap,
				Elem:        schema.TypeString,
				Computed:    true,
				Description: "Map of universe name to UUIDs.",
			},
			"codes": {
				Type:     schema.TypeList,
				Elem:     &schema.Schema{Type: schema.TypeString},
				Optional: true,
				Description: "List of universe provider codes to be matched. " +
					"Allowed values: gcp, aws, azu, onprem.",
			},
			"name": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Part of the universe name to be matched.",
			},
			"provider_uuid": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Provider UUID to be matched.",
			},
			"num_nodes": {
				Type:        schema.TypeInt,
				Optional:    true,
				Description: "Number of nodes in the universe.",
			},
			"replication_factor": {
				Type:        schema.TypeInt,
				Optional:    true,
				Description: "Replication factor of the universe.",
			},
			"is_ysql": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "Does universe have YSQL endpoints.",
			},
			"is_ycql": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "Does universe have YCQL endpoints.",
			},
		},
	}
}

func dataSourceUniverseFilterRead(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID

	codes := d.Get("codes")
	name := d.Get("name").(string)
	providerUUID := d.Get("provider_uuid").(string)
	numNodes := d.Get("num_nodes").(int)
	rf := d.Get("replication_factor").(int)
	isYSQL := d.Get("is_ysql").(bool)
	isYCQL := d.Get("is_ycql").(bool)

	var r []client.UniverseResp
	var err error
	var response *http.Response

	r, response, err = c.UniverseManagementApi.ListUniverses(ctx, cUUID).Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.DataSourceEntity,
			"Universe Filter", "Read")
		return diag.FromErr(errMessage)
	}

	if name != "" {
		result := make([]client.UniverseResp, 0)
		for _, u := range r {
			if strings.Contains(u.GetName(), name) {
				result = append(result, u)
			}
		}
		r = result
	}

	if numNodes != 0 {
		result := make([]client.UniverseResp, 0)
		for _, u := range r {
			uDetails := u.GetUniverseDetails()
			if len(uDetails.GetNodeDetailsSet()) == numNodes {
				result = append(result, u)
			}
		}
		r = result
	}

	result := make([]client.UniverseResp, 0)
	for _, u := range r {
		addUniverse := true
		uDetails := u.GetUniverseDetails()
		var cluster client.Cluster
		for _, c := range uDetails.GetClusters() {
			if c.GetClusterType() == "PRIMARY" {
				cluster = c
			}
		}
		userIntent := cluster.GetUserIntent()
		if codes != nil && len(codes.([]interface{})) != 0 {
			codesList := utils.StringSlice(codes.([]interface{}))
			if slices.Contains(*codesList, userIntent.GetProviderType()) {
				addUniverse = true
			} else if addUniverse {
				addUniverse = false
			}
		}
		if providerUUID != "" {
			if userIntent.GetProvider() == providerUUID {
				addUniverse = true
			} else if addUniverse {
				addUniverse = false
			}
		}
		if rf != 0 {
			if userIntent.GetReplicationFactor() == int32(rf) {
				addUniverse = true
			} else if addUniverse {
				addUniverse = false
			}
		}
		if isYCQL {
			if userIntent.GetEnableYCQL() == true {
				addUniverse = true
			} else if addUniverse {
				addUniverse = false
			}
		}
		if isYSQL {
			if userIntent.GetEnableYSQL() == true {
				addUniverse = true
			} else if addUniverse {
				addUniverse = false
			}
		}

		if addUniverse {
			result = append(result, u)
		}
	}
	r = result

	universes := make(map[string]string, 0)
	for _, u := range r {

		universes[u.GetName()] = u.GetUniverseUUID()
	}

	d.Set("universes", universes)
	d.SetId(strconv.Itoa(len(universes)))
	return diags
}
