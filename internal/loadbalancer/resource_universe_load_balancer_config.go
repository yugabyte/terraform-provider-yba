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

// Package loadbalancer manages the attachment of externally-created cloud load
// balancers to YBA universes (yba_universe_load_balancer_config).
package loadbalancer

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	client "github.com/yugabyte/platform-go-client"

	"github.com/yugabyte/terraform-provider-yba/internal/api"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// loadBalancerTaskTimeout bounds every universe task this resource dispatches
// (attach, re-map, disable). Node (de)registration on cloud LBs is quick, but
// the task can queue behind other universe operations holding the lock.
const loadBalancerTaskTimeout = 1 * time.Hour

// ResourceUniverseLoadBalancerConfig manages the load balancer attachment of a
// YBA universe (one resource per universe; the resource ID is the universe UUID).
func ResourceUniverseLoadBalancerConfig() *schema.Resource {
	return &schema.Resource{
		Description: "Universe Load Balancer Config. Attaches externally-created cloud load " +
			"balancers (AWS, GCP, or Azure) to a YugabyteDB Anywhere universe and lets YBA " +
			"manage their node membership through universe operations. The load balancers " +
			"must already exist in the universe's cloud account; this resource does not " +
			"create them.\n\n" +
			"~> **Note:** The underlying YBA endpoint (`update_lb_config`) is a preview API " +
			"that could change.\n\n" +
			"~> **Note:** YBA expects the load balancer to use TCP listeners, and on Azure a " +
			"frontend IP configuration must already exist. Health-check behaviour is tuned " +
			"via the `yb.universe.network_load_balancer.custom_health_check_*` universe " +
			"runtime config keys (see the `yba_runtime_config` resource); misconfigured " +
			"load balancers surface as attach task failures.",

		CreateContext: resourceUniverseLoadBalancerConfigCreate,
		ReadContext:   resourceUniverseLoadBalancerConfigRead,
		UpdateContext: resourceUniverseLoadBalancerConfigUpdate,
		DeleteContext: resourceUniverseLoadBalancerConfigDelete,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(loadBalancerTaskTimeout),
			Update: schema.DefaultTimeout(loadBalancerTaskTimeout),
			Delete: schema.DefaultTimeout(loadBalancerTaskTimeout),
		},

		Schema: map[string]*schema.Schema{
			"universe_uuid": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
				Description: "UUID of the universe to attach load balancers to. The resource " +
					"ID is this UUID (one config per universe); import with the universe UUID.",
			},
			"load_balancer": {
				Type:     schema.TypeSet,
				Required: true,
				MinItems: 1,
				Description: "Per-region load balancer mapping for the universe's primary " +
					"cluster. Each block applies its load balancer to every availability " +
					"zone of the region unless overridden via `az_overrides`.",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"region": {
							Type:     schema.TypeString,
							Required: true,
							Description: "Region code (as it appears in the universe " +
								"placement, e.g. `us-west-2`) this load balancer serves.",
						},
						"lb_name": {
							Type:     schema.TypeString,
							Required: true,
							Description: "Cloud-side load balancer identifier: the load " +
								"balancer name on AWS and Azure, the backend service name " +
								"on GCP.",
						},
						"lb_fqdn": {
							Type:     schema.TypeString,
							Optional: true,
							Description: "Optional FQDN clients use to reach this load " +
								"balancer. Stored as connection metadata on the universe's " +
								"region placement; not used to manage node membership.",
						},
						"read_replica": {
							Type:     schema.TypeBool,
							Optional: true,
							Description: "Attach this load balancer to the universe's read " +
								"replica cluster instead of the primary cluster. YBA supports " +
								"at most one read replica per universe. Defaults to `false` " +
								"(primary).",
						},
						"az_overrides": {
							Type:     schema.TypeMap,
							Optional: true,
							Elem:     &schema.Schema{Type: schema.TypeString},
							Description: "Map of availability zone name to load balancer " +
								"name for zones that should use a different load balancer " +
								"than the region default (e.g. one load balancer per AZ for " +
								"zone-local application traffic).",
						},
					},
				},
			},
		},
	}
}

// updateLoadBalancerConfigParams is the JSON body PUT to update_lb_config.
// YBA binds it to the full UniverseDefinitionTaskParams; only the universe
// UUID (validated against the path) and the clusters carrying the desired
// enableLB / lbName state are relevant.
type updateLoadBalancerConfigParams struct {
	UniverseUUID string           `json:"universeUUID"`
	Clusters     []client.Cluster `json:"clusters"`
}

func resourceUniverseLoadBalancerConfigCreate(
	ctx context.Context, d *schema.ResourceData, meta interface{},
) diag.Diagnostics {
	apiClient := meta.(*api.APIClient)
	cUUID := apiClient.CustomerID
	c := apiClient.YugawareClient
	uniUUID := d.Get("universe_uuid").(string)

	uni, response, err := c.UniverseManagementAPI.GetUniverse(ctx, cUUID, uniUUID).Execute()
	if err != nil {
		return diag.FromErr(utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"Universe Load Balancer Config", "Create - Fetch universe"))
	}

	clusters, err := buildDesiredClusters(
		uni.UniverseDetails.Clusters, d.Get("load_balancer").(*schema.Set).List())
	if err != nil {
		return diag.FromErr(err)
	}

	if diags := dispatchLoadBalancerConfig(ctx, apiClient, uniUUID, clusters,
		d.Timeout(schema.TimeoutCreate), "Create"); diags != nil {
		return diags
	}

	d.SetId(uniUUID)
	return nil
}

// buildDesiredClusters computes the config-authoritative desired LB state:
// every live LB assignment is cleared first, then the load_balancer blocks
// are overlaid. Anything the config no longer mentions is thereby detached.
func buildDesiredClusters(
	clusters []client.Cluster, blocks []interface{},
) ([]client.Cluster, error) {
	return applyLoadBalancerConfig(disableLoadBalancerConfig(clusters), blocks)
}

// dispatchLoadBalancerConfig PUTs the desired cluster LB state to
// update_lb_config and waits for the resulting universe task.
func dispatchLoadBalancerConfig(
	ctx context.Context,
	apiClient *api.APIClient,
	uniUUID string,
	clusters []client.Cluster,
	timeout time.Duration,
	operation string,
) diag.Diagnostics {
	payload := updateLoadBalancerConfigParams{UniverseUUID: uniUUID, Clusters: clusters}
	return utils.DispatchAndWait(ctx, "Universe Load Balancer Config "+operation,
		apiClient.CustomerID, apiClient.YugawareClient, timeout,
		utils.ResourceEntity, "Universe Load Balancer Config", operation,
		func() (string, *http.Response, error) {
			return apiClient.VanillaClient.UpdateLoadBalancerConfig(
				ctx, apiClient.CustomerID, uniUUID, payload, apiClient.APIKey)
		},
	)
}

// applyLoadBalancerConfig overlays the load_balancer blocks onto the
// universe's live clusters: it enables the LB flag on each targeted cluster
// and stamps the per-AZ lbName and per-region lbFQDN into its placement.
// The untouched remainder of the live cluster state rides along so the PUT
// carries the complete desired LB state for the whole universe.
func applyLoadBalancerConfig(
	clusters []client.Cluster, blocks []interface{},
) ([]client.Cluster, error) {
	for _, b := range blocks {
		block := b.(map[string]interface{})
		targetType := "PRIMARY"
		if block["read_replica"].(bool) {
			targetType = "ASYNC"
		}

		ci := clusterIndexByType(clusters, targetType)
		if ci < 0 {
			return nil, fmt.Errorf(
				"universe has no %s cluster to attach a load balancer to", targetType)
		}
		clusters[ci].UserIntent.EnableLB = utils.GetBoolPointer(true)

		if err := stampRegionLB(&clusters[ci], block); err != nil {
			return nil, err
		}
	}
	return clusters, nil
}

func clusterIndexByType(clusters []client.Cluster, clusterType string) int {
	for i := range clusters {
		if clusters[i].ClusterType == clusterType {
			return i
		}
	}
	return -1
}

// stampRegionLB writes one load_balancer block's lb_name/lb_fqdn into the
// matching region of the cluster's placement.
func stampRegionLB(cluster *client.Cluster, block map[string]interface{}) error {
	regionCode := block["region"].(string)
	lbName := block["lb_name"].(string)
	lbFQDN := block["lb_fqdn"].(string)
	azOverrides := utils.StringMap(block["az_overrides"].(map[string]interface{}))

	if cluster.PlacementInfo == nil {
		return fmt.Errorf("%s cluster has no placement info", cluster.ClusterType)
	}
	for pc := range cluster.PlacementInfo.CloudList {
		regions := cluster.PlacementInfo.CloudList[pc].RegionList
		for r := range regions {
			region := &regions[r]
			if region.GetCode() != regionCode {
				continue
			}
			if lbFQDN != "" {
				region.LbFQDN = utils.GetStringPointer(lbFQDN)
			}
			for a := range region.AzList {
				name := lbName
				if override, ok := (*azOverrides)[region.AzList[a].GetName()]; ok {
					name = override
				}
				region.AzList[a].LbName = utils.GetStringPointer(name)
			}
			return nil
		}
	}
	return fmt.Errorf("region %q not found in %s cluster placement",
		regionCode, cluster.ClusterType)
}

func resourceUniverseLoadBalancerConfigRead(
	ctx context.Context, d *schema.ResourceData, meta interface{},
) diag.Diagnostics {
	apiClient := meta.(*api.APIClient)
	cUUID := apiClient.CustomerID
	c := apiClient.YugawareClient

	uni, response, err := c.UniverseManagementAPI.GetUniverse(ctx, cUUID, d.Id()).Execute()
	if err != nil {
		if isUniverseMissing(response, err) {
			// Universe was deleted out of band; drop the attachment from state.
			d.SetId("")
			return nil
		}
		return diag.FromErr(utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"Universe Load Balancer Config", "Read"))
	}

	// The resource ID is the universe UUID; re-derive the field so imports work.
	if err := d.Set("universe_uuid", d.Id()); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("load_balancer",
		flattenLoadBalancerConfig(uni.UniverseDetails.Clusters)); err != nil {
		return diag.FromErr(err)
	}
	return nil
}

// universeMissingMarkers are the body fragments YBA uses when reporting a gone
// universe through non-404 responses (mirrors the telemetry resource's list).
var universeMissingMarkers = []string{"Cannot find universe", "does not exist"}

// isUniverseMissing collapses YBA's ways of reporting a deleted universe — a
// plain 404, or a non-404 whose body carries a missing-universe marker — so
// callers branch on one predicate instead of substring-matching in CRUD code.
func isUniverseMissing(response *http.Response, err error) bool {
	if utils.IsHTTPNotFound(response) {
		return true
	}
	body := utils.OpenAPIErrorBody(err)
	for _, m := range universeMissingMarkers {
		if strings.Contains(body, m) {
			return true
		}
	}
	return false
}

// flattenLoadBalancerConfig turns the universe's live LB placement back into
// load_balancer blocks, the inverse of applyLoadBalancerConfig. Written
// unconditionally on every Read so out-of-band changes surface as drift.
func flattenLoadBalancerConfig(clusters []client.Cluster) []interface{} {
	blocks := []interface{}{}
	for i := range clusters {
		cl := &clusters[i]
		if !cl.UserIntent.GetEnableLB() || cl.PlacementInfo == nil {
			continue
		}
		readReplica := cl.ClusterType == "ASYNC"
		for _, pc := range cl.PlacementInfo.CloudList {
			for _, region := range pc.RegionList {
				if block := flattenRegionLB(region, readReplica); block != nil {
					blocks = append(blocks, block)
				}
			}
		}
	}
	return blocks
}

// flattenRegionLB collapses a region's per-AZ lbNames into one block: the most
// common name becomes the region lb_name (ties broken by AZ placement order)
// and AZs pointing elsewhere land in az_overrides. Returns nil when no AZ in
// the region has a load balancer.
func flattenRegionLB(region client.PlacementRegion, readReplica bool) map[string]interface{} {
	counts := map[string]int{}
	var order []string
	for _, az := range region.AzList {
		name := az.GetLbName()
		if name == "" {
			continue
		}
		if counts[name] == 0 {
			order = append(order, name)
		}
		counts[name]++
	}
	if len(order) == 0 {
		return nil
	}

	regionLB := order[0]
	for _, name := range order {
		if counts[name] > counts[regionLB] {
			regionLB = name
		}
	}

	overrides := map[string]interface{}{}
	for _, az := range region.AzList {
		if name := az.GetLbName(); name != "" && name != regionLB {
			overrides[az.GetName()] = name
		}
	}

	return map[string]interface{}{
		"region":       region.GetCode(),
		"lb_name":      regionLB,
		"lb_fqdn":      region.GetLbFQDN(),
		"read_replica": readReplica,
		"az_overrides": overrides,
	}
}

func resourceUniverseLoadBalancerConfigUpdate(
	ctx context.Context, d *schema.ResourceData, meta interface{},
) diag.Diagnostics {
	apiClient := meta.(*api.APIClient)
	uniUUID := d.Id()

	uni, response, err := apiClient.YugawareClient.UniverseManagementAPI.
		GetUniverse(ctx, apiClient.CustomerID, uniUUID).Execute()
	if err != nil {
		return diag.FromErr(utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"Universe Load Balancer Config", "Update - Fetch universe"))
	}

	clusters, err := buildDesiredClusters(
		uni.UniverseDetails.Clusters, d.Get("load_balancer").(*schema.Set).List())
	if err != nil {
		return diag.FromErr(err)
	}

	return dispatchLoadBalancerConfig(ctx, apiClient, uniUUID, clusters,
		d.Timeout(schema.TimeoutUpdate), "Update")
}

// Delete disables load balancer management for the universe: an explicit
// enableLB=false with cleared lbName/lbFQDN on every cluster, so YBA removes
// the universe's nodes from all attached load balancers. The load balancers
// themselves are external resources and are never deleted here.
func resourceUniverseLoadBalancerConfigDelete(
	ctx context.Context, d *schema.ResourceData, meta interface{},
) diag.Diagnostics {
	apiClient := meta.(*api.APIClient)
	cUUID := apiClient.CustomerID
	c := apiClient.YugawareClient
	uniUUID := d.Id()

	uni, response, err := c.UniverseManagementAPI.GetUniverse(ctx, cUUID, uniUUID).Execute()
	if err != nil {
		if isUniverseMissing(response, err) {
			// Universe already gone: nothing to detach from.
			d.SetId("")
			return nil
		}
		return diag.FromErr(utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"Universe Load Balancer Config", "Delete - Fetch universe"))
	}

	if diags := dispatchLoadBalancerConfig(ctx, apiClient, uniUUID,
		disableLoadBalancerConfig(uni.UniverseDetails.Clusters),
		d.Timeout(schema.TimeoutDelete), "Delete"); diags != nil {
		return diags
	}

	d.SetId("")
	return nil
}

// disableLoadBalancerConfig strips all load balancer state from the clusters:
// enableLB explicitly false (a nil pointer would be omitted from the JSON) and
// every placement lbName/lbFQDN cleared.
func disableLoadBalancerConfig(clusters []client.Cluster) []client.Cluster {
	for i := range clusters {
		clusters[i].UserIntent.EnableLB = utils.GetBoolPointer(false)
		if clusters[i].PlacementInfo == nil {
			continue
		}
		for pc := range clusters[i].PlacementInfo.CloudList {
			regions := clusters[i].PlacementInfo.CloudList[pc].RegionList
			for r := range regions {
				regions[r].LbFQDN = nil
				for a := range regions[r].AzList {
					regions[r].AzList[a].LbName = nil
				}
			}
		}
	}
	return clusters
}
