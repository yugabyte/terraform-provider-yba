package api

import (
	"context"
	"fmt"
	client "github.com/yugabyte/platform-go-client"
	"io"
	"net/http"
)

type ApiClient struct {
	VanillaClient  *VanillaClient
	YugawareClient *client.APIClient
	ApiKeys        map[string]client.APIKey
}

func (c ApiClient) SetContextApiKey(ctx context.Context, cUUID string) context.Context {
	return context.WithValue(ctx, "apiKeys", map[string]client.APIKey{"apiKeyAuth": c.ApiKeys[cUUID]})
}

type VanillaClient struct {
	// TODO: remove this client, used for accessing non-public APIs
	Client *http.Client
	ApiKey string
	Host   string
}

func (c VanillaClient) MakeRequest(method string, url string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(method, fmt.Sprintf("http://%s/%s", c.Host, url), body)
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
