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

package onprem

import (
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	client "github.com/yugabyte/platform-go-client"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// AccessKeySchema manages access key information of on prem cloud provider
func AccessKeySchema() *schema.Schema {
	return &schema.Schema{
		Description: "Access key of provider.",
		Required:    true,
		Type:        schema.TypeList,
		MinItems:    1,
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				"creation_date": {
					Type:        schema.TypeString,
					Computed:    true,
					Description: "Creation date of Access Key.",
				},
				"expiration_date": {
					Type:        schema.TypeString,
					Computed:    true,
					Description: "Creation date of Access Key.",
				},
				"access_key_id": {
					Type:        schema.TypeList,
					Computed:    true,
					Description: "Access Key Identification.",
					Elem: &schema.Resource{
						Schema: map[string]*schema.Schema{
							"keycode": {
								Type:        schema.TypeString,
								Computed:    true,
								Description: "Key code.",
							},
							"provider_uuid": {
								Type:        schema.TypeString,
								Computed:    true,
								Description: "Provider UUID.",
							},
						},
					},
				},
				"key_info": KeyInfoSchema(),
			},
		},
	}
}

// KeyInfoSchema manages information about the access keys of on prem cloud provider
func KeyInfoSchema() *schema.Schema {
	return &schema.Schema{
		Description: "Key information to connect to YBDB nodes.",
		Required:    true,
		MinItems:    1,
		Type:        schema.TypeList,
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				"air_gap_install": {
					Type:        schema.TypeBool,
					Optional:    true,
					Default:     false,
					Description: "Air Gap Installation required. False by default.",
				},
				"delete_remote": {
					Type:        schema.TypeBool,
					Optional:    true,
					Computed:    true,
					Description: "Delete Remote.",
				},
				"install_node_exporter": {
					Type:        schema.TypeBool,
					Computed:    true,
					Description: "Install Node Exporter.",
				},
				"key_pair_name": {
					Type:        schema.TypeString,
					Required:    true,
					Description: "SSH Key Pair name.",
				},
				"ssh_private_key_file_path": {
					Type:        schema.TypeString,
					Required:    true,
					Description: "Private Key Path to access YBDB nodes.",
				},
				"node_exporter_port": {
					Type:        schema.TypeInt,
					Computed:    true,
					Description: "Node Exporter Port.",
				},
				"node_exporter_user": {
					Type:        schema.TypeString,
					Computed:    true,
					Description: "Node Exporter User.",
				},
				"ntp_servers": {
					// list of strings
					Type:        schema.TypeList,
					Elem:        &schema.Schema{Type: schema.TypeString},
					Computed:    true,
					Description: "List of NTP Servers.",
				},
				"passwordless_sudo_access": {
					Type:     schema.TypeBool,
					Computed: true,
					Description: "Can sudo actions be carried out by " +
						"user without a password.",
				},
				"private_key": {
					Type:        schema.TypeString,
					Computed:    true,
					Description: "Private Key to access YBDB nodes.",
				},
				"provision_instance_script": {
					Type:     schema.TypeString,
					Computed: true,
					Description: "Script to provision the YBDB nodes. " +
						"To be run on the YBDB nodes if skip_provisioning is set to false.",
				},
				"public_key": {
					Type:        schema.TypeString,
					Optional:    true,
					Computed:    true,
					Description: "Public Key to access YBDB nodes.",
				},
				"skip_provisioning": {
					Type:     schema.TypeBool,
					Computed: true,
					Description: "Set to true if YBDB nodes have been prepared" +
						" manually, set to false to provision during universe creation.",
				},
				"ssh_port": {
					Type:        schema.TypeInt,
					Computed:    true,
					Description: "SSH Port.",
				},
				"ssh_user": {
					Type:        schema.TypeString,
					Computed:    true,
					Description: "SSH user.",
				},
				"vault_file": {
					Type:        schema.TypeString,
					Optional:    true,
					Computed:    true,
					Description: "Vault file.",
				},
				"vault_password_file": {
					Type:        schema.TypeString,
					Optional:    true,
					Computed:    true,
					Description: "Vault password file.",
				},
			},
		},
	}
}

func buildAccessKeys(accessKeys []interface{}) (*[]client.AccessKey, error) {
	accessKeyList := make([]client.AccessKey, 0)
	for _, v := range accessKeys {
		accessKey := v.(map[string]interface{})
		keyInfo, err := buildKeyInfo(accessKey["key_info"].(interface{}))
		if err != nil {
			return nil, err
		}
		r := client.AccessKey{
			KeyInfo: keyInfo,
		}
		accessKeyList = append(accessKeyList, r)
	}
	return &accessKeyList, nil
}

func buildKeyInfo(keyInfo interface{}) (client.KeyInfo, error) {
	key := keyInfo.([]interface{})[0].(map[string]interface{})
	keyPairName := key["key_pair_name"].(string)
	var sshPrivateKeyContent *string
	var err error
	if keyPairName == "" {
		return client.KeyInfo{}, fmt.Errorf("key_pair_name is empty")
	}
	// Since Key Pair name has been provided, the user has provided
	// an access key to be used by YBA to connect to the YBDB nodes.
	// When the provider is imported, the path for the ssh_private_key_file
	// can be left empty, since the private key infomation is stored in YBA.

	// If both ssh_private_key_file_path and private_key are empty, there is
	// no access key provided to connect to the nodes, and will throw an error
	if key["ssh_private_key_file_path"].(string) != "" {
		sshFilePath := key["ssh_private_key_file_path"].(string)
		if err := utils.FileExist(sshFilePath); err != nil {
			return client.KeyInfo{}, err
		}
		sshPrivateKeyContent, err = utils.ReadSSHPrivateKey(sshFilePath)
		if err != nil {
			return client.KeyInfo{}, err
		}
	} else {
		if ybaPrivateKey, exists := key["private_key"]; !exists || ybaPrivateKey == "" {
			return client.KeyInfo{}, fmt.Errorf("ssh_private_key_file_path is empty")
		}
	}

	return client.KeyInfo{
		KeyPairName:          utils.GetStringPointer(keyPairName),
		SshPrivateKeyContent: sshPrivateKeyContent,
		VaultFile:            utils.GetStringPointer(key["vault_file"].(string)),
		VaultPasswordFile:    utils.GetStringPointer(key["vault_password_file"].(string)),
	}, nil
}

func flattenAccessKeys(accessKeys []client.AccessKey, d *schema.ResourceData) (
	res []map[string]interface{}) {
	for _, key := range accessKeys {
		r := map[string]interface{}{
			"creation_date":   time.Time.String(key.GetCreationDate()),
			"expiration_date": time.Time.String(key.GetExpirationDate()),
			"access_key_id":   flattenAccessKeyID(key.GetIdKey()),
			"key_info":        flattenKeyInfo(key.GetKeyInfo(), d),
		}
		res = append(res, r)
	}
	return res
}

func flattenAccessKeyID(idKey client.AccessKeyId) (res []map[string]interface{}) {
	id := map[string]interface{}{
		"keycode":       idKey.GetKeyCode(),
		"provider_uuid": idKey.GetProviderUUID(),
	}
	res = append(res, id)
	return res
}

func flattenKeyInfo(key client.KeyInfo, d *schema.ResourceData) (res []map[string]interface{}) {
	k := map[string]interface{}{
		"air_gap_install":           key.GetAirGapInstall(),
		"delete_remote":             key.GetDeleteRemote(),
		"install_node_exporter":     key.GetInstallNodeExporter(),
		"key_pair_name":             key.GetKeyPairName(),
		"node_exporter_port":        key.GetNodeExporterPort(),
		"node_exporter_user":        key.GetNodeExporterUser(),
		"ntp_servers":               key.GetNtpServers(),
		"passwordless_sudo_access":  key.GetPasswordlessSudoAccess(),
		"private_key":               key.GetPrivateKey(),
		"provision_instance_script": key.GetProvisionInstanceScript(),
		"public_key":                key.GetPublicKey(),
		"skip_provisioning":         key.GetSkipProvisioning(),
		"ssh_port":                  key.GetSshPort(),
		"ssh_user":                  key.GetSshUser(),
		"vault_file":                key.GetVaultFile(),
		"vault_password_file":       key.GetVaultPasswordFile(),
		"ssh_private_key_file_path": d.Get("access_keys.0.key_info.0.ssh_private_key_file_path"),
	}
	res = append(res, k)
	return res
}
