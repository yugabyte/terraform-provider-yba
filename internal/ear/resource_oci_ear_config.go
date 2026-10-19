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

package ear

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"

	"github.com/yugabyte/terraform-provider-yba/internal/api"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// OCI authConfig keys, named as in YBA's OciKmsAuthConfigField on master
// (yugabyte-db commit b12a466319c aligned them with the cloud provider).
const (
	ociKeyTenancyID     = "ociTenancyId"
	ociKeyUserID        = "ociUserId"
	ociKeyFingerprint   = "ociFingerprint"
	ociKeyPrivateKey    = "ociPrivateKeyContent"
	ociKeyAuthType      = "ociAuthType"
	ociKeyCompartmentID = "ociCompartmentId"
	ociKeyVaultID       = "ociVaultId"
	ociKeyRegion        = "ociRegion"
	ociKeyKeyName       = "ociKeyName"
	ociKeyKeyOCID       = "ociKeyOcid"
)

// YBA's OciKmsAuthType. A blank ociAuthType on read is API-key auth.
const (
	ociAuthAPIKey            = "API_KEY"
	ociAuthInstancePrincipal = "INSTANCE_PRINCIPAL"
)

// ociSettingAttrs maps the settings YBA lists in clear onto the resource.
// ociKeyName, ociKeyOcid and ociPrivateKeyContent are absent: CommonUtils
// masks any field whose name contains KEY, and those three are not in its
// exclusion list. key_name and key_id come from the by-UUID GET in
// ociReadKey. The private key is never read back.
var ociSettingAttrs = map[string]string{
	"tenancy_id":     ociKeyTenancyID,
	"user_id":        ociKeyUserID,
	"fingerprint":    ociKeyFingerprint,
	"compartment_id": ociKeyCompartmentID,
	"vault_id":       ociKeyVaultID,
	"region":         ociKeyRegion,
}

// ociEARMin is the first preview build whose OCI KMS auth config this
// resource sends: camelCase fields plus ociAuthType, including instance
// principal. yugabyte-db commit 273f9481781, first tagged 2.31.0.0-b244.
// No stable branch contains that commit. Stable 2026.1 has OCI KMS with
// different field names (TENANCY_OCID, PRIVATE_KEY, OCI_KEY_NAME) and no
// instance principal, so Stable stays empty and ociVersionAllowed fails
// every stable build except an experimental patch build, which passes
// every gate (utils.IsExperimentalPatchVersion).
var ociEARMin = utils.YBAMinimumVersion{Preview: "2.31.0.0-b244"}

// ociEARPreview is the preview admonition in the resource description and
// the detail of the warning that Create and Update return.
const ociEARPreview = "The YugabyteDB Anywhere API behind `yba_oci_ear_config` is in " +
	"preview. A later YBA release can change it in ways that are not backward compatible."

// ociAllowFlag is the global runtime config YBA requires to be true before
// it creates, edits, or deletes an OCI KMS configuration. Unset reads as
// YBA's default, which allows OCI KMS.
const ociAllowFlag = "yb.kms.allow_oci"

// ResourceOCIEARConfig defines the OCI KMS encryption-at-rest configuration.
func ResourceOCIEARConfig() *schema.Resource {
	return earResource(ociEARSpec())
}

func ociEARSpec() earSpec {
	return earSpec{
		displayName: "OCI KMS",
		apiProvider: providerOCI,
		description: "Manages a YugabyteDB Anywhere encryption-at-rest configuration that " +
			"uses an Oracle Cloud Infrastructure Vault key as the master key. YugabyteDB " +
			"Anywhere uses the key to wrap and unwrap the universe keys of each universe " +
			"that uses the configuration.\n\n" +
			"~> **Preview:** " + ociEARPreview + "\n\n" +
			"Point the configuration at an existing AES key with `key_name`, or let " +
			"YugabyteDB Anywhere create an AES-256 key with that display name. Terraform " +
			"does not manage or delete a key that YugabyteDB Anywhere creates. An existing " +
			"key must be enabled and use the AES algorithm. The display name must match " +
			"one key in the compartment.\n\n" +
			"There are two authentication modes. Set `tenancy_id`, `user_id`, " +
			"`fingerprint` and `private_key_content` for an API signing key, or set " +
			"`use_instance_principal = true` to authenticate as the YugabyteDB Anywhere " +
			"host. The host must be an OCI compute instance in a dynamic group. Either " +
			"identity needs policies to use the vault and to use keys in `compartment_id`, " +
			"and to manage keys when YugabyteDB Anywhere creates the key. YugabyteDB " +
			"Anywhere checks them when it creates or edits the configuration.\n\n" +
			"An apply fails while the global runtime config `yb.kms.allow_oci` is " +
			"false. YugabyteDB Anywhere allows OCI KMS when that key is unset or " +
			"true.\n\n" +
			"~> **Security Note:** `private_key_content` is stored in the Terraform state " +
			"file and marked sensitive. Use a secure state backend and restrict access to " +
			"state files, or use `use_instance_principal`, which keeps the key out of " +
			"Terraform.",
		fields: map[string]*schema.Schema{
			"use_instance_principal": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
				ForceNew: true,
				Description: "Authenticate as the YugabyteDB Anywhere host with an OCI " +
					"instance principal, instead of an API signing key. Uses a preview " +
					"YugabyteDB Anywhere API. A change forces replacement.",
			},
			"tenancy_id": {
				Type:         schema.TypeString,
				Optional:     true,
				RequiredWith: []string{"user_id", "fingerprint", "private_key_content"},
				ValidateFunc: validation.StringIsNotEmpty,
				Description: "OCID of the tenancy that owns the API signing key. Required, " +
					"with `user_id`, `fingerprint` and `private_key_content`, when " +
					"`use_instance_principal` is false. Can change in place.",
			},
			"user_id": {
				Type:         schema.TypeString,
				Optional:     true,
				RequiredWith: []string{"tenancy_id", "fingerprint", "private_key_content"},
				ValidateFunc: validation.StringIsNotEmpty,
				Description: "OCID of the user that owns the API signing key. Required when " +
					"`use_instance_principal` is false. Can change in place.",
			},
			"fingerprint": {
				Type:         schema.TypeString,
				Optional:     true,
				RequiredWith: []string{"tenancy_id", "user_id", "private_key_content"},
				ValidateFunc: validation.StringIsNotEmpty,
				Description: "Fingerprint of the API signing key. Required when " +
					"`use_instance_principal` is false. Can change in place.",
			},
			"private_key_content": {
				Type:         schema.TypeString,
				Optional:     true,
				Sensitive:    true,
				RequiredWith: []string{"tenancy_id", "user_id", "fingerprint"},
				ValidateFunc: validation.StringIsNotEmpty,
				Description: "PEM contents of the API signing key, inline or from " +
					"`file(...)`. Required when `use_instance_principal` is false. Can " +
					"change in place: YugabyteDB Anywhere checks the credentials, the vault " +
					"and the key before it stores the change.",
			},
			"compartment_id": {
				Type:         schema.TypeString,
				Required:     true,
				ValidateFunc: validation.StringIsNotEmpty,
				Description: "OCID of the compartment that holds the vault and the key. " +
					"Can change in place.",
			},
			"vault_id": {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validation.StringIsNotEmpty,
				Description: "OCID of the vault that holds the master key. A change forces " +
					"replacement.",
			},
			"region": {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validation.StringIsNotEmpty,
				Description: "OCI region of the vault, for example `us-ashburn-1`. A change " +
					"forces replacement.",
			},
			"key_name": {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validation.StringIsNotEmpty,
				Description: "Display name of the master key in the vault. YugabyteDB " +
					"Anywhere uses an existing AES key with this name, or creates an " +
					"AES-256 key when the name is absent and the identity can create keys. " +
					"A change forces replacement.",
			},
			"key_id": {
				Type:     schema.TypeString,
				Computed: true,
				Description: "OCID of the master key YugabyteDB Anywhere resolved from " +
					"`key_name`.",
			},
		},
		credentialFields: []string{"private_key_content"},
		buildCreate:      ociBuildCreate,
		buildEdit:        ociBuildEdit,
		flatten:          ociFlatten,
		afterRead:        ociReadKey,
		customizeDiff:    ociCustomizeDiff,
		warnings:         ociPreviewWarnings,
		runtimeFlag:      ociAllowFlag,
	}
}

// ociCustomizeDiff gates the resource on the YBA build, then checks the auth
// mode against the bool value. ConflictsWith cannot do this: it rejects the
// attribute when it is present, so an explicit use_instance_principal = false
// conflicts with the signing-key fields. Unknown signing-key values are
// checked again in ociBuildAuth.
func ociCustomizeDiff(ctx context.Context, d *schema.ResourceDiff, meta interface{}) error {
	if err := ociRequireVersion(ctx, meta); err != nil {
		return err
	}
	fields := []string{"tenancy_id", "user_id", "fingerprint", "private_key_content"}
	if d.Get("use_instance_principal").(bool) {
		for _, field := range fields {
			if !d.NewValueKnown(field) {
				continue
			}
			if d.Get(field).(string) != "" {
				return fmt.Errorf(
					"%s cannot be set when use_instance_principal is true", field)
			}
		}
		return nil
	}
	for _, field := range fields {
		if !d.NewValueKnown(field) {
			continue
		}
		if d.Get(field).(string) == "" {
			return fmt.Errorf(
				"%s is required when use_instance_principal is false", field)
		}
	}
	return nil
}

func ociPreviewWarnings(*schema.ResourceData) diag.Diagnostics {
	return diag.Diagnostics{{
		Severity: diag.Warning,
		Summary:  "yba_oci_ear_config uses a preview YugabyteDB Anywhere API",
		Detail: ociEARPreview + " Pin your provider version and read the release notes " +
			"before you upgrade.",
	}}
}

// ociRequireVersion fails the plan when the target YBA lacks the OCI KMS API
// this resource sends. A version outside YBA's scheme passes and is only
// logged (tflog.Warn, seen with TF_LOG set); the server stays the backstop.
func ociRequireVersion(ctx context.Context, meta interface{}) error {
	c, ok := meta.(*api.APIClient)
	if !ok || c == nil {
		return nil
	}
	version, err := c.AppVersion(ctx)
	if err != nil || version == "" {
		return err
	}
	if err := ociVersionAllowed(version); err != nil {
		if _, _, parseErr := utils.MeetsMinimum(version, ociEARMin); parseErr != nil &&
			!utils.IsVersionStable(version) {
			tflog.Warn(ctx, "cannot parse the YBA version; skipping the minimum-version check",
				map[string]interface{}{"version": version, "check": "yba_oci_ear_config"})
			return nil
		}
		return err
	}
	return nil
}

// ociVersionAllowed is the version decision. Experimental patch builds pass.
// Every other stable build fails: no stable line has this API. A preview
// build must be at least ociEARMin.
func ociVersionAllowed(version string) error {
	if utils.IsExperimentalPatchVersion(version) {
		return nil
	}
	if utils.IsVersionStable(version) {
		return fmt.Errorf(
			"yba_oci_ear_config is not supported by the target YBA (version=%s)", version)
	}
	ok, applied, err := utils.MeetsMinimum(version, ociEARMin)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("yba_oci_ear_config requires YugabyteDB Anywhere %s or later; "+
			"the target YBA reports %s", applied, version)
	}
	return nil
}

func ociBuildCreate(d *schema.ResourceData) (map[string]interface{}, error) {
	settings := map[string]interface{}{
		ociKeyRegion:        d.Get("region").(string),
		ociKeyCompartmentID: d.Get("compartment_id").(string),
		ociKeyVaultID:       d.Get("vault_id").(string),
		ociKeyKeyName:       d.Get("key_name").(string),
	}
	auth, err := ociBuildAuth(d)
	if err != nil {
		return nil, err
	}
	for k, v := range auth {
		settings[k] = v
	}
	return settings, nil
}

// ociBuildAuth maps the authentication arguments onto YBA's keys. Instance
// principal sends ociAuthType and none of the API-key fields. API-key auth
// sends ociAuthType API_KEY plus the four signing-key fields. YBA treats a
// missing ociAuthType as API_KEY; the value is sent so a later read does not
// have to guess.
func ociBuildAuth(d *schema.ResourceData) (map[string]interface{}, error) {
	if d.Get("use_instance_principal").(bool) {
		return map[string]interface{}{ociKeyAuthType: ociAuthInstancePrincipal}, nil
	}
	tenancy := d.Get("tenancy_id").(string)
	user := d.Get("user_id").(string)
	fingerprint := d.Get("fingerprint").(string)
	privateKey := d.Get("private_key_content").(string)
	if tenancy == "" || user == "" || fingerprint == "" || privateKey == "" {
		return nil, fmt.Errorf(
			"tenancy_id, user_id, fingerprint, and private_key_content are required when " +
				"use_instance_principal is false")
	}
	return map[string]interface{}{
		ociKeyAuthType:    ociAuthAPIKey,
		ociKeyTenancyID:   tenancy,
		ociKeyUserID:      user,
		ociKeyFingerprint: fingerprint,
		ociKeyPrivateKey:  privateKey,
	}, nil
}

// ociBuildEdit builds the edit body. YBA's EncryptionAtRestController (OCI
// case of the edit merge) keeps a stored editable field that the body leaves
// out, and rejects a change to ociAuthType, ociVaultId, ociRegion, ociKeyName
// or ociKeyOcid. An editable field goes only when the configuration changes
// it, so a compartment change does not overwrite a private key changed
// outside Terraform. Auth type cannot change in place, so this never clears
// the signing key to switch to the host identity.
func ociBuildEdit(d *schema.ResourceData) (map[string]interface{}, error) {
	settings := map[string]interface{}{}
	for attr, key := range map[string]string{
		"tenancy_id":     ociKeyTenancyID,
		"user_id":        ociKeyUserID,
		"fingerprint":    ociKeyFingerprint,
		"compartment_id": ociKeyCompartmentID,
	} {
		if d.HasChange(attr) {
			settings[key] = d.Get(attr).(string)
		}
	}
	if d.HasChange("private_key_content") {
		privateKey := d.Get("private_key_content").(string)
		if privateKey == "" {
			return nil, fmt.Errorf(
				"private_key_content is required when use_instance_principal is false")
		}
		settings[ociKeyPrivateKey] = privateKey
	}
	return settings, nil
}

// ociFlatten writes the listed non-secret settings into state. The private
// key, key name and key OCID are masked on the list (the field name contains
// KEY), so they are left for state or for ociReadKey.
func ociFlatten(d *schema.ResourceData, settings map[string]interface{}) error {
	for attr, key := range ociSettingAttrs {
		if _, ok := settings[key]; !ok {
			continue
		}
		if err := d.Set(attr, utils.StringValue(settings[key])); err != nil {
			return err
		}
	}
	authType := strings.ToUpper(utils.StringValue(settings[ociKeyAuthType]))
	return d.Set("use_instance_principal", authType == ociAuthInstancePrincipal)
}

// ociReadKey sets key_name and key_id from the by-UUID GET, which returns
// them in clear. The list masks both because the field names contain KEY.
// The GET also returns ociPrivateKeyContent in clear, so it runs only when
// state has no key_id or no key_name: the read after create, and import.
// A later refresh keeps both from state and does not call the GET. The
// private key in the response is not written to state.
func ociReadKey(ctx context.Context, d *schema.ResourceData, meta interface{}) error {
	if d.Get("key_id").(string) != "" && d.Get("key_name").(string) != "" {
		return nil
	}
	c, ok := meta.(*api.APIClient)
	if !ok || c == nil || c.VanillaClient == nil {
		return fmt.Errorf("encryption at rest config %s: no YugabyteDB Anywhere client", d.Id())
	}
	stored, err := c.VanillaClient.GetKMSConfig(ctx, c.CustomerID, d.Id(), c.APIKey)
	if err != nil {
		return err
	}
	if err := d.Set("key_name", utils.StringValue(stored[ociKeyKeyName])); err != nil {
		return err
	}
	return d.Set("key_id", utils.StringValue(stored[ociKeyKeyOCID]))
}
