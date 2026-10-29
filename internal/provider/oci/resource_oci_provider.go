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

package oci

import (
	"context"
	"fmt"

	"github.com/hashicorp/go-cty/cty"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	client "github.com/yugabyte/platform-go-client"

	"github.com/yugabyte/terraform-provider-yba/internal/provider/providerutil"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

const inUseRestriction = "YugabyteDB Anywhere rejects changes to this field " +
	"while a universe uses the provider."

// ResourceOCIProvider creates and maintains an OCI cloud provider.
func ResourceOCIProvider() *schema.Resource {
	return &schema.Resource{
		Description: "OCI Cloud Provider Resource. Use this resource to create and manage " +
			"Oracle Cloud Infrastructure providers in YugabyteDB Anywhere.\n\n" +
			"~> **Note:** This resource requires YugabyteDB Anywhere 2026.1.2.0 or later. " +
			"`terraform plan` fails on an older YBA.",

		CreateContext: resourceOCIProviderCreate,
		ReadContext:   resourceOCIProviderRead,
		UpdateContext: resourceOCIProviderUpdate,
		DeleteContext: resourceOCIProviderDelete,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Timeouts:      providerutil.DefaultTimeouts,
		Schema:        ociProviderSchema(),
		CustomizeDiff: validateOCIProvider,
	}
}

func ociProviderSchema() map[string]*schema.Schema {
	s := providerutil.CommonProviderSchema()

	s["compartment_id"] = &schema.Schema{
		Type:     schema.TypeString,
		Required: true,
		Description: "OCID of the OCI compartment YugabyteDB Anywhere uses for this provider. " +
			inUseRestriction,
	}
	s["default_region"] = &schema.Schema{
		Type:        schema.TypeString,
		Required:    true,
		Description: "Default OCI region for the provider, for example `us-ashburn-1`.",
	}
	s["use_instance_principal"] = &schema.Schema{
		Type:     schema.TypeBool,
		Optional: true,
		Default:  false,
		Description: "Authenticate as the instance principal of the YugabyteDB Anywhere host " +
			"when true. An explicit false is API-key authentication and is set together with " +
			"`tenancy_id`, `user_id`, `fingerprint`, and exactly one of `private_key_content` " +
			"or `private_key_file_path`. Omit those fields when this is true. " +
			"Default is false, which sends API_KEY authentication. " + inUseRestriction,
	}
	s["tenancy_id"] = &schema.Schema{
		Type:         schema.TypeString,
		Optional:     true,
		RequiredWith: []string{"user_id", "fingerprint"},
		Description: "OCID of the OCI tenancy. Required for API-key authentication. " +
			"Omit it when `use_instance_principal` is true. " + inUseRestriction,
	}
	s["user_id"] = &schema.Schema{
		Type:         schema.TypeString,
		Optional:     true,
		RequiredWith: []string{"tenancy_id", "fingerprint"},
		Description: "OCID of the OCI user whose API key YugabyteDB Anywhere uses. " +
			"Required for API-key authentication. " +
			"Omit it when `use_instance_principal` is true. " + inUseRestriction,
	}
	s["fingerprint"] = &schema.Schema{
		Type:         schema.TypeString,
		Optional:     true,
		RequiredWith: []string{"tenancy_id", "user_id"},
		Description: "Fingerprint of the API public key. YugabyteDB Anywhere returns it unmasked. " +
			"Required for API-key authentication. " +
			"Omit it when `use_instance_principal` is true. " + inUseRestriction,
	}
	s["private_key_content"] = &schema.Schema{
		Type:          schema.TypeString,
		Optional:      true,
		Sensitive:     true,
		ConflictsWith: []string{"private_key_file_path"},
		Description: "OCI API private key in PEM form. Set exactly one of `private_key_content` " +
			"or `private_key_file_path` for API-key authentication, including when " +
			"`use_instance_principal` is explicitly false. Omit both when " +
			"`use_instance_principal` is true. " +
			"YugabyteDB Anywhere masks this on every provider response, so Terraform keeps " +
			"the configured value. Changing it updates the provider in place.\n\n" +
			"~> **Security Note:** This value is stored in Terraform state. Use an encrypted backend.",
	}
	s["private_key_file_path"] = &schema.Schema{
		Type:          schema.TypeString,
		Optional:      true,
		ConflictsWith: []string{"private_key_content"},
		Description: "Path of an OCI API private key PEM file. The provider reads the file " +
			"when the plan is applied and sends the contents as the API key. A leading `~` " +
			"is expanded to the home directory. Set exactly one of `private_key_content` or " +
			"`private_key_file_path` for API-key authentication, including when " +
			"`use_instance_principal` is explicitly false. Omit both when " +
			"`use_instance_principal` is true. The path is not sent to YugabyteDB Anywhere " +
			"and is not read back. Overwriting the file at the same path plans an update " +
			"only when another auth field changes in that apply, because Terraform diffs " +
			"the path and not the file body. To rotate the key by itself, change the path " +
			"or set `private_key_content`, after removing the key argument from any " +
			"`lifecycle.ignore_changes`.",
	}
	s["hosted_zone_id"] = &schema.Schema{
		Type:     schema.TypeString,
		Optional: true,
		Description: "OCID of the OCI DNS zone used as the hosted zone. " +
			inUseRestriction,
	}
	s["vpc_type"] = &schema.Schema{
		Type:        schema.TypeString,
		Computed:    true,
		Description: "VPC type reported by YugabyteDB Anywhere. Read-only.",
	}
	s["ssh_keypair_name"] = &schema.Schema{
		Type:     schema.TypeString,
		Optional: true,
		Description: "Custom SSH key pair name to access YugabyteDB nodes. " +
			"Must be set together with ssh_private_key_content. " +
			"If both are omitted, YugabyteDB Anywhere generates and manages the key pair. " +
			"YBA versions keys on every update: if a key with this name already exists " +
			"it appends a timestamp. Use access_key_code to read the versioned name that was stored. " +
			"This value is not read back.",
	}
	s["ssh_private_key_content"] = &schema.Schema{
		Type:         schema.TypeString,
		Optional:     true,
		Sensitive:    true,
		RequiredWith: []string{"ssh_keypair_name"},
		Description: "SSH private key content to access YugabyteDB nodes. " +
			"Must be set together with ssh_keypair_name. " +
			"If both are omitted, YugabyteDB Anywhere generates and manages the key pair. " +
			"This value is not read back.\n\n" +
			"~> **Security Note:** This value is stored in Terraform state. Use an encrypted backend.",
	}
	s["enable_node_agent"] = &schema.Schema{
		Type:        schema.TypeBool,
		Computed:    true,
		Description: "Flag indicating if node agent is enabled for this provider. Read-only.",
	}
	s["regions"] = ociRegionsSchema()
	s["image_bundles"] = ociImageBundleSchema()
	s["yba_managed_image_bundles"] = ociYBADefaultImageBundleSchema()
	return s
}

func ociYBADefaultImageBundleSchema() *schema.Schema {
	return &schema.Schema{
		Description: "YBA managed image bundles for the provider. " +
			"At least one image_bundles or yba_managed_image_bundles block must be specified. " +
			"OCI supports up to two entries: one for x86_64 and one for aarch64. " +
			"Omit this block to stop managing YBA default images via Terraform " +
			"(any previously tracked bundles will be removed from the provider on the next apply).",
		Type:     schema.TypeList,
		Optional: true,
		MaxItems: 2,
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				"uuid": {
					Type:        schema.TypeString,
					Computed:    true,
					Description: "Image bundle UUID.",
				},
				"name": {
					Type:        schema.TypeString,
					Computed:    true,
					Description: "Image bundle name assigned by YBA.",
				},
				"arch": {
					Type:         schema.TypeString,
					Required:     true,
					Description:  "Image bundle architecture. Allowed values: x86_64, aarch64.",
					ValidateFunc: validation.StringInSlice([]string{"x86_64", "aarch64"}, false),
				},
				"use_as_default": {
					Type:             schema.TypeBool,
					Optional:         true,
					Default:          false,
					DiffSuppressFunc: providerutil.SuppressYBABundleDefaultDrift,
					Description: "Flag indicating if the image bundle should be used as default " +
						"for this architecture. When no bundle for a given architecture has this " +
						"set to true, YBA automatically promotes the first bundle as default. " +
						"Terraform will suppress the resulting true->false drift in the plan.",
				},
			},
		},
	}
}

func ociImageBundleSchema() *schema.Schema {
	return &schema.Schema{
		Description: "Custom image bundles for the OCI provider. " +
			"At least one image_bundles or yba_managed_image_bundles block must be specified. " +
			"Every bundle must supply a non-empty image OCID in region_overrides for each " +
			"configured region.",
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
					Type:     schema.TypeString,
					Computed: true,
					Description: "Bundle type: YBA_ACTIVE (YBA-managed), YBA_DEPRECATED, " +
						"or CUSTOM.",
				},
				"name": {
					Type:        schema.TypeString,
					Required:    true,
					Description: "Name of the image bundle.",
				},
				"use_as_default": {
					Type:             schema.TypeBool,
					Optional:         true,
					Default:          false,
					DiffSuppressFunc: providerutil.SuppressCustomBundleDefaultDrift,
					Description: "Flag indicating if the image bundle should be used as default " +
						"for this architecture. When no bundle for a given architecture has this " +
						"set to true, YBA automatically promotes the first bundle as default. " +
						"Terraform will suppress the resulting true->false drift in the plan.",
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
									[]string{"x86_64", "aarch64"}, false),
							},
							"ssh_user": {
								Type:     schema.TypeString,
								Required: true,
								Description: "SSH user for the image. " +
									"YugabyteDB Anywhere's own default is `opc`.",
							},
							"ssh_port": {
								Type:        schema.TypeInt,
								Optional:    true,
								Computed:    true,
								Description: "SSH port for the image. Default is 22.",
							},
							"region_overrides": {
								Type:             schema.TypeMap,
								Optional:         true,
								Elem:             &schema.Schema{Type: schema.TypeString},
								DiffSuppressFunc: suppressInactiveRegionOverride,
								Description: "Per-region image OCIDs. The key is the region code " +
									"(for example `us-ashburn-1`) and the value is the image OCID. " +
									"A non-empty image must be provided for every region configured " +
									"in the provider.",
							},
						},
					},
					Description: "Image bundle details: architecture, SSH settings, and " +
						"per-region image OCIDs.",
				},
			},
		},
	}
}

func ociRegionsSchema() *schema.Schema {
	return &schema.Schema{
		Type:        schema.TypeList,
		Required:    true,
		Description: "OCI regions associated with the provider.",
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
					DiffSuppressFunc: suppressIfOCIRegionsPureReorder,
					Description:      "OCI region code, for example `us-ashburn-1`.",
				},
				"name": {
					Type:        schema.TypeString,
					Computed:    true,
					Description: "Region name reported by YugabyteDB Anywhere. Read-only.",
				},
				"vcn_id": {
					Type:             schema.TypeString,
					Required:         true,
					DiffSuppressFunc: suppressIfOCIRegionsPureReorder,
					Description:      "OCID of the VCN for this region.",
				},
				"instance_configuration_id": {
					Type:             schema.TypeString,
					Optional:         true,
					DiffSuppressFunc: suppressIfOCIRegionsPureReorder,
					Description:      "OCID of the instance configuration for nodes in this region.",
				},
				"zones": ociZonesSchema(),
			},
		},
	}
}

func ociZonesSchema() *schema.Schema {
	return &schema.Schema{
		Type:        schema.TypeList,
		Required:    true,
		Description: "Availability domains in this region.",
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
					DiffSuppressFunc: suppressIfOCIRegionsPureReorder,
					Description: "Availability domain code in YugabyteDB Anywhere's region " +
						"metadata, for example `us-ashburn-1-AD-1`.",
				},
				"name": {
					Type:        schema.TypeString,
					Computed:    true,
					Description: "Availability domain name reported by YugabyteDB Anywhere. Read-only.",
				},
				"subnet": {
					Type:             schema.TypeString,
					Required:         true,
					DiffSuppressFunc: suppressIfOCIRegionsPureReorder,
					Description:      "OCID of the subnet for this availability domain.",
				},
			},
		},
	}
}

func ociCloudInputFromResource(d *schema.ResourceData) ociCloudInput {
	in := ociCloudInput{
		CompartmentID:        d.Get("compartment_id").(string),
		DefaultRegion:        d.Get("default_region").(string),
		HostedZoneID:         d.Get("hosted_zone_id").(string),
		UseInstancePrincipal: d.Get("use_instance_principal").(bool),
		TenancyID:            d.Get("tenancy_id").(string),
		UserID:               d.Get("user_id").(string),
		Fingerprint:          d.Get("fingerprint").(string),
		PrivateKeyContent:    d.Get("private_key_content").(string),
		PrivateKeyFilePath:   d.Get("private_key_file_path").(string),
	}
	applyConfiguredOCIKey(&in, d.GetRawConfig())
	return in
}

// stringFromRawConfig reads one configured argument. The bool is false when
// raw config is missing or the value is still unknown. A known null is empty.
func stringFromRawConfig(raw cty.Value, name string) (string, bool) {
	if raw.IsNull() || !raw.IsKnown() || !raw.Type().IsObjectType() ||
		!raw.Type().HasAttribute(name) {
		return "", false
	}
	v := raw.GetAttr(name)
	if !v.IsKnown() {
		return "", false
	}
	if v.IsNull() || v.Type() != cty.String {
		return "", true
	}
	return v.AsString(), true
}

// applyConfiguredOCIKey replaces the key fields with the configured values.
// After an import, ignore_changes keeps the empty key fields in the plan, so
// d.Get is empty there; an auth update still has to send the configured key.
func applyConfiguredOCIKey(in *ociCloudInput, raw cty.Value) {
	if raw.IsNull() || !raw.IsKnown() {
		return
	}
	if content, ok := stringFromRawConfig(raw, "private_key_content"); ok {
		in.PrivateKeyContent = content
	}
	if path, ok := stringFromRawConfig(raw, "private_key_file_path"); ok {
		in.PrivateKeyFilePath = path
	}
}

func resourceOCIProviderCreate(
	ctx context.Context, d *schema.ResourceData, meta interface{},
) diag.Diagnostics {
	c, cUUID := providerutil.GetAPIClient(meta)
	providerName := d.Get("name").(string)

	ociCloudInfo, err := buildOCICloudInfo(ociCloudInputFromResource(d))
	if err != nil {
		return diag.FromErr(err)
	}
	regionsRaw, _ := d.Get("regions").([]interface{})
	var imageBundles []client.ImageBundle
	if v := d.Get("image_bundles"); v != nil && len(v.([]interface{})) > 0 {
		imageBundles = append(imageBundles, buildOCIImageBundles(v.([]interface{}))...)
	}
	if v := d.Get("yba_managed_image_bundles"); v != nil && len(v.([]interface{})) > 0 {
		imageBundles = append(imageBundles,
			providerutil.BuildYBADefaultImageBundles(v.([]interface{}), "oci")...)
	}
	imageBundles = providerutil.EnsureImageBundleDefaults(imageBundles)
	imageBundles = ensureOCIRegionEntries(imageBundles, collectRegionCodes(regionsRaw))

	req := client.Provider{
		Code: utils.GetStringPointer(providerutil.OCIProviderCode),
		Name: utils.GetStringPointer(providerName),
		AllAccessKeys: buildOCIAccessKeys(
			d.Get("ssh_keypair_name").(string), d.Get("ssh_private_key_content").(string)),
		Regions:      buildOCIRegions(regionsRaw),
		ImageBundles: imageBundles,
		Details: &client.ProviderDetails{
			AirGapInstall: utils.GetBoolPointer(d.Get("air_gap_install").(bool)),
			SetUpChrony:   utils.GetBoolPointer(d.Get("set_up_chrony").(bool)),
			NtpServers:    providerutil.GetNTPServers(d.Get("ntp_servers")),
			CloudInfo:     &client.CloudInfo{Oci: ociCloudInfo},
		},
	}
	r, response, err := c.CloudProvidersAPI.CreateProviders(ctx, cUUID).
		CreateProviderRequest(req).Execute()
	if err != nil {
		return diag.FromErr(utils.ErrorFromHTTPResponse(
			response, err, utils.ResourceEntity, "OCI Provider", "Create"))
	}
	if r.TaskUUID != nil {
		err = providerutil.WaitForProviderTask(ctx, *r.TaskUUID, providerName, "created",
			c, cUUID, d.Timeout(schema.TimeoutCreate))
		if err != nil {
			return diag.FromErr(err)
		}
	}
	d.SetId(*r.ResourceUUID)
	return resourceOCIProviderRead(ctx, d, meta)
}

func resourceOCIProviderRead(
	ctx context.Context, d *schema.ResourceData, meta interface{},
) diag.Diagnostics {
	var diags diag.Diagnostics
	c, cUUID := providerutil.GetAPIClient(meta)
	p, err := providerutil.GetProvider(ctx, c, cUUID, d.Id())
	if err != nil {
		if providerutil.IsProviderNotFoundError(err) {
			tflog.Warn(ctx, fmt.Sprintf(
				"OCI Provider %s not found, removing from state: %v", d.Id(), err))
			d.SetId("")
			return diags
		}
		return diag.FromErr(err)
	}
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
	if err = d.Set("enable_node_agent", details.GetEnableNodeAgent()); err != nil {
		return diag.FromErr(err)
	}
	cloudInfo := details.GetCloudInfo()
	creds := flattenOCICredentials(cloudInfo.GetOci())
	for _, key := range []string{
		"compartment_id", "default_region", "hosted_zone_id", "use_instance_principal",
		"tenancy_id", "user_id", "fingerprint", "vpc_type",
	} {
		if err = d.Set(key, creds[key]); err != nil {
			return diag.FromErr(err)
		}
	}
	stateRegions, _ := d.Get("regions").([]interface{})
	alignedRegions := providerutil.AlignRegions(flattenOCIRegions(p.GetRegions()), stateRegions)
	if err = d.Set("regions", alignedRegions); err != nil {
		return diag.FromErr(err)
	}
	stateBundles, _ := d.Get("image_bundles").([]interface{})
	alignedBundles := providerutil.AlignImageBundles(
		flattenOCIImageBundles(p.GetImageBundles()), stateBundles)
	if err = d.Set("image_bundles", alignedBundles); err != nil {
		return diag.FromErr(err)
	}
	stateYBA, _ := d.Get("yba_managed_image_bundles").([]interface{})
	alignedYBA := providerutil.AlignYBADefaultImageBundles(
		providerutil.FlattenYBADefaultImageBundles(p.GetImageBundles()), stateYBA)
	if err = d.Set("yba_managed_image_bundles", alignedYBA); err != nil {
		return diag.FromErr(err)
	}
	if latest := providerutil.LatestAccessKey(p.GetAllAccessKeys()); latest != nil {
		keyInfo := latest.GetKeyInfo()
		if err = d.Set("access_key_code", keyInfo.GetKeyPairName()); err != nil {
			return diag.FromErr(err)
		}
	}
	return diags
}

// sshNameConflict rejects a rename onto a name that YBA already stores. A
// content change under the same name is not a conflict: YBA versions that key.
// An empty old name is an import (Read does not set the name), so the name in
// YBA is this provider's own key.
func sshNameConflict(oldName, newName string, existing []client.AccessKey) error {
	if oldName == "" || oldName == newName || newName == "" {
		return nil
	}
	return providerutil.ValidateSSHKeypairNameUnique(existing, newName)
}

func resourceOCIProviderUpdate(
	ctx context.Context, d *schema.ResourceData, meta interface{},
) (diags diag.Diagnostics) {
	c, cUUID := providerutil.GetAPIClient(meta)
	p, err := providerutil.GetProvider(ctx, c, cUUID, d.Id())
	if err != nil {
		return diag.FromErr(err)
	}
	defer func() {
		readDiags := resourceOCIProviderRead(ctx, d, meta)
		if !diags.HasError() {
			diags = append(diags, readDiags...)
		}
	}()

	providerReq := *p
	providerName := d.Get("name").(string)
	if d.HasChange("name") {
		providerReq.SetName(providerName)
	}
	oldRegionsRaw, newRegionsRaw := d.GetChange("regions")
	oldRegions, _ := oldRegionsRaw.([]interface{})
	newRegions, _ := newRegionsRaw.([]interface{})
	providerReq.SetRegions(mergeRegionUUIDs(oldRegions, newRegions))

	if d.HasChange("air_gap_install") || d.HasChange("ntp_servers") ||
		d.HasChange("set_up_chrony") {
		details := providerReq.GetDetails()
		details.SetAirGapInstall(d.Get("air_gap_install").(bool))
		details.SetSetUpChrony(d.Get("set_up_chrony").(bool))
		details.SetNtpServers(providerutil.GetNTPServers(d.Get("ntp_servers")))
		providerReq.SetDetails(details)
	}

	authChanged := d.HasChange("use_instance_principal") || d.HasChange("tenancy_id") ||
		d.HasChange("user_id") || d.HasChange("fingerprint") ||
		d.HasChange("private_key_content") || d.HasChange("private_key_file_path")
	if authChanged || d.HasChange("compartment_id") || d.HasChange("default_region") ||
		d.HasChange("hosted_zone_id") {
		details := providerReq.GetDetails()
		cloudInfo := details.GetCloudInfo()
		ociInfo := cloudInfo.GetOci()
		if d.HasChange("compartment_id") {
			ociInfo.SetOciCompartmentId(d.Get("compartment_id").(string))
		}
		if d.HasChange("default_region") {
			ociInfo.SetOciRegion(d.Get("default_region").(string))
		}
		if d.HasChange("hosted_zone_id") {
			ociInfo.SetOciHostedZoneId(d.Get("hosted_zone_id").(string))
		}
		if authChanged {
			keyChanged := d.HasChange("private_key_content") || d.HasChange("private_key_file_path")
			if err = setOCIAuthForUpdate(
				&ociInfo,
				ociCloudInputFromResource(d),
				keyChanged,
			); err != nil {
				utils.RevertFields(d, writeOnlyOCIFields...)
				return diag.FromErr(err)
			}
		}
		cloudInfo.SetOci(ociInfo)
		details.SetCloudInfo(cloudInfo)
		providerReq.SetDetails(details)
	}

	if d.HasChange("ssh_keypair_name") || d.HasChange("ssh_private_key_content") {
		oldName, newNameRaw := d.GetChange("ssh_keypair_name")
		newName := newNameRaw.(string)
		if vErr := sshNameConflict(
			oldName.(string), newName, p.GetAllAccessKeys(),
		); vErr != nil {
			utils.RevertFields(d, writeOnlyOCIFields...)
			return diag.FromErr(vErr)
		}
		providerReq.SetAllAccessKeys(buildOCIAccessKeys(
			newName, d.Get("ssh_private_key_content").(string)))
	}

	oldBundlesRaw, newBundlesRaw := d.GetChange("image_bundles")
	ybaConfigRaw, _ := d.Get("yba_managed_image_bundles").([]interface{})
	updatedBundles := providerutil.PrepareImageBundlesForUpdate(
		p.GetImageBundles(), oldBundlesRaw, newBundlesRaw,
		ybaConfigRaw, d.HasChange("yba_managed_image_bundles"))
	updateRegionsRaw, _ := d.Get("regions").([]interface{})
	providerReq.SetImageBundles(ensureOCIRegionEntries(
		updatedBundles, collectRegionCodes(updateRegionsRaw)))

	r, response, err := c.CloudProvidersAPI.EditProvider(ctx, cUUID, d.Id()).
		EditProviderRequest(providerReq).Execute()
	if err != nil {
		utils.RevertFields(d, writeOnlyOCIFields...)
		return diag.FromErr(utils.ErrorFromHTTPResponse(
			response, err, utils.ResourceEntity, "OCI Provider", "Update"))
	}
	if r.TaskUUID != nil {
		err = providerutil.WaitForProviderTask(ctx, *r.TaskUUID, providerName, "updated",
			c, cUUID, d.Timeout(schema.TimeoutUpdate))
		if err != nil {
			return diag.FromErr(err)
		}
	}
	return
}

var writeOnlyOCIFields = []string{
	"private_key_content", "private_key_file_path",
	"ssh_keypair_name", "ssh_private_key_content",
}

func resourceOCIProviderDelete(
	ctx context.Context, d *schema.ResourceData, meta interface{},
) diag.Diagnostics {
	var diags diag.Diagnostics
	c, cUUID := providerutil.GetAPIClient(meta)
	providerName := d.Get("name").(string)
	r, response, err := c.CloudProvidersAPI.Delete(ctx, cUUID, d.Id()).Execute()
	if err != nil {
		// YBA answers a missing provider UUID with a 400, so confirm through the list.
		if _, getErr := providerutil.GetProvider(ctx, c, cUUID, d.Id()); providerutil.
			IsProviderNotFoundError(getErr) {
			d.SetId("")
			return diags
		}
		return diag.FromErr(utils.ErrorFromHTTPResponse(
			response, err, utils.ResourceEntity, "OCI Provider", "Delete"))
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
