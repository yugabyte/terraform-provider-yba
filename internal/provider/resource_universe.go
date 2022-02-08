package provider

import (
	"context"
	"fmt"
	"github.com/go-openapi/strfmt"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	utils "github.com/yugabyte/terraform-provider-yugabyte-platform/internal"
	"github.com/yugabyte/yb-tools/yugaware-client/pkg/client/swagger/client/universe_cluster_mutations"
	"github.com/yugabyte/yb-tools/yugaware-client/pkg/client/swagger/client/universe_management"
	"github.com/yugabyte/yb-tools/yugaware-client/pkg/client/swagger/models"
	"time"
)

func resourceUniverse() *schema.Resource {
	return &schema.Resource{
		Description: "Universe Resource",

		CreateContext: resourceUniverseCreate,
		ReadContext:   resourceUniverseRead,
		UpdateContext: resourceUniverseUpdate,
		DeleteContext: resourceUniverseDelete,

		Schema: map[string]*schema.Schema{
			// Universe Delete Options
			"delete_certs": {
				Type:     schema.TypeBool,
				Optional: true,
			},
			"delete_backups": {
				Type:     schema.TypeBool,
				Optional: true,
			},
			"force_delete": {
				Type:     schema.TypeBool,
				Optional: true,
			},

			// Universe Fields
			"clusters": {
				Type:     schema.TypeList,
				Required: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"cluster_type": {
							Type:     schema.TypeString,
							Required: true,
						},
						"user_intent": {
							Type:     schema.TypeList,
							MaxItems: 1,
							Required: true,
							Elem:     userIntentSchema(),
						},
					},
				},
			},
		},
	}
}

func userIntentSchema() *schema.Resource {
	return &schema.Resource{
		Schema: map[string]*schema.Schema{
			"universe_name": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"provider_type": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"provider": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"region_list": {
				Type: schema.TypeList,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
				Optional: true,
			},
			"num_nodes": {
				Type:     schema.TypeInt,
				Optional: true,
			},
			"replication_factor": {
				Type:     schema.TypeInt,
				Optional: true,
			},
			"instance_type": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"device_info": {
				Type:     schema.TypeList,
				MaxItems: 1,
				Required: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"num_volumes": {
							Type:     schema.TypeInt,
							Optional: true,
						},
						"volume_size": {
							Type:     schema.TypeInt,
							Optional: true,
						},
						"storage_type": {
							Type:     schema.TypeString,
							Optional: true,
						},
					},
				},
			},
			"assign_public_ip": {
				Type:     schema.TypeBool,
				Optional: true,
			},
			"use_time_sync": {
				Type:     schema.TypeBool,
				Optional: true,
			},
			"enable_ysql": {
				Type:     schema.TypeBool,
				Optional: true,
			},
			"enable_yedis": {
				Type:     schema.TypeBool,
				Optional: true,
			},
			"enable_node_to_node_encrypt": {
				Type:     schema.TypeBool,
				Optional: true,
			},
			"enable_client_to_node_encrypt": {
				Type:     schema.TypeBool,
				Optional: true,
			},
			"enable_volume_encryption": {
				Type:     schema.TypeBool,
				Optional: true,
			},
			"yb_software_version": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"access_key_code": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"tserver_gflags": {
				Type:     schema.TypeMap,
				Elem:     schema.TypeString,
				Optional: true,
			},
			"master_gflags": {
				Type:     schema.TypeMap,
				Elem:     schema.TypeString,
				Optional: true,
			},
		},
	}
}

func resourceUniverseCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	c := meta.(*ApiClient).YugawareClient
	req := buildUniverse(d)
	u, err := c.PlatformAPIs.UniverseClusterMutations.CreateAllClusters(&universe_cluster_mutations.CreateAllClustersParams{
		UniverseConfigureTaskParams: req,
		CUUID:                       c.CustomerUUID(),
		Context:                     ctx,
		HTTPClient:                  c.Session(),
	},
		c.SwaggerAuth,
	)
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(string(u.Payload.ResourceUUID))
	// TODO: should this block?
	tflog.Debug(ctx, fmt.Sprintf("Waiting for universe %s to be active", d.Id()))
	err = utils.WaitForTask(ctx, u.Payload.TaskUUID, c, time.Hour)
	if err != nil {
		return diag.FromErr(err)
	}
	return resourceUniverseRead(ctx, d, meta)
}

func buildUniverse(d *schema.ResourceData) *models.UniverseConfigureTaskParams {
	u := models.UniverseConfigureTaskParams{
		Clusters: buildClusters(d.Get("clusters").([]interface{})),
	}
	return &u
}

func buildClusters(clusters []interface{}) (res []*models.Cluster) {
	for _, v := range clusters {
		cluster := v.(map[string]interface{})
		c := &models.Cluster{
			ClusterType: utils.GetStringPointer(cluster["cluster_type"].(string)),
			UserIntent:  buildUserIntent(utils.MapFromSingletonList(cluster["user_intent"].([]interface{}))),
		}
		res = append(res, c)
	}
	return res
}

func buildUserIntent(ui map[string]interface{}) *models.UserIntent {
	return &models.UserIntent{
		UniverseName:              ui["universe_name"].(string),
		ProviderType:              ui["provider_type"].(string),
		Provider:                  ui["provider"].(string),
		RegionList:                utils.UUIDSlice(ui["region_list"].([]interface{})),
		NumNodes:                  int32(ui["num_nodes"].(int)),
		ReplicationFactor:         int32(ui["replication_factor"].(int)),
		InstanceType:              ui["instance_type"].(string),
		DeviceInfo:                buildDeviceInfo(utils.MapFromSingletonList(ui["device_info"].([]interface{}))),
		AssignPublicIP:            ui["assign_public_ip"].(bool),
		UseTimeSync:               ui["use_time_sync"].(bool),
		EnableYSQL:                ui["enable_ysql"].(bool),
		EnableYEDIS:               ui["enable_yedis"].(bool),
		EnableNodeToNodeEncrypt:   ui["enable_node_to_node_encrypt"].(bool),
		EnableClientToNodeEncrypt: ui["enable_client_to_node_encrypt"].(bool),
		EnableVolumeEncryption:    ui["enable_volume_encryption"].(bool),
		YbSoftwareVersion:         ui["yb_software_version"].(string),
		AccessKeyCode:             ui["access_key_code"].(string),
		TserverGFlags:             utils.StringMap(ui["tserver_gflags"].(map[string]interface{})),
		MasterGFlags:              utils.StringMap(ui["master_gflags"].(map[string]interface{})),
	}
}

func buildDeviceInfo(di map[string]interface{}) *models.DeviceInfo {
	return &models.DeviceInfo{
		NumVolumes:  int32(di["num_volumes"].(int)),
		VolumeSize:  int32(di["volume_size"].(int)),
		StorageType: di["storage_type"].(string),
	}
}

func resourceUniverseRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	c := meta.(*ApiClient).YugawareClient
	r, err := c.PlatformAPIs.UniverseManagement.GetUniverse(&universe_management.GetUniverseParams{
		CUUID:      c.CustomerUUID(),
		UniUUID:    strfmt.UUID(d.Id()),
		Context:    ctx,
		HTTPClient: c.Session(),
	},
		c.SwaggerAuth,
	)
	if err != nil {
		return diag.FromErr(err)
	}

	u := r.Payload.UniverseDetails
	if err = d.Set("clusters", flattenClusters(u.Clusters)); err != nil {
		return diag.FromErr(err)
	}
	return diags
}

func flattenClusters(clusters []*models.Cluster) (res []map[string]interface{}) {
	for _, cluster := range clusters {
		c := map[string]interface{}{
			"cluster_type": cluster.ClusterType,
			"user_intent":  flattenUserIntent(cluster.UserIntent),
		}
		res = append(res, c)
	}
	return res
}

func flattenUserIntent(ui *models.UserIntent) []interface{} {
	v := map[string]interface{}{
		"universe_name":                 ui.UniverseName,
		"provider_type":                 ui.ProviderType,
		"provider":                      ui.Provider,
		"region_list":                   ui.RegionList,
		"num_nodes":                     ui.NumNodes,
		"replication_factor":            ui.ReplicationFactor,
		"instance_type":                 ui.InstanceType,
		"device_info":                   flattenDeviceInfo(ui.DeviceInfo),
		"assign_public_ip":              ui.AssignPublicIP,
		"use_time_sync":                 ui.UseTimeSync,
		"enable_ysql":                   ui.EnableYSQL,
		"enable_yedis":                  ui.EnableYEDIS,
		"enable_node_to_node_encrypt":   ui.EnableNodeToNodeEncrypt,
		"enable_client_to_node_encrypt": ui.EnableClientToNodeEncrypt,
		"enable_volume_encryption":      ui.EnableVolumeEncryption,
		"yb_software_version":           ui.YbSoftwareVersion,
		"access_key_code":               ui.AccessKeyCode,
		"tserver_gflags":                ui.TserverGFlags,
		"master_gflags":                 ui.MasterGFlags,
	}
	return utils.CreateSingletonList(v)
}

func flattenDeviceInfo(di *models.DeviceInfo) []interface{} {
	v := map[string]interface{}{
		"num_volumes":  di.NumVolumes,
		"volume_size":  di.VolumeSize,
		"storage_type": di.StorageType,
	}
	return utils.CreateSingletonList(v)
}

func resourceUniverseUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	c := meta.(*ApiClient).YugawareClient
	if d.HasChanges("clusters") {
		var taskIds []strfmt.UUID
		clusters := d.Get("clusters").([]interface{})
		for _, v := range clusters {
			cluster := v.(map[string]interface{})
			if cluster["cluster_type"] == "PRIMARY" {
				r, err := c.PlatformAPIs.UniverseClusterMutations.UpdatePrimaryCluster(
					&universe_cluster_mutations.UpdatePrimaryClusterParams{
						UniverseConfigureTaskParams: buildUniverse(d),
						CUUID:                       c.CustomerUUID(),
						UniUUID:                     strfmt.UUID(d.Id()),
						Context:                     ctx,
						HTTPClient:                  c.Session(),
					},
					c.SwaggerAuth,
				)
				if err != nil {
					return diag.FromErr(err)
				}
				taskIds = append(taskIds, r.Payload.TaskUUID)
			} else {
				r, err := c.PlatformAPIs.UniverseClusterMutations.UpdateReadOnlyCluster(
					&universe_cluster_mutations.UpdateReadOnlyClusterParams{
						UniverseConfigureTaskParams: buildUniverse(d),
						CUUID:                       c.CustomerUUID(),
						UniUUID:                     strfmt.UUID(d.Id()),
						Context:                     ctx,
						HTTPClient:                  c.Session(),
					},
					c.SwaggerAuth,
				)
				if err != nil {
					return diag.FromErr(err)
				}
				taskIds = append(taskIds, r.Payload.TaskUUID)
			}
		}
		tflog.Debug(ctx, fmt.Sprintf("Waiting for universe %s to be updated", d.Id()))
		for _, id := range taskIds {
			err := utils.WaitForTask(ctx, id, c, time.Hour)
			if err != nil {
				return diag.FromErr(err)
			}
		}
	}
	return resourceUniverseRead(ctx, d, meta)
}

func resourceUniverseDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	c := meta.(*ApiClient).YugawareClient
	r, err := c.PlatformAPIs.UniverseManagement.DeleteUniverse(&universe_management.DeleteUniverseParams{
		CUUID:                   c.CustomerUUID(),
		IsForceDelete:           utils.GetBoolPointer(d.Get("force_delete").(bool)),
		IsDeleteBackups:         utils.GetBoolPointer(d.Get("delete_backups").(bool)),
		IsDeleteAssociatedCerts: utils.GetBoolPointer(d.Get("delete_certs").(bool)),
		UniUUID:                 strfmt.UUID(d.Id()),
		Context:                 ctx,
		HTTPClient:              c.Session(),
	},
		c.SwaggerAuth,
	)
	if err != nil {
		return diag.FromErr(err)
	}

	tflog.Debug(ctx, fmt.Sprintf("Waiting for universe %s to be deleted", d.Id()))
	err = utils.WaitForTask(ctx, r.Payload.TaskUUID, c, time.Hour)
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId("")
	return diags
}
