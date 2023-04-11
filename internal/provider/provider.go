package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/api"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/backups"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/cloud_provider"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/customer"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/installation"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/releases"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/universe"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/user"
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
			"host": {
				Type:        schema.TypeString,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("YB_HOST", "localhost:9000"),
			},
			"api_token": {
				Type:        schema.TypeString,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("YB_API_KEY", ""),
			},
		},
		DataSourcesMap: map[string]*schema.Resource{
			"yb_provider_key":    cloud_provider.ProviderKey(),
			"yb_storage_configs": backups.StorageConfigs(),
			"yb_release_version": releases.ReleaseVersion(),
			"yb_backup_info":     backups.Lists(),
		},
		ResourcesMap: map[string]*schema.Resource{
			"yb_installation":            installation.ResourceInstallation(),
			"yb_cloud_provider":          cloud_provider.ResourceCloudProvider(),
			"yb_universe":                universe.ResourceUniverse(),
			"yb_backups":                 backups.ResourceBackups(),
			"yb_user":                    user.ResourceUser(),
			"yb_customer_resource":       customer.ResourceCustomer(),
			"yb_storage_config_resource": backups.ResourceStorageConfig(),
			"yb_releases":                releases.ResourceReleases(),
			"yb_restore":                 backups.ResourceRestore(),
		},
		ConfigureContextFunc: providerConfigure,
	}
}

func providerConfigure(ctx context.Context, d *schema.ResourceData) (interface{},
	diag.Diagnostics) {
	var diags diag.Diagnostics

	host := d.Get("host").(string)
	apiKey := d.Get("api_token").(string)

	c, err := api.NewApiClient(host, apiKey)
	if err != nil {
		return nil, diag.FromErr(err)
	}
	return c, diags
}
