package provider

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
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
		p := &schema.Provider{
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
					DefaultFunc: schema.EnvDefaultFunc("YB_HOST", "http://localhost:9000"),
				},
			},
			DataSourcesMap: map[string]*schema.Resource{
				"yb_customer": dataSourceCustomer(),
			},
			ResourcesMap: map[string]*schema.Resource{
				"scaffolding_resource": resourceScaffolding(),
			},
			ConfigureContextFunc: providerConfigure,
		}

		//p.ConfigureContextFunc = configure(version, p)

		return p
	}
}

type ApiClient struct {
	Client *http.Client
	ApiKey string
	Host   string
}

func (c ApiClient) MakeRequest(method string, url string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(method, fmt.Sprintf("%s/%s", c.Host, url), body)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-AUTH-YW-API-TOKEN", c.ApiKey)

	r, err := c.Client.Do(req)
	if err != nil {
		return nil, err
	}
	return r, err
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

	c := &ApiClient{
		Client: &http.Client{Timeout: 10 * time.Second},
		ApiKey: key,
		Host:   host,
	}

	return c, diags
}
