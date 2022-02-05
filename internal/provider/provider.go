package provider

import (
	"context"
	"errors"
	"github.com/go-logr/zapr"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/yugabyte/yb-tools/yugaware-client/pkg/client"
	"go.uber.org/zap"
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
				"yb_customer": dataSourceCustomer(),
			},
			ResourcesMap: map[string]*schema.Resource{
				"yb_cloud_provider": resourceCloudProvider(),
				"yb_universe":       resourceUniverse(),
			},
			ConfigureContextFunc: providerConfigure,
		}
	}
}

func providerConfigure(ctx context.Context, d *schema.ResourceData) (interface{}, diag.Diagnostics) {
	// Setup a User-Agent for your API client (replace the provider name for yours):
	// userAgent := p.UserAgent("terraform-provider-scaffolding", version)
	// TODO: myClient.UserAgent = userAgent

	var diags diag.Diagnostics

	key := d.Get("apikey").(string)
	host := d.Get("host").(string)
	if key == "" {
		return nil, diag.FromErr(errors.New("yugabyte platform API key is required"))
	}

	zapLog, err := zap.NewDevelopment()
	if err != nil {
		return nil, diag.FromErr(err)
	}
	log := zapr.NewLogger(zapLog)
	c, err := client.New(ctx, log, host).APIToken(key).TimeoutSeconds(10).Connect()
	if err != nil {
		return nil, diag.FromErr(err)
	}

	return c, diags
}
