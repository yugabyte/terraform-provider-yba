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
			"provider": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "YBA provider UUID. Use the same value as user_intent.provider.",
			},
			"code": {
				Type:     schema.TypeString,
				Computed: true,
				Description: "Cloud provider code (e.g. aws, gcp, azu, onprem). " +
					"Derived from the provider UUID.",
			},
			"region_list": {
				Type:        schema.TypeList,
				Optional:    true,
				Computed:    true,
				Description: "Regions participating in placement for this cloud provider.",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"uuid": {
							Type:        schema.TypeString,
							Computed:    true,
							Description: "Region UUID.",
						},
						"code": {
							Type:     schema.TypeString,
							Required: true,
							Description: "Region code identifying the target region " +
								"(e.g. us-east-1, us-central1).",
						},
						"name": {
							Type:        schema.TypeString,
							Computed:    true,
							Description: "Region display name as returned by YBA.",
						},
						"az_list": {
							Type:     schema.TypeList,
							Optional: true,
							Computed: true,
							Description: "Availability zones participating in placement " +
								"for this region. " +
								"Note: this is a positional list. When removing a zone, " +
								"the plan may show adjacent zones appearing to change " +
								"(code, num_nodes, etc.) due to index shifting. " +
								"The provider resolves zones by code before sending the " +
								"API request, so the actual operation is always correct " +
								"regardless of how the plan is displayed.",
							Elem: &schema.Resource{
								Schema: map[string]*schema.Schema{
									"uuid": {
										Type:        schema.TypeString,
										Computed:    true,
										Description: "Availability zone UUID.",
									},
									"code": {
										Type:     schema.TypeString,
										Required: true,
										Description: "Availability zone code " +
											"(e.g. us-east-1a, us-central1-a).",
									},
									"is_affinitized": {
										Type:     schema.TypeBool,
										Optional: true,
										Computed: true,
										Description: "Whether this zone is preferred (affinitized) " +
											"for read traffic. When true, YBA routes read requests " +
											"to nodes in this zone first.",
									},
									"leader_preference": {
										Type:         schema.TypeInt,
										Optional:     true,
										Computed:     true,
										ValidateFunc: validation.IntAtLeast(0),
										Description: "Master leader placement priority for this zone. " +
											"Zero means no preference. A lower non-zero value " +
											"indicates higher priority (e.g. 1 is preferred over 2). " +
											"Multiple zones may share the same value. " +
											"When any zone has a non-zero value, all unique non-zero " +
											"priority values across zones must form a contiguous " +
											"sequence with no gaps (e.g. 1,2,3 is valid; 1,3 is not). " +
											"YBA enforces this and rejects requests with gaps. " +
											"Must be non-negative. " +
											"This setting is most effective with replication_factor >= 3, " +
											"where the YBA load balancer can move leaders between zones " +
											"without data migration. " +
											"For replication_factor = 1 (single master) the setting is " +
											"effectively a no-op: there are no follower replicas in other " +
											"zones to promote, so leader placement cannot be changed " +
											"without physically migrating tablet data.",
									},
									"num_nodes": {
										Type:         schema.TypeInt,
										Optional:     true,
										Computed:     true,
										ValidateFunc: validation.IntAtLeast(0),
										Description: "Number of nodes to place in this zone. " +
											"When cloud_list is set, these per-AZ counts are " +
											"the authoritative source of truth: YBA derives " +
											"the total node count from their sum and " +
											"user_intent.num_nodes is ignored. " +
											"YBA removes any AZ whose node count reaches 0 " +
											"from the placement; do not set this to 0 unless " +
											"the intent is to remove the zone.",
									},
									"replication_factor": {
										Type:         schema.TypeInt,
										Optional:     true,
										Computed:     true,
										ValidateFunc: validation.IntAtLeast(0),
										Description: "Number of replicas placed in this zone. " +
											"The sum of per-AZ values across the cluster must " +
											"equal user_intent.replication_factor, with one " +
											"explicit exception: a 2-AZ cluster at RF=3 may use " +
											"[1,1] (sum=2). replication_factor=0 is permitted " +
											"only when the number of zones exceeds " +
											"user_intent.replication_factor. Distributions YBA " +
											"considers invalid for the topology are silently " +
											"rewritten - neither the YBA API nor the YBA UI " +
											"surfaces a warning - leaving no effective change and " +
											"causing the apply to abort. Omit this field on every " +
											"az_list entry to let YBA compute the default " +
											"distribution.",
									},
									"subnet": {
										Type:        schema.TypeString,
										Computed:    true,
										Description: "Primary subnet ID for this zone, inherited from the provider.",
									},
									"secondary_subnet": {
										Type:        schema.TypeString,
										Computed:    true,
										Description: "Secondary subnet ID for this zone, inherited from the provider.",
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

// deviceInfoElemSchema returns the shared inner schema used by both device_info
// and master_device_info. Extracting it avoids duplicating the storage-type
// validation list and field definitions.
func deviceInfoElemSchema() *schema.Resource {
	return &schema.Resource{
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
					"Not applicable for on-prem providers.",
			},
		},
	}
}

// masterDeviceInfoElemSchema is the element schema for dedicated_masters.device_info.
//
// Inheritance semantics (first apply only):
//   - All fields are Optional + Computed.
//   - On the first apply, any field left unset (zero / empty) is automatically
//     copied from user_intent.device_info (the TServer configuration).
//
// Ownership after the first apply:
//   - Once a dedicated_masters.device_info block is present in config, every
//     field inside it is owned by this block. Subsequent changes to the
//     equivalent field in user_intent.device_info do NOT propagate to the
//     master block automatically.
//   - To keep a field in sync with the TServer, remove the device_info block
//     entirely (fall back to user_intent.device_info for all master fields) or
//     manage the field explicitly in both places.
//
// To inherit ALL TServer disk settings, omit device_info entirely:
//
//	dedicated_masters {}                  -- inherits everything from TServer
//	dedicated_masters { device_info {} }  -- same on first apply; fields are
//	                                         then owned by this block going
//	                                         forward and will not track TServer
func masterDeviceInfoElemSchema() *schema.Resource {
	return &schema.Resource{
		Schema: map[string]*schema.Schema{
			"disk_iops": {
				Type:     schema.TypeInt,
				Optional: true,
				Computed: true,
				Description: "Disk IOPS for master nodes. " +
					"Inherited from user_intent.device_info on the first apply when unset. " +
					"Once this device_info block is present in config, this field is no " +
					"longer updated automatically when user_intent.device_info.disk_iops changes.",
			},
			"mount_points": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
				Description: "Disk mount points for master nodes. Required for on-prem " +
					"cluster master nodes. " +
					"Inherited from user_intent.device_info on the first apply when unset. " +
					"Once this device_info block is present in config, this field is no " +
					"longer updated automatically when user_intent.device_info.mount_points changes.",
			},
			"throughput": {
				Type:     schema.TypeInt,
				Optional: true,
				Computed: true,
				Description: "Disk throughput in MB/s for master nodes. Required for " +
					"storage types that support throughput provisioning: GP3, UltraSSD_LRS, " +
					"PremiumV2_LRS, Hyperdisk_Balanced. " +
					"Inherited from user_intent.device_info on the first apply when unset. " +
					"Once this device_info block is present in config, this field is no " +
					"longer updated automatically when user_intent.device_info.throughput changes.",
			},
			"num_volumes": {
				Type:     schema.TypeInt,
				Optional: true,
				Computed: true,
				Description: "Number of volumes per master node. " +
					"Inherited from user_intent.device_info on the first apply when unset. " +
					"Once this device_info block is present in config, this field is no " +
					"longer updated automatically when user_intent.device_info.num_volumes changes.",
			},
			"volume_size": {
				Type:     schema.TypeInt,
				Optional: true,
				Computed: true,
				Description: "Volume size in GB for master nodes. " +
					"Inherited from user_intent.device_info on the first apply when unset. " +
					"Once this device_info block is present in config, this field is no " +
					"longer updated automatically when user_intent.device_info.volume_size changes.",
			},
			"storage_type": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
				ValidateFunc: validation.StringInSlice([]string{
					"IO1", "IO2", "GP2", "GP3",
					"Scratch", "Persistent",
					"Hyperdisk_Balanced", "Hyperdisk_Extreme",
					"StandardSSD_LRS", "Premium_LRS",
					"PremiumV2_LRS", "UltraSSD_LRS",
					"Local",
				}, false),
				Description: "Storage type for master node volumes. AWS: IO1, IO2, GP2, GP3. " +
					"GCP: Scratch, Persistent, Hyperdisk_Balanced, Hyperdisk_Extreme. " +
					"Azure: StandardSSD_LRS, Premium_LRS, PremiumV2_LRS, UltraSSD_LRS. " +
					"Inherited from user_intent.device_info on the first apply when unset. " +
					"Once this device_info block is present in config, this field is no " +
					"longer updated automatically when user_intent.device_info.storage_type changes.",
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
				Type:         schema.TypeInt,
				Required:     true,
				ValidateFunc: validation.IntAtLeast(1),
				Description: "Desired total number of nodes for this universe. " +
					"When cloud_list is also set, this value is ignored by YBA: " +
					"the actual node count is determined by the sum of " +
					"cloud_list[*].region_list[*].az_list[*].num_nodes " +
					"(userAZSelected=true). Set this to match that sum to " +
					"avoid plan drift on subsequent applies.",
			},
			"replication_factor": {
				Type:         schema.TypeInt,
				Required:     true,
				ValidateFunc: validation.IntAtLeast(1),
				Description:  "Replication factor for this universe.",
			},
			"instance_type": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Instance type of universe nodes.",
			},
			"device_info": {
				Type:        schema.TypeList,
				MaxItems:    1,
				Required:    true,
				Description: "Configuration values associated with the machines used for this universe.",
				Elem:        deviceInfoElemSchema(),
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
			"dedicated_masters": {
				Type:     schema.TypeList,
				MaxItems: 1,
				Optional: true,
				Description: "When present, master processes run on dedicated nodes separate " +
					"from TServer processes. " +
					"Omitting this block runs masters co-located with TServers. " +
					"Only valid on the PRIMARY cluster; setting it on a Read Replica " +
					"(ASYNC) cluster is an error. " +
					"Once set, dedicated mode cannot be toggled off after universe creation. " +
					"\n\n" +
					"Inheritance and ownership rules:\n" +
					"  - An empty block (dedicated_masters {}) runs masters on dedicated " +
					"nodes using the same instance_type and device_info as the TServer nodes. " +
					"All master configuration tracks user_intent automatically.\n" +
					"  - Once instance_type or device_info is explicitly set inside this " +
					"block, those fields become the sole source of truth for master " +
					"configuration. Subsequent changes to the TServer fields in user_intent " +
					"do NOT propagate to the master block automatically; the operator is " +
					"responsible for keeping them in sync.",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"instance_type": {
							Type:     schema.TypeString,
							Optional: true,
							Default:  "",
							Description: "Instance type for dedicated master nodes. " +
								"When omitted (empty string), falls back to " +
								"user_intent.instance_type and continues to track it on " +
								"every apply. Once set to a non-empty value this field is " +
								"owned by this block; changes to user_intent.instance_type " +
								"no longer affect the master instance type.",
						},
						"device_info": {
							Type:     schema.TypeList,
							MaxItems: 1,
							Optional: true,
							Description: "Disk and volume configuration for dedicated master nodes. " +
								"When this block is absent, all disk settings fall back to " +
								"user_intent.device_info and continue to track it automatically. " +
								"When this block is present, each field is inherited from " +
								"user_intent.device_info on the first apply when left unset, " +
								"but subsequent changes to user_intent.device_info fields do " +
								"NOT propagate automatically -- the operator must update them " +
								"here explicitly. To return to full automatic tracking, remove " +
								"this device_info block entirely.",
							Elem: masterDeviceInfoElemSchema(),
						},
					},
				},
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
