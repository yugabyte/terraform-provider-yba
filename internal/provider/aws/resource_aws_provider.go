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

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
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

		Schema:        awsProviderSchema(),
		CustomizeDiff: validateNoDuplicateRegionsOrZones,
	}
}

func awsProviderSchema() map[string]*schema.Schema {
	// Start with common provider schema
	s := providerutil.CommonProviderSchema()

	// Add AWS-specific fields following yba-cli aws create flags
	s["access_key_id"] = &schema.Schema{
		Type:          schema.TypeString,
		Optional:      true,
		Sensitive:     true,
		ConflictsWith: []string{"use_iam_instance_profile"},
		Description: "AWS Access Key ID. Required for non-IAM role based providers. " +
			"Stored in Terraform state - use an encrypted backend for security.",
	}
	s["secret_access_key"] = &schema.Schema{
		Type:          schema.TypeString,
		Optional:      true,
		Sensitive:     true,
		RequiredWith:  []string{"access_key_id"},
		ConflictsWith: []string{"use_iam_instance_profile"},
		Description: "AWS Secret Access Key. Required with access_key_id. " +
			"Stored in Terraform state - use an encrypted backend for security.",
	}
	s["use_iam_instance_profile"] = &schema.Schema{
		Type:          schema.TypeBool,
		Optional:      true,
		Default:       false,
		ConflictsWith: []string{"access_key_id", "secret_access_key"},
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
			"Must be set together with ssh_private_key_content (self-managed mode). " +
			"If both ssh_keypair_name and ssh_private_key_content are omitted, " +
			"YugabyteDB Anywhere generates and manages the key pair (YBA-managed mode). " +
			"YBA versions keys on every update: if a key with this name already exists " +
			"it appends a timestamp (e.g. 'my-key-2026-03-18-10-01-29'). " +
			"Use access_key_code to read the actual versioned name that was stored.",
	}
	s["ssh_private_key_content"] = &schema.Schema{
		Type:         schema.TypeString,
		Optional:     true,
		Sensitive:    true,
		RequiredWith: []string{"ssh_keypair_name"},
		Description: "SSH private key content to access YugabyteDB nodes. " +
			"Must be set together with ssh_keypair_name (self-managed mode). " +
			"If both fields are omitted, YugabyteDB Anywhere generates and manages " +
			"the key pair (YBA-managed mode).",
	}
	s["skip_ssh_keypair_validation"] = &schema.Schema{
		Type:     schema.TypeBool,
		Optional: true,
		Default:  false,
		Description: "Skip SSH keypair validation and upload to AWS. " +
			"Only applies in self-managed mode (when ssh_keypair_name and " +
			"ssh_private_key_content are set). Use when the keypair already exists " +
			"in your AWS account and you do not want to grant YBA describe/create " +
			"keypair permissions. Default is false.",
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

	s["yba_managed_image_bundles"] = providerutil.YBADefaultImageBundleSchema()

	return s
}

// awsImageBundleSchema returns AWS-specific image bundle schema with region overrides.
// NOTE: global_yb_image is intentionally absent. AWS requires per-region AMI IDs via
// region_overrides; a single global image is not a valid concept for AWS providers.
// Every custom bundle must specify a non-empty AMI for each configured region.
func awsImageBundleSchema() *schema.Schema {
	return &schema.Schema{
		Description: "Custom image bundles for the AWS provider. " +
			"At least one image_bundles or yba_managed_image_bundles block must be specified. " +
			"Every bundle must supply a non-empty AMI in region_overrides for each configured region.",
		Type:     schema.TypeList,
		Optional: true,
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				"uuid": {
					Type:        schema.TypeString,
					Computed:    true,
					Description: "Image bundle UUID.",
				},
				"metadata_type": {
					Type:        schema.TypeString,
					Computed:    true,
					Description: "Bundle type: YBA_ACTIVE (YBA-managed), YBA_DEPRECATED, or CUSTOM.",
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
								ValidateFunc: validation.StringInSlice(
									[]string{"x86_64", "aarch64"},
									false,
								),
							},
							"ssh_user": {
								Type:        schema.TypeString,
								Required:    true,
								Description: "SSH user for the image.",
							},
							"ssh_port": {
								Type:        schema.TypeInt,
								Optional:    true,
								Computed:    true,
								Description: "SSH port for the image. Default is 22.",
							},
							"use_imds_v2": {
								Type:     schema.TypeBool,
								Optional: true,
								Default:  true,
								Description: "Use IMDS v2 for the image. Default is true. " +
									"Set to false to allow IMDSv1 (not recommended). " +
									"Note: Terraform may show a cosmetic plan-time warning for this field " +
									"when omitted from config - this is a known legacy SDK limitation " +
									"and does not affect behaviour.",
							},
							"region_overrides": {
								Type:     schema.TypeMap,
								Optional: true,
								Elem:     &schema.Schema{Type: schema.TypeString},
								Description: "Per-region AMI IDs for AWS. " +
									"Key is the region code (e.g. us-east-1), value is the AMI ID. " +
									"A non-empty AMI must be provided for every region configured in " +
									"the provider. Validation enforces this at plan time.",
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

// awsRegionsSchema returns the schema for AWS regions.
// NOTE: Using TypeList instead of TypeSet for simpler change detection.
// Order changes show in plan but don't trigger version updates.
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
					Type:             schema.TypeString,
					Required:         true,
					DiffSuppressFunc: suppressIfAWSRegionsPureReorder,
					Description:      "AWS region code (e.g., us-west-2, us-east-1).",
				},
				"name": {
					Type:        schema.TypeString,
					Computed:    true,
					Description: "AWS region name. Read-only.",
				},
				"vpc_id": {
					Type:             schema.TypeString,
					Optional:         true,
					DiffSuppressFunc: suppressIfAWSRegionsPureReorder,
					Description:      "VPC ID for this region.",
				},
				"security_group_id": {
					Type:             schema.TypeString,
					Optional:         true,
					DiffSuppressFunc: suppressIfAWSRegionsPureReorder,
					Description:      "Security group ID for this region.",
				},
				"zones": awsZonesSchema(),
			},
		},
	}
}

// awsZonesSchema returns the schema for AWS availability zones.
// NOTE: Using TypeList instead of TypeSet for simpler change detection.
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
					Type:             schema.TypeString,
					Required:         true,
					DiffSuppressFunc: suppressIfAWSRegionsPureReorder,
					Description:      "AWS availability zone code (e.g., us-west-2a, us-east-1b).",
				},
				"name": {
					Type:        schema.TypeString,
					Computed:    true,
					Description: "AWS availability zone name. Read-only.",
				},
				"subnet": {
					Type:             schema.TypeString,
					Required:         true,
					DiffSuppressFunc: suppressIfAWSRegionsPureReorder,
					Description:      "Subnet ID for this zone.",
				},
				"secondary_subnet": {
					Type:             schema.TypeString,
					Optional:         true,
					DiffSuppressFunc: suppressIfAWSRegionsPureReorder,
					Description:      "Secondary subnet ID for this zone.",
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

	providerName := d.Get("name").(string)

	// Build AWS cloud info (yba-cli: awsCloudInfo construction)
	awsCloudInfo, err := buildAWSCloudInfo(d)
	if err != nil {
		return diag.FromErr(err)
	}

	// Build access keys
	accessKeys := buildAWSAccessKeys(d)

	// Build regions (yba-cli: buildAWSRegions) - TypeList returns []interface{}
	regionsRaw, _ := d.Get("regions").([]interface{})
	regions := buildAWSRegions(regionsRaw)

	// Build image bundles with AWS-specific region overrides - TypeList returns []interface{}
	var imageBundles []client.ImageBundle
	if v := d.Get("image_bundles"); v != nil && len(v.([]interface{})) > 0 {
		imageBundles = append(imageBundles, buildAWSImageBundles(v.([]interface{}))...)
	}
	if v := d.Get("yba_managed_image_bundles"); v != nil && len(v.([]interface{})) > 0 {
		imageBundles = append(
			imageBundles,
			providerutil.BuildYBADefaultImageBundles(v.([]interface{}), "aws")...)
	}
	imageBundles = providerutil.EnsureImageBundleDefaults(imageBundles)

	// verifyImageBundleDetails requires an entry per region in every bundle's regions map.
	regionCodes := make([]string, 0, len(regionsRaw))
	for _, r := range regionsRaw {
		if regionMap, ok := r.(map[string]interface{}); ok {
			if code, ok := regionMap["code"].(string); ok && code != "" {
				regionCodes = append(regionCodes, code)
			}
		}
	}
	imageBundles = ensureAWSRegionEntries(imageBundles, regionCodes)

	// Build provider request (mirrors yba-cli requestBody construction)
	req := client.Provider{
		Code:          utils.GetStringPointer(providerutil.AWSProviderCode),
		Name:          utils.GetStringPointer(providerName),
		AllAccessKeys: accessKeys,
		Regions:       regions,
		ImageBundles:  imageBundles,
		Details: &client.ProviderDetails{
			AirGapInstall: utils.GetBoolPointer(d.Get("air_gap_install").(bool)),
			SetUpChrony:   utils.GetBoolPointer(d.Get("set_up_chrony").(bool)),
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
		// If the provider was deleted outside of Terraform, remove it from state
		// so that Terraform can recreate it on the next apply.
		if providerutil.IsProviderNotFoundError(err) {
			tflog.Warn(
				ctx,
				fmt.Sprintf("AWS Provider %s not found, removing from state: %v", d.Id(), err),
			)
			d.SetId("")
			return diags
		}
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
	// - ssh_private_key_content: Sensitive, not returned by the API
	// These fields are "write-only" - we preserve what's in the config/state

	// Set regions
	// Align regions with state/config to preserve order and prevent unexpected diff warnings
	stateRegions, _ := d.Get("regions").([]interface{})
	alignedRegions := providerutil.AlignRegions(flattenAWSRegions(p.GetRegions()), stateRegions)
	if err = d.Set("regions", alignedRegions); err != nil {
		return diag.FromErr(err)
	}

	// Set image bundles
	stateBundles, _ := d.Get("image_bundles").([]interface{})
	stateYBAManagedBundles, _ := d.Get("yba_managed_image_bundles").([]interface{})

	flattenedBundles := flattenAWSImageBundles(p.GetImageBundles())
	alignedBundles := providerutil.AlignImageBundles(flattenedBundles, stateBundles)
	if err = d.Set("image_bundles", alignedBundles); err != nil {
		return diag.FromErr(err)
	}

	// Always read from API so re-added bundles are picked up even when state was empty.
	flattenedYBAManagedBundles := providerutil.FlattenYBADefaultImageBundles(p.GetImageBundles())
	alignedYBAManagedBundles := providerutil.AlignYBADefaultImageBundles(
		flattenedYBAManagedBundles,
		stateYBAManagedBundles,
	)
	if err = d.Set("yba_managed_image_bundles", alignedYBAManagedBundles); err != nil {
		return diag.FromErr(err)
	}

	// Set access_key_code from the API response (read-only).
	// ssh_keypair_name and ssh_private_key_content are intentionally NOT read back:
	// - YBA versions keys on every update by appending a timestamp to the name
	//   (e.g. "my-key" -> "my-key-2026-03-18-10-01-29"), so reading back the stored
	//   name would cause a perpetual diff against the user's base name in config.
	// - ssh_private_key_content is sensitive and not returned by the API.
	// Use access_key_code to see the actual versioned name YBA assigned.
	if latest := providerutil.LatestAccessKey(p.GetAllAccessKeys()); latest != nil {
		keyInfo := latest.GetKeyInfo()
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
) (diags diag.Diagnostics) {
	c, cUUID := providerutil.GetAPIClient(meta)

	// Fetch current provider state
	p, err := providerutil.GetProvider(ctx, c, cUUID, d.Id())
	if err != nil {
		return diag.FromErr(err)
	}

	// Always refresh state before returning. On success, Read errors are
	// propagated. On failure, they are swallowed so the original error is preserved.
	defer func() {
		readDiags := resourceAWSProviderRead(ctx, d, meta)
		if !diags.HasError() {
			diags = append(diags, readDiags...)
		}
	}()

	providerReq := *p
	providerName := d.Get("name").(string)

	// Update fields that changed
	if d.HasChange("name") {
		providerReq.SetName(providerName)
	}

	// Use d.GetChange to get old state (with UUIDs) and new config
	// Merge UUIDs from old_state into new_config
	// TypeList returns []interface{} directly
	oldRegionsRaw, newRegionsRaw := d.GetChange("regions")
	oldRegions, _ := oldRegionsRaw.([]interface{})
	newRegions, _ := newRegionsRaw.([]interface{})
	mergedRegions := mergeRegionUUIDs(oldRegions, newRegions)

	providerReq.SetRegions(mergedRegions)

	// Update provider details if changed
	if d.HasChange("air_gap_install") || d.HasChange("ntp_servers") ||
		d.HasChange("set_up_chrony") {
		details := providerReq.GetDetails()
		details.SetAirGapInstall(d.Get("air_gap_install").(bool))
		details.SetSetUpChrony(d.Get("set_up_chrony").(bool))
		details.SetNtpServers(providerutil.GetNTPServers(d.Get("ntp_servers")))
		providerReq.SetDetails(details)
	}

	// Always patch cloud info to clear the deprecated provider-level useIMDSv2
	// (Java default = true forces all bundles to true via a backwards-compat block).
	{
		details := providerReq.GetDetails()
		cloudInfo := details.GetCloudInfo()
		awsInfo := cloudInfo.GetAws()

		// Disable the deprecated provider-level IMDSv2 override.
		awsInfo.SetUseIMDSv2(false)

		// Update credentials if changed
		if d.HasChange("access_key_id") || d.HasChange("secret_access_key") ||
			d.HasChange("use_iam_instance_profile") {
			isIAM := d.Get("use_iam_instance_profile").(bool)
			if isIAM {
				awsInfo.SetAwsAccessKeyID("")
				awsInfo.SetAwsAccessKeySecret("")
			} else {
				awsInfo.SetAwsAccessKeyID(d.Get("access_key_id").(string))
				awsInfo.SetAwsAccessKeySecret(d.Get("secret_access_key").(string))
			}
		}

		// Update hosted zone if changed
		if d.HasChange("hosted_zone_id") {
			awsInfo.SetAwsHostedZoneId(d.Get("hosted_zone_id").(string))
		}

		cloudInfo.SetAws(awsInfo)
		details.SetCloudInfo(cloudInfo)
		providerReq.SetDetails(details)
	}

	// Update SSH keys if changed
	// Per YBA API: To create/update a self-managed key, send an AccessKey WITHOUT IdKey
	// and WITH sshPrivateKeyContent. If IdKey is present, YBA treats it as no-op.
	if d.HasChange("ssh_keypair_name") || d.HasChange("ssh_private_key_content") ||
		d.HasChange("skip_ssh_keypair_validation") {
		providerReq.SetAllAccessKeys(buildAWSAccessKeys(d))
	}

	oldBundlesRaw, newBundlesRaw := d.GetChange("image_bundles")
	ybaConfigRaw, _ := d.Get("yba_managed_image_bundles").([]interface{})
	updatedBundles := providerutil.PrepareImageBundlesForUpdate(
		p.GetImageBundles(), oldBundlesRaw, newBundlesRaw,
		ybaConfigRaw, d.HasChange("yba_managed_image_bundles"),
	)
	// Ensure per-region entries exist in every bundle (required by verifyImageBundleDetails).
	updateRegionsRaw, _ := d.Get("regions").([]interface{})
	updateRegionCodes := make([]string, 0, len(updateRegionsRaw))
	for _, r := range updateRegionsRaw {
		if regionMap, ok := r.(map[string]interface{}); ok {
			if code, ok := regionMap["code"].(string); ok && code != "" {
				updateRegionCodes = append(updateRegionCodes, code)
			}
		}
	}
	providerReq.SetImageBundles(ensureAWSRegionEntries(updatedBundles, updateRegionCodes))

	// Execute update (yba-cli: authAPI.EditProvider())
	r, response, err := c.CloudProvidersAPI.EditProvider(ctx, cUUID, d.Id()).
		EditProviderRequest(providerReq).Execute()
	if err != nil {
		utils.RevertFields(d,
			"ssh_keypair_name", "ssh_private_key_content",
			"access_key_id", "secret_access_key",
			"skip_ssh_keypair_validation",
		)
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"AWS Provider", "Update")
		return diag.FromErr(errMessage)
	}

	if r.TaskUUID != nil {
		err = providerutil.WaitForProviderTask(ctx, *r.TaskUUID, providerName, "updated",
			c, cUUID, d.Timeout(schema.TimeoutUpdate))
		if err != nil {
			// CloudProviderEdit task is not atomic: updateProviderData and
			// updateAccessKeys commit to the DB before updateRegionsAndZones
			// and runSubTasks. Do not revert write-only fields here because
			// some may already be persisted on the server. Read (via defer)
			// reflects actual server state for all API-readable fields.
			return diag.FromErr(err)
		}
	}

	return
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
