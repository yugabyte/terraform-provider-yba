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
	"reflect"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/customdiff"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	client "github.com/yugabyte/platform-go-client"
	"github.com/yugabyte/terraform-provider-yba/internal/api"
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
			"client_root_ca": {
				Type:     schema.TypeString,
				Optional: true,
				DiffSuppressFunc: func(k, old, new string, d *schema.ResourceData) bool {
					// When TLS is enabled and this field is not set in the config file, a new root
					// certificate is created and this is populated. Subsequent runs will throw a
					// diff since this field is empty in the config file. This is to ignore the
					// difference in that case
					if len(old) > 0 && new == "" {
						return true
					}
					return false
				},
				Description: "The UUID of the clientRootCA to be used to generate client" +
					" certificates and facilitate TLS communication between server and client.",
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
			"node_details_set": {
				Type:     schema.TypeList,
				Computed: true,
				Elem:     nodeDetailsSetSchema(),
			},
		},
	}
}

func universeYBAVersionCheck(ctx context.Context, c *client.APIClient) (bool, string, error) {
	allowedVersions := utils.YBAMinimumVersion{
		Stable:  utils.YBAAllowUniverseMinVersion,
		Preview: utils.YBAAllowUniverseMinVersion,
	}
	allowed, version, err := utils.CheckValidYBAVersion(ctx, c, allowedVersions)
	if err != nil {
		return false, "", err
	}
	return allowed, version, err
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
		customdiff.ValidateChange("clusters", func(ctx context.Context, old, new, m interface{}) error {
			// if not a new universe, prevent adding read replicas
			newClusterSet := buildClusters(new.([]interface{}))
			if len(old.([]interface{})) != 0 {
				oldClusterSet := buildClusters(old.([]interface{}))
				if len(oldClusterSet) < len(newClusterSet) {
					return errors.New("Cannot add Read Replica to existing universe")
				}
			}
			return nil
		}),
		customdiff.ValidateChange("clusters", func(ctx context.Context, old, new, m interface{}) error {
			// if not a new universe, prevent systemD disablement
			newClusterSet := buildClusters(new.([]interface{}))
			if len(old.([]interface{})) != 0 {
				oldClusterSet := buildClusters(old.([]interface{}))
				oldPrimaryCluster, isPresent := getClusterByType(oldClusterSet, "PRIMARY")
				if isPresent {
					newPrimaryCluster, isNewPresent := getClusterByType(newClusterSet, "PRIMARY")
					if isNewPresent {
						if oldPrimaryCluster.UserIntent.GetUseSystemd() == true &&
							newPrimaryCluster.UserIntent.GetUseSystemd() == false {
							return errors.New("Cannot disable Systemd")
						}
					}
				}
			}
			return nil
		}),
		customdiff.ValidateChange("clusters", func(ctx context.Context, old, new, m interface{}) error {
			// if not a new universe, prevent decrease in volume size in primary
			newClusterSet := buildClusters(new.([]interface{}))
			if len(old.([]interface{})) != 0 {
				oldClusterSet := buildClusters(old.([]interface{}))
				oldPrimaryCluster, isPresent := getClusterByType(oldClusterSet, "PRIMARY")
				if isPresent {
					newPrimaryCluster, isNewPresent := getClusterByType(newClusterSet, "PRIMARY")
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
		}),
		customdiff.ValidateChange("clusters", func(ctx context.Context, old, new, m interface{}) error {
			// if not a new universe, prevent change in number of nodes if instance type hasn't
			// change in Primary
			newClusterSet := buildClusters(new.([]interface{}))
			if len(old.([]interface{})) != 0 {
				oldClusterSet := buildClusters(old.([]interface{}))
				oldPrimaryCluster, isPresent := getClusterByType(oldClusterSet, "PRIMARY")
				if isPresent {
					newPrimaryCluster, isNewPresent := getClusterByType(newClusterSet, "PRIMARY")
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
		}),
		customdiff.ValidateChange("clusters", func(ctx context.Context, old, new, m interface{}) error {
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
		}),
		customdiff.ValidateChange("clusters", func(ctx context.Context, old, new, m interface{}) error {
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
							return errors.New("Cannot change number of volumes or volume size " +
								"per node without change in instance type in Read Replica Cluster")
						}
					}
				}
			}
			return nil
		}),
		customdiff.ValidateChange("clusters", func(ctx context.Context, old, new, m interface{}) error {
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
		}),
		customdiff.ValidateChange("clusters", func(ctx context.Context, old, new, m interface{}) error {
			// check if software version of the clusters are the same
			newClusterSet := buildClusters(new.([]interface{}))
			newPrimary, isPresent := getClusterByType(newClusterSet, "PRIMARY")
			newReadOnly, isRRPresnt := getClusterByType(newClusterSet, "ASYNC")
			if len(old.([]interface{})) != 0 {
				if isPresent && isRRPresnt {
					if newPrimary.UserIntent.GetYbSoftwareVersion() !=
						newReadOnly.UserIntent.GetYbSoftwareVersion() {
						return errors.New("Cannot have different software versions for Primary " +
							"and Read Only clusters")
					}
				}
			}
			return nil
		}),
		customdiff.ValidateChange("clusters", func(ctx context.Context, old, new, m interface{}) error {
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
		}),
		customdiff.ValidateChange("clusters", func(ctx context.Context, old, new, m interface{}) error {
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
		}),
		customdiff.ValidateChange("clusters", func(ctx context.Context, old, new, m interface{}) error {
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
		}),
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
	)
}
func resourceUniverseCreate(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{}) diag.Diagnostics {
	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID

	allowed, version, err := universeYBAVersionCheck(ctx, c)
	if err != nil {
		return diag.FromErr(err)
	}

	if !allowed {

		return diag.FromErr(fmt.Errorf("Creating universes below version %s (or on restricted"+
			" versions) is not supported, currently on %s", utils.YBAAllowUniverseMinVersion,
			version))

	}

	req := buildUniverse(d)
	r, response, err := c.UniverseClusterMutationsApi.CreateAllClusters(ctx, cUUID).
		UniverseConfigureTaskParams(req).Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"Universe", "Create")
		return diag.FromErr(errMessage)
	}

	d.SetId(*r.ResourceUUID)
	tflog.Debug(ctx, fmt.Sprintf("Waiting for universe %s to be active", d.Id()))
	err = utils.WaitForTask(ctx, *r.TaskUUID, cUUID, c, d.Timeout(schema.TimeoutCreate))
	if err != nil {
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

	r, response, err := c.UniverseManagementApi.GetUniverse(ctx, cUUID, d.Id()).Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"Universe", "Read")
		return diag.FromErr(errMessage)
	}

	u := r.UniverseDetails
	if err = d.Set("client_root_ca", u.ClientRootCA); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("clusters", flattenClusters(u.Clusters)); err != nil {
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
	return diags
}

func editUniverseParameters(ctx context.Context, oldUserIntent client.UserIntent,
	newUserIntent client.UserIntent) (bool, client.UserIntent) {
	if !reflect.DeepEqual(oldUserIntent.GetInstanceTags(), newUserIntent.GetInstanceTags()) ||
		!reflect.DeepEqual(oldUserIntent.GetRegionList(), newUserIntent.GetRegionList()) ||
		oldUserIntent.GetNumNodes() != newUserIntent.GetNumNodes() ||
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

func resourceUniverseUpdate(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{}) diag.Diagnostics {
	// Only updates user intent for each cluster
	// cloud Info can have changes in zones
	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID

	allowed, version, err := universeYBAVersionCheck(ctx, c)
	if err != nil {
		return diag.FromErr(err)
	}

	if !allowed {

		return diag.FromErr(fmt.Errorf("Editing universes below version %s (or on restricted"+
			" versions) is not supported, currently on %s", utils.YBAAllowUniverseMinVersion,
			version))

	}

	if d.HasChange("clusters") {
		clusters := d.Get("clusters").([]interface{})
		updateUni, response, err := c.UniverseManagementApi.GetUniverse(ctx, cUUID, d.Id()).Execute()
		if err != nil {
			errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
				"Universe", "Update - Fetch universe")
			return diag.FromErr(errMessage)
		}
		newUni := buildUniverse(d)

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

				r, response, err := c.UniverseClusterMutationsApi.DeleteReadonlyCluster(ctx, cUUID,
					d.Id(), clusterUUID).IsForceDelete(
					d.Get("delete_options.0.force_delete").(bool)).Execute()
				if err != nil {
					errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
						"Universe", "Update - Delete Read Replica cluster")
					return diag.FromErr(errMessage)
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

			oldUserIntent := updateUni.UniverseDetails.Clusters[i].UserIntent
			newUserIntent := newUni.Clusters[i].UserIntent
			if cluster["cluster_type"] == "PRIMARY" {

				//Software Upgrade
				if oldUserIntent.GetYbSoftwareVersion() != newUserIntent.GetYbSoftwareVersion() {
					updateUni.UniverseDetails.Clusters[i].UserIntent = newUserIntent
					req := client.SoftwareUpgradeParams{
						YbSoftwareVersion: newUserIntent.GetYbSoftwareVersion(),
						Clusters:          updateUni.UniverseDetails.Clusters,
						UpgradeOption:     "Rolling",
					}
					r, response, err := c.UniverseUpgradesManagementApi.UpgradeSoftware(
						ctx, cUUID, d.Id()).SoftwareUpgradeParams(req).Execute()
					if err != nil {
						errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
							"Universe", "Update - Software")
						return diag.FromErr(errMessage)
					}
					tflog.Info(ctx, "UpgradeSoftware task is executing")
					err = utils.WaitForTask(ctx, *r.TaskUUID, cUUID, c, d.Timeout(schema.TimeoutUpdate))
					if err != nil {
						return diag.FromErr(err)
					}
				}

				updateUni, response, err = c.UniverseManagementApi.GetUniverse(ctx, cUUID, d.Id()).Execute()
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
						UpgradeOption: "Rolling",
					}
					r, response, err := c.UniverseUpgradesManagementApi.UpgradeGFlags(
						ctx, cUUID, d.Id()).GflagsUpgradeParams(req).Execute()
					if err != nil {
						errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
							"Universe", "Update - GFlags")
						return diag.FromErr(errMessage)
					}
					tflog.Info(ctx, "UpgradeGFlags task is executing")
					err = utils.WaitForTask(ctx, *r.TaskUUID, cUUID, c,
						d.Timeout(schema.TimeoutUpdate))
					if err != nil {
						return diag.FromErr(err)
					}
				}

				updateUni, response, err = c.UniverseManagementApi.GetUniverse(ctx, cUUID,
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
					}
					r, response, err := c.UniverseUpgradesManagementApi.UpgradeTls(
						ctx, cUUID, d.Id()).TlsToggleParams(req).Execute()
					if err != nil {
						errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
							"Universe", "Update - TLS Toggle")
						return diag.FromErr(errMessage)
					}
					tflog.Info(ctx, "UpgradeTLS task is executing")
					err = utils.WaitForTask(ctx, *r.TaskUUID, cUUID, c, d.Timeout(schema.TimeoutUpdate))
					if err != nil {
						return diag.FromErr(err)
					}
				}

				updateUni, response, err = c.UniverseManagementApi.GetUniverse(ctx, cUUID, d.Id()).Execute()
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
						UpgradeOption: "Rolling",
					}
					r, response, err := c.UniverseUpgradesManagementApi.UpgradeSystemd(
						ctx, cUUID, d.Id()).SystemdUpgradeParams(req).Execute()
					if err != nil {
						errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
							"Universe", "Update - Systemd")
						return diag.FromErr(errMessage)
					}
					tflog.Info(ctx, "UpgradeSystemd task is executing")
					err = utils.WaitForTask(ctx, *r.TaskUUID, cUUID, c, d.Timeout(schema.TimeoutUpdate))
					if err != nil {
						return diag.FromErr(err)
					}
				} else if oldUserIntent.GetUseSystemd() == true &&
					newUserIntent.GetUseSystemd() == false {
					tflog.Error(ctx, "Cannot disable Systemd")
				}

				updateUni, response, err = c.UniverseManagementApi.GetUniverse(ctx, cUUID, d.Id()).Execute()
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
						//Only volume size should be changed to do smart resize, other changes
						//handled in UpgradeCluster
						updateUni.UniverseDetails.Clusters[i].UserIntent.DeviceInfo.VolumeSize = (
							newUserIntent.DeviceInfo.VolumeSize)
						req := client.ResizeNodeParams{
							UpgradeOption: "Rolling",
							Clusters:      updateUni.UniverseDetails.Clusters,
							NodeDetailsSet: buildNodeDetailsRespArrayToNodeDetailsArray(
								updateUni.UniverseDetails.NodeDetailsSet),
						}
						r, response, err := c.UniverseUpgradesManagementApi.ResizeNode(
							ctx, cUUID, d.Id()).ResizeNodeParams(req).Execute()
						if err != nil {
							errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
								"Universe", "Update - Resize Nodes")
							return diag.FromErr(errMessage)
						}
						tflog.Info(ctx, "ResizeNode task is executing")
						err = utils.WaitForTask(ctx, *r.TaskUUID, cUUID, c, d.Timeout(schema.TimeoutUpdate))
						if err != nil {
							return diag.FromErr(err)
						}
					} else {
						tflog.Error(ctx, "Volume Size cannot be decreased")
					}
				}

				updateUni, response, err = c.UniverseManagementApi.GetUniverse(ctx, cUUID, d.Id()).Execute()
				if err != nil {
					errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
						"Universe", "Update - Fetch universe")
					return diag.FromErr(errMessage)
				}
				oldUserIntent = updateUni.UniverseDetails.Clusters[i].UserIntent

				// Num of nodes, Instance Type, Num of Volumes, Volume Size, User Tags changes
				var editAllowed, editZoneAllowed bool
				editAllowed, updateUni.UniverseDetails.Clusters[i].UserIntent = (
					editUniverseParameters(ctx, oldUserIntent, newUserIntent))
				if editAllowed || editZoneAllowed {
					req := client.UniverseConfigureTaskParams{
						UniverseUUID: utils.GetStringPointer(d.Id()),
						Clusters:     updateUni.UniverseDetails.Clusters,
						NodeDetailsSet: buildNodeDetailsRespArrayToNodeDetailsArray(
							updateUni.UniverseDetails.NodeDetailsSet),
					}
					r, response, err := c.UniverseClusterMutationsApi.UpdatePrimaryCluster(
						ctx, cUUID, d.Id()).UniverseConfigureTaskParams(req).Execute()
					if err != nil {
						errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity, "Universe",
							"Update - Primary Cluster")
						return diag.FromErr(errMessage)
					}
					tflog.Info(ctx, "UpdatePrimaryCluster task is executing")
					err = utils.WaitForTask(ctx, *r.TaskUUID, cUUID, c, d.Timeout(schema.TimeoutUpdate))
					if err != nil {
						return diag.FromErr(err)
					}
				}

			} else {

				//Ignore Software, GFlags, Systemd, TLS Upgrade changes to Read-Only Cluster
				updateUni, response, err := c.UniverseManagementApi.GetUniverse(ctx, cUUID, d.Id()).Execute()
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

				// Num of nodes, Instance Type, Num of Volumes, Volume Size User Tags changes
				var editAllowed bool
				editAllowed, updateUni.UniverseDetails.Clusters[i].UserIntent = (
					editUniverseParameters(ctx, oldUserIntent, newUserIntent))
				if editAllowed {
					req := client.UniverseConfigureTaskParams{
						UniverseUUID: utils.GetStringPointer(d.Id()),
						Clusters:     updateUni.UniverseDetails.Clusters,
						NodeDetailsSet: buildNodeDetailsRespArrayToNodeDetailsArray(
							updateUni.UniverseDetails.NodeDetailsSet),
					}
					r, response, err := c.UniverseClusterMutationsApi.UpdateReadOnlyCluster(
						ctx, cUUID, d.Id()).UniverseConfigureTaskParams(req).Execute()
					if err != nil {
						errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
							"Universe", "Update - Read Replica Cluster")
						return diag.FromErr(errMessage)
					}
					tflog.Info(ctx, "UpdateReadOnlyCluster task is executing")
					err = utils.WaitForTask(ctx, *r.TaskUUID, cUUID, c, d.Timeout(schema.TimeoutUpdate))
					if err != nil {
						return diag.FromErr(err)
					}
				}
			}

		}
	}

	return resourceUniverseRead(ctx, d, meta)
}

func resourceUniverseDelete(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID

	r, response, err := c.UniverseManagementApi.DeleteUniverse(ctx, cUUID, d.Id()).
		IsForceDelete(d.Get("delete_options.0.force_delete").(bool)).
		IsDeleteBackups(d.Get("delete_options.0.delete_backups").(bool)).
		IsDeleteAssociatedCerts(d.Get("delete_options.0.delete_certs").(bool)).
		Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"Universe", "Delete")
		return diag.FromErr(errMessage)
	}

	tflog.Debug(ctx, fmt.Sprintf("Waiting for universe %s to be deleted", d.Id()))
	err = utils.WaitForTask(ctx, *r.TaskUUID, cUUID, c, d.Timeout(schema.TimeoutDelete))
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId("")
	return diags
}
