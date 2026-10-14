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

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"

	"github.com/yugabyte/terraform-provider-yba/internal/api"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// CipherTrust authConfig keys, named as in YBA's CipherTrustKmsAuthConfigField.
const (
	ctKeyManagerURL   = "CIPHERTRUST_MANAGER_URL"
	ctKeyAuthType     = "AUTH_TYPE"
	ctKeyUsername     = "USERNAME"
	ctKeyPassword     = "PASSWORD"
	ctKeyRefreshToken = "REFRESH_TOKEN"
	ctKeyName         = "KEY_NAME"
	ctKeyAlgorithm    = "KEY_ALGORITHM"
	ctKeySize         = "KEY_SIZE"
)

// YBA's CipherTrustAlgorithm has one algorithm, AES, with these key sizes.
// YBA requires both keys on create; yba-cli defaults the size to 256.
const (
	ctAlgorithmAES   = "AES"
	ctDefaultKeySize = 256
)

var ctKeySizes = []int{128, 192, 256}

// YBA's CiphertrustManagerClient.AuthType values (case sensitive).
const (
	ctAuthPassword     = "PASSWORD"
	ctAuthRefreshToken = "REFRESH_TOKEN"
)

// ctAuthFields lists the credential arguments of each auth type. YBA rejects
// a configuration that sets the arguments of the other type.
var ctAuthFields = map[string][]string{
	ctAuthPassword:     {"username", "password"},
	ctAuthRefreshToken: {"refresh_token"},
}

// ctAllowFlag is the global runtime config key that YBA checks before every
// create, edit and delete of a CipherTrust configuration.
const ctAllowFlag = "yb.kms.allow_ciphertrust"

// ctMin is the first build with CipherTrust KMS: yugabyte-db commit
// 17d8237e1cc, first in 2.25.1.0-b244 and, on the stable line, in
// 2025.1.0.0-b1.
var ctMin = utils.YBAMinimumVersion{Stable: "2025.1.0.0-b1", Preview: "2.25.1.0-b244"}

// ctSettingAttrs maps the string settings YBA lists in clear onto the
// resource's attributes. Only PASSWORD and REFRESH_TOKEN are masked.
var ctSettingAttrs = map[string]string{
	"manager_url": ctKeyManagerURL,
	"auth_type":   ctKeyAuthType,
	"username":    ctKeyUsername,
	"key_name":    ctKeyName,
}

// ResourceCipherTrustEARConfig defines the CipherTrust Manager
// encryption-at-rest configuration.
func ResourceCipherTrustEARConfig() *schema.Resource {
	return earResource(cipherTrustEARSpec())
}

func cipherTrustEARSpec() earSpec {
	return earSpec{
		displayName: "CipherTrust Manager",
		apiProvider: providerCipherTrust,
		description: "Manages a YugabyteDB Anywhere encryption-at-rest configuration that " +
			"uses an AES key in Thales CipherTrust Manager as the master key. YugabyteDB " +
			"Anywhere uses the key to wrap and unwrap the universe keys of each universe " +
			"that uses the configuration.\n\n" +
			"~> **Note:** Requires YugabyteDB Anywhere 2025.1.0.0 or later, with the global " +
			"runtime config key `yb.kms.allow_ciphertrust` set to `true`, for example with " +
			"the `yba_runtime_config` resource. `terraform plan` fails on an older YBA, and " +
			"an apply fails while the key is `false`.\n\n" +
			"Point the configuration at an existing key with `key_name`, or let YugabyteDB " +
			"Anywhere create an AES key of `key_size` bits with that name. Terraform does not " +
			"manage or delete a key that YugabyteDB Anywhere creates. An existing key must be " +
			"an active AES key whose usage mask allows encrypt and decrypt.\n\n" +
			"There are two authentication modes, set by `auth_type`. With `PASSWORD`, " +
			"YugabyteDB Anywhere logs in with `username` and `password`. With " +
			"`REFRESH_TOKEN`, it uses `refresh_token`, a refresh token of a CipherTrust " +
			"Manager user. The user must be able to read the key and to encrypt and decrypt " +
			"with it, and to create keys when YugabyteDB Anywhere creates the key.\n\n" +
			"~> **Security Note:** `password` and `refresh_token` are stored in the " +
			"Terraform state file and marked sensitive. Use a secure state backend and " +
			"restrict access to state files.",
		fields: map[string]*schema.Schema{
			"manager_url": {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validation.IsURLWithHTTPorHTTPS,
				Description: "URL of the CipherTrust Manager, for example " +
					"`https://ciphertrust.example.com`. YugabyteDB Anywhere connects to it " +
					"directly. A change forces replacement.",
			},
			"auth_type": {
				Type:     schema.TypeString,
				Optional: true,
				Default:  ctAuthPassword,
				ValidateFunc: validation.StringInSlice(
					[]string{ctAuthPassword, ctAuthRefreshToken}, false),
				Description: "How YugabyteDB Anywhere authenticates: `PASSWORD` or " +
					"`REFRESH_TOKEN`. Defaults to `PASSWORD`. Can change in place, " +
					"together with the credentials of the new type.",
			},
			"username": {
				Type:     schema.TypeString,
				Optional: true,
				Description: "CipherTrust Manager user name. Required when " +
					"`auth_type = \"PASSWORD\"`, and not allowed otherwise. Can change in " +
					"place.",
			},
			"password": {
				Type:      schema.TypeString,
				Optional:  true,
				Sensitive: true,
				Description: "Password of `username`. Required when " +
					"`auth_type = \"PASSWORD\"`, and not allowed otherwise. Can change in " +
					"place: YugabyteDB Anywhere first checks that the new credentials can " +
					"decrypt the active universe key of each universe that uses the " +
					"configuration.",
			},
			"refresh_token": {
				Type:      schema.TypeString,
				Optional:  true,
				Sensitive: true,
				Description: "Refresh token of a CipherTrust Manager user. Required when " +
					"`auth_type = \"REFRESH_TOKEN\"`, and not allowed otherwise. Can change " +
					"in place.",
			},
			"key_name": {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validation.StringIsNotEmpty,
				Description: "Name of the key in CipherTrust Manager. This key is the master " +
					"key. YugabyteDB Anywhere creates it as an AES key of `key_size` bits when " +
					"it does not exist. A change forces replacement.",
			},
			"key_size": {
				Type:             schema.TypeInt,
				Optional:         true,
				Computed:         true,
				ForceNew:         true,
				ValidateFunc:     validation.IntInSlice(ctKeySizes),
				DiffSuppressFunc: ignoreAfterCreate,
				Description: "Size in bits of the AES key that YugabyteDB Anywhere creates: " +
					"`128`, `192` or `256`, and `256` when not set. YugabyteDB Anywhere uses it " +
					"only when it creates the key. For an existing key, this attribute returns " +
					"the key's actual size. A key cannot change its size, so Terraform ignores " +
					"a change to this argument after creation.",
			},
		},
		credentialFields: []string{"password", "refresh_token"},
		buildCreate:      ctBuildCreate,
		buildEdit:        ctBuildEdit,
		flatten:          ctFlatten,
		customizeDiff:    ctCustomizeDiff,
		runtimeFlag:      ctAllowFlag,
	}
}

// ctCustomizeDiff gates a new configuration on the YBA version and checks the
// credentials against auth_type. A secret is required only when the plan
// sends it (create, or a change to it or to auth_type): after an import, the
// state holds no secret (YBA lists it masked), and ignore_changes plans that
// empty value. Unknown values are checked again when the body is built.
func ctCustomizeDiff(ctx context.Context, d *schema.ResourceDiff, meta interface{}) error {
	if d.Id() == "" {
		if err := ctRequireVersion(ctx, meta); err != nil {
			return err
		}
	}
	if !d.NewValueKnown("auth_type") {
		return nil
	}
	authType := d.Get("auth_type").(string)
	if err := ctCheckOtherAuthType(authType, d.Get); err != nil {
		return err
	}
	for _, f := range ctAuthFields[authType] {
		sent := f == "username" || d.Id() == "" || d.HasChanges("auth_type", f)
		if sent && d.NewValueKnown(f) && d.Get(f).(string) == "" {
			return fmt.Errorf("%s is required when auth_type = %q", f, authType)
		}
	}
	return nil
}

// ctCheckOtherAuthType rejects a credential of an auth type other than
// authType. YBA rejects it too, but a Create or Update that left it out of the
// body would succeed, and every later plan would show it as drift.
func ctCheckOtherAuthType(authType string, get func(string) interface{}) error {
	for t, fields := range ctAuthFields {
		if t == authType {
			continue
		}
		for _, f := range fields {
			if get(f).(string) != "" {
				return fmt.Errorf("%s requires auth_type = %q (auth_type = %q)", f, t, authType)
			}
		}
	}
	return nil
}

// ctRequireVersion fails the plan when the target YBA predates CipherTrust
// KMS. A version outside YBA's scheme passes and is only logged (tflog.Warn,
// seen with TF_LOG set); the server stays the backstop.
func ctRequireVersion(ctx context.Context, meta interface{}) error {
	c, ok := meta.(*api.APIClient)
	if !ok || c == nil {
		return nil
	}
	version, err := c.AppVersion(ctx)
	if err != nil || version == "" {
		return err
	}
	ok, applied, err := utils.MeetsMinimum(version, ctMin)
	switch {
	case err != nil:
		tflog.Warn(ctx, "cannot parse the YBA version; skipping the minimum-version check",
			map[string]interface{}{"version": version, "check": "yba_ciphertrust_ear_config"})
	case !ok:
		return fmt.Errorf("yba_ciphertrust_ear_config requires YugabyteDB Anywhere %s or "+
			"later; the target YBA reports %s", applied, version)
	}
	return nil
}

// ctAuthSettings returns the credentials of auth_type that the body sends.
// The user name always goes: YBA lists it in clear, so the state holds what
// YBA holds. A secret goes when send says so; it cannot be empty then. It
// repeats the plan's checks for the values that were unknown at plan time.
func ctAuthSettings(
	d *schema.ResourceData, send func(field string) bool,
) (map[string]interface{}, error) {
	authType := d.Get("auth_type").(string)
	if err := ctCheckOtherAuthType(authType, d.Get); err != nil {
		return nil, err
	}
	settings := map[string]interface{}{ctKeyAuthType: authType}
	keys := map[string]string{
		"username": ctKeyUsername, "password": ctKeyPassword, "refresh_token": ctKeyRefreshToken,
	}
	for _, f := range ctAuthFields[authType] {
		if f != "username" && !send(f) {
			continue
		}
		v := d.Get(f).(string)
		if v == "" {
			return nil, fmt.Errorf("%s is required when auth_type = %q", f, authType)
		}
		settings[keys[f]] = v
	}
	return settings, nil
}

func ctBuildCreate(d *schema.ResourceData) (map[string]interface{}, error) {
	settings, err := ctAuthSettings(d, func(string) bool { return true })
	if err != nil {
		return nil, err
	}
	keySize := d.Get("key_size").(int)
	if keySize == 0 {
		keySize = ctDefaultKeySize
	}
	settings[ctKeyManagerURL] = d.Get("manager_url").(string)
	settings[ctKeyName] = d.Get("key_name").(string)
	settings[ctKeyAlgorithm] = ctAlgorithmAES
	settings[ctKeySize] = keySize
	return settings, nil
}

// ctBuildEdit builds the edit body. YBA's EncryptionAtRestController
// (CIPHERTRUST case of the edit merge) requires AUTH_TYPE, copies a stored
// credential of that type that the body leaves out, and replaces the stored
// configuration with the merged body, which drops the credentials of the
// other type. So a secret goes when it or auth_type changed. Otherwise it
// stays out of the body, so a user name change does not overwrite a password
// changed outside Terraform.
func ctBuildEdit(d *schema.ResourceData) (map[string]interface{}, error) {
	return ctAuthSettings(d, func(f string) bool { return d.HasChanges("auth_type", f) })
}

// ctFlatten writes the listed non-secret settings into state. The password
// and the refresh token are never read back (the list masks them).
func ctFlatten(d *schema.ResourceData, settings map[string]interface{}) error {
	for attr, key := range ctSettingAttrs {
		if err := d.Set(attr, utils.StringValue(settings[key])); err != nil {
			return err
		}
	}
	return setIntSetting(d, "key_size", settings, ctKeySize)
}
