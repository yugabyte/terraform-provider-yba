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

// Package aws provides Terraform resource for AWS cloud provider
// following patterns from yba-cli cmd/provider/aws
package aws

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	client "github.com/yugabyte/platform-go-client"
	"github.com/yugabyte/terraform-provider-yba/internal/provider/providerutil"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// ResourceAWSProvider creates and maintains AWS cloud provider resource
// Following yba-cli pattern: yba provider aws create/update/delete
func ResourceAWSProvider() *schema.Resource {
	return &schema.Resource{
		Description: "AWS Cloud Provider Resource. " +
			"Use this resource to create and manage AWS cloud providers in YugabyteDB Anywhere.",

		CreateContext: resourceAWSProviderCreate,
		ReadContext:   resourceAWSProviderRead,
		UpdateContext: resourceAWSProviderUpdate,
		DeleteContext: resourceAWSProviderDelete,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Timeouts: providerutil.DefaultTimeouts,

		Schema: awsProviderSchema(),
	}
}

func awsProviderSchema() map[string]*schema.Schema {
	// Start with common provider schema
	s := providerutil.CommonProviderSchema()

	// Add AWS-specific fields following yba-cli aws create flags
	s["access_key_id"] = &schema.Schema{
		Type:      schema.TypeString,
		Optional:  true,
		Sensitive: true,
		Description: "AWS Access Key ID. Required for non-IAM role based providers. " +
			"Stored in Terraform state - use an encrypted backend for security.",
	}
	s["secret_access_key"] = &schema.Schema{
		Type:         schema.TypeString,
		Optional:     true,
		Sensitive:    true,
		RequiredWith: []string{"access_key_id"},
		Description: "AWS Secret Access Key. Required with access_key_id. " +
			"Stored in Terraform state - use an encrypted backend for security.",
	}
	s["use_iam_instance_profile"] = &schema.Schema{
		Type:     schema.TypeBool,
		Optional: true,
		Default:  false,
		Description: "Use IAM Role from the YugabyteDB Anywhere host. " +
			"Provider creation will fail on insufficient permissions. Default is false.",
	}
	s["hosted_zone_id"] = &schema.Schema{
		Type:        schema.TypeString,
		Optional:    true,
		Description: "Hosted Zone ID corresponding to Amazon Route53.",
	}
	// Read-only fields from AWS cloud info
	s["hosted_zone_name"] = &schema.Schema{
		Type:        schema.TypeString,
		Computed:    true,
		Description: "Hosted Zone Name corresponding to Amazon Route53. Read-only.",
	}
	s["host_vpc_region"] = &schema.Schema{
		Type:        schema.TypeString,
		Computed:    true,
		Description: "AWS Host VPC Region. Read-only, populated by YBA.",
	}
	s["host_vpc_id"] = &schema.Schema{
		Type:        schema.TypeString,
		Computed:    true,
		Description: "AWS Host VPC ID. Read-only, populated by YBA.",
	}
	s["vpc_type"] = &schema.Schema{
		Type:        schema.TypeString,
		Computed:    true,
		Description: "VPC type: EXISTING or NEW. Read-only.",
	}

	// SSH configuration (yba-cli: --custom-ssh-keypair-name, --custom-ssh-keypair-file-path)
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
	s["skip_keypair_validation"] = &schema.Schema{
		Type:        schema.TypeBool,
		Optional:    true,
		Default:     false,
		Description: "Skip SSH keypair validation and upload to AWS. Default is false.",
	}

	// Common read-only fields
	s["enable_node_agent"] = &schema.Schema{
		Type:        schema.TypeBool,
		Computed:    true,
		Description: "Flag indicating if node agent is enabled for this provider. Read-only.",
	}

	// Regions and zones (yba-cli: --region, --zone)
	s["regions"] = awsRegionsSchema()

	// Image bundles (yba-cli: --image-bundle, --image-bundle-region-override)
	s["image_bundles"] = awsImageBundleSchema()

	return s
}

// awsImageBundleSchema returns AWS-specific image bundle schema with region overrides
func awsImageBundleSchema() *schema.Schema {
	return &schema.Schema{
		Description: "Image bundles associated with AWS provider. " +
			"Supported from YugabyteDB Anywhere version: " + utils.YBAAllowImageBundlesMinVersion,
		Type:     schema.TypeList,
		Optional: true,
		Computed: true,
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				"uuid": {
					Type:        schema.TypeString,
					Computed:    true,
					Description: "Image bundle UUID.",
				},
				"name": {
					Type:        schema.TypeString,
					Required:    true,
					Description: "Name of the image bundle.",
				},
				"use_as_default": {
					Type:     schema.TypeBool,
					Optional: true,
					Default:  false,
					Description: "Flag indicating if the image bundle should be " +
						"used as default for this architecture.",
				},
				"details": {
					Type:     schema.TypeList,
					Required: true,
					MaxItems: 1,
					Elem: &schema.Resource{
						Schema: map[string]*schema.Schema{
							"arch": {
								Type:        schema.TypeString,
								Required:    true,
								Description: "Image bundle architecture. Allowed values: x86_64, aarch64.",
							},
							"ssh_user": {
								Type:        schema.TypeString,
								Required:    true,
								Description: "SSH user for the image.",
							},
							"ssh_port": {
								Type:        schema.TypeInt,
								Optional:    true,
								Default:     22,
								Description: "SSH port for the image. Default is 22.",
							},
							"use_imds_v2": {
								Type:     schema.TypeBool,
								Computed: true,
								Description: "Use IMDS v2 for the image. " +
									"This is always true for AWS as IMDSv2 is enforced by YBA for security. " +
									"Read-only field.",
							},
							"global_yb_image": {
								Type:        schema.TypeString,
								Optional:    true,
								Description: "Global YB image for the bundle.",
							},
							"region_overrides": {
								Type:     schema.TypeMap,
								Optional: true,
								Elem:     &schema.Schema{Type: schema.TypeString},
								Description: "Per-region AMI overrides for AWS. " +
									"Provide region code as the key and AMI ID as the value. " +
									"Required: one override per region in the provider.",
							},
						},
					},
					Description: "Image bundle details including architecture, " +
						"SSH configuration, and region overrides.",
				},
			},
		},
	}
}

func awsRegionsSchema() *schema.Schema {
	return &schema.Schema{
		Type:        schema.TypeList,
		Required:    true,
		Description: "AWS regions associated with the provider.",
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
					Description: "AWS region name (e.g., us-west-2).",
				},
				"vpc_id": {
					Type:        schema.TypeString,
					Optional:    true,
					Description: "VPC ID for this region.",
				},
				"security_group_id": {
					Type:        schema.TypeString,
					Optional:    true,
					Description: "Security group ID for this region.",
				},
				"zones": awsZonesSchema(),
			},
		},
	}
}

func awsZonesSchema() *schema.Schema {
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
					Description: "AWS availability zone name (e.g., us-west-2a).",
				},
				"subnet": {
					Type:        schema.TypeString,
					Required:    true,
					Description: "Subnet ID for this zone.",
				},
				"secondary_subnet": {
					Type:        schema.TypeString,
					Optional:    true,
					Description: "Secondary subnet ID for this zone.",
				},
			},
		},
	}
}

func resourceAWSProviderCreate(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{},
) diag.Diagnostics {
	c, cUUID := providerutil.GetAPIClient(meta)

	// Version check (yba-cli: authAPI.NewProviderYBAVersionCheck())
	allowed, version, err := providerutil.ProviderYBAVersionCheck(ctx, c)
	if err != nil {
		return diag.FromErr(err)
	}
	if !allowed {
		return diag.FromErr(fmt.Errorf(
			"creating AWS providers is not supported below version %s, currently on %s",
			utils.YBAAllowEditProviderMinVersion, version))
	}

	providerName := d.Get("name").(string)

	// Build AWS cloud info (yba-cli: awsCloudInfo construction)
	awsCloudInfo, err := buildAWSCloudInfo(d)
	if err != nil {
		return diag.FromErr(err)
	}

	// Build access keys
	accessKeys := buildAWSAccessKeys(d)

	// Build regions (yba-cli: buildAWSRegions)
	regions := buildAWSRegions(d.Get("regions").([]interface{}))

	// Build image bundles with AWS-specific region overrides
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
		imageBundles = buildAWSImageBundles(v.([]interface{}))
	}

	// Build provider request (mirrors yba-cli requestBody construction)
	req := client.Provider{
		Code:          utils.GetStringPointer(providerutil.AWSProviderCode),
		Name:          utils.GetStringPointer(providerName),
		AllAccessKeys: accessKeys,
		Regions:       regions,
		ImageBundles:  imageBundles,
		Details: &client.ProviderDetails{
			AirGapInstall: utils.GetBoolPointer(d.Get("air_gap_install").(bool)),
			NtpServers:    providerutil.GetNTPServers(d.Get("ntp_servers")),
			CloudInfo: &client.CloudInfo{
				Aws: awsCloudInfo,
			},
		},
	}

	// Create provider (yba-cli: authAPI.CreateProvider().Execute())
	r, response, err := c.CloudProvidersAPI.CreateProviders(ctx, cUUID).
		CreateProviderRequest(req).Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"AWS Provider", "Create")
		return diag.FromErr(errMessage)
	}

	d.SetId(*r.ResourceUUID)

	// Wait for task (yba-cli: WaitForCreateProviderTask)
	if r.TaskUUID != nil {
		err = providerutil.WaitForProviderTask(ctx, *r.TaskUUID, providerName, "created",
			c, cUUID, d.Timeout(schema.TimeoutCreate))
		if err != nil {
			return diag.FromErr(err)
		}
	}

	return resourceAWSProviderRead(ctx, d, meta)
}

func resourceAWSProviderRead(
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

	// Set AWS-specific fields
	cloudInfo := details.GetCloudInfo()
	awsInfo := cloudInfo.GetAws()
	if err = d.Set("hosted_zone_id", awsInfo.GetAwsHostedZoneId()); err != nil {
		return diag.FromErr(err)
	}
	// Read-only AWS fields
	if err = d.Set("hosted_zone_name", awsInfo.GetAwsHostedZoneName()); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("host_vpc_region", awsInfo.GetHostVpcRegion()); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("host_vpc_id", awsInfo.GetHostVpcId()); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("vpc_type", string(awsInfo.GetVpcType())); err != nil {
		return diag.FromErr(err)
	}

	// Determine IAM instance profile usage
	// If access key ID is empty, provider uses IAM instance profile
	// If access key ID has any value (even masked), credentials were provided
	useIAM := awsInfo.GetAwsAccessKeyID() == ""
	if err = d.Set("use_iam_instance_profile", useIAM); err != nil {
		return diag.FromErr(err)
	}

	// Note: We intentionally do NOT read these input-only fields from the API:
	// - access_key_id, secret_access_key: Sensitive, returned masked
	// - ssh_keypair_name: YBA generates a name that differs from user input
	// - ssh_private_key_content: Sensitive
	// These fields are "write-only" - we preserve what's in the config/state

	// Set regions
	if err = d.Set("regions", flattenAWSRegions(p.GetRegions())); err != nil {
		return diag.FromErr(err)
	}

	// Set image bundles with AWS-specific fields
	if err = d.Set("image_bundles", flattenAWSImageBundles(p.GetImageBundles())); err != nil {
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

func resourceAWSProviderUpdate(
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
			"editing AWS providers is not supported below version %s, currently on %s",
			utils.YBAAllowEditProviderMinVersion, version))
	}

	// Fetch current provider state
	p, err := providerutil.GetProvider(ctx, c, cUUID, d.Id())
	if err != nil {
		return diag.FromErr(err)
	}

	providerReq := *p
	providerName := d.Get("name").(string)

	// Update fields that changed
	if d.HasChange("name") {
		providerReq.SetName(providerName)
	}

	if d.HasChange("regions") {
		providerReq.SetRegions(buildAWSRegions(d.Get("regions").([]interface{})))
	}

	// Update provider details if changed
	if d.HasChange("air_gap_install") || d.HasChange("ntp_servers") {
		details := providerReq.GetDetails()
		details.SetAirGapInstall(d.Get("air_gap_install").(bool))
		details.SetNtpServers(providerutil.GetNTPServers(d.Get("ntp_servers")))
		providerReq.SetDetails(details)
	}

	// Update AWS cloud info if credentials or hosted zone changed
	// IMPORTANT: We update individual fields on the existing cloud info,
	// NOT replace the entire object, to preserve read-only fields
	if d.HasChange("access_key_id") || d.HasChange("secret_access_key") ||
		d.HasChange("use_iam_instance_profile") || d.HasChange("hosted_zone_id") {
		details := providerReq.GetDetails()
		cloudInfo := details.GetCloudInfo()
		awsInfo := cloudInfo.GetAws()

		// Update credentials
		if d.HasChange("access_key_id") || d.HasChange("secret_access_key") ||
			d.HasChange("use_iam_instance_profile") {
			isIAM := d.Get("use_iam_instance_profile").(bool)
			if isIAM {
				// Clear credentials when using IAM
				awsInfo.SetAwsAccessKeyID("")
				awsInfo.SetAwsAccessKeySecret("")
			} else {
				awsInfo.SetAwsAccessKeyID(d.Get("access_key_id").(string))
				awsInfo.SetAwsAccessKeySecret(d.Get("secret_access_key").(string))
			}
		}

		// Update hosted zone
		if d.HasChange("hosted_zone_id") {
			awsInfo.SetAwsHostedZoneId(d.Get("hosted_zone_id").(string))
		}

		cloudInfo.SetAws(awsInfo)
		details.SetCloudInfo(cloudInfo)
		providerReq.SetDetails(details)
	}

	// Update SSH keys if changed
	if d.HasChange("ssh_keypair_name") || d.HasChange("ssh_private_key_content") ||
		d.HasChange("skip_keypair_validation") {
		providerReq.SetAllAccessKeys(buildAWSAccessKeys(d))
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
			providerReq.SetImageBundles(buildAWSImageBundles(v.([]interface{})))
		}
		// If image_bundles is empty/removed, we don't clear them - YBA manages defaults
	}

	// Execute update (yba-cli: authAPI.EditProvider())
	r, response, err := c.CloudProvidersAPI.EditProvider(ctx, cUUID, d.Id()).
		EditProviderRequest(providerReq).Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"AWS Provider", "Update")
		return diag.FromErr(errMessage)
	}

	if r.TaskUUID != nil {
		err = providerutil.WaitForProviderTask(ctx, *r.TaskUUID, providerName, "updated",
			c, cUUID, d.Timeout(schema.TimeoutUpdate))
		if err != nil {
			return diag.FromErr(err)
		}
	}

	return resourceAWSProviderRead(ctx, d, meta)
}

func resourceAWSProviderDelete(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{},
) diag.Diagnostics {
	var diags diag.Diagnostics
	c, cUUID := providerutil.GetAPIClient(meta)

	providerName := d.Get("name").(string)

	// Delete provider (yba-cli: authAPI.DeleteProvider())
	r, response, err := c.CloudProvidersAPI.Delete(ctx, cUUID, d.Id()).Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"AWS Provider", "Delete")
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
