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
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

func cloudListSchema() *schema.Resource {
	return &schema.Resource{
		Schema: map[string]*schema.Schema{
			"uuid": {
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
				Description: "Cloud Provider UUID.",
			},
			"code": {
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
				Description: "Cloud provider code.",
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
							Description: "Region UUID.",
						},
						"code": {
							Type:        schema.TypeString,
							Optional:    true,
							Computed:    true,
							Description: "Region Code.",
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
										Description: "Zone UUID.",
									},
									"is_affinitized": {
										Type:        schema.TypeBool,
										Computed:    true,
										Description: "Is it an affinitized zone.",
									},
									"name": {
										Type:        schema.TypeString,
										Optional:    true,
										Computed:    true,
										Description: "Zone name.",
									},
									"num_nodes": {
										Type:        schema.TypeInt,
										Optional:    true,
										Computed:    true,
										Description: "Number of nodes in this zone.",
									},
									"replication_factor": {
										Type:        schema.TypeInt,
										Optional:    true,
										Computed:    true,
										Description: "Replication factor in this zone.",
									},
									"secondary_subnet": {
										Type:        schema.TypeString,
										Optional:    true,
										Computed:    true,
										Description: "Secondary subnet of the zone.",
									},
									"subnet": {
										Type:        schema.TypeString,
										Optional:    true,
										Computed:    true,
										Description: "Subnet ID of zone.",
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
				Default:     false,
				Description: "Flag indicating whether a static IP should be assigned.",
			},
			"aws_arn_string": {
				Type:        schema.TypeString,
				Optional:    true,
				Default:     "",
				Description: "IP ARN String.",
			},
			"enable_exposing_service": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
				Description: "Flag to use if we need to deploy a loadbalancer/some kind of " +
					"exposing service for the cluster.",
			},
			"enable_ipv6": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     false,
				Description: "Enable IPv6.",
			},
			"enable_ycql": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     true,
				Description: "Enable YCQL. True by default.",
			},
			"enable_ycql_auth": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     false,
				Description: "Enable YCQL authentication.",
			},
			"enable_ysql_auth": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "Enable YSQL authentication.",
			},
			"image_bundle_uuid": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
				DiffSuppressFunc: func(k, old, new string, d *schema.ResourceData) bool {
					// When omitted, YBA resolves the provider's default image bundle
					// for the configured arch. Suppress the diff so the auto-assigned
					// UUID does not appear as a change on subsequent plans.
					// An explicit non-empty value always takes effect and triggers a
					// VM image upgrade on update.
					return len(old) > 0 && new == ""
				},
				Description: "Image Bundle UUID. When omitted for cloud providers " +
					"(aws, gcp, azu), YBA resolves the provider's default image bundle " +
					"for the configured arch.",
			},
			"instance_tags": {
				Type:        schema.TypeMap,
				Elem:        &schema.Schema{Type: schema.TypeString},
				Optional:    true,
				Description: "Instance Tags.",
			},
			"preferred_region": {
				Type:        schema.TypeString,
				Optional:    true,
				Default:     "",
				Description: "Preferred Region for node placement.",
			},
			"use_host_name": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     false,
				Description: "Enable to use host name instead of IP addresses to communicate.",
			},
			"use_systemd": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     true,
				Description: "Enable Systemd in universe nodes. True by default.",
			},
			"ysql_password": {
				Type:      schema.TypeString,
				Optional:  true,
				Sensitive: true,
				Description: "YSQL auth password. Required when enable_ysql_auth is true. " +
					"Stored in Terraform state - use an encrypted backend for security.",
			},
			"ycql_password": {
				Type:      schema.TypeString,
				Optional:  true,
				Default:   "",
				Sensitive: true,
				Description: "YCQL auth password. Required when enable_ycql_auth is true. " +
					"Stored in Terraform state - use an encrypted backend for security.",
			},
			"universe_name": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Universe name.",
			},
			"provider_type": {
				Type:     schema.TypeString,
				Computed: true,
				Description: "Cloud provider type. " +
					"Derived from the referenced provider UUID via the provider API.",
			},
			"provider": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Provider UUID.",
			},
			"region_list": {
				Type: schema.TypeList,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
				Required:    true,
				Description: "List of regions for node placement.",
			},
			"num_nodes": {
				Type:        schema.TypeInt,
				Required:    true,
				Description: "Number of nodes for this universe.",
			},
			"replication_factor": {
				Type:        schema.TypeInt,
				Required:    true,
				Description: "Replication factor for this universe.",
			},
			"instance_type": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Instance type of universe nodes.",
			},
			"device_info": {
				Type:     schema.TypeList,
				MaxItems: 1,
				Required: true,
				Description: "Configuration values associated with the machines used " +
					"for this universe.",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"disk_iops": {
							Type:        schema.TypeInt,
							Optional:    true,
							Description: "Disk IOPS.",
						},
						"mount_points": {
							Type:     schema.TypeString,
							Optional: true,
							Default:  "",
							Description: "Disk mount points. Required for on-prem cluster nodes. " +
								"Not allowed for any other provider type.",
						},
						"throughput": {
							Type:     schema.TypeInt,
							Optional: true,
							Description: "Disk throughput in MB/s. Required for storage types " +
								"that support throughput provisioning: GP3, UltraSSD_LRS, " +
								"PremiumV2_LRS, Hyperdisk_Balanced.",
						},
						"num_volumes": {
							Type:        schema.TypeInt,
							Required:    true,
							Description: "Number of volumes per node.",
						},
						"volume_size": {
							Type:        schema.TypeInt,
							Required:    true,
							Description: "Volume size in GB.",
						},
						"storage_type": {
							Type:     schema.TypeString,
							Optional: true,
							ValidateFunc: validation.StringInSlice([]string{
								"IO1", "IO2", "GP2", "GP3",
								"Scratch", "Persistent",
								"Hyperdisk_Balanced", "Hyperdisk_Extreme",
								"StandardSSD_LRS", "Premium_LRS",
								"PremiumV2_LRS", "UltraSSD_LRS",
								"Local",
							}, false),
							Description: "Storage type of volume. AWS: IO1, IO2, GP2, GP3. " +
								"GCP: Scratch, Persistent, Hyperdisk_Balanced, Hyperdisk_Extreme. " +
								"Azure: StandardSSD_LRS, Premium_LRS, PremiumV2_LRS, UltraSSD_LRS. " +
								"Not applicable for on-prem or Kubernetes providers.",
						},
					},
				},
			},
			"assign_public_ip": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     true,
				Description: "Assign Public IP to universe nodes. True by default.",
			},
			"use_time_sync": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     true,
				Description: "Enable time sync. True by default.",
			},
			"enable_ysql": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     true,
				Description: "Enable YSQL. True by default.",
			},
			"enable_yedis": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     false,
				Description: "Enable YEDIS. False by default.",
			},
			"enable_node_to_node_encrypt": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  true,
				Description: "Enable Encryption in Transit - Node to Node encryption." +
					" True by default.",
			},
			"enable_client_to_node_encrypt": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  true,
				Description: "Enable Encryption in Transit - Client to Node encryption." +
					" True by default.",
			},
			"enable_volume_encryption": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     false,
				Description: "Enable Encryption At Rest. False by default.",
			},
			"yb_software_version": {
				Type:     schema.TypeString,
				Required: true,
				Description: "YBDB version of the universe. Changing this field triggers a " +
					"DB version upgrade (UpgradeDBVersion). By default the upgrade pauses " +
					"at PreFinalize state for a monitoring phase; set " +
					"db_version_upgrade_options.finalize = true to commit automatically " +
					"after the upgrade task completes. " +
					"See db_version_upgrade_options for full rollback/finalize controls.",
			},
			"access_key_code": {
				Type:     schema.TypeString,
				Optional: true,
				Description: "Access Key code of provider. Required for cloud providers " +
					"(aws, gcp, azu). Not required for on-prem providers using node agents " +
					"(YNP-provisioned / skipProvisioning enabled).",
			},
			"tserver_gflags": {
				Type:        schema.TypeMap,
				Elem:        schema.TypeString,
				Optional:    true,
				Description: "Set of TServer Gflags.",
			},
			"master_gflags": {
				Type:        schema.TypeMap,
				Elem:        schema.TypeString,
				Optional:    true,
				Description: "Set of Master GFlags.",
			},
		},
	}
}

func nodeDetailsSetSchema() *schema.Resource {
	return &schema.Resource{
		Schema: map[string]*schema.Schema{
			"az_uuid": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Availaibility zone UUID of the node.",
			},
			"cloud_info": {
				Type:        schema.TypeList,
				MaxItems:    1,
				Required:    true,
				Description: "Node placement cloud info.",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"assign_public_ip": {
							Type:        schema.TypeBool,
							Optional:    true,
							Description: "True if the node has a public IP address assigned.",
						},
						"az": {
							Type:        schema.TypeString,
							Optional:    true,
							Description: "The node's availability zone.",
						},
						"cloud": {
							Type:        schema.TypeString,
							Optional:    true,
							Description: "The node's cloud provider.",
						},
						"instance_type": {
							Type:        schema.TypeString,
							Optional:    true,
							Description: "The node's instance type.",
						},
						"lun_indexes": {
							Type:        schema.TypeList,
							Optional:    true,
							Description: "Mounted disks LUN indexes.",
							Elem: &schema.Schema{
								Type: schema.TypeInt,
							},
						},
						"mount_roots": {
							Type:        schema.TypeString,
							Optional:    true,
							Description: "Mount roots.",
						},
						"private_dns": {
							Type:        schema.TypeString,
							Optional:    true,
							Description: "The node's private DNS.",
						},
						"private_ip": {
							Type:        schema.TypeString,
							Optional:    true,
							Description: "The node's private IP address.",
						},
						"public_dns": {
							Type:        schema.TypeString,
							Optional:    true,
							Description: "The node's public DNS name.",
						},
						"public_ip": {
							Type:        schema.TypeString,
							Optional:    true,
							Description: "The node's public IP address.",
						},
						"region": {
							Type:        schema.TypeString,
							Optional:    true,
							Description: "The node's region.",
						},
						"root_volume": {
							Type:        schema.TypeString,
							Optional:    true,
							Description: "Root volume ID or name.",
						},
						"secondary_private_ip": {
							Type:        schema.TypeString,
							Optional:    true,
							Description: "Secondary Private IP.",
						},
						"secondary_subnet_id": {
							Type:        schema.TypeString,
							Optional:    true,
							Description: "Secondary Subnet ID.",
						},
						"subnet_id": {
							Type:        schema.TypeString,
							Optional:    true,
							Description: "ID of the subnet on which this node is deployed.",
						},
						"use_time_sync": {
							Type:        schema.TypeBool,
							Optional:    true,
							Description: "True if `use time sync` is enabled.",
						},
					},
				},
			},
			"crons_active": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "True if cron jobs were properly configured for this node.",
			},
			"dedicated_to": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Used for configurations where each node can have only one process.",
			},
			"disks_are_mounted_by_uuid": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "Disks are mounted by UUID.",
			},
			"is_master": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "True if this node is a master.",
			},
			"is_redis_server": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "True if this node is a REDIS server.",
			},
			"is_tserver": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "True if this node is a Tablet server.",
			},
			"is_yql_server": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "True if this node is a YCQL server.",
			},
			"is_ysql_server": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "True if this node is a YSQL server.",
			},
			"last_volume_update_time": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Store last volume update time.",
			},
			"machine_image": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Machine image name.",
			},
			"master_http_port": {
				Type:        schema.TypeInt,
				Optional:    true,
				Description: "Master HTTP port.",
			},
			"master_rpc_port": {
				Type:        schema.TypeInt,
				Optional:    true,
				Description: "Master RPC port.",
			},
			"master_state": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Master state.",
			},
			"node_exporter_port": {
				Type:        schema.TypeInt,
				Optional:    true,
				Description: "Node exporter port.",
			},
			"node_idx": {
				Type:        schema.TypeInt,
				Optional:    true,
				Description: "Node ID.",
			},
			"node_name": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Node name.",
			},
			"node_uuid": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Node UUID.",
			},
			"otel_collector_metrics_port": {
				Type:        schema.TypeInt,
				Optional:    true,
				Description: "Otel collector metrics port.",
			},
			"placement_uuid": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "UUID of the cluster to which this node belongs.",
			},
			"redis_server_http_port": {
				Type:        schema.TypeInt,
				Optional:    true,
				Description: "REDIS HTTP port.",
			},
			"redis_server_rpc_port": {
				Type:        schema.TypeInt,
				Optional:    true,
				Description: "REDIS RPC port.",
			},
			"ssh_port_override": {
				Type:        schema.TypeInt,
				Optional:    true,
				Description: "SSH port override for the AMI.",
			},
			"ssh_user_override": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "SSH user override for the AMI.",
			},
			"state": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Node state.",
			},
			"tserver_http_port": {
				Type:        schema.TypeInt,
				Optional:    true,
				Description: "Tablet server HTTP port.",
			},
			"tserver_rpc_port": {
				Type:        schema.TypeInt,
				Optional:    true,
				Description: "Tablet server RPC port.",
			},
			"yb_controller_http_port": {
				Type:        schema.TypeInt,
				Optional:    true,
				Description: "Yb controller HTTP port.",
			},
			"yb_controller_rpc_port": {
				Type:        schema.TypeInt,
				Optional:    true,
				Description: "Yb controller RPC port.",
			},
			"yb_prebuilt_ami": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "True if this is a custom YB AMI.",
			},
			"yql_server_http_port": {
				Type:        schema.TypeInt,
				Optional:    true,
				Description: "YCQL HTTP port.",
			},
			"yql_server_rpc_port": {
				Type:        schema.TypeInt,
				Optional:    true,
				Description: "YCQL RPC port.",
			},
			"ysql_server_http_port": {
				Type:        schema.TypeInt,
				Optional:    true,
				Description: "YSQL HTTP port.",
			},
			"ysql_server_rpc_port": {
				Type:        schema.TypeInt,
				Optional:    true,
				Description: "YSQL RPC port.",
			},
		},
	}
}
