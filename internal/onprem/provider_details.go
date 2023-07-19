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
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	client "github.com/yugabyte/platform-go-client"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// ProviderDetailsSchema holds details of on prem cloud provider
func ProviderDetailsSchema() *schema.Schema {
	return &schema.Schema{
		Description: "Configuration details for onprem provider.",
		Required:    true,
		Type:        schema.TypeList,
		MaxItems:    1,
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				"air_gap_install": {
					Type:        schema.TypeBool,
					Optional:    true,
					Default:     false,
					Description: "Air Gap Installation required. False by default.",
				},
				"install_node_exporter": {
					Type:        schema.TypeBool,
					Optional:    true,
					Computed:    true,
					Description: "Install Node Exporter.",
				},
				"node_exporter_port": {
					Type:        schema.TypeInt,
					Optional:    true,
					Computed:    true,
					Description: "Node Exporter Port.",
				},
				"node_exporter_user": {
					Type:        schema.TypeString,
					Optional:    true,
					Computed:    true,
					Description: "Node Exporter User.",
				},
				"ntp_servers": {
					// list of strings
					Type:        schema.TypeList,
					Elem:        &schema.Schema{Type: schema.TypeString},
					Optional:    true,
					Computed:    true,
					Description: "List of NTP Servers.",
				},
				"passwordless_sudo_access": {
					Type:     schema.TypeBool,
					Required: true,
					Description: "Can sudo actions be carried out by " +
						"user without a password.",
				},
				"provision_instance_script": {
					Type:     schema.TypeString,
					Computed: true,
					Description: "Script to provision the YBDB nodes. " +
						"To be run on the YBDB nodes if skip_provisioning is set to false",
				},
				"skip_provisioning": {
					// set to true if nodes have been prepared
					// else set to false to run during universe creation
					Type:     schema.TypeBool,
					Required: true,
					Description: "Set to true if YBDB nodes have been prepared" +
						" manually, set to false to provision during universe creation.",
				},
				"ssh_port": {
					Type:        schema.TypeInt,
					Optional:    true,
					Computed:    true,
					Description: "SSH Port.",
				},
				"ssh_user": {
					Type:        schema.TypeString,
					Required:    true,
					Description: "SSH user.",
				},
				"yb_home_dir": {
					Type:        schema.TypeString,
					Optional:    true,
					Computed:    true,
					Description: "YB Home Directory.",
				},
			},
		},
	}
}

func buildProviderDetails(details interface{}) *client.ProviderDetails {
	p := details.([]interface{})[0].(map[string]interface{})
	ntpServers := p["ntp_servers"].([]interface{})
	return &client.ProviderDetails{
		AirGapInstall: utils.GetBoolPointer(p["air_gap_install"].(bool)),
		CloudInfo: &client.CloudInfo{
			Onprem: &client.OnPremCloudInfo{
				YbHomeDir: utils.GetStringPointer(p["yb_home_dir"].(string)),
			},
		},
		InstallNodeExporter:    utils.GetBoolPointer(p["install_node_exporter"].(bool)),
		NodeExporterPort:       utils.GetInt32Pointer(int32(p["node_exporter_port"].(int))),
		NodeExporterUser:       utils.GetStringPointer(p["node_exporter_user"].(string)),
		NtpServers:             utils.StringSlice(ntpServers),
		PasswordlessSudoAccess: utils.GetBoolPointer(p["passwordless_sudo_access"].(bool)),
		SkipProvisioning:       utils.GetBoolPointer(p["skip_provisioning"].(bool)),
		SshPort:                utils.GetInt32Pointer(int32(p["ssh_port"].(int))),
		SshUser:                utils.GetStringPointer(p["ssh_user"].(string)),
	}
}

func flattenProviderDetails(details client.ProviderDetails) (res []map[string]interface{}) {
	detail := map[string]interface{}{
		"air_gap_install":           details.GetAirGapInstall(),
		"install_node_exporter":     details.GetInstallNodeExporter(),
		"node_exporter_port":        details.GetNodeExporterPort(),
		"node_exporter_user":        details.GetNodeExporterUser(),
		"ntp_servers":               details.GetNtpServers(),
		"passwordless_sudo_access":  details.GetPasswordlessSudoAccess(),
		"provision_instance_script": details.GetProvisionInstanceScript(),
		"skip_provisioning":         details.GetSkipProvisioning(),
		"ssh_port":                  details.GetSshPort(),
		"ssh_user":                  details.GetSshUser(),
		"yb_home_dir":               details.GetCloudInfo().Onprem.GetYbHomeDir(),
	}
	res = append(res, detail)
	return res
}
