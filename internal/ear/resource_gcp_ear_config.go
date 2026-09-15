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

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/structure"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
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

const gcpHostIdentityRequirement = "`use_gcp_iam` and `project_id` need a YugabyteDB " +
	"Anywhere build with host-identity support for GCP KMS. Older builds reject " +
	"`use_gcp_iam` at create time and ignore `project_id`."

// ResourceGCPEARConfig defines the GCP KMS encryption-at-rest configuration.
func ResourceGCPEARConfig() *schema.Resource {
	return earResource(gcpEARSpec())
}

func gcpEARSpec() earSpec {
	return earSpec{
		resourceType: "yba_gcp_ear_config",
		displayName:  "GCP KMS",
		apiProvider:  providerGCP,
		description: "Encryption-at-rest configuration backed by a Google Cloud KMS crypto key. " +
			"YugabyteDB Anywhere wraps each universe's universe key with the crypto key and " +
			"unwraps it whenever a node needs it. Point the configuration at an existing key " +
			"ring and crypto key, or let YugabyteDB Anywhere create them when the identity " +
			"holds `cloudkms.keyRings.create` and `cloudkms.cryptoKeys.create`. An existing " +
			"crypto key must have purpose `ENCRYPT_DECRYPT`, manual rotation (no rotation " +
			"period), and an enabled primary version.\n\n" +
			"Two authentication modes are supported. Set `credentials` to a service-account " +
			"key, or set `use_gcp_iam = true` to authenticate as the YugabyteDB Anywhere host: " +
			"the attached service account on Compute Engine, workload identity on GKE, or the " +
			"key at `GOOGLE_APPLICATION_CREDENTIALS`. Either identity needs " +
			"`cloudkms.keyRings.get`, `cloudkms.cryptoKeys.get`, " +
			"`cloudkms.cryptoKeyVersions.useToEncrypt`, " +
			"`cloudkms.cryptoKeyVersions.useToDecrypt` and " +
			"`cloudkms.locations.generateRandomBytes` on the key ring's project; YugabyteDB " +
			"Anywhere checks them at create time.\n\n" +
			"~> **Note:** " + gcpHostIdentityRequirement + "\n\n" +
			"~> **Security Note:** `credentials` is stored in the Terraform state file " +
			"(marked sensitive). Use a secure backend and restrict access to your state " +
			"files, or use `use_gcp_iam` so that no key leaves the host.",
		fields: map[string]*schema.Schema{
			"credentials": {
				Type:             schema.TypeString,
				Optional:         true,
				Sensitive:        true,
				ConflictsWith:    []string{"use_gcp_iam"},
				ValidateFunc:     validation.StringIsJSON,
				DiffSuppressFunc: structure.SuppressJsonDiff,
				Description: "Service-account key JSON, inline or via `file(...)`. Required " +
					"unless `use_gcp_iam` is true. The key's `project_id` names the key ring's " +
					"project unless `project_id` is set. Can be changed in place: YugabyteDB " +
					"Anywhere checks that the new key can still unwrap every universe key " +
					"before storing it.",
			},
			"use_gcp_iam": {
				Type:          schema.TypeBool,
				Optional:      true,
				Default:       false,
				ConflictsWith: []string{"credentials"},
				Description: "Authenticate as the YugabyteDB Anywhere host instead of with a " +
					"key file: the attached service account on Compute Engine, workload " +
					"identity on GKE, or the key at `GOOGLE_APPLICATION_CREDENTIALS`. Can be " +
					"changed in place; switching from `credentials` drops the stored key.",
			},
			"project_id": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
				Description: "GCP project that owns the key ring. Defaults to the " +
					"service-account key's project, or to the host's project with " +
					"`use_gcp_iam`. Set it when the key ring lives in another project. Fixed " +
					"after creation.",
			},
			"location_id": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
				Description: "Cloud KMS location of the key ring, for example `global`, " +
					"`us-east1` or `europe`. Fixed after creation.",
			},
			"key_ring_id": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
				Description: "Key ring ID (the last segment of its resource name). Created " +
					"when missing and the identity may create key rings. Fixed after creation.",
			},
			"crypto_key_id": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
				Description: "Crypto key ID inside the key ring. This is the master key. " +
					"Created as a symmetric key when missing and the identity may create " +
					"crypto keys. Fixed after creation.",
			},
			"protection_level": {
				Type:         schema.TypeString,
				Optional:     true,
				Computed:     true,
				ForceNew:     true,
				ValidateFunc: validation.StringInSlice(gcpProtectionLevels, false),
				Description: "`SOFTWARE` or `HSM`. Used when YugabyteDB Anywhere creates the " +
					"crypto key (`SOFTWARE` when omitted). For an existing key YugabyteDB " +
					"Anywhere records the key's actual level, so leave this unset or set it " +
					"to the level the key has. Fixed after creation.",
			},
			"kms_endpoint": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
				Description: "Custom Cloud KMS endpoint, for Private Service Connect or a " +
					"restricted VIP. Fixed after creation.",
			},
		},
		credentialFields: []string{"credentials", "use_gcp_iam"},
		buildCreate:      gcpBuildCreate,
		buildEdit:        gcpBuildAuth,
		flatten:          gcpFlatten,
		createHint:       gcpCreateHint,
		customizeDiff:    gcpCustomizeDiff,
	}
}

// gcpCustomizeDiff rejects a key-file configuration without a key at plan
// time, when the value is known. Unknown values (a key read at apply time)
// are checked again in gcpBuildAuth.
func gcpCustomizeDiff(_ context.Context, d *schema.ResourceDiff, _ interface{}) error {
	if d.Get("use_gcp_iam").(bool) || !d.NewValueKnown("credentials") {
		return nil
	}
	if d.Get("credentials").(string) == "" {
		return fmt.Errorf("credentials is required when use_gcp_iam is false")
	}
	return nil
}

func gcpBuildCreate(d *schema.ResourceData) (map[string]interface{}, error) {
	settings := map[string]interface{}{
		gcpKeyLocationID:  d.Get("location_id").(string),
		gcpKeyKeyRingID:   d.Get("key_ring_id").(string),
		gcpKeyCryptoKeyID: d.Get("crypto_key_id").(string),
	}
	setIfNonEmpty(settings, gcpKeyProtectionLevel, d.Get("protection_level"))
	setIfNonEmpty(settings, gcpKeyKMSEndpoint, d.Get("kms_endpoint"))
	setIfNonEmpty(settings, gcpKeyProjectID, d.Get("project_id"))

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
// (YBA reads the project ID out of it). It is also the edit body: naming one
// mode switches YBA to it.
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
// is never read back. A stored key file next to USE_GCP_IAM means the
// YugabyteDB Anywhere build ignored the host-identity switch and kept using
// the key: report that rather than showing a clean state.
func gcpFlatten(d *schema.ResourceData, settings map[string]interface{}) error {
	for attr, key := range gcpSettingAttrs {
		v, ok := settings[key]
		if !ok {
			continue
		}
		if err := d.Set(attr, stringValue(v)); err != nil {
			return err
		}
	}
	useIAM := boolValue(settings[gcpKeyUseIAM])
	if _, hasKey := settings[gcpKeyConfig]; useIAM && hasKey {
		return fmt.Errorf(
			"encryption at rest config %s has use_gcp_iam set but YugabyteDB Anywhere still "+
				"holds a service-account key for it: this YugabyteDB Anywhere build does not "+
				"support host identity for GCP KMS and kept authenticating with the key", d.Id())
	}
	return d.Set("use_gcp_iam", useIAM)
}

func gcpCreateHint(d *schema.ResourceData) string {
	if d.Get("use_gcp_iam").(bool) {
		return "use_gcp_iam needs a YugabyteDB Anywhere build with host-identity support " +
			"for GCP KMS; the server rejected the request"
	}
	return ""
}
