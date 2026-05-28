package universe

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	client "github.com/yugabyte/platform-go-client"
	"github.com/yugabyte/terraform-provider-yba/internal/api"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// ResourceUniverseLoadBalancer manages load balancer associations on a
// YBA universe via the update_lb_config API endpoint.
func ResourceUniverseLoadBalancer() *schema.Resource {
	return &schema.Resource{
		Description: "Manages load balancer associations on a YugabyteDB Anywhere " +
			"universe. Maps GCP region codes to forwarding rule names so that " +
			"YBA can attach universe nodes to the corresponding backend services.\n\n" +
			"This resource calls the `update_lb_config` API, which is separate " +
			"from the universe create/update lifecycle. The universe must exist " +
			"before this resource is created.",

		CreateContext: resourceUniverseLBCreate,
		ReadContext:   resourceUniverseLBRead,
		UpdateContext: resourceUniverseLBUpdate,
		DeleteContext: resourceUniverseLBDelete,

		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(10 * time.Minute),
			Update: schema.DefaultTimeout(10 * time.Minute),
			Delete: schema.DefaultTimeout(10 * time.Minute),
		},

		Schema: map[string]*schema.Schema{
			"universe_uuid": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "UUID of the universe to attach load balancers to.",
			},
			"load_balancers": {
				Type:        schema.TypeMap,
				Required:    true,
				Description: "Map of region code to load balancer forwarding rule name.",
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},
		},
	}
}

func resourceUniverseLBCreate(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{},
) diag.Diagnostics {
	c := meta.(*api.APIClient)
	uUUID := d.Get("universe_uuid").(string)
	lbMap := expandLBMap(d.Get("load_balancers").(map[string]interface{}))

	d.SetId(uUUID)

	if diags := utils.DispatchAndWait(ctx, "UpdateLBConfig", c.CustomerID, c.YugawareClient,
		d.Timeout(schema.TimeoutCreate),
		utils.ResourceEntity, "Universe Load Balancer", "Create",
		func() (string, *http.Response, error) {
			return applyLBConfig(ctx, c, uUUID, lbMap, true)
		},
	); diags != nil {
		return diags
	}

	return resourceUniverseLBRead(ctx, d, meta)
}

func resourceUniverseLBRead(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{},
) diag.Diagnostics {
	c := meta.(*api.APIClient)
	uUUID := d.Id()

	uni, response, err := c.YugawareClient.UniverseManagementAPI.
		GetUniverse(ctx, c.CustomerID, uUUID).Execute()
	if err != nil {
		if utils.IsHTTPNotFound(response) || utils.IsHTTPBadRequestNotFound(response) {
			tflog.Warn(ctx, fmt.Sprintf(
				"Universe %s not found, removing LB config from state", uUUID))
			d.SetId("")
			return nil
		}
		return diag.FromErr(utils.ErrorFromHTTPResponse(response, err,
			utils.ResourceEntity, "Universe Load Balancer", "Read"))
	}

	lbMap := extractLBMapFromUniverse(uni)

	if err := d.Set("universe_uuid", uUUID); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("load_balancers", lbMap); err != nil {
		return diag.FromErr(err)
	}

	return nil
}

func resourceUniverseLBUpdate(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{},
) diag.Diagnostics {
	c := meta.(*api.APIClient)
	uUUID := d.Id()
	lbMap := expandLBMap(d.Get("load_balancers").(map[string]interface{}))

	if diags := utils.DispatchAndWait(ctx, "UpdateLBConfig", c.CustomerID, c.YugawareClient,
		d.Timeout(schema.TimeoutUpdate),
		utils.ResourceEntity, "Universe Load Balancer", "Update",
		func() (string, *http.Response, error) {
			return applyLBConfig(ctx, c, uUUID, lbMap, true)
		},
	); diags != nil {
		return diags
	}

	return resourceUniverseLBRead(ctx, d, meta)
}

func resourceUniverseLBDelete(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{},
) diag.Diagnostics {
	c := meta.(*api.APIClient)
	uUUID := d.Id()

	return utils.DispatchAndWait(ctx, "UpdateLBConfig", c.CustomerID, c.YugawareClient,
		d.Timeout(schema.TimeoutDelete),
		utils.ResourceEntity, "Universe Load Balancer", "Delete",
		func() (string, *http.Response, error) {
			return applyLBConfig(ctx, c, uUUID, nil, false)
		},
	)
}

// applyLBConfig fetches the universe, modifies LB settings on the
// primary cluster, and calls the update_lb_config API. Returns the raw
// *http.Response so callers can detect 409 universe-task conflicts.
func applyLBConfig(
	ctx context.Context,
	c *api.APIClient,
	uUUID string,
	lbMap map[string]string,
	enableLB bool,
) (string, *http.Response, error) {
	uni, response, err := c.YugawareClient.UniverseManagementAPI.
		GetUniverse(ctx, c.CustomerID, uUUID).Execute()
	if err != nil {
		return "", response, utils.ErrorFromHTTPResponse(response, err,
			utils.ResourceEntity, "Universe Load Balancer", "Get Universe")
	}

	details := uni.UniverseDetails
	cluster := findPrimaryCluster(details.Clusters)
	if cluster == nil {
		return "", nil, fmt.Errorf("universe %s: no PRIMARY cluster found", uUUID)
	}

	cluster.UserIntent.EnableLB = &enableLB
	setLBNamesOnPlacement(cluster, lbMap, enableLB)

	taskUUID, resp, err := c.VanillaClient.UpdateLBConfig(
		c.CustomerID, uUUID, details, c.APIKey)
	if err != nil {
		return "", resp, fmt.Errorf("update_lb_config on universe %s: %w", uUUID, err)
	}

	return taskUUID, resp, nil
}

func findPrimaryCluster(clusters []client.Cluster) *client.Cluster {
	for i := range clusters {
		if clusters[i].GetClusterType() == "PRIMARY" {
			return &clusters[i]
		}
	}
	return nil
}

// setLBNamesOnPlacement walks placementInfo.cloudList to set or clear
// lbName on each AZ whose region matches the provided map.
func setLBNamesOnPlacement(
	cluster *client.Cluster,
	lbMap map[string]string,
	enableLB bool,
) {
	pi := cluster.PlacementInfo
	if pi == nil {
		return
	}
	for ci := range pi.CloudList {
		cloud := &pi.CloudList[ci]
		for ri := range cloud.RegionList {
			region := &cloud.RegionList[ri]
			regionCode := region.GetCode()
			for ai := range region.AzList {
				az := &region.AzList[ai]
				if !enableLB {
					az.LbName = nil
					continue
				}
				if lbName, found := lbMap[regionCode]; found {
					az.LbName = &lbName
				}
			}
		}
	}
}

// extractLBMapFromUniverse reads the current lbName from each AZ in the
// primary cluster and returns a deduplicated region -> lbName map.
func extractLBMapFromUniverse(uni *client.UniverseResp) map[string]string {
	result := make(map[string]string)
	details := uni.UniverseDetails
	if details == nil {
		return result
	}

	cluster := findPrimaryCluster(details.Clusters)
	if cluster == nil {
		return result
	}

	if !cluster.UserIntent.GetEnableLB() {
		return result
	}

	pi := cluster.PlacementInfo
	if pi == nil {
		return result
	}
	for _, cloud := range pi.CloudList {
		for _, region := range cloud.RegionList {
			for _, az := range region.AzList {
				if lbName := az.GetLbName(); lbName != "" {
					result[region.GetCode()] = lbName
				}
			}
		}
	}

	return result
}

func expandLBMap(raw map[string]interface{}) map[string]string {
	result := make(map[string]string, len(raw))
	for k, v := range raw {
		result[k] = v.(string)
	}
	return result
}
