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

// Package azure provides Terraform resource for Azure cloud provider
// following patterns from yba-cli cmd/provider/azu
package azure

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	client "github.com/yugabyte/platform-go-client"
	"github.com/yugabyte/terraform-provider-yba/internal/provider/providerutil"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// ResourceAzureProvider creates and maintains Azure cloud provider resource
// Following yba-cli pattern: yba provider azu create/update/delete
func ResourceAzureProvider() *schema.Resource {
	return &schema.Resource{
		Description: "Azure Cloud Provider Resource. " +
			"Use this resource to create and manage Azure cloud providers in YugabyteDB Anywhere.",

		CreateContext: resourceAzureProviderCreate,
		ReadContext:   resourceAzureProviderRead,
		UpdateContext: resourceAzureProviderUpdate,
		DeleteContext: resourceAzureProviderDelete,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Timeouts: providerutil.DefaultTimeouts,

		Schema: azureProviderSchema(),
	}
}

func azureProviderSchema() map[string]*schema.Schema {
	// Start with common provider schema
	s := providerutil.CommonProviderSchema()

	// Add Azure-specific fields following yba-cli azu create flags
	s["client_id"] = &schema.Schema{
		Type:        schema.TypeString,
		Optional:    true,
		Description: "Azure Client ID for service principal authentication.",
	}
	s["client_secret"] = &schema.Schema{
		Type:         schema.TypeString,
		Optional:     true,
		Sensitive:    true,
		RequiredWith: []string{"client_id"},
		Description: "Azure Client Secret. Required with client_id. " +
			"Stored in Terraform state - use an encrypted backend for security.",
	}
	s["subscription_id"] = &schema.Schema{
		Type:         schema.TypeString,
		Optional:     true,
		RequiredWith: []string{"client_id"},
		Description:  "Azure Subscription ID. Required with client_id.",
	}
	s["tenant_id"] = &schema.Schema{
		Type:         schema.TypeString,
		Optional:     true,
		RequiredWith: []string{"client_id"},
		Description:  "Azure Tenant ID. Required with client_id.",
	}
	s["resource_group"] = &schema.Schema{
		Type:         schema.TypeString,
		Optional:     true,
		RequiredWith: []string{"client_id"},
		Description:  "Azure Resource Group. Required with client_id.",
	}
	s["hosted_zone_id"] = &schema.Schema{
		Type:        schema.TypeString,
		Optional:    true,
		Description: "Private DNS Zone for Azure.",
	}
	// Read-only Azure fields
	s["vpc_type"] = &schema.Schema{
		Type:        schema.TypeString,
		Computed:    true,
		Description: "VPC type: EXISTING or NEW. Read-only.",
	}
	s["network_subscription_id"] = &schema.Schema{
		Type:     schema.TypeString,
		Optional: true,
		Description: "Azure Network Subscription ID. " +
			"All network resources and NIC resources of VMs will be created in this subscription.",
	}
	s["network_resource_group"] = &schema.Schema{
		Type:     schema.TypeString,
		Optional: true,
		Description: "Azure Network Resource Group. " +
			"All network resources and NIC resources of VMs will be created in this group.",
	}

	// SSH configuration
	s["ssh_keypair_name"] = &schema.Schema{
		Type:     schema.TypeString,
		Optional: true,
		Description: "Custom SSH key pair name to access YugabyteDB nodes. " +
			"If not provided, YugabyteDB Anywhere will generate key pairs.",
	}
	s["ssh_private_key_content"] = &schema.Schema{
		Type:         schema.TypeString,
		Optional:     true,
		Sensitive:    true,
		RequiredWith: []string{"ssh_keypair_name"},
		Description:  "SSH private key content to access YugabyteDB nodes.",
	}

	// Common read-only fields
	s["enable_node_agent"] = &schema.Schema{
		Type:        schema.TypeBool,
		Computed:    true,
		Description: "Flag indicating if node agent is enabled for this provider. Read-only.",
	}

	// Regions and zones
	s["regions"] = azureRegionsSchema()

	// Image bundles
	s["image_bundles"] = providerutil.ImageBundleSchema()

	return s
}

func azureRegionsSchema() *schema.Schema {
	return &schema.Schema{
		Type:        schema.TypeList,
		Required:    true,
		Description: "Azure regions associated with the provider.",
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				"uuid": {
					Type:        schema.TypeString,
					Computed:    true,
					Description: "Region UUID.",
				},
				"code": {
					Type:        schema.TypeString,
					Computed:    true,
					Description: "Region code.",
				},
				"name": {
					Type:        schema.TypeString,
					Required:    true,
					Description: "Azure region name (e.g., westus2).",
				},
				"vnet": {
					Type:        schema.TypeString,
					Optional:    true,
					Description: "Virtual network name for this region.",
				},
				"security_group_id": {
					Type:        schema.TypeString,
					Optional:    true,
					Description: "Network security group ID for this region.",
				},
				"resource_group": {
					Type:        schema.TypeString,
					Optional:    true,
					Description: "Resource group for this region.",
				},
				"network_resource_group": {
					Type:        schema.TypeString,
					Optional:    true,
					Description: "Network resource group for this region.",
				},
				"zones": azureZonesSchema(),
			},
		},
	}
}

func azureZonesSchema() *schema.Schema {
	return &schema.Schema{
		Type:        schema.TypeList,
		Required:    true,
		Description: "Availability zones in this region.",
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				"uuid": {
					Type:        schema.TypeString,
					Computed:    true,
					Description: "Zone UUID.",
				},
				"code": {
					Type:        schema.TypeString,
					Computed:    true,
					Description: "Zone code.",
				},
				"name": {
					Type:        schema.TypeString,
					Required:    true,
					Description: "Azure availability zone name.",
				},
				"subnet": {
					Type:        schema.TypeString,
					Required:    true,
					Description: "Subnet for this zone.",
				},
				"secondary_subnet": {
					Type:        schema.TypeString,
					Optional:    true,
					Description: "Secondary subnet for this zone.",
				},
			},
		},
	}
}

func resourceAzureProviderCreate(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{},
) diag.Diagnostics {
	c, cUUID := providerutil.GetAPIClient(meta)

	// Version check
	allowed, version, err := providerutil.ProviderYBAVersionCheck(ctx, c)
	if err != nil {
		return diag.FromErr(err)
	}
	if !allowed {
		return diag.FromErr(fmt.Errorf(
			"creating Azure providers is not supported below version %s, currently on %s",
			utils.YBAAllowEditProviderMinVersion, version))
	}

	providerName := d.Get("name").(string)

	// Build Azure cloud info
	azureCloudInfo, err := buildAzureCloudInfo(d)
	if err != nil {
		return diag.FromErr(err)
	}

	// Build access keys
	accessKeys := buildAzureAccessKeys(d)

	// Build regions
	regions := buildAzureRegions(d.Get("regions").([]interface{}))

	// Build image bundles
	var imageBundles []client.ImageBundle
	if v := d.Get("image_bundles"); v != nil && len(v.([]interface{})) > 0 {
		imageBundleAllowed, _, err := providerutil.ImageBundlesYBAVersionCheck(ctx, c)
		if err != nil {
			return diag.FromErr(err)
		}
		if !imageBundleAllowed {
			return diag.FromErr(fmt.Errorf(
				"image bundles are not supported below version %s",
				utils.YBAAllowImageBundlesMinVersion))
		}
		imageBundles = providerutil.BuildImageBundles(v.([]interface{}))
	}

	// Build provider request
	req := client.Provider{
		Code:          utils.GetStringPointer(providerutil.AzureProviderCode),
		Name:          utils.GetStringPointer(providerName),
		AllAccessKeys: accessKeys,
		Regions:       regions,
		ImageBundles:  imageBundles,
		Details: &client.ProviderDetails{
			AirGapInstall: utils.GetBoolPointer(d.Get("air_gap_install").(bool)),
			SetUpChrony:   utils.GetBoolPointer(d.Get("set_up_chrony").(bool)),
			NtpServers:    providerutil.GetNTPServers(d.Get("ntp_servers")),
			CloudInfo: &client.CloudInfo{
				Azu: azureCloudInfo,
			},
		},
	}

	// Create provider
	r, response, err := c.CloudProvidersAPI.CreateProviders(ctx, cUUID).
		CreateProviderRequest(req).Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"Azure Provider", "Create")
		return diag.FromErr(errMessage)
	}

	d.SetId(*r.ResourceUUID)

	// Wait for task
	if r.TaskUUID != nil {
		err = providerutil.WaitForProviderTask(ctx, *r.TaskUUID, providerName, "created",
			c, cUUID, d.Timeout(schema.TimeoutCreate))
		if err != nil {
			return diag.FromErr(err)
		}
	}

	return resourceAzureProviderRead(ctx, d, meta)
}

func resourceAzureProviderRead(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{},
) diag.Diagnostics {
	var diags diag.Diagnostics
	c, cUUID := providerutil.GetAPIClient(meta)

	p, err := providerutil.GetProvider(ctx, c, cUUID, d.Id())
	if err != nil {
		return diag.FromErr(err)
	}

	// Set common fields
	if err = d.Set("name", p.GetName()); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("version", p.GetVersion()); err != nil {
		return diag.FromErr(err)
	}

	details := p.GetDetails()
	if err = d.Set("air_gap_install", details.GetAirGapInstall()); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("ntp_servers", details.GetNtpServers()); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("set_up_chrony", details.GetSetUpChrony()); err != nil {
		return diag.FromErr(err)
	}

	// Set enable_node_agent (read-only)
	if err = d.Set("enable_node_agent", details.GetEnableNodeAgent()); err != nil {
		return diag.FromErr(err)
	}

	// Set Azure-specific fields
	cloudInfo := details.GetCloudInfo()
	azureInfo := cloudInfo.GetAzu()
	if err = d.Set("subscription_id", azureInfo.GetAzuSubscriptionId()); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("resource_group", azureInfo.GetAzuRG()); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("tenant_id", azureInfo.GetAzuTenantId()); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("client_id", azureInfo.GetAzuClientId()); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("hosted_zone_id", azureInfo.GetAzuHostedZoneId()); err != nil {
		return diag.FromErr(err)
	}
	// Read-only Azure fields
	if err = d.Set("vpc_type", string(azureInfo.GetVpcType())); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("network_subscription_id", azureInfo.GetAzuNetworkSubscriptionId()); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("network_resource_group", azureInfo.GetAzuNetworkRG()); err != nil {
		return diag.FromErr(err)
	}

	// Set regions
	if err = d.Set("regions", flattenAzureRegions(p.GetRegions())); err != nil {
		return diag.FromErr(err)
	}

	// Set image bundles
	imageBundles := providerutil.FlattenImageBundles(p.GetImageBundles())
	if err = d.Set("image_bundles", imageBundles); err != nil {
		return diag.FromErr(err)
	}

	// Set access key code (read-only)
	accessKeys := p.GetAllAccessKeys()
	if len(accessKeys) > 0 {
		keyInfo := accessKeys[0].GetKeyInfo()
		if err = d.Set("access_key_code", keyInfo.GetKeyPairName()); err != nil {
			return diag.FromErr(err)
		}
	}

	return diags
}

func resourceAzureProviderUpdate(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{},
) diag.Diagnostics {
	c, cUUID := providerutil.GetAPIClient(meta)

	allowed, version, err := providerutil.ProviderYBAVersionCheck(ctx, c)
	if err != nil {
		return diag.FromErr(err)
	}
	if !allowed {
		return diag.FromErr(fmt.Errorf(
			"editing Azure providers is not supported below version %s, currently on %s",
			utils.YBAAllowEditProviderMinVersion, version))
	}

	p, err := providerutil.GetProvider(ctx, c, cUUID, d.Id())
	if err != nil {
		return diag.FromErr(err)
	}

	providerReq := *p
	providerName := d.Get("name").(string)

	if d.HasChange("name") {
		providerReq.SetName(providerName)
	}

	// Update provider details if changed (mirrors yba-cli update logic)
	if d.HasChange("air_gap_install") || d.HasChange("ntp_servers") || d.HasChange("set_up_chrony") {
		details := providerReq.GetDetails()
		details.SetAirGapInstall(d.Get("air_gap_install").(bool))
		details.SetSetUpChrony(d.Get("set_up_chrony").(bool))
		details.SetNtpServers(providerutil.GetNTPServers(d.Get("ntp_servers")))
		providerReq.SetDetails(details)
	}

	// Update Azure cloud info if credentials or network settings changed
	if d.HasChange("client_id") || d.HasChange("client_secret") ||
		d.HasChange("tenant_id") || d.HasChange("subscription_id") ||
		d.HasChange("resource_group") || d.HasChange("hosted_zone_id") ||
		d.HasChange("network_subscription_id") || d.HasChange("network_resource_group") {
		details := providerReq.GetDetails()
		cloudInfo := details.GetCloudInfo()
		azuInfo := cloudInfo.GetAzu()

		// Update credentials
		if d.HasChange("client_id") {
			azuInfo.SetAzuClientId(d.Get("client_id").(string))
		}
		if d.HasChange("client_secret") {
			azuInfo.SetAzuClientSecret(d.Get("client_secret").(string))
		}
		if d.HasChange("tenant_id") {
			azuInfo.SetAzuTenantId(d.Get("tenant_id").(string))
		}
		if d.HasChange("subscription_id") {
			azuInfo.SetAzuSubscriptionId(d.Get("subscription_id").(string))
		}
		if d.HasChange("resource_group") {
			azuInfo.SetAzuRG(d.Get("resource_group").(string))
		}

		// Update network settings
		if d.HasChange("hosted_zone_id") {
			azuInfo.SetAzuHostedZoneId(d.Get("hosted_zone_id").(string))
		}
		if d.HasChange("network_subscription_id") {
			azuInfo.SetAzuNetworkSubscriptionId(d.Get("network_subscription_id").(string))
		}
		if d.HasChange("network_resource_group") {
			azuInfo.SetAzuNetworkRG(d.Get("network_resource_group").(string))
		}

		cloudInfo.SetAzu(azuInfo)
		details.SetCloudInfo(cloudInfo)
		providerReq.SetDetails(details)
	}

	if d.HasChange("regions") {
		providerReq.SetRegions(buildAzureRegions(d.Get("regions").([]interface{})))
	}

	// Update image bundles if changed and provided
	// Note: We only update if bundles are explicitly provided. Removing image_bundles
	// from config doesn't delete existing bundles (YBA auto-generates defaults).
	if d.HasChange("image_bundles") {
		if v := d.Get("image_bundles"); v != nil && len(v.([]interface{})) > 0 {
			imageBundleAllowed, _, err := providerutil.ImageBundlesYBAVersionCheck(ctx, c)
			if err != nil {
				return diag.FromErr(err)
			}
			if !imageBundleAllowed {
				return diag.FromErr(fmt.Errorf(
					"image bundles are not supported below version %s",
					utils.YBAAllowImageBundlesMinVersion))
			}
			providerReq.SetImageBundles(providerutil.BuildImageBundles(v.([]interface{})))
		}
		// If image_bundles is empty/removed, we don't clear them - YBA manages defaults
	}

	r, response, err := c.CloudProvidersAPI.EditProvider(ctx, cUUID, d.Id()).
		EditProviderRequest(providerReq).Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"Azure Provider", "Update")
		return diag.FromErr(errMessage)
	}

	if r.TaskUUID != nil {
		err = providerutil.WaitForProviderTask(ctx, *r.TaskUUID, providerName, "updated",
			c, cUUID, d.Timeout(schema.TimeoutUpdate))
		if err != nil {
			return diag.FromErr(err)
		}
	}

	return resourceAzureProviderRead(ctx, d, meta)
}

func resourceAzureProviderDelete(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{},
) diag.Diagnostics {
	var diags diag.Diagnostics
	c, cUUID := providerutil.GetAPIClient(meta)

	providerName := d.Get("name").(string)

	r, response, err := c.CloudProvidersAPI.Delete(ctx, cUUID, d.Id()).Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"Azure Provider", "Delete")
		return diag.FromErr(errMessage)
	}

	if r.TaskUUID != nil {
		err = providerutil.WaitForProviderTask(ctx, *r.TaskUUID, providerName, "deleted",
			c, cUUID, d.Timeout(schema.TimeoutDelete))
		if err != nil {
			return diag.FromErr(err)
		}
	}

	d.SetId("")
	return diags
}
