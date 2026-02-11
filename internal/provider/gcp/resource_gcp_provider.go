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

// Package gcp provides Terraform resource for GCP cloud provider
// following patterns from yba-cli cmd/provider/gcp
package gcp

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	client "github.com/yugabyte/platform-go-client"
	"github.com/yugabyte/terraform-provider-yba/internal/provider/providerutil"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// ResourceGCPProvider creates and maintains GCP cloud provider resource
// Following yba-cli pattern: yba provider gcp create/update/delete
func ResourceGCPProvider() *schema.Resource {
	return &schema.Resource{
		Description: "GCP Cloud Provider Resource. " +
			"Use this resource to create and manage GCP cloud providers in YugabyteDB Anywhere.",

		CreateContext: resourceGCPProviderCreate,
		ReadContext:   resourceGCPProviderRead,
		UpdateContext: resourceGCPProviderUpdate,
		DeleteContext: resourceGCPProviderDelete,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Timeouts: providerutil.DefaultTimeouts,

		Schema: gcpProviderSchema(),
	}
}

func gcpProviderSchema() map[string]*schema.Schema {
	// Start with common provider schema
	s := providerutil.CommonProviderSchema()

	// Add GCP-specific fields following yba-cli gcp create flags
	s["credentials"] = &schema.Schema{
		Type:      schema.TypeString,
		Optional:  true,
		Sensitive: true,
		Description: "Google Service Account credentials JSON content. " +
			"Stored in Terraform state - use an encrypted backend for security.",
	}
	s["use_host_credentials"] = &schema.Schema{
		Type:     schema.TypeBool,
		Optional: true,
		Default:  false,
		Description: "Use credentials from the YugabyteDB Anywhere host. " +
			"Default is false.",
	}
	s["project_id"] = &schema.Schema{
		Type:        schema.TypeString,
		Optional:    true,
		Description: "GCP project ID that hosts universe nodes.",
	}
	s["shared_vpc_project_id"] = &schema.Schema{
		Type:     schema.TypeString,
		Optional: true,
		Description: "Shared VPC project ID. Use this to connect resources " +
			"from multiple GCP projects to a common VPC.",
	}
	s["network"] = &schema.Schema{
		Type:        schema.TypeString,
		Optional:    true,
		Description: "VPC network name in GCP.",
	}
	s["use_host_vpc"] = &schema.Schema{
		Type:     schema.TypeBool,
		Optional: true,
		Default:  false,
		Description: "Use VPC from the YugabyteDB Anywhere host. " +
			"If false, network must be specified. Default is false.",
	}
	s["create_vpc"] = &schema.Schema{
		Type:     schema.TypeBool,
		Optional: true,
		Default:  false,
		Description: "Create a new VPC in GCP. " +
			"If true, network must be specified as the new VPC name. Default is false.",
	}
	s["yb_firewall_tags"] = &schema.Schema{
		Type:        schema.TypeString,
		Optional:    true,
		Description: "Tags for firewall rules in GCP.",
	}
	// Read-only GCP fields
	s["host_vpc_id"] = &schema.Schema{
		Type:        schema.TypeString,
		Computed:    true,
		Description: "GCP Host VPC ID. Read-only, populated by YBA.",
	}
	s["vpc_type"] = &schema.Schema{
		Type:        schema.TypeString,
		Computed:    true,
		Description: "VPC type: EXISTING or NEW. Read-only.",
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
	s["regions"] = gcpRegionsSchema()

	// Image bundles
	s["image_bundles"] = providerutil.ImageBundleSchema()

	return s
}

func gcpRegionsSchema() *schema.Schema {
	return &schema.Schema{
		Type:        schema.TypeList,
		Required:    true,
		Description: "GCP regions associated with the provider.",
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
					Description: "GCP region name (e.g., us-west1).",
				},
				"shared_subnet": {
					Type:     schema.TypeString,
					Optional: true,
					Description: "Shared subnet for all zones in this region. " +
						"YBA will auto-discover zones and apply this subnet to each.",
				},
				"instance_template": {
					Type:        schema.TypeString,
					Optional:    true,
					Description: "Instance template for this region.",
				},
				"zones": gcpZonesSchema(),
			},
		},
	}
}

func gcpZonesSchema() *schema.Schema {
	return &schema.Schema{
		Type:        schema.TypeList,
		Computed:    true,
		Description: "Zones in this region. Auto-discovered by YBA based on the region.",
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
					Computed:    true,
					Description: "GCP zone name (e.g., us-west1-a).",
				},
				"subnet": {
					Type:        schema.TypeString,
					Computed:    true,
					Description: "Subnet for this zone (set via shared_subnet on region).",
				},
			},
		},
	}
}

func resourceGCPProviderCreate(
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
			"creating GCP providers is not supported below version %s, currently on %s",
			utils.YBAAllowEditProviderMinVersion, version))
	}

	providerName := d.Get("name").(string)

	// Build GCP cloud info
	gcpCloudInfo, err := buildGCPCloudInfo(d)
	if err != nil {
		return diag.FromErr(err)
	}

	// Build access keys
	accessKeys := buildGCPAccessKeys(d)

	// Build regions
	regions := buildGCPRegions(d.Get("regions").([]interface{}))

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
		Code:          utils.GetStringPointer(providerutil.GCPProviderCode),
		Name:          utils.GetStringPointer(providerName),
		AllAccessKeys: accessKeys,
		Regions:       regions,
		ImageBundles:  imageBundles,
		Details: &client.ProviderDetails{
			AirGapInstall: utils.GetBoolPointer(d.Get("air_gap_install").(bool)),
			NtpServers:    providerutil.GetNTPServers(d.Get("ntp_servers")),
			CloudInfo: &client.CloudInfo{
				Gcp: gcpCloudInfo,
			},
		},
	}

	// Create provider
	r, response, err := c.CloudProvidersAPI.CreateProviders(ctx, cUUID).
		CreateProviderRequest(req).Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"GCP Provider", "Create")
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

	return resourceGCPProviderRead(ctx, d, meta)
}

func resourceGCPProviderRead(
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

	// Set GCP-specific fields
	cloudInfo := details.GetCloudInfo()
	gcpInfo := cloudInfo.GetGcp()
	if err = d.Set("project_id", gcpInfo.GetGceProject()); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("shared_vpc_project_id", gcpInfo.GetSharedVPCProject()); err != nil {
		return diag.FromErr(err)
	}
	network := gcpInfo.GetDestVpcId()
	if err = d.Set("network", network); err != nil {
		return diag.FromErr(err)
	}
	// Determine create_vpc, use_host_vpc based on API response
	// Mirrors yba-cli/resource_cloud_provider.go Read logic:
	// - API UseHostVPC = false means a new VPC was created (create_vpc = true)
	// - API UseHostVPC = true with network set means using existing custom VPC (use_host_vpc = false)
	// - API UseHostVPC = true without network means using YBA host VPC (use_host_vpc = true)
	apiUseHostVPC := gcpInfo.GetUseHostVPC()
	if !apiUseHostVPC {
		// VPC was created by YBA
		if err = d.Set("create_vpc", true); err != nil {
			return diag.FromErr(err)
		}
		if err = d.Set("use_host_vpc", false); err != nil {
			return diag.FromErr(err)
		}
	} else {
		// Using existing VPC
		if err = d.Set("create_vpc", false); err != nil {
			return diag.FromErr(err)
		}
		// If network is set, user specified a custom VPC
		// If network is empty, using YBA host's VPC
		useHostVPC := network == ""
		if err = d.Set("use_host_vpc", useHostVPC); err != nil {
			return diag.FromErr(err)
		}
	}
	if err = d.Set("use_host_credentials", gcpInfo.GetUseHostCredentials()); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("yb_firewall_tags", gcpInfo.GetYbFirewallTags()); err != nil {
		return diag.FromErr(err)
	}
	// Read-only GCP fields
	if err = d.Set("host_vpc_id", gcpInfo.GetHostVpcId()); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("vpc_type", string(gcpInfo.GetVpcType())); err != nil {
		return diag.FromErr(err)
	}

	// Set regions - includes all zones from API (YBA may auto-discover additional zones)
	if err = d.Set("regions", flattenGCPRegions(p.GetRegions())); err != nil {
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

func resourceGCPProviderUpdate(
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
			"editing GCP providers is not supported below version %s, currently on %s",
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
	if d.HasChange("air_gap_install") || d.HasChange("ntp_servers") {
		details := providerReq.GetDetails()
		details.SetAirGapInstall(d.Get("air_gap_install").(bool))
		details.SetNtpServers(providerutil.GetNTPServers(d.Get("ntp_servers")))
		providerReq.SetDetails(details)
	}

	// Update GCP cloud info if credentials or network settings changed
	if d.HasChange("credentials") || d.HasChange("use_host_credentials") ||
		d.HasChange("network") || d.HasChange("yb_firewall_tags") ||
		d.HasChange("use_host_vpc") || d.HasChange("create_vpc") ||
		d.HasChange("project_id") || d.HasChange("shared_vpc_project_id") {
		details := providerReq.GetDetails()
		cloudInfo := details.GetCloudInfo()
		gcpInfo := cloudInfo.GetGcp()

		// Update credentials
		if d.HasChange("credentials") || d.HasChange("use_host_credentials") {
			useHostCreds := d.Get("use_host_credentials").(bool)
			gcpInfo.SetUseHostCredentials(useHostCreds)
			if useHostCreds {
				gcpInfo.SetGceApplicationCredentials("")
			} else {
				gcpInfo.SetGceApplicationCredentials(d.Get("credentials").(string))
			}
		}

		// Update VPC settings (mirrors yba-cli update_provider.go logic)
		// create_vpc takes precedence over use_host_vpc
		if d.HasChange("create_vpc") {
			createVPC := d.Get("create_vpc").(bool)
			if createVPC {
				gcpInfo.SetUseHostVPC(false)
				network := d.Get("network").(string)
				if network == "" {
					return diag.FromErr(fmt.Errorf("network is required when create_vpc is true"))
				}
				gcpInfo.SetDestVpcId(network)
			}
		}

		// Only handle use_host_vpc if create_vpc is not being set to true
		if !d.Get("create_vpc").(bool) {
			if d.HasChange("use_host_vpc") {
				useHostVPC := d.Get("use_host_vpc").(bool)
				if useHostVPC {
					gcpInfo.SetUseHostVPC(true)
					gcpInfo.SetDestVpcId("")
				} else {
					gcpInfo.SetUseHostVPC(false)
					network := d.Get("network").(string)
					if network == "" {
						return diag.FromErr(fmt.Errorf(
							"network is required when use_host_vpc is false"))
					}
					gcpInfo.SetDestVpcId(network)
				}
			} else if d.HasChange("network") {
				// Network change without VPC mode change
				gcpInfo.SetDestVpcId(d.Get("network").(string))
			}
		}

		if d.HasChange("yb_firewall_tags") {
			gcpInfo.SetYbFirewallTags(d.Get("yb_firewall_tags").(string))
		}
		if d.HasChange("project_id") {
			gcpInfo.SetGceProject(d.Get("project_id").(string))
		}
		if d.HasChange("shared_vpc_project_id") {
			gcpInfo.SetSharedVPCProject(d.Get("shared_vpc_project_id").(string))
		}

		cloudInfo.SetGcp(gcpInfo)
		details.SetCloudInfo(cloudInfo)
		providerReq.SetDetails(details)
	}

	if d.HasChange("regions") {
		providerReq.SetRegions(buildGCPRegions(d.Get("regions").([]interface{})))
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
			"GCP Provider", "Update")
		return diag.FromErr(errMessage)
	}

	if r.TaskUUID != nil {
		err = providerutil.WaitForProviderTask(ctx, *r.TaskUUID, providerName, "updated",
			c, cUUID, d.Timeout(schema.TimeoutUpdate))
		if err != nil {
			return diag.FromErr(err)
		}
	}

	return resourceGCPProviderRead(ctx, d, meta)
}

func resourceGCPProviderDelete(
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
			"GCP Provider", "Delete")
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
