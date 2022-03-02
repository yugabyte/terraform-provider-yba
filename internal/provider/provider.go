package provider

import (
	"context"
	"errors"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	client "github.com/yugabyte/platform-go-client"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/api"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/backups"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/cloud_provider"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/datasource"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/universe"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/user"
	"net/http"
	"time"
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

func New() func() *schema.Provider {
	return func() *schema.Provider {
		return &schema.Provider{
			Schema: map[string]*schema.Schema{
				"apikey": {
					Type:        schema.TypeString,
					Optional:    true,
					Sensitive:   true,
					DefaultFunc: schema.EnvDefaultFunc("YB_API_KEY", nil),
				},
				"host": {
					Type:        schema.TypeString,
					Optional:    true,
					DefaultFunc: schema.EnvDefaultFunc("YB_HOST", "localhost:9000"),
				},
			},
			DataSourcesMap: map[string]*schema.Resource{
				"yb_customer":        datasource.Customer(),
				"yb_provider_key":    cloud_provider.ProviderKey(),
				"yb_storage_configs": backups.StorageConfigs(),
			},
			ResourcesMap: map[string]*schema.Resource{
				"yb_cloud_provider": cloud_provider.ResourceCloudProvider(),
				"yb_universe":       universe.ResourceUniverse(),
				"yb_backups":        backups.ResourceBackups(),
				"yb_user":           user.ResourceUser(),
			},
			ConfigureContextFunc: providerConfigure,
		}
	}
}

func providerConfigure(ctx context.Context, d *schema.ResourceData) (interface{}, diag.Diagnostics) {
	var diags diag.Diagnostics

	key := d.Get("apikey").(string)
	host := d.Get("host").(string)
	if key == "" {
		return nil, diag.FromErr(errors.New("yugabyte platform API key is required"))
	}

	// create swagger go client
	cfg := client.NewConfiguration()
	cfg.Host = host
	cfg.Scheme = "http"
	cfg.DefaultHeader = map[string]string{"X-AUTH-YW-API-TOKEN": key}
	ybc := client.NewAPIClient(cfg)

	// get customer uuid
	req := ybc.SessionManagementApi.GetSessionInfo(ctx)
	r, _, err := ybc.SessionManagementApi.GetSessionInfoExecute(req)
	if err != nil {
		return nil, diag.FromErr(err)
	}
	cUUID := *r.CustomerUUID

	vc := &api.VanillaClient{
		Client: &http.Client{Timeout: 10 * time.Second},
		ApiKey: key,
		Host:   host,
	}

	return &api.ApiClient{
		YugawareClient: ybc,
		VanillaClient:  vc,
		CustomerUUID:   cUUID,
	}, diags
}
