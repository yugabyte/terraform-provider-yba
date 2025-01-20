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
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/customdiff"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	client "github.com/yugabyte/platform-go-client"
	"github.com/yugabyte/terraform-provider-yba/internal/api"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// ResourceCloudProvider creates and maintains resource for cloud providers
func ResourceCloudProvider() *schema.Resource {
	return &schema.Resource{
		Description: "Cloud Provider Resource.",

		CreateContext: resourceCloudProviderCreate,
		ReadContext:   resourceCloudProviderRead,
		DeleteContext: resourceCloudProviderDelete,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(10 * time.Minute),
			Delete: schema.DefaultTimeout(5 * time.Minute),
		},

		CustomizeDiff: resourceCloudProviderDiff(),

		Schema: map[string]*schema.Schema{
			"air_gap_install": {
				Type:     schema.TypeBool,
				Optional: true,
				ForceNew: true,
				Description: "Flag indicating if the universe should use an air-gapped " +
					"installation.",
			},
			"code": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
				ValidateDiagFunc: validation.ToDiagFunc(validation.StringInSlice(
					[]string{"gcp", "aws", "azu"}, false)),
				Description: "Code of the cloud provider. Permitted values: gcp, aws, azu.",
			},
			"config": {
				Type:     schema.TypeMap,
				Elem:     &schema.Schema{Type: schema.TypeString},
				ForceNew: true,
				Computed: true,
				Description: "Configuration values to be set for the provider. " +
					"AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY must be set for AWS providers." +
					" The contents of your google credentials must be included here for GCP " +
					"providers. AZURE_SUBSCRIPTION_ID, AZURE_RG, AZURE_TENANT_ID, AZURE_CLIENT_ID," +
					" AZURE_CLIENT_SECRET must be set for AZURE providers.",
			},
			"dest_vpc_id": {
				Type:        schema.TypeString,
				Optional:    true,
				ForceNew:    true,
				Description: "Destination VPC network.",
			},
			"host_vpc_id": {
				Type:        schema.TypeString,
				Optional:    true,
				ForceNew:    true,
				Description: "Host VPC Network.",
			},
			"host_vpc_region": {
				Type:        schema.TypeString,
				Optional:    true,
				ForceNew:    true,
				Description: "Host VPC Region.",
			},
			"key_pair_name": {
				Type:        schema.TypeString,
				Optional:    true,
				ForceNew:    true,
				Description: "Access Key Pair name.",
			},
			"name": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "Name of the provider.",
			},
			"regions":       RegionsSchema(),
			"image_bundles": ImageBundleSchema(),
			"ssh_port": {
				Type:     schema.TypeInt,
				Optional: true,
				Computed: true,
				Deprecated: "Deprecated since YugabyteDB Anywhere 2.20.3.0. " +
					"Please use 'image_bundles[*].details.ssh_port' instead.",
				Description: "Port to use for ssh commands. " +
					"Deprecated since YugabyteDB Anywhere 2.20.3.0. " +
					"Please use 'image_bundles[*].details.ssh_port' instead.",
			},
			"ssh_private_key_content": {
				Type:        schema.TypeString,
				Optional:    true,
				ForceNew:    true,
				Description: "Private key to use for ssh commands.",
			},
			"ssh_user": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
				DiffSuppressFunc: func(k, old, new string, d *schema.ResourceData) bool {
					// ssh_user field can be empty in the configuration block of the resource
					// In that event YBA uses a default ssh user as per the cloud provider
					// The discrepency of empty field in config file and value filled in state
					// file, we check if ssh user is empty and ignore the difference if true

					return len(old) > 0 && len(new) == 0
				},
				Deprecated: "Deprecated since YugabyteDB Anywhere 2.20.3.0. " +
					"Please use 'image_bundles[*].details.ssh_user' instead.",
				Description: "User to use for ssh commands. " +
					"Deprecated since YugabyteDB Anywhere 2.20.3.0. " +
					"Please use 'image_bundles[*].details.ssh_user' instead.",
			},
			"aws_config_settings": {
				Type:     schema.TypeList,
				Optional: true,
				MaxItems: 1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"hosted_zone_id": {
							Type:     schema.TypeString,
							Optional: true,
							ForceNew: true,
							Description: "Hosted Zone ID for AWS corresponsding to Amazon " +
								"Route53.",
						},
						"use_iam_instance_profile": {
							Type:     schema.TypeBool,
							Optional: true,
							Default:  false,
							Description: "Use IAM Role from the YugabyteDB Anywhere Host. Provider " +
								"creation will fail on insufficient permissions on the host. False by default.",
						},
						"access_key_id": {
							Type:      schema.TypeString,
							Optional:  true,
							Sensitive: true,
							ForceNew:  true,
							DiffSuppressFunc: func(k, old, new string, d *schema.ResourceData) bool {
								return len(old) > 0 && utils.ObfuscateString(new, 2) == old
							},
							Description: "AWS Access Key ID. Can also be set using " +
								"environment variable AWS_ACCESS_KEY_ID.",
						},
						"secret_access_key": {
							Type:      schema.TypeString,
							Optional:  true,
							Sensitive: true,
							ForceNew:  true,
							DiffSuppressFunc: func(k, old, new string, d *schema.ResourceData) bool {
								return len(old) > 0 && utils.ObfuscateString(new, 2) == old
							},
							RequiredWith: []string{"aws_config_settings.0.access_key_id"},
							Description: "AWS Secret Access Key. Can also be set using " +
								"environment variable AWS_SECRET_ACCESS_KEY.",
						},
					}},
				ForceNew:    true,
				Description: "Settings that can be configured for AWS.",
			},
			"azure_config_settings": {
				Type:     schema.TypeList,
				Optional: true,
				MaxItems: 1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"hosted_zone_id": {
							Type:        schema.TypeString,
							Optional:    true,
							ForceNew:    true,
							Description: "Private DNS Zone for Azure.",
						},
						"subscription_id": {
							Type:         schema.TypeString,
							Optional:     true,
							ForceNew:     true,
							RequiredWith: []string{"azure_config_settings.0.client_id"},
							Description: "Azure Subscription ID. Can also be set using " +
								"environment variable AZURE_SUBSCRIPTION_ID. Required with " +
								"client_id.",
						},
						"resource_group": {
							Type:         schema.TypeString,
							Optional:     true,
							ForceNew:     true,
							RequiredWith: []string{"azure_config_settings.0.client_id"},
							Description: "Azure Resource Group. Can also be set using " +
								"environment variable AZURE_RG. Required with " +
								"client_id.",
						},
						"tenant_id": {
							Type:         schema.TypeString,
							Optional:     true,
							ForceNew:     true,
							RequiredWith: []string{"azure_config_settings.0.client_id"},
							Description: "Azure Tenant ID. Can also be set using " +
								"environment variable AZURE_TENANT_ID. Required with " +
								"client_id.",
						},
						"client_id": {
							Type:     schema.TypeString,
							Optional: true,
							ForceNew: true,
							Description: "Azure Client ID. Can also be set using " +
								"environment variable AZURE_CLIENT_ID.",
						},
						"client_secret": {
							Type:      schema.TypeString,
							Optional:  true,
							Sensitive: true,
							ForceNew:  true,
							DiffSuppressFunc: func(k, old, new string, d *schema.ResourceData) bool {
								return len(old) > 0 && utils.ObfuscateString(new, 2) == old
							},
							RequiredWith: []string{"azure_config_settings.0.client_id"},
							Description: "Azure Client Secret. Can also be set using " +
								"environment variable AZURE_CLIENT_SECRET. Required with " +
								"client_id.",
						},
						"network_subscription_id": {
							Type:     schema.TypeString,
							Optional: true,
							ForceNew: true,
							Description: "Azure Network Subscription ID." +
								"All network resources and NIC resouce of VMs will " +
								"be created in this group. If left empty, the default subscription ID will be used.",
						},
						"network_resource_group": {
							Type:     schema.TypeString,
							Optional: true,
							ForceNew: true,
							Description: "Azure Network Resource Group." +
								"All network resources and NIC resouce of VMs will " +
								"be created in this group. If left empty, the default resource group will be used.",
						},
					}},
				ForceNew:    true,
				Description: "Settings that can be configured for Azure.",
			},
			"gcp_config_settings": {
				Type:     schema.TypeList,
				Optional: true,
				MaxItems: 1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"yb_firewall_tags": {
							Type:        schema.TypeString,
							Optional:    true,
							ForceNew:    true,
							Description: "Tags for firewall rules in GCP.",
						},
						"use_host_vpc": {
							Type:        schema.TypeBool,
							Optional:    true,
							ForceNew:    true,
							Description: "Enabling Host VPC in GCP.",
						},
						"use_host_credentials": {
							Type:        schema.TypeBool,
							Optional:    true,
							ForceNew:    true,
							Description: "Enabling Host Credentials in GCP.",
						},
						"project_id": {
							Type:        schema.TypeString,
							Optional:    true,
							ForceNew:    true,
							Description: "Project ID that hosts universe nodes in GCP.",
						},
						"shared_vpc_project_id": {
							Type:     schema.TypeString,
							Optional: true,
							ForceNew: true,
							Description: "Specify the project to use Shared VPC to connect " +
								"resources from multiple GCP projects to a common VPC.",
						},
						"network": {
							Type:        schema.TypeString,
							Optional:    true,
							ForceNew:    true,
							Description: "VPC network name in GCP.",
						},
						"application_credentials": {
							Type:     schema.TypeMap,
							Elem:     schema.TypeString,
							Optional: true,
							ForceNew: true,
							DiffSuppressFunc: func(k, old, new string, d *schema.ResourceData) bool {
								oldInterface, newInterface := d.GetChange("gcp_config_settings.0.application_credentials")
								oldMap := oldInterface.(map[string]interface{})
								newMap := newInterface.(map[string]interface{})
								if new == "" {
									return false
								}
								if (oldMap != nil && newMap != nil) && (oldMap["private_key_id"] != nil &&
									newMap["private_key_id"] != nil) &&
									(oldMap["private_key_id"].(string) ==
										utils.ObfuscateString(newMap["private_key_id"].(string), 1)) {
									return true
								}
								return false
							},
							Description: "Google Service Account JSON Credentials. Can also be set " +
								"by providing the JSON file path with the " +
								"environment variable GOOGLE_APPLICATION_CREDENTIALS.",
						},
					}},
				ForceNew:    true,
				Description: "Settings that can be configured for GCP.",
			},
			"ntp_servers": {
				Type:     schema.TypeList,
				Optional: true,
				ForceNew: true,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
				Description: "NTP servers. Set \"set_up_chrony\" to true to use these servers.",
			},
			"show_set_up_chrony": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     true,
				ForceNew:    true,
				Description: "Show setup chrony.",
			},
			"set_up_chrony": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     false,
				ForceNew:    true,
				Description: "Set up NTP servers.",
			},
		},
	}
}

func resourceCloudProviderDiff() schema.CustomizeDiffFunc {
	return customdiff.All(
		customdiff.IfValue("code",
			func(ctx context.Context, value, meta interface{}) bool {
				return value.(string) == "azu"
			},
			func(ctx context.Context, d *schema.ResourceDiff, meta interface{}) error {
				var errorString string
				errorMessage := "Empty env variable: "

				configInterface := d.Get("azure_config_settings").([]interface{})
				var configSettings map[string]interface{}
				if len(configInterface) > 0 && configInterface[0] != nil {
					configSettings = utils.MapFromSingletonList(
						configInterface,
					)
				}

				if len(configSettings) == 0 ||
					(configSettings["client_id"] == nil ||
						len(configSettings["client_id"].(string)) == 0) {
					_, isPresentClientID := os.LookupEnv(utils.AzureClientIDEnv)
					if !isPresentClientID {
						errorString = fmt.Sprintf("%s%s ", errorString, utils.AzureClientIDEnv)
					}
					_, isPresentClientSecret := os.LookupEnv(utils.AzureClientSecretEnv)
					if !isPresentClientSecret {
						errorString = fmt.Sprintf("%s%s ", errorString, utils.AzureClientSecretEnv)
					}
					_, isPresentSubscriptionID := os.LookupEnv(utils.AzureSubscriptionIDEnv)
					if !isPresentSubscriptionID {
						errorString = fmt.Sprintf("%s%s ", errorString, utils.AzureSubscriptionIDEnv)
					}
					_, isPresentTenantID := os.LookupEnv(utils.AzureTenantIDEnv)
					if !isPresentTenantID {
						errorString = fmt.Sprintf("%s%s ", errorString, utils.AzureTenantIDEnv)
					}
					_, isPresentRG := os.LookupEnv(utils.AzureRGEnv)
					if !isPresentRG {
						errorString = fmt.Sprintf("%s%s ", errorString, utils.AzureRGEnv)
					}
					if !(isPresentClientID && isPresentClientSecret && isPresentRG &&
						isPresentSubscriptionID && isPresentTenantID) {
						errorString = fmt.Sprintf("%s%s", errorMessage, errorString)
						return fmt.Errorf(errorString)
					}
				} else {
					clientSecret := configSettings["client_secret"]
					subscriptionID := configSettings["subscription_id"]
					tenantID := configSettings["tenant_id"]
					rg := configSettings["resource_group"]
					if clientSecret == nil || len(clientSecret.(string)) == 0 {
						errorString = "Azure Client Secret cannot be empty when " +
							"Azure Client ID is set"
						return fmt.Errorf(errorString)
					}
					if subscriptionID == nil || len(subscriptionID.(string)) == 0 {
						errorString = "Azure Subscription ID cannot be empty when " +
							"Azure Client ID is set"
						return fmt.Errorf(errorString)
					}
					if tenantID == nil || len(tenantID.(string)) == 0 {
						errorString = "Azure Tenant ID cannot be empty when " +
							"Azure Client ID is set"
						return fmt.Errorf(errorString)
					}
					if rg == nil || len(rg.(string)) == 0 {
						errorString = "Azure Resource Group cannot be empty when " +
							"Azure Client ID is set"
						return fmt.Errorf(errorString)
					}
				}
				return nil
			}),
		customdiff.IfValue("code",
			func(ctx context.Context, value, meta interface{}) bool {
				// check if AWS cloud provider creation requires access keys
				return value.(string) == "aws"
			},
			func(ctx context.Context, d *schema.ResourceDiff, meta interface{}) error {
				var errorString string
				errorMessage := "Empty env variable: "

				configInterface := d.Get("aws_config_settings").([]interface{})
				var configSettings map[string]interface{}
				var isIAM bool
				if len(configInterface) > 0 && configInterface[0] != nil {
					configSettings = utils.MapFromSingletonList(
						configInterface,
					)
					isIAM = configSettings["use_iam_instance_profile"].(bool)
				}

				// if not IAM AWS cloud provider, check for credentials in
				// aws_config_settings block or env
				if !isIAM {
					if len(configSettings) == 0 ||
						(configSettings["access_key_id"] == nil ||
							len(configSettings["access_key_id"].(string)) == 0) {
						_, isPresentAccessKeyID := os.LookupEnv(utils.AWSAccessKeyEnv)
						if !isPresentAccessKeyID {
							errorString = fmt.Sprintf("%s%s ", errorString, utils.AWSAccessKeyEnv)
						}
						_, isPresentSecretAccessKey := os.LookupEnv(utils.AWSSecretAccessKeyEnv)
						if !isPresentSecretAccessKey {
							errorString = fmt.Sprintf("%s%s ", errorString, utils.AWSSecretAccessKeyEnv)
						}
						if !(isPresentAccessKeyID && isPresentSecretAccessKey) {
							errorString = fmt.Sprintf("%s%s", errorMessage, errorString)
							return fmt.Errorf(errorString)
						}
					} else {
						secretAccessKey := configSettings["secret_access_key"]
						if secretAccessKey == nil || len(secretAccessKey.(string)) == 0 {
							errorString = "AWS Secret Access Key cannot be empty when " +
								"AWS Access Key ID is set"
							return fmt.Errorf(errorString)
						}
					}
				}
				return nil
			}),
		customdiff.IfValue("code",
			func(ctx context.Context, value, meta interface{}) bool {
				// check if GCP cloud provider creation requires access keys
				return value.(string) == "gcp"
			},
			func(ctx context.Context, d *schema.ResourceDiff, meta interface{}) error {
				errorMessage := "Empty env variable: "
				var isIAM bool

				configInterface := d.Get("gcp_config_settings").([]interface{})
				var configSettings map[string]interface{}

				if len(configInterface) > 0 && configInterface[0] != nil {
					configSettings = utils.MapFromSingletonList(
						configInterface,
					)
					isIAM = configSettings["use_host_credentials"].(bool)
				}

				// if not IAM GCP cloud provider, check for credentials in env
				if !isIAM {
					applicationCreds := configSettings["application_credentials"]
					if len(configSettings) == 0 ||
						(applicationCreds == nil ||
							len(applicationCreds.(map[string]interface{})) == 0) {
						_, isPresent := os.LookupEnv(utils.GCPCredentialsEnv)
						if !isPresent {
							return fmt.Errorf("%s%s", errorMessage, utils.GCPCredentialsEnv)
						}
					}
				}
				return nil
			}),
	)
}

// Check if the current version of YBA can support image bundles
func imageBundlesYBAVersionCheck(
	ctx context.Context,
	c *client.APIClient) (bool, string, error) {
	allowedVersions := utils.YBAMinimumVersion{
		Stable:  utils.YBAAllowImageBundlesMinVersion,
		Preview: utils.YBAAllowImageBundlesMinVersion,
	}
	allowed, version, err := utils.CheckValidYBAVersion(ctx, c, allowedVersions)
	if err != nil {
		return false, "", err
	}
	return allowed, version, err
}

func buildConfig(d *schema.ResourceData) (map[string]interface{}, error) {
	cloudCode := d.Get("code").(string)
	config := make(map[string]interface{})
	if cloudCode == "gcp" {
		var isIAM bool
		var configSettings map[string]interface{}
		configInterface := d.Get("gcp_config_settings").([]interface{})
		if len(configInterface) > 0 && configInterface[0] != nil {
			configSettings = utils.MapFromSingletonList(
				configInterface,
			)
			ybFirewallTags := configSettings["yb_firewall_tags"].(string)
			if len(ybFirewallTags) > 0 {
				config["YB_FIREWALL_TAGS"] = ybFirewallTags
			}
			useHostVpc := strconv.FormatBool(configSettings["use_host_vpc"].(bool))
			if len(useHostVpc) > 0 {
				config["use_host_vpc"] = useHostVpc
			}
			useHostCredentials := strconv.FormatBool(configSettings["use_host_credentials"].(bool))
			if len(useHostCredentials) > 0 {
				config["use_host_credentials"] = useHostCredentials
				isIAM = configSettings["use_host_credentials"].(bool)
			}
			network := configSettings["network"].(string)
			if len(network) > 0 {
				config["network"] = network
			}
		}
		if !isIAM {
			applicationCreds := configSettings["application_credentials"]
			if len(configSettings) == 0 || applicationCreds == nil ||
				len(applicationCreds.(map[string]interface{})) == 0 {
				iamConfig, err := utils.GcpGetCredentialsAsMap()
				if err != nil {
					return nil, err
				}
				for k, v := range iamConfig {
					config[k] = v
				}
			} else {
				for k, v := range applicationCreds.(map[string]interface{}) {
					config[k] = v
				}
			}
		}
		projectID := configSettings["project_id"].(string)
		if len(projectID) > 0 {
			config["project_id"] = projectID
		}
		sharedVPCProjectID := configSettings["shared_vpc_project_id"].(string)
		if len(sharedVPCProjectID) > 0 {
			config["host_project_id"] = sharedVPCProjectID
		}
	} else if cloudCode == "aws" {
		var isIAM bool
		configInterface := d.Get("aws_config_settings").([]interface{})
		var configSettings map[string]interface{}
		if len(configInterface) > 0 && configInterface[0] != nil {
			configSettings = utils.MapFromSingletonList(configInterface)
			hostedZoneID := configSettings["hosted_zone_id"].(string)
			if len(hostedZoneID) > 0 {
				config["HOSTED_ZONE_ID"] = hostedZoneID
			}
			isIAM = configSettings["use_iam_instance_profile"].(bool)

		}
		if !isIAM {
			if len(configSettings) == 0 ||
				(configSettings["access_key_id"] == nil ||
					len(configSettings["access_key_id"].(string)) == 0) {
				awsCreds, err := utils.AwsCredentialsFromEnv()
				if err != nil {
					return nil, err
				}
				config[utils.AWSAccessKeyEnv] = awsCreds.AccessKeyID
				config[utils.AWSSecretAccessKeyEnv] = awsCreds.SecretAccessKey
			} else {
				config[utils.AWSAccessKeyEnv] = configSettings["access_key_id"].(string)
				secretAccessKey := configSettings["secret_access_key"]
				if secretAccessKey != nil && len(secretAccessKey.(string)) > 0 {
					config[utils.AWSSecretAccessKeyEnv] = secretAccessKey
				}
			}
		}
	} else if cloudCode == "azu" {
		configInterface := d.Get("azure_config_settings").([]interface{})
		var configSettings map[string]interface{}
		if len(configInterface) > 0 && configInterface[0] != nil {
			configSettings = utils.MapFromSingletonList(configInterface)
			hostedZoneID := configSettings["hosted_zone_id"].(string)
			if len(hostedZoneID) > 0 {
				config["HOSTED_ZONE_ID"] = hostedZoneID
			}
			networkSubscriptionID := configSettings["network_subscription_id"].(string)
			if len(networkSubscriptionID) > 0 {
				config[utils.AzureNetworkSubscriptionIDEnv] = networkSubscriptionID
			}
			networkRG := configSettings["network_resource_group"].(string)
			if len(networkRG) > 0 {
				config[utils.AzureNetworkRGEnv] = networkRG
			}
		}
		if configSettings == nil ||
			(configSettings["client_id"] == nil ||
				len(configSettings["client_id"].(string)) == 0) {
			azureCreds, err := utils.AzureCredentialsFromEnv()
			if err != nil {
				return nil, err
			}
			config[utils.AzureClientIDEnv] = azureCreds.ClientID
			config[utils.AzureClientSecretEnv] = azureCreds.ClientSecret
			config[utils.AzureSubscriptionIDEnv] = azureCreds.SubscriptionID
			config[utils.AzureTenantIDEnv] = azureCreds.TenantID
			config[utils.AzureRGEnv] = azureCreds.ResourceGroup
		} else {
			clientID := configSettings["client_id"]
			clientSecret := configSettings["client_secret"]
			subscriptionID := configSettings["subscription_id"]
			tenantID := configSettings["tenant_id"]
			rg := configSettings["resource_group"]
			if clientID != nil && len(clientID.(string)) != 0 {
				config[utils.AzureClientIDEnv] = clientID.(string)
			}
			if clientSecret != nil && len(clientSecret.(string)) != 0 {
				config[utils.AzureClientSecretEnv] = clientSecret.(string)
			}
			if subscriptionID != nil && len(subscriptionID.(string)) != 0 {
				config[utils.AzureSubscriptionIDEnv] = subscriptionID.(string)
			}
			if tenantID != nil && len(tenantID.(string)) != 0 {
				config[utils.AzureTenantIDEnv] = tenantID.(string)
			}
			if rg != nil && len(rg.(string)) != 0 {
				config[utils.AzureRGEnv] = rg.(string)
			}
		}
	}
	return config, nil
}

func resourceCloudProviderCreate(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{},
) diag.Diagnostics {
	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID

	config, err := buildConfig(d)
	if err != nil {
		return diag.FromErr(err)
	}

	imageBundleAllowed, imageBundleVersion, err := imageBundlesYBAVersionCheck(ctx, c)
	if err != nil {
		return diag.FromErr(err)
	}
	if !imageBundleAllowed {
		return diag.FromErr(
			fmt.Errorf("Image bundle blocks are not supported below version %s, currently on %s",
				utils.YBAAllowImageBundlesMinVersion,
				imageBundleVersion))
	}
	imageBundles := buildImageBundles(d.Get("image_bundles").([]interface{}))

	req := client.Provider{
		AirGapInstall:        utils.GetBoolPointer(d.Get("air_gap_install").(bool)),
		Code:                 utils.GetStringPointer(d.Get("code").(string)),
		Config:               utils.StringMap(config),
		DestVpcId:            utils.GetStringPointer(d.Get("dest_vpc_id").(string)),
		HostVpcId:            utils.GetStringPointer(d.Get("host_vpc_id").(string)),
		HostVpcRegion:        utils.GetStringPointer(d.Get("host_vpc_region").(string)),
		KeyPairName:          utils.GetStringPointer(d.Get("key_pair_name").(string)),
		Name:                 utils.GetStringPointer(d.Get("name").(string)),
		SshPort:              utils.GetInt32Pointer(int32(d.Get("ssh_port").(int))),
		SshPrivateKeyContent: utils.GetStringPointer(d.Get("ssh_private_key_content").(string)),
		SshUser:              utils.GetStringPointer(d.Get("ssh_user").(string)),
		Regions:              buildRegions(d.Get("regions").([]interface{})),
		ImageBundles:         imageBundles,
		NtpServers:           utils.StringSlice(d.Get("ntp_servers").([]interface{})),
		ShowSetUpChrony:      utils.GetBoolPointer(d.Get("show_set_up_chrony").(bool)),
		SetUpChrony:          utils.GetBoolPointer(d.Get("set_up_chrony").(bool)),
	}
	r, response, err := c.CloudProvidersApi.CreateProviders(ctx, cUUID).CreateProviderRequest(
		req).Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"Cloud Provider", "Create")
		return diag.FromErr(errMessage)
	}

	d.SetId(*r.ResourceUUID)
	if r.TaskUUID != nil {
		tflog.Debug(ctx, fmt.Sprintf("Waiting for provider %s to be active", d.Id()))
		err = utils.WaitForTask(ctx, *r.TaskUUID, cUUID, c, d.Timeout(schema.TimeoutCreate))
		if err != nil {
			return diag.FromErr(err)
		}
	}

	return resourceCloudProviderRead(ctx, d, meta)
}

func findProvider(providers []client.Provider, uuid string) (*client.Provider, error) {
	for _, p := range providers {
		if *p.Uuid == uuid {
			return &p, nil
		}
	}
	return nil, fmt.Errorf("could not find provider %s", uuid)
}

func resourceCloudProviderRead(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{},
) diag.Diagnostics {
	var diags diag.Diagnostics

	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID

	r, response, err := c.CloudProvidersApi.GetListOfProviders(ctx, cUUID).Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"Cloud Provider", "Read")
		return diag.FromErr(errMessage)
	}

	p, err := findProvider(r, d.Id())
	if err != nil {
		return diag.FromErr(err)
	}

	details := p.GetDetails()

	if err = d.Set("air_gap_install", p.AirGapInstall); err != nil {
		return diag.FromErr(err)
	}
	ntpServersPointer := p.NtpServers
	ntpServers := make([]string, 0)
	if ntpServersPointer == nil {
		ntpServers = details.GetNtpServers()
	} else {
		ntpServers = *ntpServersPointer
	}
	if err = d.Set("ntp_servers", ntpServers); err != nil {
		return diag.FromErr(err)
	}

	showSetUpChronyPointer := p.ShowSetUpChrony
	showSetUpChrony := false
	if showSetUpChronyPointer == nil {
		showSetUpChrony = details.GetShowSetUpChrony()
	} else {
		showSetUpChrony = *showSetUpChronyPointer
	}
	if err = d.Set("show_set_up_chrony", showSetUpChrony); err != nil {
		return diag.FromErr(err)
	}

	setUpChronyPointer := p.SetUpChrony
	setUpChrony := false
	if setUpChronyPointer == nil {
		setUpChrony = details.GetSetUpChrony()
	} else {
		setUpChrony = *setUpChronyPointer
	}
	if err = d.Set("set_up_chrony", setUpChrony); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("code", p.Code); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("config", p.Config); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("name", p.Name); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("ssh_port", p.SshPort); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("ssh_private_key_content", p.SshPrivateKeyContent); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("ssh_user", p.SshUser); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("regions", flattenRegions(p.Regions)); err != nil {
		return diag.FromErr(err)
	}

	if err = d.Set(
		"image_bundles",
		flattenImageBundles(p.GetImageBundles())); err != nil {
		return diag.FromErr(err)
	}

	if p.GetCode() == "aws" {
		configInterface := d.Get("aws_config_settings").([]interface{})
		if len(configInterface) > 0 && configInterface[0] != nil {
			configSettings := utils.MapFromSingletonList(configInterface)
			accessKeyID := configSettings["access_key_id"]
			secretAccessKey := configSettings["secret_access_key"]
			hostedZoneID := configSettings["hosted_zone_id"]
			if accessKeyID != nil && len(accessKeyID.(string)) > 0 {
				configSettings["access_key_id"] = p.GetConfig()[utils.AWSAccessKeyEnv]
			}
			if secretAccessKey != nil && len(secretAccessKey.(string)) > 0 {
				configSettings["secret_access_key"] = p.GetConfig()[utils.AWSSecretAccessKeyEnv]
			}
			if hostedZoneID != nil && len(hostedZoneID.(string)) > 0 {
				configSettings["hosted_zone_id"] = p.GetConfig()["HOSTED_ZONE_ID"]
			}
			configSettingsList := make([]interface{}, 0)
			configSettingsList = append(configSettingsList, configSettings)
			if err = d.Set("aws_config_settings", configSettingsList); err != nil {
				return diag.FromErr(err)
			}
		} else {
			configSettingsList := make([]interface{}, 0)
			if err = d.Set("aws_config_settings", configSettingsList); err != nil {
				return diag.FromErr(err)
			}
		}
	}

	if p.GetCode() == "azu" {
		configInterface := d.Get("azure_config_settings").([]interface{})
		if len(configInterface) > 0 && configInterface[0] != nil {
			configSettings := utils.MapFromSingletonList(configInterface)
			clientSecret := configSettings["client_secret"]
			clientID := configSettings["client_id"]
			subscriptionID := configSettings["subscription_id"]
			tenantID := configSettings["tenant_id"]
			rg := configSettings["resource_group"]
			hostedZoneID := configSettings["hosted_zone_id"]
			networkRG := configSettings["network_resource_group"]
			networkSubscriptionID := configSettings["network_subscription_id"]
			if clientSecret != nil && len(clientSecret.(string)) > 0 {
				configSettings["client_secret"] = p.GetConfig()[utils.AzureClientSecretEnv]
			}
			if clientID != nil && len(clientID.(string)) > 0 {
				configSettings["client_id"] = p.GetConfig()[utils.AzureClientIDEnv]
			}
			if subscriptionID != nil && len(subscriptionID.(string)) > 0 {
				configSettings["subscription_id"] = p.GetConfig()[utils.AzureSubscriptionIDEnv]
			}
			if tenantID != nil && len(tenantID.(string)) > 0 {
				configSettings["tenant_id"] = p.GetConfig()[utils.AzureTenantIDEnv]
			}
			if rg != nil && len(rg.(string)) > 0 {
				configSettings["resource_group"] = p.GetConfig()[utils.AzureRGEnv]
			}
			if networkSubscriptionID != nil && len(networkSubscriptionID.(string)) > 0 {
				configSettings["network_subscription_id"] =
					p.GetConfig()[utils.AzureNetworkSubscriptionIDEnv]
			}
			if networkRG != nil && len(networkRG.(string)) > 0 {
				configSettings["network_resource_group"] = p.GetConfig()[utils.AzureNetworkRGEnv]
			}
			if hostedZoneID != nil && len(hostedZoneID.(string)) > 0 {
				configSettings["hosted_zone_id"] = p.GetConfig()["HOSTED_ZONE_ID"]
			}
			configSettingsList := make([]interface{}, 0)
			configSettingsList = append(configSettingsList, configSettings)
			if err = d.Set("azure_config_settings", configSettingsList); err != nil {
				return diag.FromErr(err)
			}
		} else {
			configSettingsList := make([]interface{}, 0)
			if err = d.Set("azure_config_settings", configSettingsList); err != nil {
				return diag.FromErr(err)
			}
		}
	}

	if p.GetCode() == "gcp" {
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
				configSettings["yb_firewall_tags"] = p.GetConfig()["yb_firewall_tags"]
			}
			if network != nil && len(network.(string)) > 0 {
				configSettings["network"] = p.GetConfig()["network"]
			}
			if projectID != nil && len(projectID.(string)) > 0 {
				configProjectID := p.GetConfig()["project_id"]
				if len(configProjectID) == 0 {
					configProjectID = p.GetConfig()["GCE_PROJECT"]
					configProjectID = strings.Trim(configProjectID, "\"")
				}
				configSettings["project_id"] = configProjectID
			}
			if sharedProjectID != nil && len(sharedProjectID.(string)) > 0 {
				configSharedProjectID := p.GetConfig()["host_project_id"]
				if len(configSharedProjectID) == 0 {
					configSharedProjectID = p.GetConfig()["GCE_HOST_PROJECT"]
					configSharedProjectID = strings.Trim(configSharedProjectID, "\"")
				}
				configSettings["shared_vpc_project_id"] = configSharedProjectID
			}
			if useHostCredentials != nil && useHostCredentials.(bool) {
				configUseHostCredentials := p.GetConfig()["use_host_credentials"]
				if len(configUseHostCredentials) > 0 {
					useHostCredsBool, err := strconv.ParseBool(configUseHostCredentials)
					if err != nil {
						return diag.FromErr(err)
					}
					configSettings["use_host_credentials"] = useHostCredsBool
				}
			}
			if useHostVPC != nil && useHostVPC.(bool) {
				configUseHostVpc := p.GetConfig()["use_host_vpc"]
				if len(configUseHostVpc) > 0 {
					useHostVpcBool, err := strconv.ParseBool(configUseHostVpc)
					if err != nil {
						return diag.FromErr(err)
					}
					configSettings["use_host_vpc"] = useHostVpcBool
				}
			}
			if applicationCreds != nil && len(applicationCreds.(map[string]interface{})) > 0 {
				credentials := p.GetConfig()
				credentialsMap := utils.MapFromSingletonList(
					[]interface{}{configSettings["application_credentials"]})
				credentialsMap["private_key"] = strings.Trim(credentials["private_key"], "\"")
				credentialsMap["private_key_id"] = strings.Trim(credentials["private_key_id"], "\"")
				configSettings["application_credentials"] = credentialsMap

			}
			configSettingsList := make([]interface{}, 0)
			configSettingsList = append(configSettingsList, configSettings)
			if err = d.Set("gcp_config_settings", configSettingsList); err != nil {
				return diag.FromErr(err)
			}

		} else {
			configSettingsList := make([]interface{}, 0)
			if err = d.Set("gcp_config_settings", configSettingsList); err != nil {
				return diag.FromErr(err)
			}
		}
	}
	return diags
}

func resourceCloudProviderDelete(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{},
) diag.Diagnostics {
	var diags diag.Diagnostics

	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID

	pUUID := d.Id()
	r, response, err := c.CloudProvidersApi.Delete(ctx, cUUID, pUUID).Execute()

	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"Cloud Provider", "Delete")
		return diag.FromErr(errMessage)
	}

	if r.TaskUUID != nil {
		tflog.Info(ctx, fmt.Sprintf("Waiting for provider %s to be deleted", d.Id()))
		err = utils.WaitForTask(ctx, *r.TaskUUID, cUUID, c, d.Timeout(schema.TimeoutDelete))
		if err != nil {
			return diag.FromErr(err)
		}
	}

	d.SetId("")
	return diags
}
