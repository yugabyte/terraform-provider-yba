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
				Type:        schema.TypeBool,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("YB_ENABLE_HTTPS", true),
				Description: "Connection to YugabyteDB Anywhere application via HTTPS. " +
					"True by default.",
			},
			"host": {
				Type:     schema.TypeString,
				Optional: true,
				Description: "IP address or Domain Name with port " +
					"for the YugabyteDB Anywhere application.",
				DefaultFunc: schema.EnvDefaultFunc("YB_HOST", "localhost:9000"),
			},
			"api_token": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "YugabyteDB Anywhere Customer API Token.",
				DefaultFunc: schema.EnvDefaultFunc("YB_API_KEY", ""),
			},
		},
		DataSourcesMap: map[string]*schema.Resource{
			"yba_provider_key":     cloud_provider.ProviderKey(),
			"yba_storage_configs":  backups.StorageConfigs(),
			"yba_release_version":  releases.ReleaseVersion(),
			"yba_backup_info":      backups.Lists(),
			"yba_onprem_preflight": onprem.PreflifghtCheck(),
		},
		ResourcesMap: map[string]*schema.Resource{
			"yba_installation":            installation.ResourceInstallation(),
			"yba_cloud_provider":          cloud_provider.ResourceCloudProvider(),
			"yba_universe":                universe.ResourceUniverse(),
			"yba_backups":                 backups.ResourceBackups(),
			"yba_user":                    user.ResourceUser(),
			"yba_customer_resource":       customer.ResourceCustomer(),
			"yba_storage_config_resource": backups.ResourceStorageConfig(),
			"yba_releases":                releases.ResourceReleases(),
			"yba_restore":                 backups.ResourceRestore(),
			"yba_onprem_provider":         onprem.ResourceOnPremProvider(),
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
	return c, diags
}
