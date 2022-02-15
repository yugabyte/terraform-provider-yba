package universe

import (
	"context"
	"fmt"
	"github.com/go-openapi/strfmt"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/api"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/utils"
	"github.com/yugabyte/yb-tools/yugaware-client/pkg/client/swagger/client/universe_cluster_mutations"
	"github.com/yugabyte/yb-tools/yugaware-client/pkg/client/swagger/client/universe_management"
	"github.com/yugabyte/yb-tools/yugaware-client/pkg/client/swagger/models"
	"time"
)

func ResourceUniverse() *schema.Resource {
	return &schema.Resource{
		Description: "Universe Resource",

		CreateContext: resourceUniverseCreate,
		ReadContext:   resourceUniverseRead,
		UpdateContext: resourceUniverseUpdate,
		DeleteContext: resourceUniverseDelete,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

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
			"allow_insecure": {
				Type:     schema.TypeBool,
				Optional: true,
			},
			"capability": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"client_root_ca": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"cmk_arn": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"clusters": {
				Type:     schema.TypeList,
				Required: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"uuid": {
							Type:     schema.TypeString,
							Computed: true,
						},
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
						"cloud_list": {
							Type:     schema.TypeList,
							Optional: true,
							Elem:     cloudListSchema(),
						},
					},
				},
			},
			"communication_ports": {
				Type:     schema.TypeList,
				Optional: true,
				MaxItems: 1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"master_http_port": {
							Type:     schema.TypeInt,
							Optional: true,
						},
						"master_rpc_port": {
							Type:     schema.TypeInt,
							Optional: true,
						},
						"node_exporter_port": {
							Type:     schema.TypeInt,
							Optional: true,
						},
						"redis_server_http_port": {
							Type:     schema.TypeInt,
							Optional: true,
						},
						"redis_server_rpc_port": {
							Type:     schema.TypeInt,
							Optional: true,
						},
						"tserver_http_port": {
							Type:     schema.TypeInt,
							Optional: true,
						},
						"tserver_rpc_port": {
							Type:     schema.TypeInt,
							Optional: true,
						},
						"yql_server_http_port": {
							Type:     schema.TypeInt,
							Optional: true,
						},
						"yql_server_rpc_port": {
							Type:     schema.TypeInt,
							Optional: true,
						},
						"ysql_server_http_port": {
							Type:     schema.TypeInt,
							Optional: true,
						},
						"ysql_server_rpc_port": {
							Type:     schema.TypeInt,
							Optional: true,
						},
					},
				},
			},
		},
	}
}

func cloudListSchema() *schema.Resource {
	return &schema.Resource{
		Schema: map[string]*schema.Schema{
			"uuid": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"code": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"region_list": {
				Type:     schema.TypeList,
				Optional: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"uuid": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"code": {
							Type:     schema.TypeString,
							Optional: true,
						},
						"az_list": {
							Type:     schema.TypeList,
							Optional: true,
							Elem: &schema.Resource{
								Schema: map[string]*schema.Schema{
									"uuid": {
										Type:     schema.TypeString,
										Optional: true,
									},
									"is_affinitized": {
										Type:     schema.TypeBool,
										Computed: true,
									},
									"name": {
										Type:     schema.TypeString,
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
									"secondary_subnet": {
										Type:     schema.TypeString,
										Optional: true,
									},
									"subnet": {
										Type:     schema.TypeString,
										Optional: true,
									},
								},
							},
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
			"assign_static_ip": {
				Type:     schema.TypeBool,
				Optional: true,
			},
			"aws_arn_string": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"enable_exposing_service": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"enable_ipv6": {
				Type:     schema.TypeBool,
				Optional: true,
			},
			"enable_ycql": {
				Type:     schema.TypeBool,
				Optional: true,
			},
			"enable_ycql_auth": {
				Type:     schema.TypeBool,
				Optional: true,
			},
			"enable_ysql_auth": {
				Type:     schema.TypeBool,
				Optional: true,
			},
			"instance_tags": {
				Type:     schema.TypeMap,
				Elem:     &schema.Schema{Type: schema.TypeString},
				Optional: true,
			},
			"preferred_region": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"use_host_name": {
				Type:     schema.TypeBool,
				Optional: true,
			},
			"use_systemd": {
				Type:     schema.TypeBool,
				Optional: true,
			},
			"ysql_password": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"ycql_password": {
				Type:     schema.TypeString,
				Optional: true,
			},
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
						"disk_iops": {
							Type:     schema.TypeInt,
							Optional: true,
						},
						"mount_points": {
							Type:     schema.TypeString,
							Optional: true,
						},
						"storage_class": {
							Type:     schema.TypeString,
							Optional: true,
						},
						"throughput": {
							Type:     schema.TypeInt,
							Optional: true,
						},
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
				Default:  true,
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
	c := meta.(*api.ApiClient).YugawareClient
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
		AllowInsecure:      d.Get("allow_insecure").(bool),
		Capability:         d.Get("capability").(string),
		ClientRootCA:       strfmt.UUID(d.Get("client_root_ca").(string)),
		CmkArn:             d.Get("cmk_arn").(string),
		Clusters:           buildClusters(d.Get("clusters").([]interface{})),
		CommunicationPorts: buildCommunicationPorts(utils.MapFromSingletonList(d.Get("communication_ports").([]interface{}))),
	}
	return &u
}

func buildCommunicationPorts(cp map[string]interface{}) *models.CommunicationPorts {
	return &models.CommunicationPorts{
		MasterHTTPPort:      int32(cp["master_http_port"].(int)),
		MasterRPCPort:       int32(cp["master_rpc_port"].(int)),
		NodeExporterPort:    int32(cp["node_exporter_port"].(int)),
		RedisServerHTTPPort: int32(cp["redis_server_http_port"].(int)),
		RedisServerRPCPort:  int32(cp["redis_server_rpc_port"].(int)),
		TserverHTTPPort:     int32(cp["tserver_http_port"].(int)),
		TserverRPCPort:      int32(cp["tserver_rpc_port"].(int)),
		YqlServerHTTPPort:   int32(cp["yql_server_http_port"].(int)),
		YqlServerRPCPort:    int32(cp["yql_server_rpc_port"].(int)),
		YsqlServerHTTPPort:  int32(cp["ysql_server_http_port"].(int)),
		YsqlServerRPCPort:   int32(cp["ysql_server_rpc_port"].(int)),
	}
}

func buildClusters(clusters []interface{}) (res []*models.Cluster) {
	for _, v := range clusters {
		cluster := v.(map[string]interface{})
		c := &models.Cluster{
			ClusterType: utils.GetStringPointer(cluster["cluster_type"].(string)),
			UserIntent:  buildUserIntent(utils.MapFromSingletonList(cluster["user_intent"].([]interface{}))),
			PlacementInfo: &models.PlacementInfo{
				CloudList: buildCloudList(cluster["cloud_list"].([]interface{})),
			},
		}
		res = append(res, c)
	}
	return res
}

func buildCloudList(cl []interface{}) (res []*models.PlacementCloud) {
	for _, v := range cl {
		c := v.(map[string]interface{})
		pc := &models.PlacementCloud{
			Code:       c["code"].(string),
			RegionList: buildRegionList(c["region_list"].([]interface{})),
		}
		res = append(res, pc)
	}
	return res
}

func buildRegionList(cl []interface{}) (res []*models.PlacementRegion) {
	for _, v := range cl {
		r := v.(map[string]interface{})
		pr := &models.PlacementRegion{
			Code:   r["code"].(string),
			AzList: buildAzList(r["az_list"].([]interface{})),
		}
		res = append(res, pr)
	}
	return res
}

func buildAzList(cl []interface{}) (res []*models.PlacementAZ) {
	for _, v := range cl {
		az := v.(map[string]interface{})
		paz := &models.PlacementAZ{
			IsAffinitized:     az["is_affinitized"].(bool),
			Name:              az["name"].(string),
			NumNodesInAZ:      int32(az["num_nodes"].(int)),
			ReplicationFactor: int32(az["replication_factor"].(int)),
			SecondarySubnet:   az["secondary_subnet"].(string),
			Subnet:            az["subnet"].(string),
		}
		res = append(res, paz)
	}
	return res
}

func buildUserIntent(ui map[string]interface{}) *models.UserIntent {
	return &models.UserIntent{
		AssignStaticPublicIP:      ui["assign_static_ip"].(bool),
		AwsArnString:              ui["aws_arn_string"].(string),
		EnableExposingService:     ui["enable_exposing_service"].(string),
		EnableIPV6:                ui["enable_ipv6"].(bool),
		EnableYCQL:                ui["enable_ycql"].(bool),
		EnableYCQLAuth:            ui["enable_ycql_auth"].(bool),
		EnableYSQLAuth:            ui["enable_ysql_auth"].(bool),
		InstanceTags:              utils.StringMap(ui["instance_tags"].(map[string]interface{})),
		PreferredRegion:           strfmt.UUID(ui["preferred_region"].(string)),
		UseHostname:               ui["use_host_name"].(bool),
		UseSystemd:                ui["use_systemd"].(bool),
		YsqlPassword:              ui["ysql_password"].(string),
		YcqlPassword:              ui["ycql_password"].(string),
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
		DiskIops:     int32(di["disk_iops"].(int)),
		MountPoints:  di["mount_points"].(string),
		StorageClass: di["storage_class"].(string),
		Throughput:   int32(di["throughput"].(int)),
		NumVolumes:   int32(di["num_volumes"].(int)),
		VolumeSize:   int32(di["volume_size"].(int)),
		StorageType:  di["storage_type"].(string),
	}
}

func resourceUniverseRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	c := meta.(*api.ApiClient).YugawareClient
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
	if err = d.Set("allow_insecure", u.AllowInsecure); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("capability", u.Capability); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("client_root_ca", u.ClientRootCA); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("cmk_arn", u.CmkArn); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("clusters", flattenClusters(u.Clusters)); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("communication_ports", flattenCommunicationPorts(u.CommunicationPorts)); err != nil {
		return diag.FromErr(err)
	}
	return diags
}

func flattenCommunicationPorts(cp *models.CommunicationPorts) []interface{} {
	v := map[string]interface{}{
		"master_http_port":       cp.MasterHTTPPort,
		"master_rpc_port":        cp.MasterRPCPort,
		"node_exporter_port":     cp.NodeExporterPort,
		"redis_server_http_port": cp.RedisServerHTTPPort,
		"redis_server_rpc_port":  cp.RedisServerRPCPort,
		"tserver_http_port":      cp.TserverHTTPPort,
		"tserver_rpc_port":       cp.TserverRPCPort,
		"yql_server_http_port":   cp.YqlServerHTTPPort,
		"yql_server_rpc_port":    cp.YqlServerRPCPort,
		"ysql_server_http_port":  cp.YsqlServerHTTPPort,
		"ysql_server_rpc_port":   cp.YsqlServerRPCPort,
	}
	return utils.CreateSingletonList(v)
}

func flattenClusters(clusters []*models.Cluster) (res []map[string]interface{}) {
	for _, cluster := range clusters {
		c := map[string]interface{}{
			"uuid":         cluster.UUID,
			"cluster_type": cluster.ClusterType,
			"user_intent":  flattenUserIntent(cluster.UserIntent),
			"cloud_list":   flattenCloudList(cluster.PlacementInfo.CloudList),
		}
		res = append(res, c)
	}
	return res
}

func flattenCloudList(cl []*models.PlacementCloud) (res []interface{}) {
	for _, c := range cl {
		pc := map[string]interface{}{
			"uuid":        c.UUID,
			"code":        c.Code,
			"region_list": flattenRegionList(c.RegionList),
		}
		res = append(res, pc)
	}
	return res
}

func flattenRegionList(cl []*models.PlacementRegion) (res []interface{}) {
	for _, r := range cl {
		pr := map[string]interface{}{
			"uuid":    r.UUID,
			"code":    r.Code,
			"az_list": flattenAzList(r.AzList),
		}
		res = append(res, pr)
	}
	return res
}

func flattenAzList(cl []*models.PlacementAZ) (res []interface{}) {
	for _, az := range cl {
		paz := map[string]interface{}{
			"uuid":               az.UUID,
			"is_affinitized":     az.IsAffinitized,
			"name":               az.Name,
			"num_nodes":          az.NumNodesInAZ,
			"replication_factor": az.ReplicationFactor,
			"secondary_subnet":   az.SecondarySubnet,
			"subnet":             az.Subnet,
		}
		res = append(res, paz)
	}
	return res
}

func flattenUserIntent(ui *models.UserIntent) []interface{} {
	v := map[string]interface{}{
		"assign_static_ip":              ui.AssignStaticPublicIP,
		"aws_arn_string":                ui.AwsArnString,
		"enable_exposing_service":       ui.EnableExposingService,
		"enable_ipv6":                   ui.EnableIPV6,
		"enable_ycql":                   ui.EnableYCQL,
		"enable_ycql_auth":              ui.EnableYCQLAuth,
		"enable_ysql_auth":              ui.EnableYSQLAuth,
		"instance_tags":                 ui.InstanceTags,
		"preferred_region":              ui.PreferredRegion,
		"use_host_name":                 ui.UseHostname,
		"use_systemd":                   ui.UseSystemd,
		"ysql_password":                 ui.YsqlPassword,
		"ycql_password":                 ui.YcqlPassword,
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
		"disk_iops":     di.DiskIops,
		"mount_points":  di.MountPoints,
		"storage_class": di.StorageClass,
		"throughput":    di.Throughput,
		"num_volumes":   di.NumVolumes,
		"volume_size":   di.VolumeSize,
		"storage_type":  di.StorageType,
	}
	return utils.CreateSingletonList(v)
}

func resourceUniverseUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	c := meta.(*api.ApiClient).YugawareClient
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

	c := meta.(*api.ApiClient).YugawareClient
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
