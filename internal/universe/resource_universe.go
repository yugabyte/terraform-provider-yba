package universe

import (
	"context"
	"errors"
	"fmt"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	client "github.com/yugabyte/platform-go-client"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/api"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/utils"
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
			"customer_id": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},

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
				Computed: true,
			},
			"capability": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
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
							Computed: true,
							Elem:     cloudListSchema(),
						},
					},
				},
			},
			"communication_ports": {
				Type:     schema.TypeList,
				Optional: true,
				Computed: true,
				MaxItems: 1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"master_http_port": {
							Type:     schema.TypeInt,
							Optional: true,
							Computed: true,
						},
						"master_rpc_port": {
							Type:     schema.TypeInt,
							Optional: true,
							Computed: true,
						},
						"node_exporter_port": {
							Type:     schema.TypeInt,
							Optional: true,
							Computed: true,
						},
						"redis_server_http_port": {
							Type:     schema.TypeInt,
							Optional: true,
							Computed: true,
						},
						"redis_server_rpc_port": {
							Type:     schema.TypeInt,
							Optional: true,
							Computed: true,
						},
						"tserver_http_port": {
							Type:     schema.TypeInt,
							Optional: true,
							Computed: true,
						},
						"tserver_rpc_port": {
							Type:     schema.TypeInt,
							Optional: true,
							Computed: true,
						},
						"yql_server_http_port": {
							Type:     schema.TypeInt,
							Optional: true,
							Computed: true,
						},
						"yql_server_rpc_port": {
							Type:     schema.TypeInt,
							Optional: true,
							Computed: true,
						},
						"ysql_server_http_port": {
							Type:     schema.TypeInt,
							Optional: true,
							Computed: true,
						},
						"ysql_server_rpc_port": {
							Type:     schema.TypeInt,
							Optional: true,
							Computed: true,
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
				Computed: true,
			},
			"region_list": {
				Type:     schema.TypeList,
				Optional: true,
				Computed: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"uuid": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"code": {
							Type:     schema.TypeString,
							Optional: true,
							Computed: true,
						},
						"az_list": {
							Type:     schema.TypeList,
							Optional: true,
							Computed: true,
							Elem: &schema.Resource{
								Schema: map[string]*schema.Schema{
									"uuid": {
										Type:     schema.TypeString,
										Optional: true,
										Computed: true,
									},
									"is_affinitized": {
										Type:     schema.TypeBool,
										Computed: true,
									},
									"name": {
										Type:     schema.TypeString,
										Optional: true,
										Computed: true,
									},
									"num_nodes": {
										Type:     schema.TypeInt,
										Optional: true,
										Computed: true,
									},
									"replication_factor": {
										Type:     schema.TypeInt,
										Optional: true,
										Computed: true,
									},
									"secondary_subnet": {
										Type:     schema.TypeString,
										Optional: true,
										Computed: true,
									},
									"subnet": {
										Type:     schema.TypeString,
										Optional: true,
										Computed: true,
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
				Computed: true,
			},
			"enable_ipv6": {
				Type:     schema.TypeBool,
				Optional: true,
			},
			"enable_ycql": {
				Type:     schema.TypeBool,
				Optional: true,
				Computed: true,
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
				Computed: true,
			},
			"enable_yedis": {
				Type:     schema.TypeBool,
				Optional: true,
				Computed: true,
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

	cUUID := d.Get("customer").(string)
	ctx = meta.(*api.ApiClient).SetContextApiKey(ctx, d.Get("customer_id").(string))
	req := buildUniverse(d)
	r, _, err := c.UniverseClusterMutationsApi.CreateAllClusters(ctx, cUUID).UniverseConfigureTaskParams(req).Execute()
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(*r.ResourceUUID)
	tflog.Debug(ctx, fmt.Sprintf("Waiting for universe %s to be active", d.Id()))
	err = utils.WaitForTask(ctx, *r.TaskUUID, cUUID, c, time.Hour)
	if err != nil {
		return diag.FromErr(err)
	}
	return resourceUniverseRead(ctx, d, meta)
}

func buildUniverse(d *schema.ResourceData) client.UniverseConfigureTaskParams {
	return client.UniverseConfigureTaskParams{
		AllowInsecure:      utils.GetBoolPointer(d.Get("allow_insecure").(bool)),
		Capability:         utils.GetStringPointer(d.Get("capability").(string)),
		ClientRootCA:       utils.GetStringPointer(d.Get("client_root_ca").(string)),
		CmkArn:             utils.GetStringPointer(d.Get("cmk_arn").(string)),
		Clusters:           buildClusters(d.Get("clusters").([]interface{})),
		CommunicationPorts: buildCommunicationPorts(utils.MapFromSingletonList(d.Get("communication_ports").([]interface{}))),
	}
}

func buildCommunicationPorts(cp map[string]interface{}) *client.CommunicationPorts {
	if len(cp) == 0 {
		return &client.CommunicationPorts{}
	}
	return &client.CommunicationPorts{
		MasterHttpPort:      utils.GetInt32Pointer(int32(cp["master_http_port"].(int))),
		MasterRpcPort:       utils.GetInt32Pointer(int32(cp["master_rpc_port"].(int))),
		NodeExporterPort:    utils.GetInt32Pointer(int32(cp["node_exporter_port"].(int))),
		RedisServerHttpPort: utils.GetInt32Pointer(int32(cp["redis_server_http_port"].(int))),
		RedisServerRpcPort:  utils.GetInt32Pointer(int32(cp["redis_server_rpc_port"].(int))),
		TserverHttpPort:     utils.GetInt32Pointer(int32(cp["tserver_http_port"].(int))),
		TserverRpcPort:      utils.GetInt32Pointer(int32(cp["tserver_rpc_port"].(int))),
		YqlServerHttpPort:   utils.GetInt32Pointer(int32(cp["yql_server_http_port"].(int))),
		YqlServerRpcPort:    utils.GetInt32Pointer(int32(cp["yql_server_rpc_port"].(int))),
		YsqlServerHttpPort:  utils.GetInt32Pointer(int32(cp["ysql_server_http_port"].(int))),
		YsqlServerRpcPort:   utils.GetInt32Pointer(int32(cp["ysql_server_rpc_port"].(int))),
	}
}

func buildClusters(clusters []interface{}) (res []client.Cluster) {
	for _, v := range clusters {
		cluster := v.(map[string]interface{})
		c := client.Cluster{
			ClusterType: cluster["cluster_type"].(string),
			UserIntent:  buildUserIntent(utils.MapFromSingletonList(cluster["user_intent"].([]interface{}))),
		}
		if len(cluster["cloud_list"].([]interface{})) > 0 {
			c.PlacementInfo = &client.PlacementInfo{
				CloudList: buildCloudList(cluster["cloud_list"].([]interface{})),
			}
		}

		res = append(res, c)
	}
	return res
}

func buildCloudList(cl []interface{}) (res []client.PlacementCloud) {
	for _, v := range cl {
		c := v.(map[string]interface{})
		pc := client.PlacementCloud{
			Code:       utils.GetStringPointer(c["code"].(string)),
			RegionList: buildRegionList(c["region_list"].([]interface{})),
		}
		res = append(res, pc)
	}
	return res
}

func buildRegionList(cl []interface{}) *[]client.PlacementRegion {
	var res []client.PlacementRegion
	for _, v := range cl {
		r := v.(map[string]interface{})
		pr := client.PlacementRegion{
			Code:   utils.GetStringPointer(r["code"].(string)),
			AzList: buildAzList(r["az_list"].([]interface{})),
		}
		res = append(res, pr)
	}
	return &res
}

func buildAzList(cl []interface{}) *[]client.PlacementAZ {
	var res []client.PlacementAZ
	for _, v := range cl {
		az := v.(map[string]interface{})
		paz := client.PlacementAZ{
			IsAffinitized:     utils.GetBoolPointer(az["is_affinitized"].(bool)),
			Name:              utils.GetStringPointer(az["name"].(string)),
			NumNodesInAZ:      utils.GetInt32Pointer(int32(az["num_nodes"].(int))),
			ReplicationFactor: utils.GetInt32Pointer(int32(az["replication_factor"].(int))),
			SecondarySubnet:   utils.GetStringPointer(az["secondary_subnet"].(string)),
			Subnet:            utils.GetStringPointer(az["subnet"].(string)),
		}
		res = append(res, paz)
	}
	return &res
}

func buildUserIntent(ui map[string]interface{}) client.UserIntent {
	return client.UserIntent{
		AssignStaticPublicIP:      utils.GetBoolPointer(ui["assign_static_ip"].(bool)),
		AwsArnString:              utils.GetStringPointer(ui["aws_arn_string"].(string)),
		EnableExposingService:     utils.GetStringPointer(ui["enable_exposing_service"].(string)),
		EnableIPV6:                utils.GetBoolPointer(ui["enable_ipv6"].(bool)),
		EnableYCQL:                utils.GetBoolPointer(ui["enable_ycql"].(bool)),
		EnableYCQLAuth:            utils.GetBoolPointer(ui["enable_ycql_auth"].(bool)),
		EnableYSQLAuth:            utils.GetBoolPointer(ui["enable_ysql_auth"].(bool)),
		InstanceTags:              utils.StringMap(ui["instance_tags"].(map[string]interface{})),
		PreferredRegion:           utils.GetStringPointer(ui["preferred_region"].(string)),
		UseHostname:               utils.GetBoolPointer(ui["use_host_name"].(bool)),
		UseSystemd:                utils.GetBoolPointer(ui["use_systemd"].(bool)),
		YsqlPassword:              utils.GetStringPointer(ui["ysql_password"].(string)),
		YcqlPassword:              utils.GetStringPointer(ui["ycql_password"].(string)),
		UniverseName:              utils.GetStringPointer(ui["universe_name"].(string)),
		ProviderType:              utils.GetStringPointer(ui["provider_type"].(string)),
		Provider:                  utils.GetStringPointer(ui["provider"].(string)),
		RegionList:                utils.StringSlice(ui["region_list"].([]interface{})),
		NumNodes:                  utils.GetInt32Pointer(int32(ui["num_nodes"].(int))),
		ReplicationFactor:         utils.GetInt32Pointer(int32(ui["replication_factor"].(int))),
		InstanceType:              utils.GetStringPointer(ui["instance_type"].(string)),
		DeviceInfo:                buildDeviceInfo(utils.MapFromSingletonList(ui["device_info"].([]interface{}))),
		AssignPublicIP:            utils.GetBoolPointer(ui["assign_public_ip"].(bool)),
		UseTimeSync:               utils.GetBoolPointer(ui["use_time_sync"].(bool)),
		EnableYSQL:                utils.GetBoolPointer(ui["enable_ysql"].(bool)),
		EnableYEDIS:               utils.GetBoolPointer(ui["enable_yedis"].(bool)),
		EnableNodeToNodeEncrypt:   utils.GetBoolPointer(ui["enable_node_to_node_encrypt"].(bool)),
		EnableClientToNodeEncrypt: utils.GetBoolPointer(ui["enable_client_to_node_encrypt"].(bool)),
		EnableVolumeEncryption:    utils.GetBoolPointer(ui["enable_volume_encryption"].(bool)),
		YbSoftwareVersion:         utils.GetStringPointer(ui["yb_software_version"].(string)),
		AccessKeyCode:             utils.GetStringPointer(ui["access_key_code"].(string)),
		TserverGFlags:             utils.StringMap(ui["tserver_gflags"].(map[string]interface{})),
		MasterGFlags:              utils.StringMap(ui["master_gflags"].(map[string]interface{})),
	}
}

func buildDeviceInfo(di map[string]interface{}) *client.DeviceInfo {
	return &client.DeviceInfo{
		DiskIops:     utils.GetInt32Pointer(int32(di["disk_iops"].(int))),
		MountPoints:  utils.GetStringPointer(di["mount_points"].(string)),
		StorageClass: utils.GetStringPointer(di["storage_class"].(string)),
		Throughput:   utils.GetInt32Pointer(int32(di["throughput"].(int))),
		NumVolumes:   utils.GetInt32Pointer(int32(di["num_volumes"].(int))),
		VolumeSize:   utils.GetInt32Pointer(int32(di["volume_size"].(int))),
		StorageType:  utils.GetStringPointer(di["storage_type"].(string)),
	}
}

func resourceUniverseRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	c := meta.(*api.ApiClient).YugawareClient

	cUUID := d.Get("customer_id").(string)
	ctx = meta.(*api.ApiClient).SetContextApiKey(ctx, d.Get("customer_id").(string))
	r, _, err := c.UniverseManagementApi.GetUniverse(ctx, cUUID, d.Id()).Execute()
	if err != nil {
		return diag.FromErr(err)
	}

	u := r.UniverseDetails
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

func flattenCommunicationPorts(cp *client.CommunicationPorts) []interface{} {
	v := map[string]interface{}{
		"master_http_port":       cp.MasterHttpPort,
		"master_rpc_port":        cp.MasterRpcPort,
		"node_exporter_port":     cp.NodeExporterPort,
		"redis_server_http_port": cp.RedisServerHttpPort,
		"redis_server_rpc_port":  cp.RedisServerRpcPort,
		"tserver_http_port":      cp.TserverHttpPort,
		"tserver_rpc_port":       cp.TserverRpcPort,
		"yql_server_http_port":   cp.YqlServerHttpPort,
		"yql_server_rpc_port":    cp.YqlServerRpcPort,
		"ysql_server_http_port":  cp.YsqlServerHttpPort,
		"ysql_server_rpc_port":   cp.YsqlServerRpcPort,
	}
	return utils.CreateSingletonList(v)
}

func flattenClusters(clusters []client.Cluster) (res []map[string]interface{}) {
	for _, cluster := range clusters {
		c := map[string]interface{}{
			"uuid":         cluster.Uuid,
			"cluster_type": cluster.ClusterType,
			"user_intent":  flattenUserIntent(cluster.UserIntent),
			"cloud_list":   flattenCloudList(cluster.PlacementInfo.CloudList),
		}
		res = append(res, c)
	}
	return res
}

func flattenCloudList(cl []client.PlacementCloud) (res []interface{}) {
	for _, c := range cl {
		pc := map[string]interface{}{
			"uuid":        c.Uuid,
			"code":        c.Code,
			"region_list": flattenRegionList(*c.RegionList),
		}
		res = append(res, pc)
	}
	return res
}

func flattenRegionList(cl []client.PlacementRegion) (res []interface{}) {
	for _, r := range cl {
		pr := map[string]interface{}{
			"uuid":    r.Uuid,
			"code":    r.Code,
			"az_list": flattenAzList(*r.AzList),
		}
		res = append(res, pr)
	}
	return res
}

func flattenAzList(cl []client.PlacementAZ) (res []interface{}) {
	for _, az := range cl {
		paz := map[string]interface{}{
			"uuid":               az.Uuid,
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

func flattenUserIntent(ui client.UserIntent) []interface{} {
	v := map[string]interface{}{
		"assign_static_ip":              ui.AssignStaticPublicIP,
		"aws_arn_string":                ui.AwsArnString,
		"enable_exposing_service":       ui.EnableExposingService,
		"enable_ipv6":                   ui.EnableIPV6,
		"enable_ycql":                   ui.EnableYCQL,
		"enable_ycql_auth":              ui.EnableYCQLAuth,
		"enable_ysql_auth":              ui.EnableYSQLAuth,
		"instance_tags":                 utils.GetStringMap(ui.InstanceTags),
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
		"tserver_gflags":                utils.GetStringMap(ui.TserverGFlags),
		"master_gflags":                 utils.GetStringMap(ui.MasterGFlags),
	}
	return utils.CreateSingletonList(v)
}

func flattenDeviceInfo(di *client.DeviceInfo) []interface{} {
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
	// Only updates user intent for each cluster
	c := meta.(*api.ApiClient).YugawareClient

	cUUID := d.Get("customer_id").(string)
	ctx = meta.(*api.ApiClient).SetContextApiKey(ctx, d.Get("customer_id").(string))
	if d.HasChanges("clusters") {
		var taskIds []string
		clusters := d.Get("clusters").([]interface{})
		updateUni, _, err := c.UniverseManagementApi.GetUniverse(ctx, cUUID, d.Id()).Execute()
		if err != nil {
			return diag.FromErr(errors.New(fmt.Sprintf("Unable to find universe %s", d.Id())))
		}
		newUni := buildUniverse(d)

		for i, v := range clusters {
			if !d.HasChange(fmt.Sprintf("clusters.%d", i)) {
				continue
			}
			cluster := v.(map[string]interface{})
			updateUni.UniverseDetails.Clusters[i].UserIntent = newUni.Clusters[i].UserIntent
			req := client.UniverseConfigureTaskParams{
				UniverseUUID: utils.GetStringPointer(d.Id()),
				Clusters:     updateUni.UniverseDetails.Clusters,
			}
			if cluster["cluster_type"] == "PRIMARY" {
				r, _, err := c.UniverseClusterMutationsApi.UpdatePrimaryCluster(ctx, cUUID, d.Id()).UniverseConfigureTaskParams(req).Execute()
				if err != nil {
					return diag.FromErr(err)
				}
				taskIds = append(taskIds, *r.TaskUUID)
			} else {
				r, _, err := c.UniverseClusterMutationsApi.UpdateReadOnlyCluster(ctx, cUUID, d.Id()).UniverseConfigureTaskParams(req).Execute()
				if err != nil {
					return diag.FromErr(err)
				}
				taskIds = append(taskIds, *r.TaskUUID)
			}
		}
		tflog.Debug(ctx, fmt.Sprintf("Waiting for universe %s to be updated", d.Id()))
		for _, id := range taskIds {
			err := utils.WaitForTask(ctx, id, cUUID, c, time.Hour)
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

	cUUID := d.Get("customer_id").(string)
	ctx = meta.(*api.ApiClient).SetContextApiKey(ctx, d.Get("customer_id").(string))
	r, _, err := c.UniverseManagementApi.DeleteUniverse(ctx, cUUID, d.Id()).
		IsForceDelete(d.Get("force_delete").(bool)).
		IsDeleteBackups(d.Get("delete_backups").(bool)).
		IsDeleteAssociatedCerts(d.Get("delete_certs").(bool)).
		Execute()
	if err != nil {
		return diag.FromErr(err)
	}

	tflog.Debug(ctx, fmt.Sprintf("Waiting for universe %s to be deleted", d.Id()))
	err = utils.WaitForTask(ctx, *r.TaskUUID, cUUID, c, time.Hour)
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId("")
	return diags
}
