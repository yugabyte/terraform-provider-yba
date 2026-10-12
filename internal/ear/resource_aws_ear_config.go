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
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// AWS authConfig keys, named as in YBA's AwsKmsAuthConfigField.
const (
	awsKeyAccessKeyID     = "AWS_ACCESS_KEY_ID"
	awsKeySecretAccessKey = "AWS_SECRET_ACCESS_KEY"
	awsKeyKMSEndpoint     = "AWS_KMS_ENDPOINT"
	awsKeyRegion          = "AWS_REGION"
	awsKeyCMKID           = "cmk_id"
)

// awsSettingAttrs maps the non-secret settings YBA lists onto the resource's
// attributes. A key YBA does not list reads as "" (no endpoint override).
var awsSettingAttrs = map[string]string{
	"region":       awsKeyRegion,
	"cmk_id":       awsKeyCMKID,
	"kms_endpoint": awsKeyKMSEndpoint,
}

// ResourceAWSEARConfig defines the AWS KMS encryption-at-rest configuration.
func ResourceAWSEARConfig() *schema.Resource {
	return earResource(awsEARSpec())
}

func awsEARSpec() earSpec {
	return earSpec{
		displayName: "AWS KMS",
		apiProvider: providerAWS,
		description: "Manages a YugabyteDB Anywhere encryption-at-rest configuration that " +
			"uses an AWS KMS key as the master key. YugabyteDB Anywhere uses the KMS key to " +
			"wrap and unwrap the universe keys of each universe that uses the " +
			"configuration.\n\n" +
			"Point the configuration at an existing symmetric KMS key with `cmk_id`, or let " +
			"YugabyteDB Anywhere create one. Terraform does not manage or delete a key that " +
			"YugabyteDB Anywhere creates.\n\n" +
			"There are two authentication modes. Set `access_key_id` and " +
			"`secret_access_key`, or leave both unset to authenticate as the YugabyteDB " +
			"Anywhere host through the AWS default credential chain, for example the " +
			"instance profile on EC2. Without access keys, YugabyteDB Anywhere does not use " +
			"`region`: the AWS SDK on the host selects the region, so the KMS key must be " +
			"in that region. The identity needs `kms:DescribeKey`, " +
			"`kms:GenerateDataKeyWithoutPlaintext`, `kms:Encrypt` and `kms:Decrypt` on the " +
			"KMS key, which YugabyteDB Anywhere checks when it creates the configuration. " +
			"YugabyteDB Anywhere also gives the KMS key an alias for each universe that uses " +
			"it, which needs `kms:ListAliases`, `kms:CreateAlias`, `kms:UpdateAlias` and " +
			"`kms:DeleteAlias`.\n\n" +
			"~> **Security Note:** `access_key_id` and `secret_access_key` are stored in the " +
			"Terraform state file and marked sensitive. Use a secure state backend and " +
			"restrict access to state files, or authenticate as the YugabyteDB Anywhere " +
			"host, which keeps the keys out of Terraform.",
		fields: map[string]*schema.Schema{
			"access_key_id": {
				Type:         schema.TypeString,
				Optional:     true,
				Sensitive:    true,
				RequiredWith: []string{"secret_access_key"},
				Description: "AWS access key ID. Set it with `secret_access_key`, or leave " +
					"both unset to authenticate as the YugabyteDB Anywhere host. Can change " +
					"in place: YugabyteDB Anywhere first checks that the new credentials can " +
					"decrypt the active universe key of each universe that uses the " +
					"configuration.",
			},
			"secret_access_key": {
				Type:         schema.TypeString,
				Optional:     true,
				Sensitive:    true,
				RequiredWith: []string{"access_key_id"},
				Description: "AWS secret access key for `access_key_id`. Can change in " +
					"place, together with `access_key_id`.",
			},
			"region": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
				Description: "AWS region of the KMS key, for example `us-west-2`. Used only " +
					"with access keys. A change forces replacement.",
			},
			"cmk_id": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
				ForceNew: true,
				Description: "Key ID or key ARN of an existing symmetric KMS key. This key " +
					"is the master key. When not set, YugabyteDB Anywhere creates a KMS key " +
					"with a key policy for the account root and the configured identity, " +
					"and this attribute returns its key ID. To check the credentials first, " +
					"it also creates a test key and schedules its deletion. A change forces " +
					"replacement.",
			},
			"kms_endpoint": {
				Type:         schema.TypeString,
				Optional:     true,
				RequiredWith: []string{"access_key_id"},
				Description: "Custom AWS KMS endpoint, for example a VPC endpoint. " +
					"A value without a scheme uses HTTPS. " +
					"Requires access keys. Can change in place.",
			},
		},
		credentialFields: []string{"access_key_id", "secret_access_key"},
		buildCreate:      awsBuildCreate,
		buildEdit:        awsBuildEdit,
		flatten:          awsFlatten,
	}
}

// awsBuildCreate leaves unset settings out of the body: YBA reads a present
// cmk_id, even an empty one, as the key to use instead of creating one.
func awsBuildCreate(d *schema.ResourceData) (map[string]interface{}, error) {
	settings := map[string]interface{}{awsKeyRegion: d.Get("region").(string)}
	utils.SetIfNonEmpty(settings, awsKeyCMKID, d.Get("cmk_id"))
	utils.SetIfNonEmpty(settings, awsKeyAccessKeyID, d.Get("access_key_id"))
	utils.SetIfNonEmpty(settings, awsKeySecretAccessKey, d.Get("secret_access_key"))
	utils.SetIfNonEmpty(settings, awsKeyKMSEndpoint, d.Get("kms_endpoint"))
	return settings, nil
}

// awsBuildEdit builds the edit body. YBA's EncryptionAtRestController (AWS
// case of the edit merge) copies the editable keys in the body over the
// stored configuration and keeps the rest, so an empty string is how a
// removed value is cleared: blank keys mean the host's credentials. The keys
// go only when they changed, so an endpoint change does not overwrite keys
// changed outside Terraform.
func awsBuildEdit(d *schema.ResourceData) (map[string]interface{}, error) {
	settings := map[string]interface{}{awsKeyKMSEndpoint: d.Get("kms_endpoint").(string)}
	if d.HasChanges("access_key_id", "secret_access_key") {
		settings[awsKeyAccessKeyID] = d.Get("access_key_id").(string)
		settings[awsKeySecretAccessKey] = d.Get("secret_access_key").(string)
	}
	return settings, nil
}

// awsFlatten writes the listed non-secret settings into state. The access
// keys are never read back (the list masks them).
func awsFlatten(d *schema.ResourceData, settings map[string]interface{}) error {
	for attr, key := range awsSettingAttrs {
		if err := d.Set(attr, utils.StringValue(settings[key])); err != nil {
			return err
		}
	}
	return nil
}
