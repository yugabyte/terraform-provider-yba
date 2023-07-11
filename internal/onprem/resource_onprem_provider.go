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

package onprem

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/customdiff"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	client "github.com/yugabyte/platform-go-client"
	"github.com/yugabyte/terraform-provider-yba/internal/api"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
	"golang.org/x/exp/slices"
)

// ResourceOnPremProvider creates and maintains resource for OnPrem providers
func ResourceOnPremProvider() *schema.Resource {
	return &schema.Resource{
		Description: "OnPrem Provider Resource.",

		CreateContext: resourceOnPremProviderCreate,
		ReadContext:   resourceOnPremProviderRead,
		UpdateContext: resourceOnPremProviderUpdate,
		DeleteContext: resourceOnPremProviderDelete,

		CustomizeDiff: resourceOnPremDiff(),

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(30 * time.Minute),
			Update: schema.DefaultTimeout(15 * time.Minute),
			Delete: schema.DefaultTimeout(15 * time.Minute),
		},

		Schema: map[string]*schema.Schema{
			"code": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "Code of the OnPrem provider.",
			},
			"name": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Name of the provider.",
			},
			"version": {
				Type:        schema.TypeInt,
				Computed:    true,
				Description: "Version of the provider.",
			},
			"instance_types": InstanceTypesSchema(),
			"node_instances": NodeInstanceSchema(),
			"regions":        RegionsSchema(),
			"access_keys":    AccessKeySchema(),
			"details":        ProviderDetailsSchema(),
		},
	}
}

func resourceOnPremDiff() schema.CustomizeDiffFunc {
	return customdiff.All(
		customdiff.ValidateValue("access_keys",
			func(ctx context.Context, value, meta interface{}) error {
				if value == nil {
					return fmt.Errorf("Access Keys block cannot be empty")
				}
				_, err := buildAccessKeys(value.([]interface{}))
				if err != nil {
					return err
				}
				return nil
			},
		),
		customdiff.IfValueChange("name",
			func(ctx context.Context, old, new, meta interface{}) bool {
				return !reflect.DeepEqual(old, new) && old.(string) != ""
			},
			func(ctx context.Context, d *schema.ResourceDiff, meta interface{}) error {
				// if universes exist for this provider, block edit
				c := meta.(*api.APIClient).YugawareClient
				cUUID := meta.(*api.APIClient).CustomerID
				_, exists, err := utils.GetUniversesForProvider(ctx, c, cUUID, d.Id(), "")
				if err != nil {
					return err
				}
				if exists {
					return fmt.Errorf("Universe exists for this provider, editing name is blocked")
				}
				return nil
			},
		),
		customdiff.IfValueChange("regions",
			func(ctx context.Context, old, new, meta interface{}) bool {
				return !reflect.DeepEqual(old, new) && len(old.([]interface{})) != 0
			},
			func(ctx context.Context, d *schema.ResourceDiff, meta interface{}) error {
				// if universes exist for this provider, block edit
				c := meta.(*api.APIClient).YugawareClient
				cUUID := meta.(*api.APIClient).CustomerID
				_, exists, err := utils.GetUniversesForProvider(ctx, c, cUUID, d.Id(), "")
				if err != nil {
					return err
				}
				if exists {
					return fmt.Errorf("Universe exists for this provider, editing regions is blocked")
				}
				return nil
			},
		),
		customdiff.IfValueChange("access_keys",
			func(ctx context.Context, old, new, meta interface{}) bool {
				return !reflect.DeepEqual(old, new) && len(old.([]interface{})) != 0
			},
			func(ctx context.Context, d *schema.ResourceDiff, meta interface{}) error {
				// if universes exist for this provider, block edit
				/*c := meta.(*api.APIClient).YugawareClient
				cUUID := meta.(*api.APIClient).CustomerID
				_, exists, err := utils.GetUniversesForProvider(ctx, c, cUUID, d.Id(), "")
				if err != nil {
					return err
				}
				if exists {
					return fmt.Errorf("Universe exists for this provider, editing access_keys is blocked")
				}*/
				return fmt.Errorf("access keys cannot be edited")
				//return nil
			},
		),
		customdiff.IfValueChange("details",
			func(ctx context.Context, old, new, meta interface{}) bool {
				return !reflect.DeepEqual(old, new) && len(old.([]interface{})) != 0
			},
			func(ctx context.Context, d *schema.ResourceDiff, meta interface{}) error {
				// if universes exist for this provider, block edit
				c := meta.(*api.APIClient).YugawareClient
				cUUID := meta.(*api.APIClient).CustomerID
				_, exists, err := utils.GetUniversesForProvider(ctx, c, cUUID, d.Id(), "")
				if err != nil {
					return err
				}
				if exists {
					return fmt.Errorf("Universe exists for this provider, editing details is blocked")
				}
				return nil
			},
		),
		customdiff.IfValueChange("node_instances",
			func(ctx context.Context, old, new, meta interface{}) bool {
				return !reflect.DeepEqual(old, new) && len(old.([]interface{})) != 0
			},
			func(ctx context.Context, d *schema.ResourceDiff, meta interface{}) error {
				// if node is in use, restrict removal
				c := meta.(*api.APIClient).YugawareClient
				cUUID := meta.(*api.APIClient).CustomerID
				pUUID := d.Id()
				nodeInstancesInterface := d.Get("node_instances").([]interface{})
				newNodeIPs := make([]string, 0)
				for _, i := range nodeInstancesInterface {
					node := i.(map[string]interface{})
					newNodeIPs = append(newNodeIPs, node["ip"].(string))
				}
				existingNodeInstances, err := nodeInstancesRead(ctx, c, cUUID, pUUID)
				if err != nil {
					return err
				}
				inUseNodes := make([]string, 0)
				for _, n := range existingNodeInstances {
					if n.GetInUse() {
						details := n.GetDetails()
						ip := details.GetIp()
						if !slices.Contains(newNodeIPs, ip) {
							inUseNodes = append(inUseNodes, ip)
						}
					}
				}
				if len(inUseNodes) > 0 {
					return fmt.Errorf("Cannot remove in use nodes: %v", inUseNodes)
				}
				return nil
			},
		),
	)
}

func resourceOnPremProviderCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) (
	diag.Diagnostics) {
	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID

	// Step 1: Create OnPrem Cloud Provider
	// Setp 2: Use provider information to create instance type
	// Step 3: User instance type information to add instances to provider

	code := "onprem"

	accessKeys, err := buildAccessKeys(d.Get("access_keys").([]interface{}))
	if err != nil {
		diag.FromErr(err)
	}
	req := client.Provider{
		Code:          utils.GetStringPointer(code),
		Name:          utils.GetStringPointer(d.Get("name").(string)),
		Regions:       buildRegions(d.Get("regions").([]interface{})),
		AllAccessKeys: accessKeys,
		Details:       buildProviderDetails(d.Get("details").(interface{})),
	}

	r, response, err := c.CloudProvidersApi.CreateProviders(ctx, cUUID).CreateProviderRequest(
		req).Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"On Prem Provider", "Create")
		return diag.FromErr(errMessage)
	}
	pUUID := *r.ResourceUUID

	if r.TaskUUID != nil {
		tflog.Debug(ctx, fmt.Sprintf("Waiting for on prem provider %s to be active", pUUID))
		err = utils.WaitForTask(ctx, *r.TaskUUID, cUUID, c, d.Timeout(schema.TimeoutCreate))
		if err != nil {
			return diag.FromErr(err)
		}
	}

	// Create Instance Types
	if d.Get("instance_types") != nil && len(d.Get("instance_types").([]interface{})) > 0 {
		tflog.Info(ctx, fmt.Sprintf("Creating instance types for provider %s", pUUID))

		instanceTypeRespList, err := instanceTypesCreate(ctx, c, cUUID, pUUID, d.Get(
			"instance_types").([]interface{}))
		if err != nil {
			return diag.FromErr(err)
		}
		d.Set("instance_type", instanceTypeRespList)
	}

	// Create Node Instances
	if d.Get("node_instances") != nil && len(d.Get("node_instances").([]interface{})) > 0 {
		tflog.Info(ctx, fmt.Sprintf("Adding node instance for provider %s", pUUID))
		nodeInstancesList, err := nodeInstancesCreate(ctx, c, cUUID, pUUID,
			d.Get("node_instances").([]interface{}))
		if err != nil {
			return diag.FromErr(err)
		}
		d.Set("node_instances", nodeInstancesList)
	}

	d.SetId(pUUID)
	return resourceOnPremProviderRead(ctx, d, meta)
}

func findProvider(providers []client.Provider, uuid string) (*client.Provider, error) {
	for _, p := range providers {
		if *p.Uuid == uuid {
			return &p, nil
		}
	}
	return nil, fmt.Errorf("Could not find provider %s", uuid)
}

func resourceOnPremProviderRead(ctx context.Context, d *schema.ResourceData, meta interface{}) (
	diag.Diagnostics) {
	var diags diag.Diagnostics
	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID
	pUUID := d.Id()

	r, response, err := c.CloudProvidersApi.GetListOfProviders(ctx, cUUID).Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"On Prem Provider", "Read")
		return diag.FromErr(errMessage)
	}

	// Get order of instance type
	instanceTypesOrderList := make([]string, 0)
	instanceTypesInterface := d.Get("instance_types")
	if instanceTypesInterface != nil && len(instanceTypesInterface.([]interface{})) > 0 {
		for _, t := range instanceTypesInterface.([]interface{}) {
			i := t.(map[string]interface{})
			idKey := i["instance_type_key"].([]interface{})[0].(map[string]interface{})
			instanceTypesOrderList = append(instanceTypesOrderList,
				idKey["instance_type_code"].(string))
		}
	}

	// Get order of node ips
	nodeInstancesOrderList := make([]string, 0)
	nodeInstancesInterface := d.Get("node_instances")
	if nodeInstancesInterface != nil && len(nodeInstancesInterface.([]interface{})) > 0 {
		for _, m := range nodeInstancesInterface.([]interface{}) {
			n := m.(map[string]interface{})
			nodeInstancesOrderList = append(nodeInstancesOrderList, n["ip"].(string))
		}
	}

	p, err := findProvider(r, pUUID)
	if err != nil {
		return diag.FromErr(err)
	}

	if err = d.Set("code", p.GetCode()); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("name", p.GetName()); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("version", p.GetVersion()); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("regions", flattenRegions(p.GetRegions())); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("access_keys", flattenAccessKeys(p.GetAllAccessKeys(), d)); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("details", flattenProviderDetails(p.GetDetails())); err != nil {
		return diag.FromErr(err)
	}

	instanceTypes, err := instanceTypesRead(ctx, c, cUUID, pUUID)
	if err != nil {
		return diag.FromErr(err)
	}
	err = d.Set("instance_types", flattenInstanceTypes(instanceTypes, instanceTypesOrderList))
	if err != nil {
		return diag.FromErr(err)
	}

	nodeInstances, err := nodeInstancesRead(ctx, c, cUUID, pUUID)
	if err != nil {
		return diag.FromErr(err)
	}
	err = d.Set("node_instances", flattenNodeInstances(nodeInstances, nodeInstancesOrderList))
	if err != nil {
		return diag.FromErr(err)
	}

	return diags
}

func resourceOnPremProviderUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) (
	diag.Diagnostics) {
	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID
	pUUID := d.Id()
	// All user facing fields are editable when provider is not used in a universe

	providers, response, err := c.CloudProvidersApi.GetListOfProviders(ctx, cUUID).Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"On Prem Provider", "Update - Fetch Latest Values")
		return diag.FromErr(errMessage)
	}

	p, err := findProvider(providers, pUUID)
	if err != nil {
		return diag.FromErr(err)
	}
	providerReq := *p

	nameChange := d.HasChange("name")
	regionsChange := d.HasChange("regions")
	accessKeysChange := d.HasChange("access_keys")
	detailsChange := d.HasChange("details")

	allowed, version, err := editOnpremYBAVersionCheck(ctx, c)
	if err != nil {
		return diag.FromErr(err)
	}

	if !allowed && (nameChange || regionsChange || accessKeysChange || detailsChange) {
		return diag.FromErr(fmt.Errorf("Editing provider below version %s (or on restricted "+
			"versions) is not supported, currently on %s", utils.YBAAllowEditProviderMinVersion,
			version))
	}

	if nameChange {
		providerReq.SetName(d.Get("name").(string))
	}

	if regionsChange {
		regions := buildRegions(d.Get("regions").([]interface{}))
		regionsReq := createRequestForEditRegions(providerReq.GetRegions(), regions)
		providerReq.SetRegions(regionsReq)
	}

	if accessKeysChange {
		/*accessKeysReq, err := buildAccessKeys(d.Get("access_keys").([]interface{}))
		if err != nil {
			return diag.FromErr(err)
		}
		keyInfo := (*accessKeysReq)[0].GetKeyInfo()
		keyInfoReq := client.KeyInfo{
			SshPrivateKeyContent: utils.GetStringPointer(keyInfo.GetSshPrivateKeyContent()),
			KeyPairName: utils.GetStringPointer(keyInfo.GetKeyPairName()),
		}
		accessKeys := client.AccessKey{
			KeyInfo: keyInfoReq,
		}
		newAccessKeys := make([]client.AccessKey, 0)
		newAccessKeys = append(newAccessKeys, accessKeys)
		providerReq.SetAllAccessKeys(newAccessKeys)*/
	}

	if detailsChange {
		providerReq.SetDetails(*buildProviderDetails(d.Get("details").([]interface{})))
	}

	if nameChange || regionsChange || accessKeysChange || detailsChange {

		r, response, err := c.CloudProvidersApi.EditProvider(ctx, cUUID, pUUID).EditProviderRequest(
			providerReq).Execute()
		if err != nil {
			errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
				"On Prem Provider", "Update")
			return diag.FromErr(errMessage)
		}

		if r.TaskUUID != nil {
			tflog.Debug(ctx, fmt.Sprintf("Waiting for on prem provider %s to be active", pUUID))
			err = utils.WaitForTask(ctx, *r.TaskUUID, cUUID, c, d.Timeout(schema.TimeoutCreate))
			if err != nil {
				return diag.FromErr(err)
			}
		}

	}

	if d.HasChange("instance_types") {
		instanceTypesInterface := make([]interface{}, 0)
		existingInstanceTypesResp, err := instanceTypesRead(ctx, c, cUUID, pUUID)
		if err != nil {
			return diag.FromErr(err)
		}
		existingInstanceTypes := buildInstanceTypeFromInstanceTypeResp(existingInstanceTypesResp)
		for _, instanceType := range existingInstanceTypes {
			typeCode := instanceType.GetInstanceTypeCode()
			tflog.Info(ctx,
				fmt.Sprintf("Removing instance type %s from provider %s", typeCode, pUUID))
			err := instanceTypeDelete(ctx, c, cUUID, pUUID, typeCode)
			if err != nil {
				return diag.FromErr(err)
			}
		}
		if d.Get("instance_types").(interface{}) != nil {
			instanceTypesInterface = d.Get("instance_types").([]interface{})
			tflog.Info(ctx, fmt.Sprintf("Adding instance types to provider %s", pUUID))
			_, err := instanceTypesCreate(ctx, c, cUUID, pUUID, instanceTypesInterface)
			if err != nil {
				return diag.FromErr(err)
			}
		}
	}

	if d.HasChange("node_instances") {
		existingNodeInstances, err := nodeInstancesRead(ctx, c, cUUID, pUUID)
		if err != nil {
			return diag.FromErr(err)
		}
		for _, node := range existingNodeInstances {
			details := node.GetDetails()
			ip := details.GetIp()
			if !node.GetInUse() {
				// do not delete nodes in use, would throw an error
				tflog.Info(ctx,
					fmt.Sprintf("Removing node instance %s from provider %s", ip, pUUID))
				err := nodeInstanceDelete(ctx, c, cUUID, pUUID, ip)
				if err != nil {
					return diag.FromErr(err)
				}
			} else {
				tflog.Info(ctx, fmt.Sprintf("Node %s in use, removing skipped", ip))
			}
		}
		// Can't add IPs that already exist
		if d.Get("node_instances").(interface{}) != nil {
			nodeInstancesInterface := d.Get("node_instances").([]interface{})
			existingNodeInstancesAfterDelete, err := nodeInstancesRead(ctx, c, cUUID, pUUID)
			if err != nil {
				return diag.FromErr(err)
			}
			var existingIPs []string
			existingIPs = make([]string, 0)
			for _, ip := range existingNodeInstancesAfterDelete {
				details := ip.GetDetails()
				existingIPs = append(existingIPs, details.GetIp())
			}
			// remove the existing nodes from interface before create
			nodeInstanceCreateList := make([]interface{}, 0)
			for _, n := range nodeInstancesInterface {
				node := n.(map[string]interface{})
				if !slices.Contains(existingIPs, node["ip"].(string)) {
					nodeInstanceCreateList = append(nodeInstanceCreateList, node)
				}
			}
			if len(nodeInstanceCreateList) > 0 {
				tflog.Info(ctx, fmt.Sprintf("Adding node instances to provider %s", pUUID))
				_, err = nodeInstancesCreate(ctx, c, cUUID, pUUID, nodeInstanceCreateList)
				if err != nil {
					return diag.FromErr(err)
				}
			}
		}
	}

	return resourceOnPremProviderRead(ctx, d, meta)
}

func resourceOnPremProviderDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) (
	diag.Diagnostics) {
	// delete provider
	// instance type, node instances, regions and az are deleted due to foreign key constraint
	var diags diag.Diagnostics

	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID

	pUUID := d.Id()
	_, response, err := c.CloudProvidersApi.Delete(ctx, cUUID, pUUID).Execute()

	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"On Prem Provider", "Delete")
		return diag.FromErr(errMessage)
	}

	d.SetId("")
	return diags
}

func editOnpremYBAVersionCheck(ctx context.Context, c *client.APIClient) (bool, string, error) {
	allowedVersions := []string{utils.YBAAllowEditProviderMinVersion}
	allowed, version, err := utils.CheckValidYBAVersion(ctx, c, allowedVersions)
	if err != nil {
		return false, "", err
	}
	return allowed, version, err
}
