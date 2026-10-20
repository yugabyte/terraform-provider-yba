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

package storageconfig

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/go-cty/cty"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	client "github.com/yugabyte/platform-go-client"

	"github.com/yugabyte/terraform-provider-yba/internal/api"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

const ociStorageConfigType = "OCI"

// ociStorageMin is a build on each release line that contains
// CustomerConfigStorageOCIData (yugabyte-db 03c2a37, PLAT-21449). The OCI
// provider floor (2026.1.2.0-b29 / 2.31.0.0-b133) is earlier and does not
// have this class. The published stable release 2026.1.2.0-b137 does, as
// does preview 2.31.0.0-b386.
var ociStorageMin = utils.YBAMinimumVersion{
	Stable:  "2026.1.2.0-b137",
	Preview: "2.31.0.0-b386",
}

// ociNativeNamespace captures the /n/<namespace>/ segment of a native Object
// Storage URL. s3:// locations do not match.
var ociNativeNamespace = regexp.MustCompile(`^https://objectstorage\.[^/]+/n/([^/]+)`)

// ResourceOCIStorageConfig defines the OCI Object Storage backup configuration.
func ResourceOCIStorageConfig() *schema.Resource {
	return &schema.Resource{
		Description: "OCI Object Storage configuration for YugabyteDB Anywhere backups. " +
			"Use either a Customer Secret Key (S3-compatible access key, secret, and host) " +
			"or instance principal. The two modes are mutually exclusive. Do not put an OCI " +
			"API signing key here; that credential belongs on the OCI provider.\n\n" +
			"~> **Security Note:** `access_key_id` and `secret_access_key` are stored in the " +
			"Terraform state file and marked sensitive. Use a secure state backend and " +
			"restrict access to state files, or set `use_oci_iam`, which keeps the keys " +
			"out of Terraform.\n\n" +
			"~> **Note:** Requires YugabyteDB Anywhere 2026.1.2.0 or later. " +
			"`terraform plan` fails on an older YBA.",

		CreateContext: resourceOCIStorageConfigCreate,
		ReadContext:   resourceOCIStorageConfigRead,
		UpdateContext: resourceOCIStorageConfigUpdate,
		DeleteContext: resourceStorageConfigDelete,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(5 * time.Minute),
			Update: schema.DefaultTimeout(5 * time.Minute),
			Delete: schema.DefaultTimeout(5 * time.Minute),
		},

		CustomizeDiff: resourceOCIStorageConfigCustomizeDiff,

		Schema: map[string]*schema.Schema{
			"name": {
				Type:             schema.TypeString,
				Required:         true,
				DiffSuppressFunc: suppressTrimmedString,
				Description: "Name of the OCI storage configuration. " +
					"YugabyteDB Anywhere trims leading and trailing whitespace.",
			},
			"backup_location": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
				Description: "Bucket URI in s3:// form or native HTTPS form " +
					"(https://objectstorage.<region>.oraclecloud.com/n/<namespace>/b/<bucket>). " +
					"Government and dedicated-region hosts are accepted. " +
					"Changing this value forces resource recreation.",
			},
			"region": {
				Type:     schema.TypeString,
				Required: true,
				Description: "OCI region of the default bucket (for example, us-sanjose-1). " +
					"Also the signing region for S3-compatible credentials.",
			},
			"namespace": {
				Type:     schema.TypeString,
				Optional: true,
				Description: "Object Storage namespace. Required when use_oci_iam is true. " +
					"When set on a native HTTPS backup location, it must match the " +
					"/n/<namespace>/ segment. Optional for S3-compatible credentials.",
			},
			"use_oci_iam": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
				Description: "Authenticate with the instance principal of the YugabyteDB Anywhere " +
					"host and the universe nodes. When true, access_key_id, secret_access_key, " +
					"and host_base must be omitted, and namespace is required. Default: false.",
			},
			"access_key_id": {
				Type:         schema.TypeString,
				Optional:     true,
				Sensitive:    true,
				RequiredWith: []string{"secret_access_key"},
				Description: "Customer Secret Key access key. Required, with secret_access_key " +
					"and host_base, when use_oci_iam is false. Can change in place.",
			},
			"secret_access_key": {
				Type:         schema.TypeString,
				Optional:     true,
				Sensitive:    true,
				RequiredWith: []string{"access_key_id"},
				Description: "Customer Secret Key secret. Required, with access_key_id and " +
					"host_base, when use_oci_iam is false. Can change in place.",
			},
			"host_base": {
				Type:     schema.TypeString,
				Optional: true,
				Description: "S3-compatible endpoint, for example " +
					"namespace.compat.objectstorage.us-sanjose-1.oraclecloud.com. " +
					"Required when use_oci_iam is false. YugabyteDB Anywhere applies " +
					"path-style access for this mode.",
			},
			"region_locations": {
				Type:        schema.TypeList,
				Optional:    true,
				Description: "Region-specific backup locations. Each region appears at most once.",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"region": {
							Type:        schema.TypeString,
							Required:    true,
							Description: "Universe placement region (for example, us-ashburn-1).",
						},
						"location": {
							Type:        schema.TypeString,
							Required:    true,
							Description: "Bucket URI for this region, in s3:// or native HTTPS form.",
						},
						"host_base": {
							Type:     schema.TypeString,
							Optional: true,
							Description: "S3-compatible endpoint for this region. Required on " +
								"every region when use_oci_iam is false, because Object Storage " +
								"buckets are regional.",
						},
					},
				},
			},
			"config_uuid": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "UUID of the storage configuration.",
			},
		},
	}
}

func resourceOCIStorageConfigCustomizeDiff(
	ctx context.Context,
	d *schema.ResourceDiff,
	meta interface{},
) error {
	if err := validateOCIStorageVersion(ctx, meta); err != nil {
		return err
	}
	if err := validateNoDuplicateRegionLocations(ctx, d, meta); err != nil {
		return err
	}
	spec, err := ociSpecFrom(d)
	if err != nil {
		return err
	}
	return validateOCIStorage(spec)
}

func resourceOCIStorageConfigCreate(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{},
) diag.Diagnostics {
	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID

	spec, err := ociSpecFrom(d)
	if err != nil {
		return diag.FromErr(err)
	}
	data, err := buildOCIData(spec)
	if err != nil {
		return diag.FromErr(err)
	}

	req := client.CustomerConfig{
		ConfigName:   strings.TrimSpace(spec.name),
		CustomerUUID: cUUID,
		Data:         data,
		Name:         ociStorageConfigType,
		Type:         "STORAGE",
	}

	r, response, err := c.CustomerConfigurationAPI.CreateCustomerConfig(ctx, cUUID).
		Config(req).
		Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"OCI Storage Config", "Create")
		return diag.FromErr(errMessage)
	}

	d.SetId(*r.ConfigUUID)
	return resourceOCIStorageConfigRead(ctx, d, meta)
}

func resourceOCIStorageConfigRead(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{},
) diag.Diagnostics {
	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID

	r, response, err := c.CustomerConfigurationAPI.GetListOfCustomerConfig(ctx, cUUID).Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"OCI Storage Config", "Read")
		return diag.FromErr(errMessage)
	}

	config, err := findStorageConfig(r, d.Id(), ociStorageConfigType)
	if err != nil {
		if utils.IsResourceNotFoundError(err) {
			tflog.Warn(ctx, fmt.Sprintf(
				"OCI Storage Config %s not found, removing from state: %v", d.Id(), err))
			d.SetId("")
			return nil
		}
		return diag.FromErr(err)
	}

	flat, err := flattenOCIData(config.GetData())
	if err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("name", config.ConfigName); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("config_uuid", config.ConfigUUID); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("backup_location", flat.backupLocation); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("region", flat.region); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("namespace", flat.namespace); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("use_oci_iam", flat.useIAM); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("host_base", flat.hostBase); err != nil {
		return diag.FromErr(err)
	}
	// YBA masks OCI_S3_ACCESS_KEY_ID and OCI_S3_SECRET_ACCESS_KEY on list
	// (a field whose name contains KEY or SECRET). Keep the configured
	// values, so a change to either key is an in-place update.
	locations := make([]map[string]interface{}, 0, len(flat.locations))
	for _, loc := range flat.locations {
		locations = append(locations, map[string]interface{}{
			"region":    loc.region,
			"location":  loc.location,
			"host_base": loc.hostBase,
		})
	}
	if err = d.Set("region_locations", locations); err != nil {
		return diag.FromErr(err)
	}
	return nil
}

func resourceOCIStorageConfigUpdate(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{},
) (diags diag.Diagnostics) {
	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID

	defer func() {
		readDiags := resourceOCIStorageConfigRead(ctx, d, meta)
		if !diags.HasError() {
			diags = append(diags, readDiags...)
		}
	}()

	spec, err := ociSpecFrom(d)
	if err != nil {
		utils.RevertFields(d, "access_key_id", "secret_access_key")
		return diag.FromErr(err)
	}
	data, err := buildOCIData(spec)
	if err != nil {
		utils.RevertFields(d, "access_key_id", "secret_access_key")
		return diag.FromErr(err)
	}

	req := client.CustomerConfig{
		ConfigName:   strings.TrimSpace(spec.name),
		CustomerUUID: cUUID,
		Data:         data,
		Name:         ociStorageConfigType,
		Type:         "STORAGE",
	}

	_, response, err := c.CustomerConfigurationAPI.EditCustomerConfig(ctx, cUUID, d.Id()).
		Config(req).
		Execute()
	if err != nil {
		utils.RevertFields(d, "access_key_id", "secret_access_key")
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"OCI Storage Config", "Update")
		return diag.FromErr(errMessage)
	}
	return
}

type ociConfigSource interface {
	Get(key string) interface{}
	GetRawConfigAt(path cty.Path) (cty.Value, diag.Diagnostics)
}

type ociLocationSpec struct {
	region      string
	location    string
	hostBase    string
	hostUnknown bool
}

type ociStorageSpec struct {
	name             string
	nameUnknown      bool
	backupLocation   string
	region           string
	namespace        string
	namespaceUnknown bool
	useIAM           bool
	useIAMUnknown    bool
	accessKeyID      string
	secretAccessKey  string
	accessKnown      bool
	secretKnown      bool
	hostBase         string
	hostUnknown      bool
	locations        []ociLocationSpec
}

type ociStorageFlat struct {
	backupLocation string
	region         string
	namespace      string
	useIAM         bool
	hostBase       string
	locations      []ociLocationSpec
}

func ociSpecFrom(d ociConfigSource) (ociStorageSpec, error) {
	name, nameKnown, err := rawConfigString(d, cty.GetAttrPath("name"))
	if err != nil {
		return ociStorageSpec{}, err
	}
	namespace, namespaceKnown, err := rawConfigString(d, cty.GetAttrPath("namespace"))
	if err != nil {
		return ociStorageSpec{}, err
	}
	host, hostKnown, err := rawConfigString(d, cty.GetAttrPath("host_base"))
	if err != nil {
		return ociStorageSpec{}, err
	}
	access, accessKnown, err := rawConfigString(d, cty.GetAttrPath("access_key_id"))
	if err != nil {
		return ociStorageSpec{}, err
	}
	secret, secretKnown, err := rawConfigString(d, cty.GetAttrPath("secret_access_key"))
	if err != nil {
		return ociStorageSpec{}, err
	}
	useIAM, useIAMKnown, err := rawConfigBool(d, "use_oci_iam")
	if err != nil {
		return ociStorageSpec{}, err
	}
	backup, _ := d.Get("backup_location").(string)
	region, _ := d.Get("region").(string)
	spec := ociStorageSpec{
		name:             name,
		nameUnknown:      !nameKnown,
		backupLocation:   backup,
		region:           region,
		namespace:        namespace,
		namespaceUnknown: !namespaceKnown,
		useIAM:           useIAM,
		useIAMUnknown:    !useIAMKnown,
		accessKeyID:      access,
		secretAccessKey:  secret,
		accessKnown:      accessKnown,
		secretKnown:      secretKnown,
		hostBase:         host,
		hostUnknown:      !hostKnown,
	}
	rawLocs, _ := d.Get("region_locations").([]interface{})
	for i, raw := range rawLocs {
		if raw == nil {
			continue
		}
		loc := raw.(map[string]interface{})
		locHost, locHostKnown, err := rawConfigString(d, cty.GetAttrPath("region_locations").
			Index(cty.NumberIntVal(int64(i))).GetAttr("host_base"))
		if err != nil {
			return ociStorageSpec{}, err
		}
		locRegion, _ := loc["region"].(string)
		locLocation, _ := loc["location"].(string)
		spec.locations = append(spec.locations, ociLocationSpec{
			region:      locRegion,
			location:    locLocation,
			hostBase:    locHost,
			hostUnknown: !locHostKnown,
		})
	}
	return spec, nil
}

func rawConfigString(d ociConfigSource, path cty.Path) (string, bool, error) {
	v, diags := d.GetRawConfigAt(path)
	if diags.HasError() {
		return "", false, fmt.Errorf("read argument %s: %s", path, diags[0].Summary)
	}
	if v.IsNull() {
		return "", true, nil
	}
	if !v.IsKnown() {
		return "", false, nil
	}
	if v.Type() != cty.String {
		return "", true, nil
	}
	return v.AsString(), true, nil
}

func rawConfigBool(d ociConfigSource, name string) (bool, bool, error) {
	v, diags := d.GetRawConfigAt(cty.GetAttrPath(name))
	if diags.HasError() {
		return false, false, fmt.Errorf("read argument %s: %s", name, diags[0].Summary)
	}
	if v.IsNull() {
		b, _ := d.Get(name).(bool)
		return b, true, nil
	}
	if !v.IsKnown() {
		return false, false, nil
	}
	if v.Type() != cty.Bool {
		return false, true, nil
	}
	return v.True(), true, nil
}

func validateOCIStorageVersion(ctx context.Context, meta interface{}) error {
	c, ok := meta.(*api.APIClient)
	if !ok || c == nil {
		return nil
	}
	version, err := c.AppVersion(ctx)
	if err != nil {
		return err
	}
	return ociStorageVersionError(ctx, version)
}

func ociStorageVersionError(ctx context.Context, version string) error {
	if version == "" {
		return nil
	}
	ok, applied, err := utils.MeetsMinimum(version, ociStorageMin)
	switch {
	case err != nil:
		tflog.Warn(ctx, "cannot parse the YBA version; skipping the minimum-version check",
			map[string]interface{}{"version": version, "check": "yba_oci_storage_config"})
	case !ok:
		return fmt.Errorf(
			"yba_oci_storage_config requires YugabyteDB Anywhere %s or later "+
				"(stable: %s, preview: %s); the target YBA reports %s",
			applied, ociStorageMin.Stable, ociStorageMin.Preview, version)
	}
	return nil
}

func validateOCIStorage(spec ociStorageSpec) error {
	if !spec.nameUnknown && strings.TrimSpace(spec.name) == "" {
		return fmt.Errorf("name must not be empty")
	}
	// An unknown value is computed in this apply. Require only a known empty
	// value; buildOCIData checks again at apply, when every value is known.
	switch {
	case spec.useIAMUnknown:
		// The mode is computed in this apply. buildOCIData checks it at apply.
	case spec.useIAM:
		if !spec.namespaceUnknown && spec.namespace == "" {
			return fmt.Errorf("namespace is required when use_oci_iam is true")
		}
		if (!spec.hostUnknown && spec.hostBase != "") ||
			(spec.accessKnown && spec.accessKeyID != "") ||
			(spec.secretKnown && spec.secretAccessKey != "") {
			return fmt.Errorf(
				"access_key_id, secret_access_key, and host_base conflict with use_oci_iam")
		}
	default:
		if spec.accessKnown && spec.accessKeyID == "" {
			return fmt.Errorf("access_key_id is required when use_oci_iam is false")
		}
		if spec.secretKnown && spec.secretAccessKey == "" {
			return fmt.Errorf("secret_access_key is required when use_oci_iam is false")
		}
		if !spec.hostUnknown && spec.hostBase == "" {
			return fmt.Errorf("host_base is required when use_oci_iam is false")
		}
	}
	err := namespaceMatchesLocation(spec.namespace, spec.backupLocation, "backup_location")
	if err != nil {
		return err
	}
	for _, loc := range spec.locations {
		if !spec.useIAMUnknown && !spec.useIAM && !loc.hostUnknown && loc.hostBase == "" {
			return fmt.Errorf(
				"region_locations.host_base is required for region %q when use_oci_iam is false",
				loc.region)
		}
		if err := namespaceMatchesLocation(
			spec.namespace, loc.location, "region_locations.location"); err != nil {
			return err
		}
	}
	return nil
}

func namespaceMatchesLocation(namespace, location, field string) error {
	if namespace == "" || location == "" {
		return nil
	}
	match := ociNativeNamespace.FindStringSubmatch(location)
	if match == nil || match[1] == namespace {
		return nil
	}
	return fmt.Errorf(
		"namespace %q must match namespace %q in %s", namespace, match[1], field)
}

func buildOCIData(spec ociStorageSpec) (map[string]interface{}, error) {
	if err := validateOCIStorage(spec); err != nil {
		return nil, err
	}
	data := map[string]interface{}{
		"BACKUP_LOCATION": spec.backupLocation,
		"OCI_REGION":      spec.region,
		"USE_OCI_IAM":     spec.useIAM,
	}
	if spec.namespace != "" {
		data["OCI_NAMESPACE"] = spec.namespace
	}
	if !spec.useIAM {
		data["OCI_S3_ACCESS_KEY_ID"] = spec.accessKeyID
		data["OCI_S3_SECRET_ACCESS_KEY"] = spec.secretAccessKey
		data["OCI_S3_HOST_BASE"] = spec.hostBase
	}
	if len(spec.locations) > 0 {
		locations := make([]map[string]interface{}, 0, len(spec.locations))
		for _, loc := range spec.locations {
			item := map[string]interface{}{
				"REGION":   loc.region,
				"LOCATION": loc.location,
			}
			if loc.hostBase != "" {
				item["OCI_S3_HOST_BASE"] = loc.hostBase
			}
			locations = append(locations, item)
		}
		data["REGION_LOCATIONS"] = locations
	}
	return data, nil
}

func flattenOCIData(data map[string]interface{}) (ociStorageFlat, error) {
	flat := ociStorageFlat{}
	var err error
	if flat.backupLocation, err = configString(data, "BACKUP_LOCATION"); err != nil {
		return flat, err
	}
	if flat.region, err = configString(data, "OCI_REGION"); err != nil {
		return flat, err
	}
	if flat.namespace, err = configString(data, "OCI_NAMESPACE"); err != nil {
		return flat, err
	}
	if flat.hostBase, err = configString(data, "OCI_S3_HOST_BASE"); err != nil {
		return flat, err
	}
	if v, ok := data["USE_OCI_IAM"]; ok && v != nil {
		flat.useIAM, err = configBool(v)
		if err != nil {
			return flat, fmt.Errorf("USE_OCI_IAM: %w", err)
		}
	}
	rawLocs, ok := data["REGION_LOCATIONS"]
	if !ok || rawLocs == nil {
		return flat, nil
	}
	locs, ok := rawLocs.([]interface{})
	if !ok {
		return flat, fmt.Errorf("REGION_LOCATIONS has type %T", rawLocs)
	}
	for _, raw := range locs {
		if raw == nil {
			continue
		}
		loc, ok := raw.(map[string]interface{})
		if !ok {
			return flat, fmt.Errorf("REGION_LOCATIONS entry has type %T", raw)
		}
		item := ociLocationSpec{}
		if item.region, err = configString(loc, "REGION"); err != nil {
			return flat, err
		}
		if item.location, err = configString(loc, "LOCATION"); err != nil {
			return flat, err
		}
		if item.hostBase, err = configString(loc, "OCI_S3_HOST_BASE"); err != nil {
			return flat, err
		}
		flat.locations = append(flat.locations, item)
	}
	return flat, nil
}

func configString(data map[string]interface{}, key string) (string, error) {
	v, ok := data[key]
	if !ok || v == nil {
		return "", nil
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("storage config field %s has type %T", key, v)
	}
	return s, nil
}

func configBool(v interface{}) (bool, error) {
	switch t := v.(type) {
	case bool:
		return t, nil
	case string:
		return strconv.ParseBool(t)
	default:
		return false, fmt.Errorf("has type %T", v)
	}
}

func suppressTrimmedString(_, old, new string, _ *schema.ResourceData) bool {
	return strings.TrimSpace(old) == strings.TrimSpace(new)
}
