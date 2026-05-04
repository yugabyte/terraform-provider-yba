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

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/yugabyte/terraform-provider-yba/internal/api"
	"github.com/yugabyte/terraform-provider-yba/internal/backups"
	"github.com/yugabyte/terraform-provider-yba/internal/cloud_provider"
	"github.com/yugabyte/terraform-provider-yba/internal/customer"
	"github.com/yugabyte/terraform-provider-yba/internal/installation"
	"github.com/yugabyte/terraform-provider-yba/internal/onprem"
	"github.com/yugabyte/terraform-provider-yba/internal/releases"
	"github.com/yugabyte/terraform-provider-yba/internal/universe"
	"github.com/yugabyte/terraform-provider-yba/internal/user"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"

	// New provider packages following yba-cli patterns
	awsProvider "github.com/yugabyte/terraform-provider-yba/internal/provider/aws"
	azureProvider "github.com/yugabyte/terraform-provider-yba/internal/provider/azure"
	gcpProvider "github.com/yugabyte/terraform-provider-yba/internal/provider/gcp"
	"github.com/yugabyte/terraform-provider-yba/internal/storageconfig"
	"github.com/yugabyte/terraform-provider-yba/internal/telemetry"
)

func init() {
	// Set descriptions to support markdown syntax, this will be used in document generation
	// and the language server.
	schema.DescriptionKind = schema.StringMarkdown

	// Customize the content of descriptions when output. For example you can add defaults on
	// to the exported descriptions if present.
	// schema.SchemaDescriptionBuilder = func(s *schema.Schema) string {
	// 	desc := s.Description
	// 	if s.Default != nil {
	// 		desc += fmt.Sprintf(" Defaults to `%v`.", s.Default)
	// 	}
	// 	return strings.TrimSpace(desc)
	// }
}

// New creates a new terraform provider with input values
func New() *schema.Provider {
	return &schema.Provider{
		Schema: map[string]*schema.Schema{
			"enable_https": {
				Type:     schema.TypeBool,
				Optional: true,
				DefaultFunc: schema.MultiEnvDefaultFunc(
					[]string{"YBA_ENABLE_HTTPS", "YB_ENABLE_HTTPS"},
					true,
				),
				Description: "Connection to YugabyteDB Anywhere application via HTTPS. " +
					"True by default.",
			},
			"host": {
				Type:     schema.TypeString,
				Optional: true,
				Description: "IP address or Domain Name with port " +
					"for the YugabyteDB Anywhere application.",
				// Accept either YBA_HOST or YB_HOST (prefer YBA_*)
				DefaultFunc: schema.MultiEnvDefaultFunc(
					[]string{"YBA_HOST", "YB_HOST"},
					"localhost:9000",
				),
			},
			"api_token": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "YugabyteDB Anywhere Customer API Token.",
				// Accept YBA_API_TOKEN, YBA_API_KEY, or YB_API_KEY (in order of preference)
				DefaultFunc: schema.MultiEnvDefaultFunc(
					[]string{"YBA_API_TOKEN", "YBA_API_KEY", "YB_API_KEY"},
					"",
				),
			},
		},
		DataSourcesMap: map[string]*schema.Resource{
			"yba_provider_filter":        cloud_provider.ProviderFilter(),
			"yba_provider_key":           cloud_provider.ProviderKey(),
			"yba_provider_regions":       cloud_provider.ProviderRegions(),
			"yba_provider_image_bundles": cloud_provider.ProviderImageBundles(),
			"yba_storage_configs":        backups.StorageConfigs(),
			"yba_release_version":        releases.ReleaseVersion(),
			"yba_backup_info":            backups.Lists(),
			"yba_onprem_preflight":       onprem.PreflightCheck(),
			"yba_onprem_nodes":           onprem.NodeInstanceFilter(),
			"yba_universe_filter":        universe.UniverseFilter(),
			"yba_universe_schema":        universe.DataSourceUniverseSchema(),
		},
		ResourcesMap: map[string]*schema.Resource{
			"yba_installer":         installation.ResourceYBAInstaller(),
			"yba_cloud_provider":    cloud_provider.ResourceCloudProvider(),
			"yba_universe":          universe.ResourceUniverse(),
			"yba_backup":            backups.ResourceBackup(),
			"yba_backup_schedule":   backups.ResourceBackupSchedule(),
			"yba_backups":           backups.ResourceBackupsDeprecated(),
			"yba_user":              user.ResourceUser(),
			"yba_customer_resource": customer.ResourceCustomer(),
			// Deprecated: use yba_s3/gcs/azure/nfs_storage_config instead
			"yba_storage_config_resource": backups.ResourceStorageConfig(),
			"yba_restore":                 backups.ResourceRestore(),
			"yba_onprem_provider":         onprem.ResourceOnPremProvider(),
			"yba_onprem_node_instance":    onprem.ResourceOnPremNodeInstances(),

			// New provider resources following yba-cli patterns
			// These provide a cleaner, more modular API for cloud providers
			"yba_aws_provider":   awsProvider.ResourceAWSProvider(),
			"yba_gcp_provider":   gcpProvider.ResourceGCPProvider(),
			"yba_azure_provider": azureProvider.ResourceAzureProvider(),

			// New storage configuration resources (cleaner alternative to yba_storage_config_resource)
			"yba_s3_storage_config":    storageconfig.ResourceS3StorageConfig(),
			"yba_gcs_storage_config":   storageconfig.ResourceGCSStorageConfig(),
			"yba_azure_storage_config": storageconfig.ResourceAzureStorageConfig(),
			"yba_nfs_storage_config":   storageconfig.ResourceNFSStorageConfig(),

			// Telemetry / observability export resources for log and metric pipelines.
			"yba_telemetry_provider":        telemetry.ResourceTelemetryProvider(),
			"yba_universe_telemetry_config": telemetry.ResourceUniverseTelemetryConfig(),
			"yba_runtime_config":            telemetry.ResourceRuntimeConfig(),
		},
		ConfigureContextFunc: providerConfigure,
	}
}

func providerConfigure(ctx context.Context, d *schema.ResourceData) (interface{},
	diag.Diagnostics) {
	var diags diag.Diagnostics

	host := d.Get("host").(string)
	apiKey := d.Get("api_token").(string)
	enableHTTPS := d.Get("enable_https").(bool)

	c, err := api.NewAPIClient(enableHTTPS, host, apiKey)
	if err != nil {
		return nil, diag.FromErr(err)
	}

	// Unauthenticated bootstrap mode: when no api_token is set, the
	// provider is being used to install YBA via yba_installer on a host
	// where YBA is not yet running. Skip the version check — resources
	// that require it (cloud providers, etc.) need an api_token anyway.
	if apiKey == "" {
		return c, diags
	}

	// Enforce minimum YBA version requirement
	// The Terraform provider requires YBA >= 2024.2.0.0-b1
	if err := utils.CheckMinimumYBAVersion(ctx, c.YugawareClient); err != nil {
		return nil, diag.FromErr(err)
	}

	return c, diags
}
