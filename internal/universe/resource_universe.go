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
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/hashicorp/go-cty/cty"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/customdiff"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	client "github.com/yugabyte/platform-go-client"
	"github.com/yugabyte/terraform-provider-yba/internal/api"
	"github.com/yugabyte/terraform-provider-yba/internal/provider/providerutil"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// ResourceUniverse creates and maintains resource for universes
func ResourceUniverse() *schema.Resource {
	return &schema.Resource{
		Description: "Universe Resource.",

		CreateContext: resourceUniverseCreate,
		ReadContext:   resourceUniverseRead,
		UpdateContext: resourceUniverseUpdate,
		DeleteContext: resourceUniverseDelete,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(60 * time.Minute),
			Update: schema.DefaultTimeout(60 * time.Minute),
			Delete: schema.DefaultTimeout(30 * time.Minute),
		},

		CustomizeDiff: resourceUniverseDiff(),
		Schema: map[string]*schema.Schema{
			// Universe Delete Options
			"delete_options": {
				Type:     schema.TypeList,
				Optional: true,
				MaxItems: 1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"delete_certs": {
							Type:     schema.TypeBool,
							Optional: true,
							Default:  false,
							Description: "Flag indicating whether the certificates should be " +
								"deleted with the universe. False by default.",
						},
						"delete_backups": {
							Type:     schema.TypeBool,
							Optional: true,
							Default:  false,
							Description: "Flag indicating whether the backups should be " +
								"deleted with the universe. False by default.",
						},
						"force_delete": {
							Type:        schema.TypeBool,
							Optional:    true,
							Default:     false,
							Description: "Force delete universe with errors. False by default.",
						},
					},
				},
			},
			// Universe Fields
			"root_ca": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
				DiffSuppressFunc: func(k, old, new string, d *schema.ResourceData) bool {
					// When TLS is enabled and this field is not set in the config, YBA creates
					// and assigns a root CA automatically. Suppress the diff so that the
					// auto-assigned UUID does not appear as a change on subsequent plans.
					// An explicit value in config always takes effect.
					return len(old) > 0 && new == ""
				},
				Description: "The UUID of the rootCA used for node-to-node TLS encryption." +
					" When not set, YBA creates and assigns a root CA automatically.",
			},
			"client_root_ca": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
				DiffSuppressFunc: func(k, old, new string, d *schema.ResourceData) bool {
					// When TLS is enabled and this field is not set in the config, YBA creates
					// and assigns a root CA automatically. Suppress the diff so that the
					// auto-assigned UUID does not appear as a change on subsequent plans.
					// An explicit value in config always takes effect.
					return len(old) > 0 && new == ""
				},
				Description: "The UUID of the clientRootCA to be used to generate client" +
					" certificates and facilitate TLS communication between server and client." +
					" When set to a different value than root_ca, separate certificates are used" +
					" for node-to-node and client-to-node TLS. When not set, root_ca is reused" +
					" for client-to-node TLS.",
			},
			"arch": {
				Type:     schema.TypeString,
				Optional: true,
				Default:  "x86_64",
				ValidateFunc: validation.StringInSlice(
					[]string{"x86_64", "aarch64"}, false),
				Description: "The architecture of the universe nodes." +
					" Allowed values are x86_64 and aarch64.",
			},
			"clusters": {
				Type:     schema.TypeList,
				Required: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"uuid": {
							Type:        schema.TypeString,
							Computed:    true,
							Description: "Cluster UUID.",
						},
						"cluster_type": {
							Type:     schema.TypeString,
							Required: true,
							ValidateDiagFunc: validation.ToDiagFunc(validation.StringInSlice(
								[]string{"PRIMARY", "ASYNC"}, false)),
							Description: "The type of cluster, primary or read replica (async)." +
								" Allowed values are PRIMARY or ASYNC.",
						},
						"user_intent": {
							Type:     schema.TypeList,
							MaxItems: 1,
							Required: true,
							Elem:     userIntentSchema(),
							Description: "Configuration values used in universe creation. Only " +
								"these values can be updated.",
						},
						"cloud_list": {
							Type:     schema.TypeList,
							Optional: true,
							Computed: true,
							Elem:     cloudListSchema(),
							Description: "Cloud, region, and zone placement information " +
								"for the universe.",
						},
					},
				},
			},
			"communication_ports": {
				Type:        schema.TypeList,
				Optional:    true,
				Computed:    true,
				MaxItems:    1,
				Description: "Communication ports.",
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
							Type:        schema.TypeInt,
							Optional:    true,
							Computed:    true,
							Description: "Redis (YEDIS) server HTTP port. Cannot be changed after universe creation.",
						},
						"redis_server_rpc_port": {
							Type:        schema.TypeInt,
							Optional:    true,
							Computed:    true,
							Description: "Redis (YEDIS) server RPC port. Cannot be changed after universe creation.",
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
							Type:        schema.TypeInt,
							Optional:    true,
							Computed:    true,
							Description: "YCQL server HTTP port. Cannot be changed after universe creation.",
						},
						"yql_server_rpc_port": {
							Type:        schema.TypeInt,
							Optional:    true,
							Computed:    true,
							Description: "YCQL server RPC port. Cannot be changed after universe creation.",
						},
						"ysql_server_http_port": {
							Type:        schema.TypeInt,
							Optional:    true,
							Computed:    true,
							Description: "YSQL server HTTP port. Cannot be changed after universe creation.",
						},
						"ysql_server_rpc_port": {
							Type:        schema.TypeInt,
							Optional:    true,
							Computed:    true,
							Description: "YSQL server RPC port. Cannot be changed after universe creation.",
						},
						"yb_controller_rpc_port": {
							Type:        schema.TypeInt,
							Optional:    true,
							Computed:    true,
							Description: "YB Controller RPC port. Cannot be changed after universe creation.",
						},
					},
				},
			},
			"node_details_set": {
				Type:     schema.TypeList,
				Computed: true,
				Elem:     nodeDetailsSetSchema(),
			},
			"db_version_upgrade_options": {
				Type:     schema.TypeList,
				Optional: true,
				MaxItems: 1,
				Description: "Options controlling the DB version upgrade path (UpgradeDBVersion). " +
					"By default finalize = false pauses the upgrade in PreFinalize state for a " +
					"monitoring phase; flip to true and re-apply to commit, or set " +
					"rollback = true to revert to the previous DB version.",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"finalize": {
							Type:     schema.TypeBool,
							Optional: true,
							Default:  false,
							Description: "Whether to finalize the DB version upgrade. When false " +
								"(default), the upgrade pauses at PreFinalize state for a monitoring " +
								"phase; set to true and re-apply to commit when ready. When true, " +
								"FinalizeUpgrade is called automatically after the upgrade task " +
								"completes.",
						},
						"rollback": {
							Type:     schema.TypeBool,
							Optional: true,
							Default:  false,
							Description: "Set to true to roll back a pending DB version upgrade " +
								"when db_version_upgrade_state is PreFinalize. Mutually exclusive " +
								"with finalize = true. After rollback the universe returns to Ready " +
								"state running the previous DB version. The provider automatically " +
								"resets this field to false in state after a successful rollback.",
						},
					},
				},
			},
			"node_restart_settings": {
				Type:     schema.TypeList,
				Optional: true,
				MaxItems: 1,
				Description: "Controls how node restarts are performed during upgrade operations " +
					"(DB version, GFlags, Systemd, Finalize, Rollback). When omitted, " +
					"YugabyteDB Anywhere platform defaults apply: Rolling strategy with " +
					"180000 ms (3 minutes) sleep after each master and TServer restart.",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"upgrade_option": {
							Type:     schema.TypeString,
							Optional: true,
							Default:  "Rolling",
							ValidateDiagFunc: validation.ToDiagFunc(
								validation.StringInSlice(
									[]string{"Rolling", "Non-Rolling", "Non-Restart"}, false)),
							Description: "Node restart strategy applied to all upgrade operations. " +
								"Allowed values: Rolling, Non-Rolling, Non-Restart. Defaults to " +
								"Rolling (YugabyteDB Anywhere platform default). TLS toggle always " +
								"uses Non-Rolling; ResizeNode and VMImageUpgrade always use Rolling, " +
								"regardless of this setting.",
						},
						"sleep_after_master_restart_millis": {
							Type:     schema.TypeInt,
							Optional: true,
							Default:  180000,
							Description: "Milliseconds to sleep after each master node restart. " +
								"Defaults to 180000 (3 minutes), matching the YugabyteDB Anywhere " +
								"platform default.",
						},
						"sleep_after_tserver_restart_millis": {
							Type:     schema.TypeInt,
							Optional: true,
							Default:  180000,
							Description: "Milliseconds to sleep after each TServer node restart. " +
								"Defaults to 180000 (3 minutes), matching the YugabyteDB Anywhere " +
								"platform default.",
						},
					},
				},
			},
			"db_version_upgrade_state": {
				Type:     schema.TypeString,
				Computed: true,
				Description: "Current DB version upgrade state reported by YugabyteDB Anywhere. " +
					"Possible values: Ready, Upgrading, UpgradeFailed, PreFinalize, Finalizing, " +
					"FinalizeFailed, RollingBack, RollbackFailed.",
			},
		},
	}
}

// accessKeyCodeUnknownInPlan returns true when the access_key_code for the
// given clusterType is unknown in the raw plan -- meaning its value comes from
// a data source that Terraform has deferred to after apply (e.g. because the
// provider resource it depends on is being modified in the same apply).
//
// An unknown raw-plan value (cty.UnknownVal) must be distinguished from an
// explicitly-null value (cty.NullVal), which is what Terraform produces when
// the user removes the field from their config entirely. Only the unknown case
// should suppress the plan-time validation; the null case is a real omission
// and must still be rejected.
func accessKeyCodeUnknownInPlan(d *schema.ResourceDiff, clusterType string) bool {
	rawPlan := d.GetRawPlan()
	if rawPlan == cty.NilVal || !rawPlan.IsKnown() || rawPlan.IsNull() {
		return false
	}
	clusters := rawPlan.GetAttr("clusters")
	if !clusters.IsKnown() || clusters.IsNull() {
		return false
	}
	for _, clusterVal := range clusters.AsValueSlice() {
		if !clusterVal.IsKnown() || clusterVal.IsNull() {
			continue
		}
		ct := clusterVal.GetAttr("cluster_type")
		if !ct.IsKnown() || ct.IsNull() || ct.AsString() != clusterType {
			continue
		}
		ui := clusterVal.GetAttr("user_intent")
		if !ui.IsKnown() || ui.IsNull() {
			return false
		}
		uiSlice := ui.AsValueSlice()
		if len(uiSlice) == 0 {
			return false
		}
		akc := uiSlice[0].GetAttr("access_key_code")
		return !akc.IsKnown()
	}
	return false
}

func getClusterByType(clusters []client.Cluster, clusterType string) (client.Cluster, bool) {

	for _, v := range clusters {
		if v.ClusterType == clusterType {
			return v, true
		}
	}
	return client.Cluster{}, false
}

func resourceUniverseDiff() schema.CustomizeDiffFunc {
	return customdiff.All(
		customdiff.ValidateChange(
			"clusters",
			func(ctx context.Context, old, new, m interface{}) error {
				// if not a new universe, prevent adding read replicas
				newClusterSet := buildClusters(new.([]interface{}))
				if len(old.([]interface{})) != 0 {
					oldClusterSet := buildClusters(old.([]interface{}))
					if len(oldClusterSet) < len(newClusterSet) {
						return errors.New("Cannot add Read Replica to existing universe")
					}
				}
				return nil
			},
		),
		customdiff.ValidateChange(
			"clusters",
			func(ctx context.Context, old, new, m interface{}) error {
				// if not a new universe, prevent systemD disablement
				newClusterSet := buildClusters(new.([]interface{}))
				if len(old.([]interface{})) != 0 {
					oldClusterSet := buildClusters(old.([]interface{}))
					oldPrimaryCluster, isPresent := getClusterByType(oldClusterSet, "PRIMARY")
					if isPresent {
						newPrimaryCluster, isNewPresent := getClusterByType(
							newClusterSet,
							"PRIMARY",
						)
						if isNewPresent {
							if oldPrimaryCluster.UserIntent.GetUseSystemd() == true &&
								newPrimaryCluster.UserIntent.GetUseSystemd() == false {
								return errors.New("Cannot disable Systemd")
							}
						}
					}
				}
				return nil
			},
		),
		customdiff.ValidateChange(
			"clusters",
			func(ctx context.Context, old, new, m interface{}) error {
				// if not a new universe, prevent decrease in volume size in primary
				newClusterSet := buildClusters(new.([]interface{}))
				if len(old.([]interface{})) != 0 {
					oldClusterSet := buildClusters(old.([]interface{}))
					oldPrimaryCluster, isPresent := getClusterByType(oldClusterSet, "PRIMARY")
					if isPresent {
						newPrimaryCluster, isNewPresent := getClusterByType(
							newClusterSet,
							"PRIMARY",
						)
						if isNewPresent {
							if oldPrimaryCluster.UserIntent.DeviceInfo.GetVolumeSize() >
								newPrimaryCluster.UserIntent.DeviceInfo.GetVolumeSize() {
								return errors.New("Cannot decrease Volume Size of nodes in " +
									"Primary Cluster")
							}
						}
					}
				}
				return nil
			},
		),
		customdiff.ValidateChange(
			"clusters",
			func(ctx context.Context, old, new, m interface{}) error {
				// if not a new universe, prevent change in number of nodes if instance type hasn't
				// change in Primary
				newClusterSet := buildClusters(new.([]interface{}))
				if len(old.([]interface{})) != 0 {
					oldClusterSet := buildClusters(old.([]interface{}))
					oldPrimaryCluster, isPresent := getClusterByType(oldClusterSet, "PRIMARY")
					if isPresent {
						newPrimaryCluster, isNewPresent := getClusterByType(
							newClusterSet,
							"PRIMARY",
						)
						if isNewPresent {
							if (oldPrimaryCluster.UserIntent.GetInstanceType() ==
								newPrimaryCluster.UserIntent.GetInstanceType()) &&
								(oldPrimaryCluster.UserIntent.DeviceInfo.GetNumVolumes() !=
									newPrimaryCluster.UserIntent.DeviceInfo.GetNumVolumes()) {
								return errors.New("Cannot change number of volumes per node " +
									"without change in instance type in Primary Cluster")
							}
						}
					}
				}
				return nil
			},
		),
		customdiff.ValidateChange(
			"clusters",
			func(ctx context.Context, old, new, m interface{}) error {
				// if not a new universe, prevent decrease in volume size in read replica
				newClusterSet := buildClusters(new.([]interface{}))
				if len(old.([]interface{})) != 0 {
					oldClusterSet := buildClusters(old.([]interface{}))
					oldPrimaryCluster, isPresent := getClusterByType(oldClusterSet, "ASYNC")
					if isPresent {
						newPrimaryCluster, isNewPresent := getClusterByType(newClusterSet, "ASYNC")
						if isNewPresent {
							if oldPrimaryCluster.UserIntent.DeviceInfo.GetVolumeSize() >
								newPrimaryCluster.UserIntent.DeviceInfo.GetVolumeSize() {
								return errors.New("Cannot decrease Volume Size of nodes in " +
									"Read Replica Cluster")
							}
						}
					}
				}
				return nil
			},
		),
		customdiff.ValidateChange(
			"clusters",
			func(ctx context.Context, old, new, m interface{}) error {
				// if not a new universe, prevent change in number of nodes if instance type hasn't
				// change in Read Replica
				newClusterSet := buildClusters(new.([]interface{}))
				if len(old.([]interface{})) != 0 {
					oldClusterSet := buildClusters(old.([]interface{}))
					oldPrimaryCluster, isPresent := getClusterByType(oldClusterSet, "ASYNC")
					if isPresent {
						newPrimaryCluster, isNewPresent := getClusterByType(newClusterSet, "ASYNC")
						if isNewPresent {
							if (oldPrimaryCluster.UserIntent.GetInstanceType() ==
								newPrimaryCluster.UserIntent.GetInstanceType()) &&
								((oldPrimaryCluster.UserIntent.DeviceInfo.GetNumVolumes() !=
									newPrimaryCluster.UserIntent.DeviceInfo.GetNumVolumes()) ||
									(oldPrimaryCluster.UserIntent.DeviceInfo.GetVolumeSize() !=
										newPrimaryCluster.UserIntent.DeviceInfo.GetVolumeSize())) {
								return errors.New(
									"Cannot change number of volumes or volume size " +
										"per node without change in instance type in Read Replica Cluster",
								)
							}
						}
					}
				}
				return nil
			},
		),
		customdiff.ValidateChange(
			"clusters",
			func(ctx context.Context, old, new, m interface{}) error {
				// check if universe name of the clusters are the same
				newClusterSet := buildClusters(new.([]interface{}))
				newPrimary, isPresent := getClusterByType(newClusterSet, "PRIMARY")
				newReadOnly, isRRPresnt := getClusterByType(newClusterSet, "ASYNC")
				if isPresent && isRRPresnt {
					if newPrimary.UserIntent.UniverseName == nil {
						return errors.New("Universe name cannot be empty")
					}
					if newReadOnly.UserIntent.UniverseName == nil {
						return errors.New("Universe name cannot be empty")
					}
					if newPrimary.UserIntent.GetUniverseName() !=
						newReadOnly.UserIntent.GetUniverseName() {
						return errors.New("Cannot have different universe names for Primary " +
							"and Read Only clusters")
					}
				}
				return nil
			},
		),
		customdiff.ValidateChange(
			"clusters",
			func(ctx context.Context, old, new, m interface{}) error {
				// check if software version of the clusters are the same
				newClusterSet := buildClusters(new.([]interface{}))
				newPrimary, isPresent := getClusterByType(newClusterSet, "PRIMARY")
				newReadOnly, isRRPresnt := getClusterByType(newClusterSet, "ASYNC")
				if len(old.([]interface{})) != 0 {
					if isPresent && isRRPresnt {
						if newPrimary.UserIntent.GetYbSoftwareVersion() !=
							newReadOnly.UserIntent.GetYbSoftwareVersion() {
							return errors.New(
								"Cannot have different software versions for Primary " +
									"and Read Only clusters",
							)
						}
					}
				}
				return nil
			},
		),
		customdiff.ValidateChange(
			"clusters",
			func(ctx context.Context, old, new, m interface{}) error {
				// check if systemD setting of the clusters are the same
				newClusterSet := buildClusters(new.([]interface{}))
				newPrimary, isPresent := getClusterByType(newClusterSet, "PRIMARY")
				newReadOnly, isRRPresnt := getClusterByType(newClusterSet, "ASYNC")
				if isPresent && isRRPresnt {
					if newPrimary.UserIntent.GetUseSystemd() !=
						newReadOnly.UserIntent.GetUseSystemd() {
						return errors.New("Cannot have different systemD settings for Primary " +
							"and Read Only clusters")
					}
				}
				return nil
			},
		),
		customdiff.ValidateChange(
			"clusters",
			func(ctx context.Context, old, new, m interface{}) error {
				// check if Gflags setting of the clusters are the same
				newClusterSet := buildClusters(new.([]interface{}))
				newPrimary, isPresent := getClusterByType(newClusterSet, "PRIMARY")
				newReadOnly, isRRPresnt := getClusterByType(newClusterSet, "ASYNC")
				if isPresent && isRRPresnt {
					if !reflect.DeepEqual(newPrimary.UserIntent.GetMasterGFlags(),
						newReadOnly.UserIntent.GetMasterGFlags()) ||
						!reflect.DeepEqual(newPrimary.UserIntent.GetTserverGFlags(),
							newReadOnly.UserIntent.GetTserverGFlags()) {
						return errors.New("Cannot have different Gflags settings for Primary " +
							"and Read Only clusters")
					}
				}
				return nil
			},
		),
		customdiff.ValidateChange(
			"clusters",
			func(ctx context.Context, old, new, m interface{}) error {
				// check if TLS setting of the clusters are the same
				newClusterSet := buildClusters(new.([]interface{}))
				newPrimary, isPresent := getClusterByType(newClusterSet, "PRIMARY")
				newReadOnly, isRRPresnt := getClusterByType(newClusterSet, "ASYNC")
				if isPresent && isRRPresnt {
					if newPrimary.UserIntent.GetEnableClientToNodeEncrypt() !=
						newReadOnly.UserIntent.GetEnableClientToNodeEncrypt() ||
						newPrimary.UserIntent.GetEnableNodeToNodeEncrypt() !=
							newReadOnly.UserIntent.GetEnableNodeToNodeEncrypt() {
						return errors.New("Cannot have different TLS settings for Primary " +
							"and Read Only clusters")
					}
				}
				return nil
			},
		),
		customdiff.ValidateChange(
			"clusters",
			func(ctx context.Context, old, new, m interface{}) error {
				// if not a new universe, prevent VM image upgrade on unsupported providers
				newClusterSet := buildClusters(new.([]interface{}))
				if len(old.([]interface{})) != 0 {
					oldClusterSet := buildClusters(old.([]interface{}))
					oldPrimaryCluster, isPresent := getClusterByType(oldClusterSet, "PRIMARY")
					if isPresent {
						newPrimaryCluster, isNewPresent := getClusterByType(
							newClusterSet,
							"PRIMARY",
						)
						if isNewPresent {
							if oldPrimaryCluster.UserIntent.GetImageBundleUUID() !=
								newPrimaryCluster.UserIntent.GetImageBundleUUID() {
								providerType := newPrimaryCluster.UserIntent.GetProviderType()
								if providerType != "aws" && providerType != "gcp" &&
									providerType != "azu" {
									return errors.New("VM Image upgrade is only supported " +
										"for aws, gcp, and azu providers")
								}
							}
						}
					}
				}
				return nil
			},
		),
		customdiff.ValidateChange(
			"clusters",
			func(ctx context.Context, old, new, m interface{}) error {
				// if not a new universe, prevent VM image upgrade on unsupported
				// providers for read replica
				newClusterSet := buildClusters(new.([]interface{}))
				if len(old.([]interface{})) != 0 {
					oldClusterSet := buildClusters(old.([]interface{}))
					oldRRCluster, isPresent := getClusterByType(oldClusterSet, "ASYNC")
					if isPresent {
						newRRCluster, isNewPresent := getClusterByType(
							newClusterSet,
							"ASYNC",
						)
						if isNewPresent {
							if oldRRCluster.UserIntent.GetImageBundleUUID() !=
								newRRCluster.UserIntent.GetImageBundleUUID() {
								providerType := newRRCluster.UserIntent.GetProviderType()
								if providerType != "aws" && providerType != "gcp" &&
									providerType != "azu" {
									return errors.New("VM Image upgrade is only supported " +
										"for aws, gcp, and azu providers in Read Replica Cluster")
								}
							}
						}
					}
				}
				return nil
			},
		),
		customdiff.ValidateValue("clusters", func(ctx context.Context, value,
			meta interface{}) error {
			// block adding instance tags to on prem nodes
			// mount path is required for on prem
			// storage type should not be given
			clusterSet := buildClusters(value.([]interface{}))
			primary, isPresent := getClusterByType(clusterSet, "PRIMARY")
			readOnly, isRRPresnt := getClusterByType(clusterSet, "ASYNC")
			if isPresent {
				primaryUI := primary.GetUserIntent()
				if primaryUI.GetProviderType() == "onprem" {
					err := errors.New("Error in onprem primary cluster definition: ")
					if len(primaryUI.GetInstanceTags()) > 0 {
						errMessage := "Cannot add instance tags to onprem primary cluster."
						err = fmt.Errorf("%w %s", err, errMessage)
					}
					if len(primaryUI.DeviceInfo.GetMountPoints()) == 0 {
						errMessage := "Mount points are compulsory for onprem clusters."
						err = fmt.Errorf("%w %s", err, errMessage)
					}
					if len(primaryUI.DeviceInfo.GetStorageType()) > 0 {
						errMessage := "Cannot specify storage type for onprem clusters."
						err = fmt.Errorf("%w %s", err, errMessage)
					}
					if err.Error() != "Error in onprem primary cluster definition: " {
						return err
					}
				}
			}
			if isRRPresnt {
				readUI := readOnly.GetUserIntent()
				if readUI.GetProviderType() == "onprem" {
					err := errors.New("Error in onprem read replica cluster definition: ")
					if len(readUI.GetInstanceTags()) > 0 {
						errMessage := "Cannot add instance tags to onprem read replica clusters."
						err = fmt.Errorf("%w %s", err, errMessage)
					}
					if len(readUI.DeviceInfo.GetMountPoints()) == 0 {
						errMessage := "Mount points are compulsory for onprem clusters."
						err = fmt.Errorf("%w %s", err, errMessage)
					}
					if len(readUI.DeviceInfo.GetStorageType()) > 0 {
						errMessage := "Cannot specify storage type for onprem clusters."
						err = fmt.Errorf("%w %s", err, errMessage)
					}
					if err.Error() != "Error in onprem read replica cluster definition: " {
						return err
					}
				}
			}
			return nil
		}),
		customdiff.ValidateValue("clusters", func(ctx context.Context, value,
			meta interface{}) error {
			// storage types that require disk_iops to be provisioned
			iopsRequired := map[string]bool{
				"IO1": true, "IO2": true, "GP3": true,
				"UltraSSD_LRS": true, "PremiumV2_LRS": true,
				"Hyperdisk_Balanced": true, "Hyperdisk_Extreme": true,
			}
			// storage types that also require throughput to be provisioned
			throughputRequired := map[string]bool{
				"GP3": true, "UltraSSD_LRS": true,
				"PremiumV2_LRS": true, "Hyperdisk_Balanced": true,
			}
			validateDeviceInfo := func(
				di *client.DeviceInfo,
				providerType string,
				clusterLabel string,
			) error {
				var errs []string
				storageType := di.GetStorageType()
				if len(di.GetMountPoints()) > 0 && providerType != "onprem" {
					errs = append(errs,
						"mount_points can only be specified for on-prem provider clusters")
				}
				if len(storageType) > 0 {
					if iopsRequired[storageType] && di.GetDiskIops() <= 0 {
						errs = append(errs,
							fmt.Sprintf("disk_iops is required for storage_type %s",
								storageType))
					}
					if throughputRequired[storageType] && di.GetThroughput() <= 0 {
						errs = append(errs,
							fmt.Sprintf("throughput is required for storage_type %s",
								storageType))
					}
				}
				if len(errs) > 0 {
					return fmt.Errorf("Error in %s cluster device_info: %s",
						clusterLabel, strings.Join(errs, "; "))
				}
				return nil
			}
			clusterSet := buildClusters(value.([]interface{}))
			primary, isPresent := getClusterByType(clusterSet, "PRIMARY")
			readOnly, isRRPresent := getClusterByType(clusterSet, "ASYNC")
			if isPresent {
				primaryUI := primary.GetUserIntent()
				if err := validateDeviceInfo(
					primaryUI.DeviceInfo,
					primaryUI.GetProviderType(),
					"PRIMARY",
				); err != nil {
					return err
				}
			}
			if isRRPresent {
				readUI := readOnly.GetUserIntent()
				if err := validateDeviceInfo(
					readUI.DeviceInfo,
					readUI.GetProviderType(),
					"READ REPLICA",
				); err != nil {
					return err
				}
			}
			return nil
		}),
		customdiff.ValidateChange(
			"db_version_upgrade_options",
			func(ctx context.Context, old, new, m interface{}) error {
				newOpts := new.([]interface{})
				if len(newOpts) == 0 {
					return nil
				}
				opt := newOpts[0].(map[string]interface{})
				if opt["rollback"].(bool) && opt["finalize"].(bool) {
					return errors.New(
						"rollback and finalize are mutually exclusive: " +
							"set finalize = false when using rollback = true, " +
							"and set rollback = false when using finalize = true")
				}
				return nil
			},
		),
		// When rollback is true, require that yb_software_version in the PRIMARY
		// cluster config matches the universe's previous DB version. This prevents a
		// spurious upgrade diff on the next plan after rollback (since after rollback the
		// state will reflect the previous version), and prevents accidental re-upgrade
		// if the user forgets to remove rollback = true from their config.
		func(ctx context.Context, d *schema.ResourceDiff, m interface{}) error {
			if d.Id() == "" {
				return nil
			}
			newOptsRaw := d.Get("db_version_upgrade_options").([]interface{})
			if len(newOptsRaw) == 0 || newOptsRaw[0] == nil {
				return nil
			}
			opt := newOptsRaw[0].(map[string]interface{})
			if !opt["rollback"].(bool) {
				return nil
			}
			c := m.(*api.APIClient).YugawareClient
			cUUID := m.(*api.APIClient).CustomerID
			uni, _, err := c.UniverseManagementAPI.GetUniverse(ctx, cUUID, d.Id()).Execute()
			if err != nil {
				// Do not block the plan if the universe cannot be fetched.
				return nil
			}
			prevCfg := uni.UniverseDetails.PrevYBSoftwareConfig
			if prevCfg == nil || prevCfg.SoftwareVersion == nil || *prevCfg.SoftwareVersion == "" {
				return fmt.Errorf(
					"rollback is true but the universe has no previous software " +
						"version to roll back to (prevYBSoftwareConfig is absent)")
			}
			prevVersion := prevCfg.GetSoftwareVersion()
			clustersRaw := d.Get("clusters").([]interface{})
			for _, clRaw := range clustersRaw {
				cl, ok := clRaw.(map[string]interface{})
				if !ok {
					continue
				}
				if cl["cluster_type"] != "PRIMARY" {
					continue
				}
				uiRaw, ok := cl["user_intent"].([]interface{})
				if !ok || len(uiRaw) == 0 {
					continue
				}
				ui, ok := uiRaw[0].(map[string]interface{})
				if !ok {
					continue
				}
				configVersion, _ := ui["yb_software_version"].(string)
				if configVersion != prevVersion {
					return fmt.Errorf(
						"when rollback is true, yb_software_version must be set "+
							"to the universe's previous DB version %q (currently %q); "+
							"update yb_software_version = %q in your configuration to "+
							"prevent a spurious upgrade diff after rollback",
						prevVersion, configVersion, prevVersion)
				}
			}
			return nil
		},
		customdiff.ValidateValue(
			"clusters",
			func(ctx context.Context, value, meta interface{}) error {
				clusterSet := buildClusters(value.([]interface{}))
				primary, isPresent := getClusterByType(clusterSet, "PRIMARY")
				if !isPresent {
					return nil
				}
				ui := primary.GetUserIntent()
				if !ui.GetUseSystemd() {
					return errors.New(
						"use_systemd must be true: non-systemd universes are not supported")
				}
				if ui.GetEnableYSQLAuth() && ui.GetYsqlPassword() == "" {
					return errors.New(
						"ysql_password is required when enable_ysql_auth is true")
				}
				if ui.GetEnableYCQLAuth() && ui.GetYcqlPassword() == "" {
					return errors.New(
						"ycql_password is required when enable_ycql_auth is true")
				}
				return nil
			},
		),
		func(ctx context.Context, d *schema.ResourceDiff, meta interface{}) error {
			// Validate per-cluster provider-dependent constraints. provider_type is
			// Computed and not known at plan time, so GetProvider is called once per
			// cluster to resolve the provider code and run all checks together:
			//   1. access_key_code is required for cloud providers (aws, gcp, azu).
			//   2. image_bundle_uuid is not applicable for on-prem providers.
			//   3. When image_bundle_uuid is omitted for a cloud provider, the
			//      provider must have a default image bundle for the configured arch
			//      so the YBA API auto-resolution will succeed.
			// API errors are silenced and deferred to the real create/update call
			// so plan does not fail on connectivity issues alone.
			c := meta.(*api.APIClient).YugawareClient
			cUUID := meta.(*api.APIClient).CustomerID
			arch := d.Get("arch").(string)
			cloudProviders := map[string]bool{"aws": true, "gcp": true, "azu": true}

			// checkCluster validates provider-dependent constraints for one cluster.
			// clusterType is "PRIMARY" or "ASYNC" and is needed to locate the
			// correct cluster in the raw plan for Check 1.
			checkCluster := func(cl client.Cluster, clusterType, label string) error {
				ui := cl.GetUserIntent()
				providerUUID := ui.GetProvider()
				if providerUUID == "" {
					return nil
				}
				p, err := providerutil.GetProvider(ctx, c, cUUID, providerUUID)
				if err != nil {
					return nil
				}
				code := p.GetCode()

				// Check 1: access_key_code required for cloud providers.
				//
				// When access_key_code is sourced from a data source (e.g.
				// data.yba_provider_key) and the upstream provider resource is
				// being modified in the same apply, Terraform marks the data
				// source output as "known after apply". ResourceDiff.Get returns
				// an empty string for such unknown values, which would cause a
				// false validation failure here.
				//
				// The raw plan distinguishes the two cases:
				//   - Unknown value (!IsKnown): data source deferred; skip and
				//     let the API enforce this constraint on apply.
				//   - Null/empty value: user explicitly removed the field; fail.
				if cloudProviders[code] && ui.GetAccessKeyCode() == "" {
					if !accessKeyCodeUnknownInPlan(d, clusterType) {
						return fmt.Errorf(
							"access_key_code is required for cloud providers "+
								"(aws, gcp, azu) in the %s cluster", label)
					}
				}

				// Check 2: image_bundle_uuid not applicable for on-prem.
				if code == "onprem" && ui.GetImageBundleUUID() != "" {
					return fmt.Errorf(
						"image_bundle_uuid is not applicable for on-prem providers "+
							"in the %s cluster", label)
				}

				// Check 3: cloud provider with no image_bundle_uuid must have a
				// default bundle for the configured arch so API auto-resolution works.
				if cloudProviders[code] && ui.GetImageBundleUUID() == "" {
					for _, b := range p.GetImageBundles() {
						if b.GetUseAsDefault() && b.Details.GetArch() == arch {
							return nil
						}
					}
					return fmt.Errorf(
						"image_bundle_uuid is not set for the %s cluster and provider %s "+
							"has no default image bundle for arch %q: set image_bundle_uuid "+
							"explicitly or mark a bundle as default for this architecture",
						label, providerUUID, arch)
				}

				return nil
			}

			clusterSet := buildClusters(d.Get("clusters").([]interface{}))
			if primary, ok := getClusterByType(clusterSet, "PRIMARY"); ok {
				if err := checkCluster(primary, "PRIMARY", "primary"); err != nil {
					return err
				}
			}
			if readOnly, ok := getClusterByType(clusterSet, "ASYNC"); ok {
				if err := checkCluster(readOnly, "ASYNC", "read replica"); err != nil {
					return err
				}
			}
			return nil
		},
		// --- PENDING UPDATE SUPPORT ---
		// The validators in this block prevent in-place changes to fields that
		// are present in the schema and have a corresponding YBA API update path
		// but for which the provider does not yet call that API.  Each validator
		// is self-contained so it can be deleted independently once the matching
		// update logic is wired into resourceUniverseUpdate.

		// YBA ToggleProtocol API: enable_ycql, enable_ysql, enable_yedis.
		// Remove this block when ToggleProtocol is implemented.
		customdiff.ValidateChange("clusters",
			func(ctx context.Context, old, new, m interface{}) error {
				if len(old.([]interface{})) == 0 {
					return nil
				}
				oldClusters := buildClusters(old.([]interface{}))
				newClusters := buildClusters(new.([]interface{}))
				for i, oldCl := range oldClusters {
					if i >= len(newClusters) {
						continue
					}
					newCl := newClusters[i]
					if oldCl.UserIntent.GetEnableYCQL() != newCl.UserIntent.GetEnableYCQL() {
						return errors.New(
							"enable_ycql cannot be changed after universe creation: " +
								"ToggleProtocol update support is not yet implemented in " +
								"this provider version")
					}
					if oldCl.UserIntent.GetEnableYSQL() != newCl.UserIntent.GetEnableYSQL() {
						return errors.New(
							"enable_ysql cannot be changed after universe creation: " +
								"ToggleProtocol update support is not yet implemented in " +
								"this provider version")
					}
					if oldCl.UserIntent.GetEnableYEDIS() != newCl.UserIntent.GetEnableYEDIS() {
						return errors.New(
							"enable_yedis cannot be changed after universe creation: " +
								"ToggleProtocol update support is not yet implemented in " +
								"this provider version")
					}
				}
				return nil
			},
		),

		// YBA YSQL/YCQL auth update APIs: enable_ysql_auth, enable_ycql_auth.
		// Remove this block when auth toggle is implemented.
		customdiff.ValidateChange("clusters",
			func(ctx context.Context, old, new, m interface{}) error {
				if len(old.([]interface{})) == 0 {
					return nil
				}
				oldClusters := buildClusters(old.([]interface{}))
				newClusters := buildClusters(new.([]interface{}))
				for i, oldCl := range oldClusters {
					if i >= len(newClusters) {
						continue
					}
					newCl := newClusters[i]
					if oldCl.UserIntent.GetEnableYSQLAuth() !=
						newCl.UserIntent.GetEnableYSQLAuth() {
						return errors.New(
							"enable_ysql_auth cannot be changed after universe creation: " +
								"auth toggle update support is not yet implemented in " +
								"this provider version")
					}
					if oldCl.UserIntent.GetEnableYCQLAuth() !=
						newCl.UserIntent.GetEnableYCQLAuth() {
						return errors.New(
							"enable_ycql_auth cannot be changed after universe creation: " +
								"auth toggle update support is not yet implemented in " +
								"this provider version")
					}
				}
				return nil
			},
		),

		// The YBA server only blocks PRIMARY RF changes (unless the enableRFChange
		// runtime flag is on). Read replica (ASYNC) RF changes are supported and
		// are handled by UpdateReadOnlyCluster. Block PRIMARY-only here.
		// Remove this block when PRIMARY RF change is supported by this provider.
		customdiff.ValidateChange("clusters",
			func(ctx context.Context, old, new, m interface{}) error {
				if len(old.([]interface{})) == 0 {
					return nil
				}
				oldClusters := buildClusters(old.([]interface{}))
				newClusters := buildClusters(new.([]interface{}))
				for i, oldCl := range oldClusters {
					if i >= len(newClusters) {
						continue
					}
					newCl := newClusters[i]
					if oldCl.ClusterType != "PRIMARY" {
						continue
					}
					if oldCl.UserIntent.GetReplicationFactor() !=
						newCl.UserIntent.GetReplicationFactor() {
						return errors.New(
							"replication_factor cannot be changed on the PRIMARY cluster " +
								"after universe creation: in-place RF change is not " +
								"supported by this provider version")
					}
				}
				return nil
			},
		),

		// YBA EncryptionAtRest API: enable_volume_encryption.
		// Remove this block when EncryptionAtRest toggle is implemented.
		customdiff.ValidateChange("clusters",
			func(ctx context.Context, old, new, m interface{}) error {
				if len(old.([]interface{})) == 0 {
					return nil
				}
				oldClusters := buildClusters(old.([]interface{}))
				newClusters := buildClusters(new.([]interface{}))
				for i, oldCl := range oldClusters {
					if i >= len(newClusters) {
						continue
					}
					newCl := newClusters[i]
					if oldCl.UserIntent.GetEnableVolumeEncryption() !=
						newCl.UserIntent.GetEnableVolumeEncryption() {
						return errors.New(
							"enable_volume_encryption cannot be changed after universe " +
								"creation: EncryptionAtRest update support is not yet " +
								"implemented in this provider version")
					}
				}
				return nil
			},
		),

		// YBA cert rotation API: root_ca, client_root_ca.
		// Remove this block when cert rotation is implemented.
		customdiff.ValidateChange("root_ca",
			func(ctx context.Context, old, new, m interface{}) error {
				oldVal := old.(string)
				newVal := new.(string)
				// Allow initial assignment (create) and allow clearing the field
				// (handled by DiffSuppressFunc).  Block only an explicit change
				// from one non-empty UUID to a different non-empty UUID.
				if oldVal != "" && newVal != "" && oldVal != newVal {
					return errors.New(
						"root_ca cannot be changed after universe creation: " +
							"cert rotation update support is not yet implemented in " +
							"this provider version")
				}
				return nil
			},
		),
		customdiff.ValidateChange("client_root_ca",
			func(ctx context.Context, old, new, m interface{}) error {
				oldVal := old.(string)
				newVal := new.(string)
				// Allow initial assignment (create) and allow clearing the field
				// (handled by DiffSuppressFunc).  Block only an explicit change
				// from one non-empty UUID to a different non-empty UUID.
				if oldVal != "" && newVal != "" && oldVal != newVal {
					return errors.New(
						"client_root_ca cannot be changed after universe creation: " +
							"cert rotation update support is not yet implemented in " +
							"this provider version")
				}
				return nil
			},
		),

		// provider UUID is immutable after universe creation; no migration API exists.
		// Remove this block when provider migration is supported.
		customdiff.ValidateChange("clusters",
			func(ctx context.Context, old, new, m interface{}) error {
				if len(old.([]interface{})) == 0 {
					return nil
				}
				oldClusters := buildClusters(old.([]interface{}))
				newClusters := buildClusters(new.([]interface{}))
				for i, oldCl := range oldClusters {
					if i >= len(newClusters) {
						continue
					}
					newCl := newClusters[i]
					if oldCl.UserIntent.GetProvider() != newCl.UserIntent.GetProvider() {
						return errors.New(
							"provider cannot be changed after universe creation: " +
								"provider migration is not supported by this provider version")
					}
				}
				return nil
			},
		),

		// assign_public_ip is a create-time networking setting with no update API.
		// Remove this block when assign_public_ip update is implemented.
		customdiff.ValidateChange("clusters",
			func(ctx context.Context, old, new, m interface{}) error {
				if len(old.([]interface{})) == 0 {
					return nil
				}
				oldClusters := buildClusters(old.([]interface{}))
				newClusters := buildClusters(new.([]interface{}))
				for i, oldCl := range oldClusters {
					if i >= len(newClusters) {
						continue
					}
					newCl := newClusters[i]
					if oldCl.UserIntent.GetAssignPublicIP() != newCl.UserIntent.GetAssignPublicIP() {
						return errors.New(
							"assign_public_ip cannot be changed after universe creation: " +
								"update support is not yet implemented in this provider version")
					}
				}
				return nil
			},
		),

		// assign_static_ip is a create-time networking setting with no update API.
		// Remove this block when assign_static_ip update is implemented.
		customdiff.ValidateChange("clusters",
			func(ctx context.Context, old, new, m interface{}) error {
				if len(old.([]interface{})) == 0 {
					return nil
				}
				oldClusters := buildClusters(old.([]interface{}))
				newClusters := buildClusters(new.([]interface{}))
				for i, oldCl := range oldClusters {
					if i >= len(newClusters) {
						continue
					}
					newCl := newClusters[i]
					if oldCl.UserIntent.GetAssignStaticPublicIP() !=
						newCl.UserIntent.GetAssignStaticPublicIP() {
						return errors.New(
							"assign_static_ip cannot be changed after universe creation: " +
								"update support is not yet implemented in this provider version")
					}
				}
				return nil
			},
		),

		// enable_ipv6 is a create-time networking setting; the YBA server rejects
		// IPv6 changes on VM universes.
		// Remove this block when enable_ipv6 update is implemented.
		customdiff.ValidateChange("clusters",
			func(ctx context.Context, old, new, m interface{}) error {
				if len(old.([]interface{})) == 0 {
					return nil
				}
				oldClusters := buildClusters(old.([]interface{}))
				newClusters := buildClusters(new.([]interface{}))
				for i, oldCl := range oldClusters {
					if i >= len(newClusters) {
						continue
					}
					newCl := newClusters[i]
					if oldCl.UserIntent.GetEnableIPV6() != newCl.UserIntent.GetEnableIPV6() {
						return errors.New(
							"enable_ipv6 cannot be changed after universe creation: " +
								"update support is not yet implemented in this provider version")
					}
				}
				return nil
			},
		),

		// use_host_name is deprecated in the YBA server and has no update API.
		// Remove this block when use_host_name update is implemented.
		customdiff.ValidateChange("clusters",
			func(ctx context.Context, old, new, m interface{}) error {
				if len(old.([]interface{})) == 0 {
					return nil
				}
				oldClusters := buildClusters(old.([]interface{}))
				newClusters := buildClusters(new.([]interface{}))
				for i, oldCl := range oldClusters {
					if i >= len(newClusters) {
						continue
					}
					newCl := newClusters[i]
					if oldCl.UserIntent.GetUseHostname() != newCl.UserIntent.GetUseHostname() {
						return errors.New(
							"use_host_name cannot be changed after universe creation: " +
								"this field is deprecated in the YBA server and has no " +
								"update path in this provider version")
					}
				}
				return nil
			},
		),

		// use_time_sync is a create-time node setting with no update API.
		// Remove this block when use_time_sync update is implemented.
		customdiff.ValidateChange("clusters",
			func(ctx context.Context, old, new, m interface{}) error {
				if len(old.([]interface{})) == 0 {
					return nil
				}
				oldClusters := buildClusters(old.([]interface{}))
				newClusters := buildClusters(new.([]interface{}))
				for i, oldCl := range oldClusters {
					if i >= len(newClusters) {
						continue
					}
					newCl := newClusters[i]
					if oldCl.UserIntent.GetUseTimeSync() != newCl.UserIntent.GetUseTimeSync() {
						return errors.New(
							"use_time_sync cannot be changed after universe creation: " +
								"update support is not yet implemented in this provider version")
					}
				}
				return nil
			},
		),

		// aws_arn_string changes require a full node replacement in AWS.
		// Remove this block when aws_arn_string update (or force-replace) is implemented.
		customdiff.ValidateChange("clusters",
			func(ctx context.Context, old, new, m interface{}) error {
				if len(old.([]interface{})) == 0 {
					return nil
				}
				oldClusters := buildClusters(old.([]interface{}))
				newClusters := buildClusters(new.([]interface{}))
				for i, oldCl := range oldClusters {
					if i >= len(newClusters) {
						continue
					}
					newCl := newClusters[i]
					if oldCl.UserIntent.GetAwsArnString() != newCl.UserIntent.GetAwsArnString() {
						return errors.New(
							"aws_arn_string cannot be changed after universe creation: " +
								"update support is not yet implemented in this provider version")
					}
				}
				return nil
			},
		),

		// YBA ConfigureYSQL/ConfigureYCQL APIs: ysql_password, ycql_password.
		// The provider does not call those APIs, so a password change in config
		// would be written to state without affecting the live universe.
		// Remove this block when password-change update is implemented.
		customdiff.ValidateChange("clusters",
			func(ctx context.Context, old, new, m interface{}) error {
				if len(old.([]interface{})) == 0 {
					return nil
				}
				oldClusters := buildClusters(old.([]interface{}))
				newClusters := buildClusters(new.([]interface{}))
				for i, oldCl := range oldClusters {
					if i >= len(newClusters) {
						continue
					}
					newCl := newClusters[i]
					oldYSQL := oldCl.UserIntent.GetYsqlPassword()
					newYSQL := newCl.UserIntent.GetYsqlPassword()
					if oldYSQL != "" && newYSQL != oldYSQL {
						return errors.New(
							"ysql_password cannot be changed after universe creation: " +
								"password update support is not yet implemented in " +
								"this provider version")
					}
					oldYCQL := oldCl.UserIntent.GetYcqlPassword()
					newYCQL := newCl.UserIntent.GetYcqlPassword()
					if oldYCQL != "" && newYCQL != oldYCQL {
						return errors.New(
							"ycql_password cannot be changed after universe creation: " +
								"password update support is not yet implemented in " +
								"this provider version")
					}
				}
				return nil
			},
		),
		// --- END PENDING UPDATE SUPPORT ---
	)
}

// resolveProviderTypes fills in provider_type in each cluster's user_intent
// by calling GetProvider for that cluster's provider UUID.
// Returns an error only if a required lookup actually fails; missing / empty
// provider UUIDs are skipped silently.
func resolveProviderTypes(
	ctx context.Context,
	c *client.APIClient,
	cUUID string,
	d *schema.ResourceData,
) error {
	clusters := d.Get("clusters").([]interface{})
	changed := false
	for _, clRaw := range clusters {
		cl, ok := clRaw.(map[string]interface{})
		if !ok {
			continue
		}
		uiRaw, ok := cl["user_intent"].([]interface{})
		if !ok || len(uiRaw) == 0 {
			continue
		}
		ui, ok := uiRaw[0].(map[string]interface{})
		if !ok {
			continue
		}
		providerUUID := ui["provider"].(string)
		if providerUUID == "" {
			continue
		}
		p, err := providerutil.GetProvider(ctx, c, cUUID, providerUUID)
		if err != nil {
			return err
		}
		ui["provider_type"] = p.GetCode()
		changed = true
	}
	if changed {
		if err := d.Set("clusters", clusters); err != nil {
			return fmt.Errorf("failed to set resolved provider_type: %w", err)
		}
	}
	return nil
}

func resourceUniverseCreate(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{}) diag.Diagnostics {
	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID

	if err := resolveProviderTypes(ctx, c, cUUID, d); err != nil {
		return diag.FromErr(err)
	}
	req := buildUniverse(d)
	r, response, err := c.UniverseClusterMutationsAPI.CreateAllClusters(ctx, cUUID).
		UniverseConfigureTaskParams(req).Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"Universe", "Create")
		return diag.FromErr(errMessage)
	}
	d.SetId(*r.ResourceUUID)
	tflog.Debug(ctx, fmt.Sprintf("Waiting for universe %s to be active", d.Id()))
	if err = utils.WaitForTask(ctx, r.GetTaskUUID(), cUUID, c,
		d.Timeout(schema.TimeoutCreate)); err != nil {
		return diag.FromErr(err)
	}
	return resourceUniverseRead(ctx, d, meta)
}

func resourceUniverseRead(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID

	r, response, err := c.UniverseManagementAPI.GetUniverse(ctx, cUUID, d.Id()).Execute()
	if err != nil {
		// If the universe was deleted outside of Terraform, remove it from state
		// so that Terraform can recreate it on the next apply.
		// YBA returns 400 Bad Request with "Cannot find" for deleted universes.
		if utils.IsHTTPNotFound(response) || utils.IsHTTPBadRequestNotFound(response) {
			tflog.Warn(
				ctx,
				fmt.Sprintf("Universe %s not found, removing from state: %v", d.Id(), err),
			)
			d.SetId("")
			return diags
		}
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"Universe", "Read")
		return diag.FromErr(errMessage)
	}

	u := r.UniverseDetails
	if err = d.Set("root_ca", u.RootCA); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("client_root_ca", u.ClientRootCA); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("arch", u.GetArch()); err != nil {
		return diag.FromErr(err)
	}
	newClusters := flattenClusters(u.Clusters)
	restoreRedactedPasswords(ctx, newClusters, d.Get("clusters").([]interface{}))
	if err = d.Set("clusters", newClusters); err != nil {
		return diag.FromErr(err)
	}
	err = d.Set("communication_ports", flattenCommunicationPorts(u.CommunicationPorts))
	if err != nil {
		return diag.FromErr(err)
	}
	err = d.Set("node_details_set", flattenNodeDetailsSet(u.GetNodeDetailsSet()))
	if err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("db_version_upgrade_state", u.GetSoftwareUpgradeState()); err != nil {
		return diag.FromErr(err)
	}
	return diags
}

// restrictedCommPorts lists the ports that cannot be changed after universe creation.
// YSQL, YCQL, and YEDIS ports require dedicated universe actions to change; the YB
// Controller port is set at creation time only.
var restrictedCommPorts = []string{
	"yql_server_http_port",
	"yql_server_rpc_port",
	"ysql_server_http_port",
	"ysql_server_rpc_port",
	"redis_server_http_port",
	"redis_server_rpc_port",
	"yb_controller_rpc_port",
}

// validateCommPortsNotRestricted returns an error if any port that cannot be changed
// after universe creation differs between old and new state.
func validateCommPortsNotRestricted(d *schema.ResourceData) error {
	if !d.HasChange("communication_ports") {
		return nil
	}
	oldRaw, newRaw := d.GetChange("communication_ports")
	oldList, ok1 := oldRaw.([]interface{})
	newList, ok2 := newRaw.([]interface{})
	if !ok1 || !ok2 || len(oldList) == 0 || len(newList) == 0 {
		return nil
	}
	oldCP, ok1 := oldList[0].(map[string]interface{})
	newCP, ok2 := newList[0].(map[string]interface{})
	if !ok1 || !ok2 {
		return nil
	}
	var changed []string
	for _, port := range restrictedCommPorts {
		if oldCP[port] != newCP[port] {
			changed = append(changed, port)
		}
	}
	if len(changed) > 0 {
		return fmt.Errorf(
			"the following communication ports cannot be changed after universe creation: %s. "+
				"Remove these fields from your configuration or reset them to their current values.",
			strings.Join(changed, ", "),
		)
	}
	return nil
}

func editUniverseParameters(ctx context.Context, oldUserIntent client.UserIntent,
	newUserIntent client.UserIntent) (bool, client.UserIntent) {
	if !reflect.DeepEqual(oldUserIntent.GetInstanceTags(), newUserIntent.GetInstanceTags()) ||
		!reflect.DeepEqual(oldUserIntent.GetRegionList(), newUserIntent.GetRegionList()) ||
		oldUserIntent.GetNumNodes() != newUserIntent.GetNumNodes() ||
		oldUserIntent.GetReplicationFactor() != newUserIntent.GetReplicationFactor() ||
		oldUserIntent.GetInstanceType() != newUserIntent.GetInstanceType() ||
		oldUserIntent.DeviceInfo.GetNumVolumes() != newUserIntent.DeviceInfo.GetNumVolumes() ||
		oldUserIntent.DeviceInfo.GetVolumeSize() != newUserIntent.DeviceInfo.GetVolumeSize() {
		editNumVolume := true
		editVolumeSize := true // this is only for RR cluster, primary cluster resize is handled
		// by resize node task
		numVolumes := oldUserIntent.DeviceInfo.GetNumVolumes()
		volumeSize := oldUserIntent.DeviceInfo.GetVolumeSize()
		if (oldUserIntent.DeviceInfo.GetNumVolumes() !=
			newUserIntent.DeviceInfo.GetNumVolumes()) &&
			(oldUserIntent.GetInstanceType() == newUserIntent.GetInstanceType()) {
			tflog.Error(ctx, "Cannot edit Number of Volumes per instance without an edit to"+
				" Instance Type, Ignoring Change")
			editNumVolume = false
		}
		if (oldUserIntent.DeviceInfo.GetVolumeSize() !=
			newUserIntent.DeviceInfo.GetVolumeSize()) &&
			(oldUserIntent.GetInstanceType() == newUserIntent.GetInstanceType()) {
			tflog.Error(ctx, "Cannot edit Volume size per instance without an edit to Instance "+
				"Type, Ignoring Change for ReadOnly Cluster")
			tflog.Info(ctx, "Above error is not for Primary Cluster. Node resize applied through"+
				"a separate task")
			editVolumeSize = false
		} else if oldUserIntent.DeviceInfo.GetVolumeSize() > newUserIntent.DeviceInfo.GetVolumeSize() {
			tflog.Error(ctx, "Cannot decrease volume size per instance, Ignoring Change")
			editVolumeSize = false
		}
		oldUserIntent = newUserIntent
		if !editNumVolume {
			oldUserIntent.DeviceInfo.NumVolumes = &numVolumes
		}
		if !editVolumeSize {
			oldUserIntent.DeviceInfo.VolumeSize = &volumeSize
		}
		return true, oldUserIntent
	}
	return false, oldUserIntent

}

func runFinalizeUpgrade(
	ctx context.Context,
	c *client.APIClient,
	cUUID string,
	uniUUID string,
	clusters []client.Cluster,
	upgradeOption string,
	sleepAfterMasterMs int32,
	sleepAfterTServerMs int32,
	timeout time.Duration,
) diag.Diagnostics {
	finalizeReq := client.FinalizeUpgradeParams{
		Clusters:                       clusters,
		UpgradeOption:                  upgradeOption,
		UpgradeSystemCatalog:           true,
		SleepAfterMasterRestartMillis:  sleepAfterMasterMs,
		SleepAfterTServerRestartMillis: sleepAfterTServerMs,
	}
	return utils.DispatchAndWait(ctx, "Finalize Upgrade", cUUID, c, timeout,
		utils.ResourceEntity, "Universe", "Update - Finalize Upgrade",
		func() (string, *http.Response, error) {
			r, resp, err := c.UniverseUpgradesManagementAPI.FinalizeUpgrade(
				ctx, cUUID, uniUUID).FinalizeUpgradeParams(finalizeReq).Execute()
			if err != nil {
				return "", resp, err
			}
			return r.GetTaskUUID(), resp, nil
		},
	)
}

func resourceUniverseUpdate(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{}) (diags diag.Diagnostics) {
	// Only updates user intent for each cluster
	// cloud Info can have changes in zones
	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID

	defer func() {
		diags = append(resourceUniverseRead(ctx, d, meta), diags...)
	}()

	// Reject any attempt to change ports that are immutable after universe creation.
	if err := validateCommPortsNotRestricted(d); err != nil {
		return diag.FromErr(err)
	}

	// Read node_restart_settings once with explicit fallbacks. When the block is absent,
	// d.Get returns zero values ("" / 0) rather than the schema defaults, so we apply the
	// YBA platform defaults here: Rolling strategy, 180000 ms sleep (3 minutes).
	upgradeOption := d.Get("node_restart_settings.0.upgrade_option").(string)
	if upgradeOption == "" {
		upgradeOption = "Rolling"
	}
	sleepAfterMasterMs := int32(
		d.Get("node_restart_settings.0.sleep_after_master_restart_millis").(int),
	)
	if sleepAfterMasterMs == 0 {
		sleepAfterMasterMs = 180000
	}
	sleepAfterTServerMs := int32(
		d.Get("node_restart_settings.0.sleep_after_tserver_restart_millis").(int),
	)
	if sleepAfterTServerMs == 0 {
		sleepAfterTServerMs = 180000
	}

	// Rollback is a universe-level operation (not per-cluster): the YBA handler reads
	// prevYBSoftwareConfig from universe-wide details to determine the version to revert to,
	// and rolls back all clusters simultaneously. It must run before the cluster-change loop
	// so that a rollback + other cluster edits in the same apply are both processed.
	if d.HasChange("db_version_upgrade_options") &&
		d.Get("db_version_upgrade_options.0.rollback").(bool) {
		currentUni, response, err := c.UniverseManagementAPI.GetUniverse(ctx, cUUID, d.Id()).
			Execute()
		if err != nil {
			errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
				"Universe", "Update - Fetch for rollback")
			return diag.FromErr(errMessage)
		}
		upgradeState := currentUni.UniverseDetails.GetSoftwareUpgradeState()
		if upgradeState != "PreFinalize" {
			tflog.Warn(ctx, fmt.Sprintf(
				"rollback is true but universe db_version_upgrade_state is %q "+
					"(not PreFinalize); skipping rollback. Reset rollback = false "+
					"in your configuration.", upgradeState))
		} else {
			rollbackReq := client.RollbackUpgradeParams{
				Clusters:                       currentUni.UniverseDetails.Clusters,
				UpgradeOption:                  upgradeOption,
				SleepAfterMasterRestartMillis:  sleepAfterMasterMs,
				SleepAfterTServerRestartMillis: sleepAfterTServerMs,
			}
			if diags := utils.DispatchAndWait(ctx, "Rollback Upgrade", cUUID, c,
				d.Timeout(schema.TimeoutUpdate),
				utils.ResourceEntity, "Universe", "Update - Rollback Upgrade",
				func() (string, *http.Response, error) {
					r, resp, err := c.UniverseUpgradesManagementAPI.RollbackUpgrade(
						ctx, cUUID, d.Id()).RollbackUpgradeParams(rollbackReq).Execute()
					if err != nil {
						return "", resp, err
					}
					return r.GetTaskUUID(), resp, nil
				},
			); diags != nil {
				return diags
			}
			// Reset rollback to false in state after a successful rollback.
			// This intentionally creates a plan diff (state=false vs config=true) on the
			// next run, which signals to the user that they must set rollback = false
			// in their configuration to reach a steady state. Without this reset, state
			// would stay true and no diff would appear, leaving a stale value in state that
			// silently re-triggers the rollback logic on every apply until the user changes
			// their config anyway.
			if opts, ok := d.GetOk("db_version_upgrade_options"); ok {
				optList := opts.([]interface{})
				if len(optList) > 0 && optList[0] != nil {
					opt := optList[0].(map[string]interface{})
					opt["rollback"] = false
					if err := d.Set("db_version_upgrade_options", optList); err != nil {
						return diag.FromErr(err)
					}
				}
			}
		}
	}

	// Explicit finalize after a monitoring phase: triggered when finalize flips from
	// false to true while the universe is already in PreFinalize state. This lets the user
	// commit the upgrade simply by setting finalize = true and re-applying.
	if d.HasChange("db_version_upgrade_options") &&
		d.Get("db_version_upgrade_options.0.finalize").(bool) {
		oldOpts, _ := d.GetChange("db_version_upgrade_options")
		oldAutoFinalize := false
		if opts := oldOpts.([]interface{}); len(opts) > 0 && opts[0] != nil {
			oldAutoFinalize = opts[0].(map[string]interface{})["finalize"].(bool)
		}
		if !oldAutoFinalize {
			currentUni, response, err := c.UniverseManagementAPI.GetUniverse(ctx, cUUID, d.Id()).
				Execute()
			if err != nil {
				errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
					"Universe", "Update - Fetch for finalize")
				return diag.FromErr(errMessage)
			}
			if currentUni.UniverseDetails.GetSoftwareUpgradeState() == "PreFinalize" {
				if diags := runFinalizeUpgrade(ctx, c, cUUID, d.Id(),
					currentUni.UniverseDetails.Clusters,
					upgradeOption,
					sleepAfterMasterMs, sleepAfterTServerMs,
					d.Timeout(schema.TimeoutUpdate)); diags != nil {
					return diags
				}
			}
		}
	}

	if d.HasChange("clusters") {
		clusters := d.Get("clusters").([]interface{})
		updateUni, response, err := c.UniverseManagementAPI.GetUniverse(ctx, cUUID, d.Id()).
			Execute()
		if err != nil {
			errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
				"Universe", "Update - Fetch universe")
			return diag.FromErr(errMessage)
		}
		newUni := buildUniverse(d)

		// Detect image bundle changes and scale direction across all clusters
		var imageBundleUpgrades []client.ImageBundleUpgradeInfo
		hasScaleOut := false
		for j, cl := range updateUni.UniverseDetails.Clusters {
			if j >= len(newUni.Clusters) {
				continue
			}
			oldIB := cl.UserIntent.GetImageBundleUUID()
			newIB := newUni.Clusters[j].UserIntent.GetImageBundleUUID()
			// Skip when newIB is empty: the user omitted image_bundle_uuid in config
			// (DiffSuppressFunc handles the no-diff case in state). Dispatching a
			// VMImageUpgrade with an empty bundle UUID would fail or corrupt the universe.
			if newIB != "" && oldIB != newIB {
				imageBundleUpgrades = append(imageBundleUpgrades,
					*client.NewImageBundleUpgradeInfo(cl.GetUuid(), newIB))
			}
			if newUni.Clusters[j].UserIntent.GetNumNodes() >
				cl.UserIntent.GetNumNodes() {
				hasScaleOut = true
			}
		}

		// VM Image Upgrade BEFORE cluster operations if scaling out.
		// New nodes will be provisioned with the new image directly.
		if len(imageBundleUpgrades) > 0 && hasScaleOut {
			if diagErr := performVMImageUpgrade(
				ctx, c, cUUID, d, updateUni, imageBundleUpgrades,
				sleepAfterMasterMs, sleepAfterTServerMs,
			); diagErr != nil {
				return diagErr
			}
			imageBundleUpgrades = nil

			updateUni, response, err = c.UniverseManagementAPI.GetUniverse(
				ctx, cUUID, d.Id()).Execute()
			if err != nil {
				errMessage := utils.ErrorFromHTTPResponse(
					response, err, utils.ResourceEntity,
					"Universe", "Update - Fetch universe")
				return diag.FromErr(errMessage)
			}
			newUni = buildUniverse(d)
		}

		if len(clusters) > 2 {
			tflog.Error(ctx, "Cannot have more than 1 Read only cluster")
		} else {
			if len(updateUni.UniverseDetails.Clusters) < len(clusters) {
				tflog.Error(ctx, "Currently not supporting adding Read Replicas after universe creation")
			} else if len(updateUni.UniverseDetails.Clusters) > len(clusters) {
				var clusterUUID string
				for _, v := range updateUni.UniverseDetails.Clusters {
					if v.ClusterType == "ASYNC" {
						clusterUUID = *v.Uuid
					}
				}

				if diags := utils.DispatchAndWait(ctx, "Delete Read Replica Cluster", cUUID, c,
					d.Timeout(schema.TimeoutUpdate),
					utils.ResourceEntity, "Universe", "Update - Delete Read Replica cluster",
					func() (string, *http.Response, error) {
						r, resp, err := c.UniverseClusterMutationsAPI.DeleteReadonlyCluster(
							ctx, cUUID, d.Id(), clusterUUID).IsForceDelete(
							d.Get("delete_options.0.force_delete").(bool)).Execute()
						if err != nil {
							return "", resp, err
						}
						return r.GetTaskUUID(), resp, nil
					},
				); diags != nil {
					return diags
				}
			}
		}
		for i, v := range clusters {
			if !d.HasChange(fmt.Sprintf("clusters.%d", i)) {
				continue
			}
			cluster := v.(map[string]interface{})

			oldUserIntent := updateUni.UniverseDetails.Clusters[i].UserIntent
			newUserIntent := newUni.Clusters[i].UserIntent
			if cluster["cluster_type"] == "PRIMARY" {

				// Software Upgrade
				if oldUserIntent.GetYbSoftwareVersion() != newUserIntent.GetYbSoftwareVersion() {
					updateUni.UniverseDetails.Clusters[i].UserIntent = newUserIntent

					finalize := d.Get("db_version_upgrade_options.0.finalize").(bool)

					req := client.SoftwareUpgradeParams{
						YbSoftwareVersion:              newUserIntent.GetYbSoftwareVersion(),
						Clusters:                       updateUni.UniverseDetails.Clusters,
						UpgradeOption:                  upgradeOption,
						UpgradeSystemCatalog:           true,
						SleepAfterMasterRestartMillis:  sleepAfterMasterMs,
						SleepAfterTServerRestartMillis: sleepAfterTServerMs,
					}

					if diags := utils.DispatchAndWait(ctx, "DB Version Upgrade", cUUID, c,
						d.Timeout(schema.TimeoutUpdate),
						utils.ResourceEntity, "Universe", "Update - DB Version Upgrade",
						func() (string, *http.Response, error) {
							r, resp, err := c.UniverseUpgradesManagementAPI.UpgradeDBVersion(
								ctx, cUUID, d.Id()).SoftwareUpgradeParams(req).Execute()
							if err != nil {
								return "", resp, err
							}
							return r.GetTaskUUID(), resp, nil
						},
					); diags != nil {
						return diags
					}

					// Finalize after upgrade if configured
					if finalize {
						updateUni, response, err = c.UniverseManagementAPI.GetUniverse(
							ctx, cUUID, d.Id()).Execute()
						if err != nil {
							errMessage := utils.ErrorFromHTTPResponse(
								response, err, utils.ResourceEntity,
								"Universe", "Update - Fetch post-upgrade state",
							)
							return diag.FromErr(errMessage)
						}
						upgradeState := updateUni.UniverseDetails.GetSoftwareUpgradeState()
						if upgradeState == "PreFinalize" {
							tflog.Info(ctx, "Universe is in PreFinalize state, finalizing upgrade")
							if diags := runFinalizeUpgrade(ctx, c, cUUID, d.Id(),
								updateUni.UniverseDetails.Clusters,
								upgradeOption,
								sleepAfterMasterMs, sleepAfterTServerMs,
								d.Timeout(schema.TimeoutUpdate)); diags != nil {
								return diags
							}
						} else {
							tflog.Info(ctx, fmt.Sprintf(
								"Universe db_version_upgrade_state is %q, skipping finalize",
								upgradeState))
						}
					}
				}

				updateUni, response, err = c.UniverseManagementAPI.GetUniverse(ctx, cUUID, d.Id()).
					Execute()
				if err != nil {
					errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
						"Universe", "Update - Fetch universe")
					return diag.FromErr(errMessage)
				}
				oldUserIntent = updateUni.UniverseDetails.Clusters[i].UserIntent

				//GFlag Update
				if !reflect.DeepEqual(oldUserIntent.GetMasterGFlags(),
					newUserIntent.GetMasterGFlags()) ||
					!reflect.DeepEqual(oldUserIntent.GetTserverGFlags(),
						newUserIntent.GetTserverGFlags()) {
					updateUni.UniverseDetails.Clusters[i].UserIntent = newUserIntent
					req := client.GFlagsUpgradeParams{
						MasterGFlags:  newUserIntent.GetMasterGFlags(),
						TserverGFlags: newUserIntent.GetTserverGFlags(),
						Clusters:      updateUni.UniverseDetails.Clusters,
						UpgradeOption: upgradeOption,
						SleepAfterMasterRestartMillis: int32(
							sleepAfterMasterMs,
						),
						SleepAfterTServerRestartMillis: int32(
							sleepAfterTServerMs,
						),
					}
					if diags := utils.DispatchAndWait(ctx, "GFlags Upgrade", cUUID, c,
						d.Timeout(schema.TimeoutUpdate),
						utils.ResourceEntity, "Universe", "Update - GFlags",
						func() (string, *http.Response, error) {
							r, resp, err := c.UniverseUpgradesManagementAPI.UpgradeGFlags(
								ctx, cUUID, d.Id()).GflagsUpgradeParams(req).Execute()
							if err != nil {
								return "", resp, err
							}
							return r.GetTaskUUID(), resp, nil
						},
					); diags != nil {
						return diags
					}
				}

				updateUni, response, err = c.UniverseManagementAPI.GetUniverse(ctx, cUUID,
					d.Id()).Execute()
				if err != nil {
					errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
						"Universe", "Update - Fetch universe")
					return diag.FromErr(errMessage)
				}
				oldUserIntent = updateUni.UniverseDetails.Clusters[i].UserIntent

				//TLS Toggle
				if (oldUserIntent.GetEnableClientToNodeEncrypt() !=
					newUserIntent.GetEnableClientToNodeEncrypt()) ||
					(oldUserIntent.GetEnableNodeToNodeEncrypt() !=
						newUserIntent.GetEnableNodeToNodeEncrypt()) {
					if newUserIntent.EnableClientToNodeEncrypt != nil {
						updateUni.UniverseDetails.Clusters[i].UserIntent.EnableClientToNodeEncrypt =
							newUserIntent.EnableClientToNodeEncrypt
					}
					if newUserIntent.EnableNodeToNodeEncrypt != nil {
						updateUni.UniverseDetails.Clusters[i].UserIntent.EnableNodeToNodeEncrypt =
							newUserIntent.EnableNodeToNodeEncrypt
					}
					//updateUni.UniverseDetails.Clusters[i].UserIntent = newUserIntent
					req := client.TlsToggleParams{
						EnableClientToNodeEncrypt: newUserIntent.GetEnableClientToNodeEncrypt(),
						EnableNodeToNodeEncrypt:   newUserIntent.GetEnableNodeToNodeEncrypt(),
						Clusters:                  updateUni.UniverseDetails.Clusters,
						UpgradeOption:             "Non-Rolling",
						SleepAfterMasterRestartMillis: int32(
							sleepAfterMasterMs,
						),
						SleepAfterTServerRestartMillis: int32(
							sleepAfterTServerMs,
						),
					}
					if diags := utils.DispatchAndWait(ctx, "TLS Toggle", cUUID, c,
						d.Timeout(schema.TimeoutUpdate),
						utils.ResourceEntity, "Universe", "Update - TLS Toggle",
						func() (string, *http.Response, error) {
							r, resp, err := c.UniverseUpgradesManagementAPI.UpgradeTls(
								ctx, cUUID, d.Id()).TlsToggleParams(req).Execute()
							if err != nil {
								return "", resp, err
							}
							return r.GetTaskUUID(), resp, nil
						},
					); diags != nil {
						return diags
					}
				}

				updateUni, response, err = c.UniverseManagementAPI.GetUniverse(ctx, cUUID, d.Id()).
					Execute()
				if err != nil {
					errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
						"Universe", "Update - Fetch universe")
					return diag.FromErr(errMessage)
				}
				oldUserIntent = updateUni.UniverseDetails.Clusters[i].UserIntent

				//Systemd upgrade
				if oldUserIntent.GetUseSystemd() == false &&
					newUserIntent.GetUseSystemd() == true {
					updateUni.UniverseDetails.Clusters[i].UserIntent = newUserIntent
					req := client.SystemdUpgradeParams{
						Clusters:      updateUni.UniverseDetails.Clusters,
						UpgradeOption: upgradeOption,
						SleepAfterMasterRestartMillis: int32(
							sleepAfterMasterMs,
						),
						SleepAfterTServerRestartMillis: int32(
							sleepAfterTServerMs,
						),
					}
					if diags := utils.DispatchAndWait(ctx, "Systemd Upgrade", cUUID, c,
						d.Timeout(schema.TimeoutUpdate),
						utils.ResourceEntity, "Universe", "Update - Systemd",
						func() (string, *http.Response, error) {
							r, resp, err := c.UniverseUpgradesManagementAPI.UpgradeSystemd(
								ctx, cUUID, d.Id()).SystemdUpgradeParams(req).Execute()
							if err != nil {
								return "", resp, err
							}
							return r.GetTaskUUID(), resp, nil
						},
					); diags != nil {
						return diags
					}
				} else if oldUserIntent.GetUseSystemd() == true &&
					newUserIntent.GetUseSystemd() == false {
					tflog.Error(ctx, "Cannot disable Systemd")
				}

				updateUni, response, err = c.UniverseManagementAPI.GetUniverse(ctx, cUUID, d.Id()).
					Execute()
				if err != nil {
					errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
						"Universe", "Update - Fetch universe")
					return diag.FromErr(errMessage)
				}
				oldUserIntent = updateUni.UniverseDetails.Clusters[i].UserIntent

				// Resize Nodes
				// Call separate task only when instance type is same, else will be handled in
				// UpdatePrimaryCluster
				if (oldUserIntent.GetInstanceType() == newUserIntent.GetInstanceType()) &&
					(oldUserIntent.DeviceInfo.GetVolumeSize() !=
						newUserIntent.DeviceInfo.GetVolumeSize()) {
					if oldUserIntent.DeviceInfo.GetVolumeSize() <
						newUserIntent.DeviceInfo.GetVolumeSize() {
						// Only volume size should be changed to do smart resize,
						// other changes handled in UpgradeCluster
						newVolSize := newUserIntent.DeviceInfo.VolumeSize
						updateUni.UniverseDetails.Clusters[i].UserIntent.DeviceInfo.VolumeSize = newVolSize
						req := client.ResizeNodeParams{
							UpgradeOption: "Rolling",
							Clusters:      updateUni.UniverseDetails.Clusters,
							NodeDetailsSet: buildNodeDetailsRespArrayToNodeDetailsArray(
								updateUni.UniverseDetails.NodeDetailsSet,
							),
							SleepAfterMasterRestartMillis: int32(
								sleepAfterMasterMs,
							),
							SleepAfterTServerRestartMillis: int32(
								sleepAfterTServerMs,
							),
						}
						if diags := utils.DispatchAndWait(ctx, "Resize Nodes", cUUID, c,
							d.Timeout(schema.TimeoutUpdate),
							utils.ResourceEntity, "Universe", "Update - Resize Nodes",
							func() (string, *http.Response, error) {
								r, resp, err := c.UniverseUpgradesManagementAPI.ResizeNode(
									ctx, cUUID, d.Id()).ResizeNodeParams(req).Execute()
								if err != nil {
									return "", resp, err
								}
								return r.GetTaskUUID(), resp, nil
							},
						); diags != nil {
							return diags
						}
					} else {
						tflog.Error(ctx, "Volume Size cannot be decreased")
					}
				}

				updateUni, response, err = c.UniverseManagementAPI.GetUniverse(ctx, cUUID, d.Id()).
					Execute()
				if err != nil {
					errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
						"Universe", "Update - Fetch universe")
					return diag.FromErr(errMessage)
				}
				oldUserIntent = updateUni.UniverseDetails.Clusters[i].UserIntent

				// Num of nodes, Instance Type, Num of Volumes, Volume Size, User Tags changes
				var editAllowed, editZoneAllowed bool
				editAllowed, updateUni.UniverseDetails.Clusters[i].UserIntent = editUniverseParameters(
					ctx,
					oldUserIntent,
					newUserIntent,
				)
				if editAllowed || editZoneAllowed {
					effectiveCommPorts := updateUni.UniverseDetails.CommunicationPorts
					if d.HasChange("communication_ports") {
						effectiveCommPorts = buildCommunicationPorts(
							utils.MapFromSingletonList(
								d.Get("communication_ports").([]interface{})))
					}
					req := client.UniverseConfigureTaskParams{
						UniverseUUID: utils.GetStringPointer(d.Id()),
						Clusters:     updateUni.UniverseDetails.Clusters,
						NodeDetailsSet: buildNodeDetailsRespArrayToNodeDetailsArray(
							updateUni.UniverseDetails.NodeDetailsSet),
						CommunicationPorts: effectiveCommPorts,
					}
					if diags := utils.DispatchAndWait(ctx, "Update Primary Cluster", cUUID, c,
						d.Timeout(schema.TimeoutUpdate),
						utils.ResourceEntity, "Universe", "Update - Primary Cluster",
						func() (string, *http.Response, error) {
							r, resp, err := c.UniverseClusterMutationsAPI.UpdatePrimaryCluster(
								ctx, cUUID, d.Id()).UniverseConfigureTaskParams(req).Execute()
							if err != nil {
								return "", resp, err
							}
							return r.GetTaskUUID(), resp, nil
						},
					); diags != nil {
						return diags
					}
				}

			} else {

				//Ignore Software, GFlags, Systemd, TLS Upgrade changes to Read-Only Cluster
				updateUni, response, err := c.UniverseManagementAPI.GetUniverse(ctx, cUUID, d.Id()).Execute()
				if err != nil {
					errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
						"Universe", "Update - Fetch universe")
					return diag.FromErr(errMessage)
				}
				oldUserIntent := updateUni.UniverseDetails.Clusters[i].UserIntent
				if oldUserIntent.GetYbSoftwareVersion() != newUserIntent.GetYbSoftwareVersion() {
					tflog.Info(ctx, "Software Upgrade is applied only via change in Primary "+
						"Cluster User Intent, ignoring")
				}
				if !reflect.DeepEqual(oldUserIntent.GetMasterGFlags(), newUserIntent.GetMasterGFlags()) ||
					!reflect.DeepEqual(oldUserIntent.GetTserverGFlags(), newUserIntent.GetTserverGFlags()) {
					tflog.Info(ctx, "GFlags Upgrade is applied only via change in Primary "+
						"Cluster User Intent, ignoring")
				}
				if oldUserIntent.GetUseSystemd() != newUserIntent.GetUseSystemd() {
					tflog.Info(ctx, "System Upgrade is applied only via change in Primary "+
						"Cluster User Intent, ignoring")
				}
				if (oldUserIntent.GetEnableClientToNodeEncrypt() !=
					newUserIntent.GetEnableClientToNodeEncrypt()) ||
					oldUserIntent.GetEnableNodeToNodeEncrypt() != newUserIntent.GetEnableNodeToNodeEncrypt() {
					tflog.Info(ctx, "TLS Toggle is applied only via change in Primary Cluster"+
						" User Intent, ignoring")
				}

				// Num of nodes, Instance Type, Num of Volumes, Volume Size, User Tags changes
				var editAllowed bool
				editAllowed, updateUni.UniverseDetails.Clusters[i].UserIntent = editUniverseParameters(
					ctx, oldUserIntent, newUserIntent)
				if editAllowed {
					effectiveCommPorts := updateUni.UniverseDetails.CommunicationPorts
					if d.HasChange("communication_ports") {
						effectiveCommPorts = buildCommunicationPorts(
							utils.MapFromSingletonList(
								d.Get("communication_ports").([]interface{})))
					}
					req := client.UniverseConfigureTaskParams{
						UniverseUUID: utils.GetStringPointer(d.Id()),
						Clusters:     updateUni.UniverseDetails.Clusters,
						NodeDetailsSet: buildNodeDetailsRespArrayToNodeDetailsArray(
							updateUni.UniverseDetails.NodeDetailsSet),
						CommunicationPorts: effectiveCommPorts,
					}
					if diags := utils.DispatchAndWait(ctx, "Update Read Replica Cluster", cUUID, c,
						d.Timeout(schema.TimeoutUpdate),
						utils.ResourceEntity, "Universe", "Update - Read Replica Cluster",
						func() (string, *http.Response, error) {
							r, resp, err := c.UniverseClusterMutationsAPI.UpdateReadOnlyCluster(
								ctx, cUUID, d.Id()).UniverseConfigureTaskParams(req).Execute()
							if err != nil {
								return "", resp, err
							}
							return r.GetTaskUUID(), resp, nil
						},
					); diags != nil {
						return diags
					}
				}
			}
		}
		// VM Image Upgrade AFTER cluster operations if scaling in or no scale change.
		// Avoids upgrading nodes that are about to be removed.
		// imageBundleUpgrades is nil if already executed before the loop (scale-out case).
		if len(imageBundleUpgrades) > 0 {
			updateUni, response, err = c.UniverseManagementAPI.GetUniverse(
				ctx, cUUID, d.Id()).Execute()
			if err != nil {
				errMessage := utils.ErrorFromHTTPResponse(
					response, err, utils.ResourceEntity,
					"Universe", "Update - Fetch universe")
				return diag.FromErr(errMessage)
			}
			if diagErr := performVMImageUpgrade(
				ctx, c, cUUID, d, updateUni, imageBundleUpgrades,
				sleepAfterMasterMs, sleepAfterTServerMs,
			); diagErr != nil {
				return diagErr
			}
		}
	}

	// Handle editable communication port changes that occurred without any cluster changes.
	// When clusters also changed, ports are already bundled in the UpdatePrimaryCluster call
	// above. This path covers the case where ONLY ports changed.
	if d.HasChange("communication_ports") && !d.HasChange("clusters") {
		fetchedUni, response, err := c.UniverseManagementAPI.GetUniverse(ctx, cUUID, d.Id()).
			Execute()
		if err != nil {
			errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
				"Universe", "Update - Fetch universe for port update")
			return diag.FromErr(errMessage)
		}
		newCommPorts := buildCommunicationPorts(
			utils.MapFromSingletonList(d.Get("communication_ports").([]interface{})))
		req := client.UniverseConfigureTaskParams{
			UniverseUUID: utils.GetStringPointer(d.Id()),
			Clusters:     fetchedUni.UniverseDetails.Clusters,
			NodeDetailsSet: buildNodeDetailsRespArrayToNodeDetailsArray(
				fetchedUni.UniverseDetails.NodeDetailsSet),
			CommunicationPorts: newCommPorts,
		}
		if diags := utils.DispatchAndWait(ctx, "Update Communication Ports", cUUID, c,
			d.Timeout(schema.TimeoutUpdate),
			utils.ResourceEntity, "Universe", "Update - Communication Ports",
			func() (string, *http.Response, error) {
				r, resp, err := c.UniverseClusterMutationsAPI.UpdatePrimaryCluster(
					ctx, cUUID, d.Id()).UniverseConfigureTaskParams(req).Execute()
				if err != nil {
					return "", resp, err
				}
				return r.GetTaskUUID(), resp, nil
			},
		); diags != nil {
			return diags
		}
	}

	return
}

func performVMImageUpgrade(
	ctx context.Context,
	c *client.APIClient,
	cUUID string,
	d *schema.ResourceData,
	updateUni *client.UniverseResp,
	imageBundleUpgrades []client.ImageBundleUpgradeInfo,
	sleepAfterMasterMs int32,
	sleepAfterTServerMs int32,
) diag.Diagnostics {
	for _, ibUpgrade := range imageBundleUpgrades {
		for k := range updateUni.UniverseDetails.Clusters {
			if updateUni.UniverseDetails.Clusters[k].GetUuid() ==
				ibUpgrade.ClusterUuid {
				ib := ibUpgrade.ImageBundleUuid
				updateUni.UniverseDetails.Clusters[k].UserIntent.ImageBundleUUID = &ib
			}
		}
	}
	req := client.VMImageUpgradeParams{
		Clusters:                       updateUni.UniverseDetails.Clusters,
		UpgradeOption:                  "Rolling",
		ImageBundles:                   imageBundleUpgrades,
		SleepAfterMasterRestartMillis:  sleepAfterMasterMs,
		SleepAfterTServerRestartMillis: sleepAfterTServerMs,
	}
	return utils.DispatchAndWait(ctx, "VM Image Upgrade", cUUID, c,
		d.Timeout(schema.TimeoutUpdate),
		utils.ResourceEntity, "Universe", "Update - VM Image",
		func() (string, *http.Response, error) {
			r, resp, err := c.UniverseUpgradesManagementAPI.UpgradeVMImage(
				ctx, cUUID, d.Id()).VmimageUpgradeParams(req).Execute()
			if err != nil {
				return "", resp, err
			}
			return r.GetTaskUUID(), resp, nil
		},
	)
}

// unrecoverableFailedTasks are the YBA task names where a failed task leaves the
// universe in a state that requires force-delete to clean up.
var unrecoverableFailedTasks = map[string]bool{
	"CreateUniverse":        true,
	"ReadOnlyClusterCreate": true,
}

// isFailedCreate returns true when the universe is stuck after a failed create flow
// (primary or read replica). Signaled by: no update currently running, last update
// did not succeed, and the last task was a creation task. In this state a normal
// delete will typically fail, so escalating to force-delete is safe, there's no
// data to preserve. This corresponds to the scenario where a user would have
// marked the resource tainted in Terraform state.
func isFailedCreate(details *client.UniverseDefinitionTaskParamsResp) bool {
	if details == nil {
		return false
	}
	return !details.GetUpdateInProgress() &&
		!details.GetUpdateSucceeded() &&
		unrecoverableFailedTasks[details.GetUpdatingTask()]
}

func resourceUniverseDelete(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{}) diag.Diagnostics {

	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID
	universeID := d.Id()

	forceDeleteConfig := d.Get("delete_options.0.force_delete").(bool)
	deleteBackups := d.Get("delete_options.0.delete_backups").(bool)
	deleteCerts := d.Get("delete_options.0.delete_certs").(bool)

	// runDelete dispatches DeleteUniverse with the given force value and waits for
	// the task to complete. Returned as a local helper so the escalation path can
	// call it a second time without duplicating the boilerplate.
	runDelete := func(force bool) diag.Diagnostics {
		return utils.DispatchAndWait(ctx, "Delete Universe", cUUID, c,
			d.Timeout(schema.TimeoutDelete),
			utils.ResourceEntity, "Universe", "Delete",
			func() (string, *http.Response, error) {
				r, resp, err := c.UniverseManagementAPI.DeleteUniverse(ctx, cUUID, universeID).
					IsForceDelete(force).
					IsDeleteBackups(deleteBackups).
					IsDeleteAssociatedCerts(deleteCerts).
					Execute()
				if err != nil {
					return "", resp, err
				}
				return r.GetTaskUUID(), resp, nil
			},
		)
	}

	// First attempt: honor the user's force_delete preference as-is. On a healthy
	// universe this succeeds and we return. On a transient failure (YBA briefly
	// unreachable, CSP throttling, etc.) this also returns an error, we surface
	// that to the user rather than masking it, unless the failure looks like the
	// specific failed-create fingerprint described below.
	diags := runDelete(forceDeleteConfig)
	if !diags.HasError() {
		d.SetId("")
		return diags
	}

	// First attempt failed. If the user already asked for force_delete, there's
	// nothing further to try, the escalation path would be a no-op.
	if forceDeleteConfig {
		return diags
	}

	// Check whether the universe is in a failed-create state. If yes, retry with
	// force=true: there is no data to preserve, and a normal delete will never
	// succeed against this fingerprint (primary or RR create left half-provisioned).
	// If the fingerprint does not match, return the original error, the user's
	// force_delete=false preference stands and the failure is treated as legitimate.
	uni, _, fetchErr := c.UniverseManagementAPI.GetUniverse(ctx, cUUID, universeID).Execute()
	if fetchErr != nil {
		tflog.Warn(ctx, fmt.Sprintf(
			"Universe %s delete failed; could not fetch universe to check for "+
				"failed-create state (%v). Returning the original delete error "+
				"without escalating.", universeID, fetchErr))
		return diags
	}
	if uni == nil || !isFailedCreate(uni.UniverseDetails) {
		return diags
	}

	tflog.Warn(ctx, fmt.Sprintf(
		"Universe %s delete failed and the universe is in a failed-create state "+
			"(updateInProgress=false, updateSucceeded=false, updatingTask=%q); "+
			"retrying with force_delete=true to clean up half-provisioned resources.",
		universeID, uni.UniverseDetails.GetUpdatingTask()))

	diags = runDelete(true)
	if !diags.HasError() {
		d.SetId("")
	}
	return diags
}
