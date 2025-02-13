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

package utils

import client "github.com/yugabyte/platform-go-client"

// Provider struct for Provider
type Provider struct {
	// Provider active status
	Active *bool `json:"active,omitempty"`
	AirGapInstall *bool `json:"airGapInstall,omitempty"`
	AllAccessKeys *[]client.AccessKey `json:"allAccessKeys,omitempty"`
	// Provider cloud code
	Code *string `json:"code,omitempty"`
	Config *map[string]string `json:"config,omitempty"`
	// Customer uuid
	CustomerUUID *string `json:"customerUUID,omitempty"`
	// Deprecated since YBA version 2.17.2.0
	DestVpcId *string `json:"destVpcId,omitempty"`
	Details *ProviderDetails `json:"details,omitempty"`
	// Deprecated since YBA version 2.17.2.0
	HostVpcId *string `json:"hostVpcId,omitempty"`
	// Deprecated since YBA version 2.17.2.0
	HostVpcRegion *string `json:"hostVpcRegion,omitempty"`
	ImageBundles []client.ImageBundle `json:"imageBundles"`
	KeyPairName *string `json:"keyPairName,omitempty"`
	// Provider name
	Name *string `json:"name,omitempty"`
	// Deprecated since YBA version 2.17.2.0, Use details.ntpServers instead.
	NtpServers *[]string `json:"ntpServers,omitempty"`
	Regions []client.Region `json:"regions"`
	// Deprecated since YBA version 2.17.2.0. User details.setUpChrony instead.
	SetUpChrony *bool `json:"setUpChrony,omitempty"`
	// Deprecated since YBA version 2.17.2.0. Use details.showSetUpChrony instead.
	ShowSetUpChrony *bool `json:"showSetUpChrony,omitempty"`
	SshPort *int32 `json:"sshPort,omitempty"`
	SshPrivateKeyContent *string `json:"sshPrivateKeyContent,omitempty"`
	SshUser *string `json:"sshUser,omitempty"`
	// Current usability state
	UsabilityState *string `json:"usabilityState,omitempty"`
	// Provider uuid
	Uuid *string `json:"uuid,omitempty"`
	// Provider version
	Version *int64 `json:"version,omitempty"`
}

// ProviderDetails struct for ProviderDetails
type ProviderDetails struct {
	AirGapInstall *bool `json:"airGapInstall,omitempty"`
	CloudInfo *CloudInfo `json:"cloudInfo,omitempty"`
	EnableNodeAgent *bool `json:"enableNodeAgent,omitempty"`
	InstallNodeExporter *bool `json:"installNodeExporter,omitempty"`
	NodeExporterPort *int32 `json:"nodeExporterPort,omitempty"`
	NodeExporterUser *string `json:"nodeExporterUser,omitempty"`
	NtpServers *[]string `json:"ntpServers,omitempty"`
	PasswordlessSudoAccess *bool `json:"passwordlessSudoAccess,omitempty"`
	ProvisionInstanceScript *string `json:"provisionInstanceScript,omitempty"`
	SetUpChrony *bool `json:"setUpChrony,omitempty"`
	ShowSetUpChrony *bool `json:"showSetUpChrony,omitempty"`
	SkipProvisioning *bool `json:"skipProvisioning,omitempty"`
	SshPort *int32 `json:"sshPort,omitempty"`
	SshUser *string `json:"sshUser,omitempty"`
}

// CloudInfo struct for CloudInfo
type CloudInfo struct {
	Aws *client.AWSCloudInfo `json:"aws,omitempty"`
	Azu *client.AzureCloudInfo `json:"azu,omitempty"`
	Gcp *GCPCloudInfo `json:"gcp,omitempty"`
	Kubernetes *client.KubernetesInfo `json:"kubernetes,omitempty"`
	Local *client.LocalCloudInfo `json:"local,omitempty"`
	Onprem *client.OnPremCloudInfo `json:"onprem,omitempty"`
}

// GCPCloudInfo struct for GCPCloudInfo
type GCPCloudInfo struct {
	DestVpcId *string `json:"destVpcId,omitempty"`
	GceApplicationCredentialsPath *string `json:"gceApplicationCredentialsPath,omitempty"`
	GceApplicationCredentials interface{} `json:"gceApplicationCredentials,omitempty"`
	GceProject *string `json:"gceProject,omitempty"`
	HostVpcId *string `json:"hostVpcId,omitempty"`
	SharedVPCProject *string `json:"sharedVPCProject,omitempty"`
	UseHostCredentials *bool `json:"useHostCredentials,omitempty"`
	UseHostVPC *bool `json:"useHostVPC,omitempty"`
	// New/Existing VPC for provider creation
	VpcType *string `json:"vpcType,omitempty"`
	YbFirewallTags *string `json:"ybFirewallTags,omitempty"`
}
