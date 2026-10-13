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
	"strconv"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"

	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// Azure authConfig keys, named as in YBA's AzuKmsAuthConfigField.
const (
	azureKeyClientID     = "CLIENT_ID"
	azureKeyClientSecret = "CLIENT_SECRET"
	azureKeyTenantID     = "TENANT_ID"
	azureKeyVaultURL     = "AZU_VAULT_URL"
	azureKeyName         = "AZU_KEY_NAME"
	azureKeyAlgorithm    = "AZU_KEY_ALGORITHM"
	azureKeySize         = "AZU_KEY_SIZE"
)

// YBA's AzuAlgorithm has one algorithm, RSA, with these key sizes. YBA
// requires both keys on create; yba-cli and the UI default the size to 2048.
const (
	azureAlgorithmRSA   = "RSA"
	azureDefaultKeySize = 2048
)

var azureKeySizes = []int{2048, 3072, 4096}

// azureSettingAttrs maps the string settings YBA lists in clear onto the
// resource's attributes. Only CLIENT_SECRET is masked.
var azureSettingAttrs = map[string]string{
	"client_id": azureKeyClientID,
	"tenant_id": azureKeyTenantID,
	"vault_url": azureKeyVaultURL,
	"key_name":  azureKeyName,
}

// ResourceAzureEARConfig defines the Azure Key Vault encryption-at-rest
// configuration.
func ResourceAzureEARConfig() *schema.Resource {
	return earResource(azureEARSpec())
}

func azureEARSpec() earSpec {
	return earSpec{
		displayName: "Azure Key Vault",
		apiProvider: providerAzure,
		description: "Manages a YugabyteDB Anywhere encryption-at-rest configuration that " +
			"uses an Azure Key Vault key as the master key. YugabyteDB Anywhere uses the key " +
			"to wrap and unwrap the universe keys of each universe that uses the " +
			"configuration.\n\n" +
			"Point the configuration at an existing key with `key_name`, or let YugabyteDB " +
			"Anywhere create an RSA key with that name. Terraform does not manage or delete a " +
			"key that YugabyteDB Anywhere creates. An existing key must be an RSA key that is " +
			"enabled, has no expiration date, has no activation date in the future, and " +
			"permits the wrap key and unwrap key operations.\n\n" +
			"There are two authentication modes. Set `client_secret` to authenticate as the " +
			"service principal `client_id`. Or leave `client_secret` unset to authenticate " +
			"as the YugabyteDB Anywhere host through the Azure default credential chain, for " +
			"example a managed identity of the Azure VM. In this mode, `client_id` is the " +
			"client ID of the managed identity. The identity needs the key permissions Get, " +
			"Wrap Key and Unwrap Key, and Create when YugabyteDB Anywhere creates the key. " +
			"YugabyteDB Anywhere checks them when it creates or edits the configuration. " +
			"After the key gets a new version, YugabyteDB Anywhere also needs List to unwrap " +
			"the universe keys that an older version wrapped. With Azure RBAC on the key " +
			"vault, the Key Vault Crypto User role grants Get, List, Wrap Key and Unwrap " +
			"Key.\n\n" +
			"~> **Security Note:** `client_secret` is stored in the Terraform state file and " +
			"marked sensitive. Use a secure state backend and restrict access to state " +
			"files, or authenticate as the YugabyteDB Anywhere host, which keeps the secret " +
			"out of Terraform.",
		fields: map[string]*schema.Schema{
			"client_id": {
				Type:         schema.TypeString,
				Required:     true,
				ValidateFunc: validation.StringIsNotEmpty,
				Description: "Client ID of the service principal when `client_secret` is " +
					"set, or of the managed identity on the YugabyteDB Anywhere host when it " +
					"is not. Can change in place.",
			},
			"tenant_id": {
				Type:         schema.TypeString,
				Required:     true,
				ValidateFunc: validation.StringIsNotEmpty,
				Description:  "Azure tenant ID of `client_id`. Can change in place.",
			},
			"client_secret": {
				Type:      schema.TypeString,
				Optional:  true,
				Sensitive: true,
				Description: "Client secret of the service principal `client_id`. " +
					"Leave it unset to authenticate as the YugabyteDB Anywhere host. Can " +
					"change in place: YugabyteDB Anywhere first checks that the new " +
					"credentials can unwrap the active universe key of each universe that " +
					"uses the configuration. Removing it switches the configuration to the " +
					"host's identity.",
			},
			"vault_url": {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validation.IsURLWithHTTPS,
				Description: "URL of the key vault, for example " +
					"`https://my-vault.vault.azure.net/`. A change forces replacement.",
			},
			"key_name": {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validation.StringIsNotEmpty,
				Description: "Name of the key in the key vault. This key is the master key. " +
					"YugabyteDB Anywhere creates it as an RSA key of `key_size` bits when it " +
					"does not exist and the identity has the Create permission. A change " +
					"forces replacement.",
			},
			"key_size": {
				Type:         schema.TypeInt,
				Optional:     true,
				Computed:     true,
				ForceNew:     true,
				ValidateFunc: validation.IntInSlice(azureKeySizes),
				Description: "Size in bits of the RSA key that YugabyteDB Anywhere creates: " +
					"`2048`, `3072` or `4096`, and `2048` when not set. For an existing key, " +
					"YugabyteDB Anywhere stores the value but does not compare it with the " +
					"key. A change forces replacement.",
			},
		},
		credentialFields: []string{"client_secret"},
		buildCreate:      azureBuildCreate,
		buildEdit:        azureBuildEdit,
		flatten:          azureFlatten,
	}
}

// azureBuildCreate leaves an unset client_secret out of the body. Without a
// secret, YBA authenticates through DefaultAzureCredential with CLIENT_ID as
// the managed identity's client ID.
func azureBuildCreate(d *schema.ResourceData) (map[string]interface{}, error) {
	keySize := d.Get("key_size").(int)
	if keySize == 0 {
		keySize = azureDefaultKeySize
	}
	settings := map[string]interface{}{
		azureKeyClientID:  d.Get("client_id").(string),
		azureKeyTenantID:  d.Get("tenant_id").(string),
		azureKeyVaultURL:  d.Get("vault_url").(string),
		azureKeyName:      d.Get("key_name").(string),
		azureKeyAlgorithm: azureAlgorithmRSA,
		azureKeySize:      keySize,
	}
	utils.SetIfNonEmpty(settings, azureKeyClientSecret, d.Get("client_secret"))
	return settings, nil
}

// azureBuildEdit builds the edit body. YBA's EncryptionAtRestController (AZU
// case of the edit merge) keeps a stored CLIENT_ID, CLIENT_SECRET or
// TENANT_ID that the body leaves out, so an empty string is how the secret is
// removed: without a secret, YBA authenticates as the host. A configured
// secret goes only when it changed, so a client_id or tenant_id change does
// not overwrite a secret changed outside Terraform. Without a configured
// secret, the empty string always goes: state cannot show a secret that YBA
// still stores (after an import, or after a failed edit, which YBA stores
// before it checks the credentials), and YBA uses a stored secret with any
// client_id.
func azureBuildEdit(d *schema.ResourceData) (map[string]interface{}, error) {
	settings := map[string]interface{}{
		azureKeyClientID: d.Get("client_id").(string),
		azureKeyTenantID: d.Get("tenant_id").(string),
	}
	if secret := d.Get("client_secret").(string); secret == "" || d.HasChange("client_secret") {
		settings[azureKeyClientSecret] = secret
	}
	return settings, nil
}

// azureFlatten writes the listed non-secret settings into state. The client
// secret is never read back (the list masks it). YBA stores AZU_KEY_SIZE as
// the create body sent it: a JSON number from the UI, yba-cli and this
// provider, but a string from a client that sends one.
func azureFlatten(d *schema.ResourceData, settings map[string]interface{}) error {
	for attr, key := range azureSettingAttrs {
		if err := d.Set(attr, utils.StringValue(settings[key])); err != nil {
			return err
		}
	}
	size := utils.StringValue(settings[azureKeySize])
	if size == "" {
		return nil
	}
	n, err := strconv.Atoi(size)
	if err != nil {
		return fmt.Errorf("encryption at rest config %s: %s %q is not an integer",
			d.Id(), azureKeySize, size)
	}
	return d.Set("key_size", n)
}
