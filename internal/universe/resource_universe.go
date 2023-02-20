package universe

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	client "github.com/yugabyte/platform-go-client"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/api"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/utils"
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

		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(60 * time.Minute),
			Update: schema.DefaultTimeout(30 * time.Minute),
			Delete: schema.DefaultTimeout(30 * time.Minute),
		},

		Schema: map[string]*schema.Schema{
			// Universe Delete Options
			"delete_options": {
				Type:     schema.TypeList,
				Optional: true,
				MaxItems: 1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"delete_certs": {
							Type:        schema.TypeBool,
							Optional:    true,
							Description: "Flag indicating whether the certificates should be deleted with the universe",
						},
						"delete_backups": {
							Type:        schema.TypeBool,
							Optional:    true,
							Description: "Flag indicating whether the backups should be deleted with the universe",
						},
						"force_delete": {
							Type:        schema.TypeBool,
							Optional:    true,
							Description: "", // TODO: document
						},
					},
				},
			},

			// Universe Fields
			"client_root_ca": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "", // TODO: document
			},
			"clusters": {
				Type:     schema.TypeList,
				Required: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"uuid": {
							Type:        schema.TypeString,
							Computed:    true,
							Description: "Cluster UUID",
						},
						"cluster_type": {
							Type:        schema.TypeString,
							Required:    true,
							Description: "Cluster Type. Permitted values: PRIMARY, ASYNC",
						},
						"user_intent": {
							Type:        schema.TypeList,
							MaxItems:    1,
							Required:    true,
							Elem:        userIntentSchema(),
							Description: "Configuration values used in universe creation. Only these values can be updated.",
						},
						"cloud_list": {
							Type:        schema.TypeList,
							Optional:    true,
							Computed:    true,
							Elem:        cloudListSchema(),
							Description: "Cloud, region, and zone placement information for the universe",
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
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
				Description: "", // TODO: document
			},
			"code": {
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
				Description: "", // TODO: document
			},
			"region_list": {
				Type:     schema.TypeList,
				Optional: true,
				Computed: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"uuid": {
							Type:        schema.TypeString,
							Computed:    true,
							Optional:    true,
							Description: "Region UUID",
						},
						"code": {
							Type:        schema.TypeString,
							Optional:    true,
							Computed:    true,
							Description: "", // TODO: document
						},
						"az_list": {
							Type:     schema.TypeList,
							Optional: true,
							Computed: true,
							Elem: &schema.Resource{
								Schema: map[string]*schema.Schema{
									"uuid": {
										Type:        schema.TypeString,
										Optional:    true,
										Computed:    true,
										Description: "Zone UUID",
									},
									"is_affinitized": {
										Type:        schema.TypeBool,
										Computed:    true,
										Description: "", // TODO: document
									},
									"name": {
										Type:        schema.TypeString,
										Optional:    true,
										Computed:    true,
										Description: "Zone name",
									},
									"num_nodes": {
										Type:        schema.TypeInt,
										Optional:    true,
										Computed:    true,
										Description: "Number of nodes in this zone",
									},
									"replication_factor": {
										Type:        schema.TypeInt,
										Optional:    true,
										Computed:    true,
										Description: "Replication factor in this zone",
									},
									"secondary_subnet": {
										Type:        schema.TypeString,
										Optional:    true,
										Computed:    true,
										Description: "", // TODO: document
									},
									"subnet": {
										Type:        schema.TypeString,
										Optional:    true,
										Computed:    true,
										Description: "", // TODO: document
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
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "Flag indicating whether a static IP should be assigned",
			},
			"aws_arn_string": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "", // TODO: document
			},
			"enable_exposing_service": {
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
				Description: "", // TODO: document
			},
			"enable_ipv6": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "", // TODO: document
			},
			"enable_ycql": {
				Type:        schema.TypeBool,
				Optional:    true,
				Computed:    true,
				Description: "", // TODO: document
			},
			"enable_ycql_auth": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "", // TODO: document
			},
			"enable_ysql_auth": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "", // TODO: document
			},
			"instance_tags": {
				Type:        schema.TypeMap,
				Elem:        &schema.Schema{Type: schema.TypeString},
				Optional:    true,
				Description: "", // TODO: document
			},
			"preferred_region": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "", // TODO: document
			},
			"use_host_name": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "", // TODO: document
			},
			"use_systemd": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "", // TODO: document
			},
			"ysql_password": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "", // TODO: document
			},
			"ycql_password": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "", // TODO: document
			},
			"universe_name": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "", // TODO: document
			},
			"provider_type": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "", // TODO: document
			},
			"provider": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "", // TODO: document
			},
			"region_list": {
				Type: schema.TypeList,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
				Optional:    true,
				Description: "", // TODO: document
			},
			"num_nodes": {
				Type:        schema.TypeInt,
				Optional:    true,
				Description: "Number of nodes for this universe",
			},
			"replication_factor": {
				Type:        schema.TypeInt,
				Optional:    true,
				Description: "Replicated factor for this universe",
			},
			"instance_type": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "", // TODO: document
			},
			"device_info": {
				Type:        schema.TypeList,
				MaxItems:    1,
				Required:    true,
				Description: "Configuration values associated with the machines used for this universe",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"disk_iops": {
							Type:        schema.TypeInt,
							Optional:    true,
							Description: "", // TODO: document
						},
						"mount_points": {
							Type:        schema.TypeString,
							Optional:    true,
							Description: "", // TODO: document
						},
						"storage_class": {
							Type:        schema.TypeString,
							Optional:    true,
							Description: "", // TODO: document
						},
						"throughput": {
							Type:        schema.TypeInt,
							Optional:    true,
							Description: "", // TODO: document
						},
						"num_volumes": {
							Type:        schema.TypeInt,
							Optional:    true,
							Description: "", // TODO: document
						},
						"volume_size": {
							Type:        schema.TypeInt,
							Optional:    true,
							Description: "", // TODO: document
						},
						"storage_type": {
							Type:        schema.TypeString,
							Optional:    true,
							Description: "", // TODO: document
						},
					},
				},
			},
			"assign_public_ip": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "", // TODO: document
			},
			"use_time_sync": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "", // TODO: document
			},
			"enable_ysql": {
				Type:        schema.TypeBool,
				Optional:    true,
				Computed:    true,
				Description: "", // TODO: document
			},
			"enable_yedis": {
				Type:        schema.TypeBool,
				Optional:    true,
				Computed:    true,
				Description: "", // TODO: document
			},
			"enable_node_to_node_encrypt": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "", // TODO: document
			},
			"enable_client_to_node_encrypt": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "", // TODO: document
			},
			"enable_volume_encryption": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "", // TODO: document
			},
			"yb_software_version": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "", // TODO: document
			},
			"access_key_code": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "", // TODO: document
			},
			"tserver_gflags": {
				Type:        schema.TypeMap,
				Elem:        schema.TypeString,
				Optional:    true,
				Description: "", // TODO: document
			},
			"master_gflags": {
				Type:        schema.TypeMap,
				Elem:        schema.TypeString,
				Optional:    true,
				Description: "", // TODO: document
			},
		},
	}
}

func resourceUniverseCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	c := meta.(*api.ApiClient).YugawareClient
	cUUID := meta.(*api.ApiClient).CustomerId

	req := buildUniverse(d)
	r, _, err := c.UniverseClusterMutationsApi.CreateAllClusters(ctx, cUUID).UniverseConfigureTaskParams(req).Execute()
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(*r.ResourceUUID)
	tflog.Debug(ctx, fmt.Sprintf("Waiting for universe %s to be active", d.Id()))
	err = utils.WaitForTask(ctx, *r.TaskUUID, cUUID, c, d.Timeout(schema.TimeoutCreate))
	if err != nil {
		return diag.FromErr(err)
	}
	return resourceUniverseRead(ctx, d, meta)
}

func buildUniverse(d *schema.ResourceData) client.UniverseConfigureTaskParams {
	clusters := buildClusters(d.Get("clusters").([]interface{}))
	return client.UniverseConfigureTaskParams{
		ClientRootCA:       utils.GetStringPointer(d.Get("client_root_ca").(string)),
		Clusters:           clusters,
		CommunicationPorts: buildCommunicationPorts(utils.MapFromSingletonList(d.Get("communication_ports").([]interface{}))),
	}
}

func buildUniverseDefinitionTaskParams(d *schema.ResourceData) client.UniverseDefinitionTaskParams {
	return client.UniverseDefinitionTaskParams{
		ClientRootCA:       utils.GetStringPointer(d.Get("client_root_ca").(string)),
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

func buildNodeDetailsRespArrayToNodeDetailsArray(nodes *[]client.NodeDetailsResp) *[]client.NodeDetails {
	var nodesDetails []client.NodeDetails
	for _, v := range *nodes {
		nodeDetail := client.NodeDetails{
			AzUuid:                v.AzUuid,
			CloudInfo:             v.CloudInfo,
			CronsActive:           v.CronsActive,
			DisksAreMountedByUUID: v.DisksAreMountedByUUID,
			IsMaster:              v.IsMaster,
			IsRedisServer:         v.IsRedisServer,
			IsTserver:             v.IsTserver,
			IsYqlServer:           v.IsYqlServer,
			IsYsqlServer:          v.IsYsqlServer,
			MachineImage:          v.MachineImage,
			MasterHttpPort:        v.MasterHttpPort,
			MasterRpcPort:         v.MasterRpcPort,
			MasterState:           v.MasterState,
			NodeExporterPort:      v.NodeExporterPort,
			NodeIdx:               v.NodeIdx,
			NodeName:              v.NodeName,
			NodeUuid:              v.NodeUuid,
			PlacementUuid:         v.PlacementUuid,
			RedisServerHttpPort:   v.RedisServerHttpPort,
			RedisServerRpcPort:    v.RedisServerRpcPort,
			State:                 v.State,
			TserverHttpPort:       v.TserverHttpPort,
			TserverRpcPort:        v.TserverRpcPort,
			YbPrebuiltAmi:         v.YbPrebuiltAmi,
			YqlServerHttpPort:     v.YqlServerHttpPort,
			YqlServerRpcPort:      v.YqlServerRpcPort,
			YsqlServerHttpPort:    v.YsqlServerHttpPort,
			YsqlServerRpcPort:     v.YsqlServerRpcPort,
		}
		nodesDetails = append(nodesDetails, nodeDetail)
	}
	return &nodesDetails
}

func resourceUniverseRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	c := meta.(*api.ApiClient).YugawareClient
	cUUID := meta.(*api.ApiClient).CustomerId

	r, _, err := c.UniverseManagementApi.GetUniverse(ctx, cUUID, d.Id()).Execute()
	if err != nil {
		return diag.FromErr(err)
	}

	u := r.UniverseDetails
	if err = d.Set("client_root_ca", u.ClientRootCA); err != nil {
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
		"instance_tags":                 ui.GetInstanceTags(),
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
		"tserver_gflags":                ui.GetTserverGFlags(),
		"master_gflags":                 ui.GetMasterGFlags(),
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

func editUniverseParameters(ctx context.Context, old_user_intent client.UserIntent, new_user_intent client.UserIntent) (bool, client.UserIntent) {
	if !reflect.DeepEqual(*old_user_intent.InstanceTags, *new_user_intent.InstanceTags) ||
		!reflect.DeepEqual(*old_user_intent.RegionList, new_user_intent.RegionList) ||
		*old_user_intent.NumNodes != *new_user_intent.NumNodes ||
		*old_user_intent.InstanceType != *new_user_intent.InstanceType ||
		*old_user_intent.DeviceInfo.NumVolumes != *new_user_intent.DeviceInfo.NumVolumes {
		edit_num_volume := true
		num_volumes := *old_user_intent.DeviceInfo.NumVolumes
		if (*old_user_intent.DeviceInfo.NumVolumes != *new_user_intent.DeviceInfo.NumVolumes) &&
			(*old_user_intent.InstanceType == *new_user_intent.InstanceType) {
			tflog.Info(ctx, "Cannot edit Number of Volumes per instance without an edit to Instance Type, Ignoring Change")
			edit_num_volume = false
		}
		old_user_intent = new_user_intent
		if !edit_num_volume {
			old_user_intent.DeviceInfo.NumVolumes = &num_volumes
		}
		return true, old_user_intent
	}
	return false, old_user_intent

}
/*
	** [WIP] editing AZ Zones
func editClusterZone(ctx context.Context, old_cluster client.Cluster, new_cluster client.Cluster) (bool, client.Cluster) {
	if !reflect.DeepEqual(old_cluster.PlacementInfo.CloudList, new_cluster.PlacementInfo.CloudList) &&
		reflect.DeepEqual(old_cluster.UserIntent.RegionList, new_cluster.UserIntent.RegionList) {
		old_cluster.PlacementInfo.CloudList = new_cluster.PlacementInfo.CloudList
		//need to change node detail set
		return true, old_cluster
	}
	return false, old_cluster
}
*/

func resourceUniverseUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	// Only updates user intent for each cluster
	// cloud Info can have changes in zones
	c := meta.(*api.ApiClient).YugawareClient
	cUUID := meta.(*api.ApiClient).CustomerId

	//var taskIds []string
	if d.HasChange("clusters") {
		clusters := d.Get("clusters").([]interface{})
		updateUni, _, err := c.UniverseManagementApi.GetUniverse(ctx, cUUID, d.Id()).Execute()
		if err != nil {
			return diag.FromErr(errors.New(fmt.Sprintf("Unable to find universe %s", d.Id())))
		}
		newUni := buildUniverse(d)

		if len(clusters) > 2 {
			tflog.Error(ctx, "Cannot have more than 1 Read only cluster")
		} else {
			if len(updateUni.UniverseDetails.Clusters) < len(clusters) {
				tflog.Error(ctx, "Currently not supporting adding Read Replicas after universe creation")
				/*err, req := formatCreateReadOnlyClusterRequestBody(ctx, d, updateUni)
				if err != nil {
					return diag.FromErr(err)
				}
				r, _, err := c.UniverseClusterMutationsApi.CreateReadOnlyCluster(ctx, cUUID, d.Id()).UniverseConfigureTaskParams(req).Execute()
				if err != nil {
					return diag.FromErr(err)
				}
				tflog.Info(ctx, "CreateReadOnlyCluster task is executing")
				err = utils.WaitForTask(ctx, *r.TaskUUID, cUUID, c, d.Timeout(schema.TimeoutUpdate))
				if err != nil {
					return diag.FromErr(err)
				}*/
			} else if len(updateUni.UniverseDetails.Clusters) > len(clusters) {
				var clusterUuid string
				for _, v := range updateUni.UniverseDetails.Clusters {
					if v.ClusterType == "ASYNC" {
						clusterUuid = *v.Uuid
					}
				}

				r, _, err := c.UniverseClusterMutationsApi.DeleteReadonlyCluster(ctx, cUUID, d.Id(), clusterUuid).
					IsForceDelete(d.Get("delete_options.0.force_delete").(bool)).Execute()
				if err != nil {
					return diag.FromErr(err)
				}
				tflog.Info(ctx, "DeleteReadOnlyCluster task is executing")
				err = utils.WaitForTask(ctx, *r.TaskUUID, cUUID, c, d.Timeout(schema.TimeoutUpdate))
				if err != nil {
					return diag.FromErr(err)
				}
			}
		}
		for i, v := range clusters {
			if !d.HasChange(fmt.Sprintf("clusters.%d", i)) {
				continue
			}
			cluster := v.(map[string]interface{})

			old_user_intent := updateUni.UniverseDetails.Clusters[i].UserIntent
			new_user_intent := newUni.Clusters[i].UserIntent
			if cluster["cluster_type"] == "PRIMARY" {

				//Software Upgrade
				if *old_user_intent.YbSoftwareVersion != *new_user_intent.YbSoftwareVersion {
					updateUni.UniverseDetails.Clusters[i].UserIntent = new_user_intent
					req := client.SoftwareUpgradeParams{
						YbSoftwareVersion: *new_user_intent.YbSoftwareVersion,
						Clusters:          updateUni.UniverseDetails.Clusters,
						UpgradeOption:     "Rolling",
					}
					r, _, err := c.UniverseUpgradesManagementApi.UpgradeSoftware(ctx, cUUID, d.Id()).SoftwareUpgradeParams(req).Execute()
					if err != nil {
						return diag.FromErr(err)
					}
					tflog.Info(ctx, "UpgradeSoftware task is executing")
					err = utils.WaitForTask(ctx, *r.TaskUUID, cUUID, c, d.Timeout(schema.TimeoutUpdate))
					if err != nil {
						return diag.FromErr(err)
					}
				}

				updateUni, _, err = c.UniverseManagementApi.GetUniverse(ctx, cUUID, d.Id()).Execute()
				if err != nil {
					return diag.FromErr(errors.New(fmt.Sprintf("Unable to find universe %s", d.Id())))
				}
				old_user_intent = updateUni.UniverseDetails.Clusters[i].UserIntent

				//GFlag Update
				if !reflect.DeepEqual(*old_user_intent.MasterGFlags, *new_user_intent.MasterGFlags) ||
					!reflect.DeepEqual(*old_user_intent.TserverGFlags, *new_user_intent.TserverGFlags) {
					updateUni.UniverseDetails.Clusters[i].UserIntent = new_user_intent
					req := client.GFlagsUpgradeParams{
						MasterGFlags:  *new_user_intent.MasterGFlags,
						TserverGFlags: *new_user_intent.TserverGFlags,
						Clusters:      updateUni.UniverseDetails.Clusters,
						UpgradeOption: "Rolling",
					}
					r, _, err := c.UniverseUpgradesManagementApi.UpgradeGFlags(ctx, cUUID, d.Id()).GflagsUpgradeParams(req).Execute()
					if err != nil {
						return diag.FromErr(err)
					}
					tflog.Info(ctx, "UpgradeGFlags task is executing")
					err = utils.WaitForTask(ctx, *r.TaskUUID, cUUID, c, d.Timeout(schema.TimeoutUpdate))
					if err != nil {
						return diag.FromErr(err)
					}
				}

				updateUni, _, err = c.UniverseManagementApi.GetUniverse(ctx, cUUID, d.Id()).Execute()
				if err != nil {
					return diag.FromErr(errors.New(fmt.Sprintf("Unable to find universe %s", d.Id())))
				}
				old_user_intent = updateUni.UniverseDetails.Clusters[i].UserIntent

				//TLS Toggle
				if *old_user_intent.EnableClientToNodeEncrypt != *new_user_intent.EnableClientToNodeEncrypt ||
					*old_user_intent.EnableNodeToNodeEncrypt != *new_user_intent.EnableNodeToNodeEncrypt {
					updateUni.UniverseDetails.Clusters[i].UserIntent = new_user_intent
					req := client.TlsToggleParams{
						EnableClientToNodeEncrypt: *new_user_intent.EnableClientToNodeEncrypt,
						EnableNodeToNodeEncrypt:   *new_user_intent.EnableNodeToNodeEncrypt,
						Clusters:                  updateUni.UniverseDetails.Clusters,
						UpgradeOption:             "Non-Rolling",
					}
					r, _, err := c.UniverseUpgradesManagementApi.UpgradeTls(ctx, cUUID, d.Id()).TlsToggleParams(req).Execute()
					if err != nil {
						return diag.FromErr(err)
					}
					tflog.Info(ctx, "UpgradeTLS task is executing")
					err = utils.WaitForTask(ctx, *r.TaskUUID, cUUID, c, d.Timeout(schema.TimeoutUpdate))
					if err != nil {
						return diag.FromErr(err)
					}
				}

				updateUni, _, err = c.UniverseManagementApi.GetUniverse(ctx, cUUID, d.Id()).Execute()
				if err != nil {
					return diag.FromErr(errors.New(fmt.Sprintf("Unable to find universe %s", d.Id())))
				}
				old_user_intent = updateUni.UniverseDetails.Clusters[i].UserIntent

				//SystemD upgrade
				if (*&new_user_intent.UseSystemd != nil) && (*old_user_intent.UseSystemd != *new_user_intent.UseSystemd) &&
					(*old_user_intent.UseSystemd == false) {
					updateUni.UniverseDetails.Clusters[i].UserIntent = new_user_intent
					req := client.SystemdUpgradeParams{

						Clusters:      updateUni.UniverseDetails.Clusters,
						UpgradeOption: "Rolling",
					}
					r, _, err := c.UniverseUpgradesManagementApi.UpgradeSystemd(ctx, cUUID, d.Id()).SystemdUpgradeParams(req).Execute()
					if err != nil {
						return diag.FromErr(err)
					}
					tflog.Info(ctx, "UpgradeSystemd task is executing")
					err = utils.WaitForTask(ctx, *r.TaskUUID, cUUID, c, d.Timeout(schema.TimeoutUpdate))
					if err != nil {
						return diag.FromErr(err)
					}
				} else if *old_user_intent.UseSystemd == true {
					tflog.Error(ctx, "Cannot disable Systemd")
				}

				updateUni, _, err = c.UniverseManagementApi.GetUniverse(ctx, cUUID, d.Id()).Execute()
				if err != nil {
					return diag.FromErr(errors.New(fmt.Sprintf("Unable to find universe %s", d.Id())))
				}
				old_user_intent = updateUni.UniverseDetails.Clusters[i].UserIntent

				// Resize Nodes
				if *old_user_intent.DeviceInfo.VolumeSize != *new_user_intent.DeviceInfo.VolumeSize {
					if *old_user_intent.DeviceInfo.VolumeSize < *new_user_intent.DeviceInfo.VolumeSize {
						//Only volume size should be changed to do smart resize, other changes handled in UpgradeCluster
						updateUni.UniverseDetails.Clusters[i].UserIntent.DeviceInfo.VolumeSize = new_user_intent.DeviceInfo.VolumeSize
						req := client.ResizeNodeParams{
							UpgradeOption:  "Rolling",
							Clusters:       updateUni.UniverseDetails.Clusters,
							NodeDetailsSet: buildNodeDetailsRespArrayToNodeDetailsArray(updateUni.UniverseDetails.NodeDetailsSet),
						}
						r, _, err := c.UniverseUpgradesManagementApi.ResizeNode(ctx, cUUID, d.Id()).ResizeNodeParams(req).Execute()
						if err != nil {
							return diag.FromErr(err)
						}
						tflog.Info(ctx, "ResizeNode task is executing")
						//taskIds = append(taskIds, *r.TaskUUID)
						err = utils.WaitForTask(ctx, *r.TaskUUID, cUUID, c, d.Timeout(schema.TimeoutUpdate))
						if err != nil {
							return diag.FromErr(err)
						}
					} else {
						tflog.Error(ctx, "Volume Size cannot be decreased")
					}
				}

				updateUni, _, err = c.UniverseManagementApi.GetUniverse(ctx, cUUID, d.Id()).Execute()
				if err != nil {
					return diag.FromErr(errors.New(fmt.Sprintf("Unable to find universe %s", d.Id())))
				}
				old_user_intent = updateUni.UniverseDetails.Clusters[i].UserIntent

				// Num of nodes, Instance Type, Num of Volumes, Volume Size, User Tags changes
				var edit_allowed, edit_zone_allowed bool
				edit_allowed, updateUni.UniverseDetails.Clusters[i].UserIntent = editUniverseParameters(ctx, old_user_intent, new_user_intent)
				edit_zone_allowed, updateUni.UniverseDetails.Clusters[i] = editClusterZone(ctx, updateUni.UniverseDetails.Clusters[i], newUni.Clusters[i])
				if edit_allowed || edit_zone_allowed {
					req := client.UniverseConfigureTaskParams{
						UniverseUUID:   utils.GetStringPointer(d.Id()),
						Clusters:       updateUni.UniverseDetails.Clusters,
						NodeDetailsSet: buildNodeDetailsRespArrayToNodeDetailsArray(updateUni.UniverseDetails.NodeDetailsSet),
					}
					r, _, err := c.UniverseClusterMutationsApi.UpdatePrimaryCluster(ctx, cUUID, d.Id()).UniverseConfigureTaskParams(req).Execute()
					if err != nil {
						return diag.FromErr(err)
					}
					//taskIds = append(taskIds, *r.TaskUUID)
					tflog.Info(ctx, "UpdatePrimaryCluster task is executing")
					err = utils.WaitForTask(ctx, *r.TaskUUID, cUUID, c, d.Timeout(schema.TimeoutUpdate))
					if err != nil {
						return diag.FromErr(err)
					}
				}

			} else {

				//Ignore Software, GFlags, SystemD, TLS Upgrade changes to Read-Only Cluster
				updateUni, _, err := c.UniverseManagementApi.GetUniverse(ctx, cUUID, d.Id()).Execute()
				if err != nil {
					return diag.FromErr(errors.New(fmt.Sprintf("Unable to find universe %s", d.Id())))
				}
				old_user_intent := updateUni.UniverseDetails.Clusters[i].UserIntent
				if *old_user_intent.YbSoftwareVersion != *new_user_intent.YbSoftwareVersion {
					tflog.Info(ctx, "Software Upgrade is applied only via change in Primary Cluster User Intent, ignoring")
				}
				if !reflect.DeepEqual(*old_user_intent.MasterGFlags, *new_user_intent.MasterGFlags) ||
					!reflect.DeepEqual(*old_user_intent.TserverGFlags, *new_user_intent.TserverGFlags) {
					tflog.Info(ctx, "GFlags Upgrade is applied only via change in Primary Cluster User Intent, ignoring")
				}
				if *old_user_intent.DeviceInfo.VolumeSize != *new_user_intent.DeviceInfo.VolumeSize {
					tflog.Error(ctx, "Volume Resize of Read Replica currently not be edited")
				}
				if (new_user_intent.UseSystemd != nil) && (*old_user_intent.UseSystemd != *new_user_intent.UseSystemd) {
					tflog.Info(ctx, "System Upgrade is applied only via change in Primary Cluster User Intent, ignoring")
				}
				if *old_user_intent.EnableClientToNodeEncrypt != *new_user_intent.EnableClientToNodeEncrypt ||
					*old_user_intent.EnableNodeToNodeEncrypt != *new_user_intent.EnableNodeToNodeEncrypt {
					tflog.Info(ctx, "TLS Toggle is applied only via change in Primary Cluster User Intent, ignoring")
				}

				// Num of nodes, Instance Type, Num of Volumes, User Tags changes
				var edit_allowed bool
				edit_allowed, updateUni.UniverseDetails.Clusters[i].UserIntent = editUniverseParameters(ctx, old_user_intent, new_user_intent)
				if edit_allowed {
					req := client.UniverseConfigureTaskParams{
						UniverseUUID:   utils.GetStringPointer(d.Id()),
						Clusters:       updateUni.UniverseDetails.Clusters,
						NodeDetailsSet: buildNodeDetailsRespArrayToNodeDetailsArray(updateUni.UniverseDetails.NodeDetailsSet),
					}
					r, _, err := c.UniverseClusterMutationsApi.UpdateReadOnlyCluster(ctx, cUUID, d.Id()).UniverseConfigureTaskParams(req).Execute()
					if err != nil {
						return diag.FromErr(err)
					}
					tflog.Info(ctx, "UpdateReadOnlyCluster task is executing")
					err = utils.WaitForTask(ctx, *r.TaskUUID, cUUID, c, d.Timeout(schema.TimeoutUpdate))
					if err != nil {
						return diag.FromErr(err)
					}
				}

				if (*old_user_intent.EnableClientToNodeEncrypt != *new_user_intent.EnableClientToNodeEncrypt) ||
					(*old_user_intent.EnableNodeToNodeEncrypt != *new_user_intent.EnableNodeToNodeEncrypt) {
					tflog.Info(ctx, "TLS Upgrade is applied only via change in Primary Cluster User Intent, ignoring")
				}
			}

		}
	}

	// wait for all tasks to complete
	/*for _, id := range taskIds {
		err := utils.WaitForTask(ctx, id, cUUID, c, d.Timeout(schema.TimeoutUpdate))
		if err != nil {
			return diag.FromErr(err)
		}
	}*/
	return resourceUniverseRead(ctx, d, meta)
}

func resourceUniverseDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	c := meta.(*api.ApiClient).YugawareClient
	cUUID := meta.(*api.ApiClient).CustomerId

	r, _, err := c.UniverseManagementApi.DeleteUniverse(ctx, cUUID, d.Id()).
		IsForceDelete(d.Get("delete_options.0.force_delete").(bool)).
		IsDeleteBackups(d.Get("delete_options.0.delete_backups").(bool)).
		IsDeleteAssociatedCerts(d.Get("delete_options.0.delete_certs").(bool)).
		Execute()
	if err != nil {
		return diag.FromErr(err)
	}

	tflog.Debug(ctx, fmt.Sprintf("Waiting for universe %s to be deleted", d.Id()))
	err = utils.WaitForTask(ctx, *r.TaskUUID, cUUID, c, d.Timeout(schema.TimeoutDelete))
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId("")
	return diags
}
