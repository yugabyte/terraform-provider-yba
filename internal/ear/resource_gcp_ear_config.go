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
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/structure"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"

	"github.com/yugabyte/terraform-provider-yba/internal/api"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// GCP authConfig keys, named as in YBA's GcpKmsAuthConfigField.
const (
	gcpKeyConfig          = "GCP_CONFIG"
	gcpKeyUseIAM          = "USE_GCP_IAM"
	gcpKeyProjectID       = "GCP_PROJECT_ID"
	gcpKeyLocationID      = "LOCATION_ID"
	gcpKeyProtectionLevel = "PROTECTION_LEVEL"
	gcpKeyKMSEndpoint     = "GCP_KMS_ENDPOINT"
	gcpKeyKeyRingID       = "KEY_RING_ID"
	gcpKeyCryptoKeyID     = "CRYPTO_KEY_ID"
)

var gcpProtectionLevels = []string{"SOFTWARE", "HSM"}

// gcpSettingAttrs maps the non-secret settings YBA lists onto the resource's
// attributes. Every one of them is fixed after creation.
var gcpSettingAttrs = map[string]string{
	"location_id":      gcpKeyLocationID,
	"key_ring_id":      gcpKeyKeyRingID,
	"crypto_key_id":    gcpKeyCryptoKeyID,
	"protection_level": gcpKeyProtectionLevel,
	"kms_endpoint":     gcpKeyKMSEndpoint,
	"project_id":       gcpKeyProjectID,
}

// gcpHostIdentityMin is the first build with USE_GCP_IAM and GCP_PROJECT_ID:
// yugabyte-db commit 7252ad16745, first in 2.31.0.0-b473. No stable branch has
// the commit, so Stable stays empty and gcpRequireHostIdentity fails every
// stable build. An older build keeps the stored key on an edit to USE_GCP_IAM,
// which gcpFlatten then rejects on every Read, and ignores GCP_PROJECT_ID.
var gcpHostIdentityMin = utils.YBAMinimumVersion{Preview: "2.31.0.0-b473"}

// gcpHostIdentityRequirement renders into the resource docs, which name stable
// releases only.
const gcpHostIdentityRequirement = "`use_gcp_iam` and `project_id` work only with " +
	"YugabyteDB Anywhere preview releases. No stable release supports them, and " +
	"`terraform plan` fails when you set either of them on a stable release."

// ResourceGCPEARConfig defines the GCP KMS encryption-at-rest configuration.
func ResourceGCPEARConfig() *schema.Resource {
	return earResource(gcpEARSpec())
}

func gcpEARSpec() earSpec {
	return earSpec{
		displayName: "GCP KMS",
		apiProvider: providerGCP,
		description: "Manages a YugabyteDB Anywhere encryption-at-rest configuration that " +
			"uses a Google Cloud KMS crypto key as the master key. YugabyteDB Anywhere uses " +
			"the crypto key to wrap and unwrap the universe keys of each universe that uses " +
			"the configuration.\n\n" +
			"Point the configuration at an existing key ring and crypto key, or let " +
			"YugabyteDB Anywhere create them. To create them, the identity needs " +
			"`cloudkms.keyRings.create` and `cloudkms.cryptoKeys.create`. An existing crypto " +
			"key must have the purpose `ENCRYPT_DECRYPT`, manual rotation (no rotation " +
			"period), and an enabled primary version.\n\n" +
			"There are two authentication modes. Set `credentials` to a service-account key, " +
			"or set `use_gcp_iam = true` to authenticate as the YugabyteDB Anywhere host: the " +
			"attached service account on Compute Engine, workload identity on GKE, or the key " +
			"file at `GOOGLE_APPLICATION_CREDENTIALS`. Either identity needs these permissions " +
			"on the key ring's project: `cloudkms.keyRings.get`, `cloudkms.cryptoKeys.get`, " +
			"`cloudkms.cryptoKeyVersions.useToEncrypt`, " +
			"`cloudkms.cryptoKeyVersions.useToDecrypt` and " +
			"`cloudkms.locations.generateRandomBytes`. YugabyteDB Anywhere checks them when " +
			"it creates the configuration.\n\n" +
			"~> **Note:** " + gcpHostIdentityRequirement + "\n\n" +
			"~> **Security Note:** `credentials` is stored in the Terraform state file and " +
			"marked sensitive. Use a secure state backend and restrict access to state " +
			"files, or use `use_gcp_iam`, which keeps the key out of Terraform.",
		fields: map[string]*schema.Schema{
			"credentials": {
				Type:             schema.TypeString,
				Optional:         true,
				Sensitive:        true,
				ConflictsWith:    []string{"use_gcp_iam"},
				ValidateFunc:     validation.StringIsJSON,
				DiffSuppressFunc: structure.SuppressJsonDiff,
				Description: "Service-account key JSON, inline or from `file(...)`. Required " +
					"unless `use_gcp_iam` is true. The key's `project_id` is the key ring's " +
					"project unless `project_id` is set. Can change in place: YugabyteDB " +
					"Anywhere first checks that the new key can unwrap the active universe key " +
					"of each universe that uses the configuration.",
			},
			"use_gcp_iam": {
				Type:          schema.TypeBool,
				Optional:      true,
				Default:       false,
				ConflictsWith: []string{"credentials"},
				Description: "Authenticate as the YugabyteDB Anywhere host instead of with a " +
					"key file: the attached service account on Compute Engine, workload " +
					"identity on GKE, or the key file at `GOOGLE_APPLICATION_CREDENTIALS`. " +
					"Works only with YugabyteDB Anywhere preview releases. Can change in " +
					"place; YugabyteDB Anywhere deletes the stored key when the configuration " +
					"changes to the host identity.",
			},
			"project_id": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
				Description: "GCP project that owns the key ring. Defaults to the project of " +
					"the service-account key, or to the host's project with `use_gcp_iam`. Set " +
					"it when the key ring is in another project. Works only with YugabyteDB " +
					"Anywhere preview releases. A change forces replacement.",
			},
			"location_id": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
				Description: "Cloud KMS location of the key ring, for example `global`, " +
					"`us-east1` or `europe`. A change forces replacement.",
			},
			"key_ring_id": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
				Description: "ID of the key ring, which is the last part of its resource " +
					"name. YugabyteDB Anywhere creates the key ring when it does not exist and " +
					"the identity can create key rings. A change forces replacement.",
			},
			"crypto_key_id": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
				Description: "ID of the crypto key in the key ring. This key is the master " +
					"key. YugabyteDB Anywhere creates it as a symmetric key when it does not " +
					"exist and the identity can create crypto keys. A change forces " +
					"replacement.",
			},
			"protection_level": {
				Type:         schema.TypeString,
				Optional:     true,
				Computed:     true,
				ForceNew:     true,
				ValidateFunc: validation.StringInSlice(gcpProtectionLevels, false),
				Description: "`SOFTWARE` or `HSM`: the protection level of a crypto key that " +
					"YugabyteDB Anywhere creates, `SOFTWARE` when not set. For an existing " +
					"key, YugabyteDB Anywhere records the key's actual level, so leave this " +
					"unset or set it to that level. A change forces replacement.",
			},
			"kms_endpoint": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
				Description: "Custom Cloud KMS endpoint as `host:port`, for Private Service " +
					"Connect or a restricted VIP. A change forces replacement.",
			},
		},
		credentialFields: []string{"credentials", "use_gcp_iam"},
		buildCreate:      gcpBuildCreate,
		buildEdit:        gcpBuildAuth,
		flatten:          gcpFlatten,
		customizeDiff:    gcpCustomizeDiff,
	}
}

// gcpCustomizeDiff rejects a key-file configuration without a key at plan
// time, when the value is known. Unknown values (a key read at apply time)
// are checked again in gcpBuildAuth. It also gates use_gcp_iam and project_id
// on the YBA version, but only when the plan sets or changes them, so an
// unchanged configuration plans as before.
func gcpCustomizeDiff(ctx context.Context, d *schema.ResourceDiff, meta interface{}) error {
	gated := ""
	switch {
	case d.HasChange("use_gcp_iam") && d.Get("use_gcp_iam").(bool):
		gated = "use_gcp_iam"
	case d.HasChange("project_id") &&
		(!d.NewValueKnown("project_id") || d.Get("project_id").(string) != ""):
		gated = "project_id"
	}
	if gated != "" {
		if err := gcpRequireHostIdentity(ctx, meta, gated); err != nil {
			return err
		}
	}
	if d.Get("use_gcp_iam").(bool) || !d.NewValueKnown("credentials") {
		return nil
	}
	if d.Get("credentials").(string) == "" {
		return fmt.Errorf("credentials is required when use_gcp_iam is false")
	}
	return nil
}

// gcpRequireHostIdentity fails the plan when the target YBA lacks the field.
// A version outside YBA's scheme passes with a warning; the server stays the
// backstop.
func gcpRequireHostIdentity(ctx context.Context, meta interface{}, field string) error {
	c, ok := meta.(*api.APIClient)
	if !ok || c == nil {
		return nil
	}
	version, err := c.AppVersion(ctx)
	if err != nil || version == "" {
		return err
	}
	if utils.IsVersionStable(version) && !utils.IsExperimentalPatchVersion(version) {
		return fmt.Errorf("%s requires a YugabyteDB Anywhere preview release; "+
			"the target YBA reports %s", field, version)
	}
	ok, applied, err := utils.MeetsMinimum(version, gcpHostIdentityMin)
	switch {
	case err != nil:
		tflog.Warn(ctx, "cannot parse the YBA version; skipping the minimum-version check",
			map[string]interface{}{"version": version, "check": field})
	case !ok:
		return fmt.Errorf("%s requires YugabyteDB Anywhere %s or later; "+
			"the target YBA reports %s", field, applied, version)
	}
	return nil
}

func gcpBuildCreate(d *schema.ResourceData) (map[string]interface{}, error) {
	settings := map[string]interface{}{
		gcpKeyLocationID:  d.Get("location_id").(string),
		gcpKeyKeyRingID:   d.Get("key_ring_id").(string),
		gcpKeyCryptoKeyID: d.Get("crypto_key_id").(string),
	}
	utils.SetIfNonEmpty(settings, gcpKeyProtectionLevel, d.Get("protection_level"))
	utils.SetIfNonEmpty(settings, gcpKeyKMSEndpoint, d.Get("kms_endpoint"))
	utils.SetIfNonEmpty(settings, gcpKeyProjectID, d.Get("project_id"))

	auth, err := gcpBuildAuth(d)
	if err != nil {
		return nil, err
	}
	for k, v := range auth {
		settings[k] = v
	}
	return settings, nil
}

// gcpBuildAuth maps the authentication arguments onto YBA's keys: USE_GCP_IAM
// for the host identity, else GCP_CONFIG carrying the key as a JSON object
// (YBA reads the project ID out of it). It is also the edit body. YBA's
// EncryptionAtRestController (GCP case of the edit merge) switches to the
// mode the body names: USE_GCP_IAM drops the stored GCP_CONFIG, and
// GCP_CONFIG without USE_GCP_IAM drops the stored flag.
func gcpBuildAuth(d *schema.ResourceData) (map[string]interface{}, error) {
	if d.Get("use_gcp_iam").(bool) {
		return map[string]interface{}{gcpKeyUseIAM: true}, nil
	}
	raw := d.Get("credentials").(string)
	if raw == "" {
		return nil, fmt.Errorf("credentials is required when use_gcp_iam is false")
	}
	var key map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &key); err != nil {
		return nil, fmt.Errorf("credentials is not a JSON object: %w", err)
	}
	return map[string]interface{}{gcpKeyConfig: key}, nil
}

// gcpFlatten writes the listed non-secret settings into state. The key file
// is never read back. A build with host-identity support never stores a key
// file next to USE_GCP_IAM (create rejects the pair, an edit to the host
// identity drops the key). An older build does not know the flag: an edit
// stores it and keeps the key, and keeps authenticating with the key. Report
// that rather than a clean state that claims the host identity is in use.
func gcpFlatten(d *schema.ResourceData, settings map[string]interface{}) error {
	for attr, key := range gcpSettingAttrs {
		v, ok := settings[key]
		if !ok {
			continue
		}
		if err := d.Set(attr, utils.StringValue(v)); err != nil {
			return err
		}
	}
	useIAM := utils.BoolValue(settings[gcpKeyUseIAM])
	if _, hasKey := settings[gcpKeyConfig]; useIAM && hasKey {
		return fmt.Errorf(
			"encryption at rest config %s: YugabyteDB Anywhere stores USE_GCP_IAM next to a "+
				"GCP_CONFIG key file", d.Id())
	}
	return d.Set("use_gcp_iam", useIAM)
}
