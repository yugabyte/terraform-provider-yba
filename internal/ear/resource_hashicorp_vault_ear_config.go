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
	"fmt"
	"regexp"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"

	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// HashiCorp Vault authConfig keys, named as in YBA's HashicorpVaultConfigParams.
const (
	hcvKeyAddress       = "HC_VAULT_ADDRESS"
	hcvKeyToken         = "HC_VAULT_TOKEN"
	hcvKeyRoleID        = "HC_VAULT_ROLE_ID"
	hcvKeySecretID      = "HC_VAULT_SECRET_ID"
	hcvKeyAuthNamespace = "HC_VAULT_AUTH_NAMESPACE"
	hcvKeyEngine        = "HC_VAULT_ENGINE"
	hcvKeyMountPath     = "HC_VAULT_MOUNT_PATH"
	hcvKeyName          = "HC_VAULT_KEY_NAME"
)

// YBA supports one secrets engine, transit. yba-cli and the UI default the
// mount path and the key name to these values, and YBA uses the key name when
// a configuration does not store one.
const (
	hcvEngineTransit    = "transit"
	hcvDefaultMountPath = "transit/"
	hcvDefaultKeyName   = "key_yugabyte"
)

// hcvSettingAttrs maps the settings YBA lists in clear onto the resource's
// attributes. Only HC_VAULT_TOKEN and HC_VAULT_SECRET_ID are masked.
var hcvSettingAttrs = map[string]string{
	"vault_address":  hcvKeyAddress,
	"role_id":        hcvKeyRoleID,
	"auth_namespace": hcvKeyAuthNamespace,
	"mount_path":     hcvKeyMountPath,
}

// ResourceHashicorpVaultEARConfig defines the HashiCorp Vault
// encryption-at-rest configuration.
func ResourceHashicorpVaultEARConfig() *schema.Resource {
	return earResource(hashicorpVaultEARSpec())
}

func hashicorpVaultEARSpec() earSpec {
	return earSpec{
		displayName: "HashiCorp Vault",
		apiProvider: providerHashicorp,
		description: "Manages a YugabyteDB Anywhere encryption-at-rest configuration that " +
			"uses a key of the HashiCorp Vault transit secrets engine as the master key. " +
			"YugabyteDB Anywhere uses the key to wrap and unwrap the universe keys of each " +
			"universe that uses the configuration.\n\n" +
			"Point the configuration at an existing transit key with `key_name`, or let " +
			"YugabyteDB Anywhere create the key. Terraform does not manage or delete a key " +
			"that YugabyteDB Anywhere creates.\n\n" +
			"There are two authentication modes. Set `token` to authenticate with a Vault " +
			"token, or set `role_id` and `secret_id` to log in with the AppRole auth method " +
			"at `auth/approle`.\n\n" +
			"~> **Security Note:** `token` and `secret_id` are stored in the Terraform state " +
			"file and marked sensitive. Use a secure state backend and restrict access to " +
			"state files.",
		fields: map[string]*schema.Schema{
			"vault_address": {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validation.IsURLWithHTTPorHTTPS,
				Description: "Address of the Vault server, for example " +
					"`https://vault.example.com:8200`. YugabyteDB Anywhere connects to it " +
					"directly. A change forces replacement.",
			},
			"token": {
				Type:         schema.TypeString,
				Optional:     true,
				Sensitive:    true,
				ExactlyOneOf: []string{"token", "role_id"},
				ValidateFunc: validation.StringIsNotEmpty,
				Description: "Vault token. Set it, or set `role_id` and `secret_id`. Can " +
					"change in place: YugabyteDB Anywhere first checks that the new " +
					"credentials can decrypt the active universe key of each universe that " +
					"uses the configuration.",
			},
			"role_id": {
				Type:         schema.TypeString,
				Optional:     true,
				ExactlyOneOf: []string{"token", "role_id"},
				RequiredWith: []string{"secret_id"},
				ValidateFunc: validation.StringIsNotEmpty,
				Description: "Role ID of the AppRole. Set it with `secret_id` instead of " +
					"`token`. Can change in place.",
			},
			"secret_id": {
				Type:         schema.TypeString,
				Optional:     true,
				Sensitive:    true,
				RequiredWith: []string{"role_id"},
				ValidateFunc: validation.StringIsNotEmpty,
				Description: "Secret ID of the AppRole `role_id`. YugabyteDB Anywhere logs " +
					"in with it each time it uses the key, so create it with no use limit, and " +
					"change it before it expires. Can change in place.",
			},
			"auth_namespace": {
				Type:         schema.TypeString,
				Optional:     true,
				RequiredWith: []string{"role_id"},
				Description: "Vault Enterprise namespace of the AppRole auth method. " +
					"Requires `role_id`. `mount_path` must then start with the namespace, " +
					"for example `ns1/transit/` for the namespace `ns1`. Can change in place.",
			},
			"mount_path": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
				Default:  hcvDefaultMountPath,
				// YBA appends the operation to the mount path as is, so a path
				// without the trailing slash fails in the create task.
				ValidateFunc: validation.StringMatch(regexp.MustCompile(`^.+/$`),
					"must end with /"),
				Description: "Mount path of the transit secrets engine, ending with `/`. " +
					"Defaults to `transit/`. A change forces replacement.",
			},
			"key_name": {
				Type:         schema.TypeString,
				Optional:     true,
				ForceNew:     true,
				Default:      hcvDefaultKeyName,
				ValidateFunc: validation.StringIsNotEmpty,
				Description: "Name of the transit key. This key is the master key. " +
					"YugabyteDB Anywhere creates it when it does not exist. Defaults to " +
					"`key_yugabyte`. A change forces replacement.",
			},
		},
		credentialFields: []string{"token", "secret_id"},
		buildCreate:      hcvBuildCreate,
		buildEdit:        hcvBuildEdit,
		flatten:          hcvFlatten,
	}
}

// hcvRequireAuth catches a credential that was unknown at plan time and is
// empty at apply time. YBA would accept an edit body without credentials and
// keep the stored ones.
func hcvRequireAuth(d *schema.ResourceData) error {
	if d.Get("token").(string) == "" && d.Get("role_id").(string) == "" {
		return fmt.Errorf("token or role_id is required")
	}
	return nil
}

// hcvBuildCreate sends the token, or the AppRole credentials with the
// namespace when it is set. YBA rejects a body with both.
func hcvBuildCreate(d *schema.ResourceData) (map[string]interface{}, error) {
	if err := hcvRequireAuth(d); err != nil {
		return nil, err
	}
	settings := map[string]interface{}{
		hcvKeyAddress:   d.Get("vault_address").(string),
		hcvKeyEngine:    hcvEngineTransit,
		hcvKeyMountPath: d.Get("mount_path").(string),
		hcvKeyName:      d.Get("key_name").(string),
	}
	if token := d.Get("token").(string); token != "" {
		settings[hcvKeyToken] = token
		return settings, nil
	}
	settings[hcvKeyRoleID] = d.Get("role_id").(string)
	settings[hcvKeySecretID] = d.Get("secret_id").(string)
	utils.SetIfNonEmpty(settings, hcvKeyAuthNamespace, d.Get("auth_namespace"))
	return settings, nil
}

// hcvBuildEdit builds the edit body. YBA's EncryptionAtRestController
// (HASHICORP case of the edit merge) requires HC_VAULT_ADDRESS, keeps a stored
// HC_VAULT_AUTH_NAMESPACE that the body leaves out, and replaces the stored
// configuration with the merged body. When the body has neither
// HC_VAULT_TOKEN nor both AppRole keys, the merge copies every stored
// credential into it; a body with HC_VAULT_ROLE_ID alone gets the stored role
// ID back. So the credentials of the configured mode go when the mode changes
// (role_id changes from or to empty) or when they change, and the two AppRole
// keys go together. Otherwise they stay out of the body, so a namespace change
// does not overwrite a secret ID changed outside Terraform. The namespace
// always goes, as "" in token mode, so that a namespace YBA stores does not
// survive a switch to a token.
func hcvBuildEdit(d *schema.ResourceData) (map[string]interface{}, error) {
	if err := hcvRequireAuth(d); err != nil {
		return nil, err
	}
	settings := map[string]interface{}{
		hcvKeyAddress:       d.Get("vault_address").(string),
		hcvKeyAuthNamespace: d.Get("auth_namespace").(string),
	}
	if token := d.Get("token").(string); token != "" {
		if d.HasChanges("token", "role_id") {
			settings[hcvKeyToken] = token
		}
		return settings, nil
	}
	if d.HasChanges("role_id", "secret_id") {
		settings[hcvKeyRoleID] = d.Get("role_id").(string)
		settings[hcvKeySecretID] = d.Get("secret_id").(string)
	}
	return settings, nil
}

// hcvFlatten writes the listed non-secret settings into state. The token and
// the secret ID are never read back (the list masks them). A configuration
// without HC_VAULT_KEY_NAME uses YBA's default key name.
func hcvFlatten(d *schema.ResourceData, settings map[string]interface{}) error {
	for attr, key := range hcvSettingAttrs {
		if err := d.Set(attr, utils.StringValue(settings[key])); err != nil {
			return err
		}
	}
	keyName := utils.StringValue(settings[hcvKeyName])
	if keyName == "" {
		keyName = hcvDefaultKeyName
	}
	return d.Set("key_name", keyName)
}
