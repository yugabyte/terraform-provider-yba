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

package cloud_provider

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	client "github.com/yugabyte/platform-go-client"

	"github.com/yugabyte/terraform-provider-yba/internal/api"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

func gcpCreateProviderRestAPI(
	reqBody utils.Provider,
	meta interface{}) (client.YBPTask, error) {

	cUUID := meta.(*api.APIClient).CustomerID
	vc := meta.(*api.APIClient).VanillaClient

	token := meta.(*api.APIClient).APIKey
	errorTag := fmt.Errorf("Provider, Operation: Create")

	reqBytes, err := json.Marshal(reqBody)
	if err != nil {
		return client.YBPTask{},
			fmt.Errorf("%w: %s", errorTag, err.Error())
	}

	reqBuf := bytes.NewBuffer(reqBytes)

	var req *http.Request
	if vc.EnableHTTPS {
		http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
		req, err = http.NewRequest("POST", fmt.Sprintf("https://%s/api/v1/customers/%s/providers",
			vc.Host, cUUID), reqBuf)
	} else {
		req, err = http.NewRequest("POST", fmt.Sprintf("http://%s/api/v1/customers/%s/providers",
			vc.Host, cUUID), reqBuf)
	}

	if err != nil {
		return client.YBPTask{}, fmt.Errorf("%w: %s", errorTag, err.Error())
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-AUTH-YW-API-TOKEN", token)

	r, err := vc.Client.Do(req)
	if err != nil {
		return client.YBPTask{},
			fmt.Errorf("%w: Error occured during POST call for create provider %s",
				errorTag,
				err.Error())
	}

	var body []byte
	body, err = io.ReadAll(r.Body)
	if err != nil {
		return client.YBPTask{},
			fmt.Errorf("%w: Error reading create provider response body %s",
				errorTag,
				err.Error())
	}

	responseBody := client.YBPTask{}
	if err = json.Unmarshal(body, &responseBody); err != nil {
		return client.YBPTask{},
			fmt.Errorf("%w: Failed unmarshalling create provider response body %s",
				errorTag,
				err.Error())
	}

	if responseBody.TaskUUID != nil {
		return responseBody, nil
	}

	responseBodyError := utils.YbaStructuredError{}
	if err = json.Unmarshal(body, &responseBodyError); err != nil {
		return client.YBPTask{},
			fmt.Errorf("%w: Failed unmarshalling create provider error response body %s",
				errorTag,
				err.Error())
	}

	errorMessage := utils.ErrorFromResponseBody(responseBodyError)
	return client.YBPTask{},
		fmt.Errorf("%w: Error fetching task uuid for create GCP provider: %s", errorTag, errorMessage)

}

func gcpProviderCreate(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{},
	imageBundles []client.ImageBundle) (client.YBPTask, error) {

	tflog.Info(ctx, "Creating GCP provider using REST function")

	var isIAM bool
	var configSettings map[string]interface{}

	cloudInfo := &utils.CloudInfo{}
	gcpCloudInfo := &utils.GCPCloudInfo{}

	configInterface := d.Get("gcp_config_settings").([]interface{})
	if len(configInterface) > 0 && configInterface[0] != nil {
		configSettings = utils.MapFromSingletonList(
			configInterface,
		)
		ybFirewallTags := configSettings["yb_firewall_tags"].(string)
		if len(ybFirewallTags) > 0 {
			gcpCloudInfo.YbFirewallTags = utils.GetStringPointer(ybFirewallTags)
		}
		useHostVpc := strconv.FormatBool(configSettings["use_host_vpc"].(bool))
		if len(useHostVpc) > 0 {
			gcpCloudInfo.UseHostVPC = utils.GetBoolPointer(configSettings["use_host_vpc"].(bool))
		}
		useHostCredentials := strconv.FormatBool(configSettings["use_host_credentials"].(bool))
		if len(useHostCredentials) > 0 {
			gcpCloudInfo.UseHostCredentials = utils.GetBoolPointer(
				configSettings["use_host_credentials"].(bool))
			isIAM = configSettings["use_host_credentials"].(bool)
		}
		network := configSettings["network"].(string)
		if len(network) > 0 {
			gcpCloudInfo.DestVpcId = utils.GetStringPointer(network)
		}
	}
	if !isIAM {
		applicationCreds := configSettings["application_credentials"]
		if len(configSettings) == 0 || applicationCreds == nil ||
			len(applicationCreds.(map[string]interface{})) == 0 {
			creds, err := utils.GcpGetCredentialsAsMap()
			if err != nil {
				return client.YBPTask{}, err
			}
			gcpCloudInfo.GceApplicationCredentials = creds
		} else {
			gcpCloudInfo.GceApplicationCredentials = applicationCreds.(map[string]interface{})

		}
	}
	projectID := configSettings["project_id"].(string)
	if len(projectID) > 0 {
		gcpCloudInfo.GceProject = utils.GetStringPointer(projectID)
	}
	sharedVPCProjectID := configSettings["shared_vpc_project_id"].(string)
	if len(sharedVPCProjectID) > 0 {
		gcpCloudInfo.SharedVPCProject = utils.GetStringPointer(sharedVPCProjectID)
	}
	cloudInfo.Gcp = gcpCloudInfo

	providerDetails := &utils.ProviderDetails{
		AirGapInstall:   utils.GetBoolPointer(d.Get("air_gap_install").(bool)),
		SshPort:         utils.GetInt32Pointer(int32(d.Get("ssh_port").(int))),
		SshUser:         utils.GetStringPointer(d.Get("ssh_user").(string)),
		NtpServers:      utils.StringSlice(d.Get("ntp_servers").([]interface{})),
		ShowSetUpChrony: utils.GetBoolPointer(d.Get("show_set_up_chrony").(bool)),
		SetUpChrony:     utils.GetBoolPointer(d.Get("set_up_chrony").(bool)),
		CloudInfo:       cloudInfo,
	}
	req := utils.Provider{
		ImageBundles:         imageBundles,
		Code:                 utils.GetStringPointer(d.Get("code").(string)),
		DestVpcId:            utils.GetStringPointer(d.Get("dest_vpc_id").(string)),
		HostVpcId:            utils.GetStringPointer(d.Get("host_vpc_id").(string)),
		HostVpcRegion:        utils.GetStringPointer(d.Get("host_vpc_region").(string)),
		KeyPairName:          utils.GetStringPointer(d.Get("key_pair_name").(string)),
		Name:                 utils.GetStringPointer(d.Get("name").(string)),
		SshPort:              utils.GetInt32Pointer(int32(d.Get("ssh_port").(int))),
		SshPrivateKeyContent: utils.GetStringPointer(d.Get("ssh_private_key_content").(string)),
		SshUser:              utils.GetStringPointer(d.Get("ssh_user").(string)),
		Regions:              buildRegions(d.Get("regions").([]interface{})),
		Details:              providerDetails,
	}

	r, err := gcpCreateProviderRestAPI(req, meta)
	if err != nil {
		return client.YBPTask{}, err
	}

	return r, nil
}

func gcpGetProviderRestAPI(
	meta interface{}, pUUID string) (utils.Provider, error) {

	cUUID := meta.(*api.APIClient).CustomerID
	vc := meta.(*api.APIClient).VanillaClient

	token := meta.(*api.APIClient).APIKey
	errorTag := fmt.Errorf("Provider, Operation: Get")

	var err error

	var req *http.Request
	if vc.EnableHTTPS {
		http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
		req, err = http.NewRequest("GET", fmt.Sprintf("https://%s/api/v1/customers/%s/providers/%s",
			vc.Host, cUUID, pUUID), nil)
	} else {
		req, err = http.NewRequest("GET", fmt.Sprintf("http://%s/api/v1/customers/%s/providers/%s",
			vc.Host, cUUID, pUUID), nil)
	}
	if err != nil {
		return utils.Provider{}, fmt.Errorf("%w: %s", errorTag, err.Error())
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-AUTH-YW-API-TOKEN", token)

	r, err := vc.Client.Do(req)
	if err != nil {
		return utils.Provider{}, fmt.Errorf("%w: Error occured during GET call for get provider %s",
			errorTag,
			err.Error())
	}

	var body []byte
	body, err = io.ReadAll(r.Body)
	if err != nil {
		return utils.Provider{}, fmt.Errorf("%w: Error reading get provider response body %s",
			errorTag,
			err.Error())
	}

	responseBody := utils.Provider{}
	if err = json.Unmarshal(body, &responseBody); err != nil {
		return utils.Provider{}, fmt.Errorf("%w: Failed unmarshalling get provider response body %s",
			errorTag,
			err.Error())
	}

	if len(*responseBody.Uuid) != 0 {
		return responseBody, nil
	}
	responseBodyError := utils.YbaStructuredError{}
	if err = json.Unmarshal(body, &responseBodyError); err != nil {
		return utils.Provider{},
			fmt.Errorf("%w: Failed unmarshalling get provider error response body %s",
				errorTag,
				err.Error())
	}

	errorMessage := utils.ErrorFromResponseBody(responseBodyError)
	return utils.Provider{},
		fmt.Errorf("%w: Error fetching get provider: %s", errorTag, errorMessage)

}

func gcpProviderRead(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{}) error {
	// Set all the terraform fields

	UUID := d.Id()

	tflog.Info(ctx, fmt.Sprintf("Reading GCP provider from REST function %s", UUID))

	provider, err := gcpGetProviderRestAPI(meta, UUID)
	if err != nil {
		return err
	}

	details := provider.Details

	cloudInfo := details.CloudInfo

	gcp := cloudInfo.Gcp

	if err = d.Set("air_gap_install", details.AirGapInstall); err != nil {
		return err
	}

	if err = d.Set("ntp_servers", details.NtpServers); err != nil {
		return err
	}

	if err = d.Set("show_set_up_chrony", details.ShowSetUpChrony); err != nil {
		return err
	}

	if err = d.Set("set_up_chrony", details.SetUpChrony); err != nil {
		return err
	}

	if err = d.Set("code", provider.Code); err != nil {
		return err
	}

	if err = d.Set("config", provider.Config); err != nil {
		return err
	}

	if err = d.Set("name", provider.Name); err != nil {
		return err
	}

	if err = d.Set("ssh_port", details.SshPort); err != nil {
		return err
	}
	if err = d.Set("ssh_private_key_content", provider.SshPrivateKeyContent); err != nil {
		return err
	}
	if err = d.Set("ssh_user", details.SshUser); err != nil {
		return err
	}
	if err = d.Set("regions", flattenRegions(provider.Regions)); err != nil {
		return err
	}

	if err = d.Set(
		"image_bundles",
		flattenImageBundles(provider.ImageBundles)); err != nil {
		return err
	}

	configInterface := d.Get("gcp_config_settings").([]interface{})
	if len(configInterface) > 0 && configInterface[0] != nil {
		configSettings := utils.MapFromSingletonList(configInterface)
		applicationCreds := configSettings["application_credentials"]
		ybFirewallTags := configSettings["yb_firewall_tags"]
		network := configSettings["network"]
		projectID := configSettings["project_id"]
		sharedProjectID := configSettings["shared_vpc_project_id"]
		useHostCredentials := configSettings["use_host_credentials"]
		useHostVPC := configSettings["use_host_vpc"]

		if ybFirewallTags != nil && len(ybFirewallTags.(string)) > 0 {
			configSettings["yb_firewall_tags"] = gcp.YbFirewallTags
		}
		if network != nil && len(network.(string)) > 0 {
			configSettings["network"] = gcp.DestVpcId
		}
		if projectID != nil && len(projectID.(string)) > 0 {
			configSettings["project_id"] = gcp.GceProject
		}
		if sharedProjectID != nil && len(sharedProjectID.(string)) > 0 {
			configSettings["shared_vpc_project_id"] = gcp.SharedVPCProject
		}
		if useHostCredentials != nil && useHostCredentials.(bool) {
			configSettings["use_host_credentials"] = gcp.UseHostCredentials

		}
		if useHostVPC != nil && useHostVPC.(bool) {
			configSettings["use_host_vpc"] = gcp.UseHostVPC
		}
		if applicationCreds != nil && len(applicationCreds.(map[string]interface{})) > 0 {
			// check if this is string or map[string]string

			var checkInterfaceIsMap map[string]interface{}
			var checkInterfaceIsString string
			var credentials map[string]string
			if reflect.TypeOf(gcp.GceApplicationCredentials) == reflect.TypeOf(checkInterfaceIsMap) {
				credentialsInterface := gcp.GceApplicationCredentials
				credentials = *utils.StringMap(credentialsInterface.(map[string]interface{}))
			} else if reflect.TypeOf(gcp.GceApplicationCredentials) ==
				reflect.TypeOf(checkInterfaceIsString) {
				err := json.Unmarshal([]byte(gcp.GceApplicationCredentials.(string)), &credentials)
				if err != nil {
					return err
				}
			}
			credentialsMap := utils.MapFromSingletonList(
				[]interface{}{configSettings["application_credentials"]})
			credentialsMap["private_key"] = strings.Trim(credentials["private_key"], "\"")
			credentialsMap["private_key_id"] = strings.Trim(credentials["private_key_id"], "\"")
			configSettings["application_credentials"] = credentialsMap
		}

		configSettingsList := make([]interface{}, 0)
		configSettingsList = append(configSettingsList, configSettings)
		if err = d.Set("gcp_config_settings", configSettingsList); err != nil {
			return err
		}
	} else {
		configSettingsList := make([]interface{}, 0)
		if err = d.Set("gcp_config_settings", configSettingsList); err != nil {
			return err
		}
	}
	return nil
}
