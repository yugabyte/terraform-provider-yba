package api

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	client "github.com/yugabyte/platform-go-client"
	"io"
	"net/http"
)

func SetContextApiKey(ctx context.Context, key string) context.Context {
	return context.WithValue(ctx, client.ContextAPIKeys, map[string]client.APIKey{"apiKeyAuth": {Key: key}})
}

func GetConnectionInfo(d *schema.ResourceData) (string, string) {
	m := d.Get("connection_info").([]interface{})[0].(map[string]interface{})
	return m["cuuid"].(string), m["api_token"].(string)
}

type ApiClient struct {
	VanillaClient  *VanillaClient
	YugawareClient *client.APIClient
}

func NewApiClient(vc *VanillaClient, yc *client.APIClient) *ApiClient {
	return &ApiClient{
		VanillaClient:  vc,
		YugawareClient: yc,
	}
}

type VanillaClient struct {
	// TODO: remove this client, used for accessing non-public APIs
	Client *http.Client
	Host   string
}

func (c VanillaClient) MakeRequest(method string, url string, body io.Reader, apiKey string) (*http.Response, error) {
	req, err := http.NewRequest(method, fmt.Sprintf("http://%s/%s", c.Host, url), body)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-AUTH-YW-API-TOKEN", apiKey)

	r, err := c.Client.Do(req)
	if err != nil {
		return nil, err
	}
	return r, err
}
