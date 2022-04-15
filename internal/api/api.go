package api

import (
	"context"
	"errors"
	"fmt"
	client "github.com/yugabyte/platform-go-client"
	"io"
	"net/http"
	"time"
)

type ApiClient struct {
	VanillaClient  *VanillaClient
	YugawareClient *client.APIClient
	ApiKey         string
	CustomerId     string
}

func NewApiClient(host string, apiKey string) (*ApiClient, error) {
	// create swagger go client
	cfg := client.NewConfiguration()
	cfg.Host = host
	cfg.Scheme = "http"
	if apiKey != "" {
		cfg.DefaultHeader = map[string]string{"X-AUTH-YW-API-TOKEN": apiKey}
	}
	ywc := client.NewAPIClient(cfg)

	// create vanilla client for non-public APIs
	vc := &VanillaClient{
		Client: &http.Client{Timeout: 10 * time.Second},
		Host:   host,
	}

	// create wrapper client
	c := &ApiClient{
		VanillaClient:  vc,
		YugawareClient: ywc,
		ApiKey:         apiKey,
	}

	// authenticate if api token is provided
	if apiKey != "" {
		r, _, err := c.YugawareClient.SessionManagementApi.GetSessionInfo(context.Background()).Execute()
		if err != nil {
			return nil, err
		}
		if !r.HasCustomerUUID() {
			return nil, errors.New("could not retrieve customer id")
		}
		c.CustomerId = *r.CustomerUUID
	}
	return c, nil
}

func (c ApiClient) Authenticate() error {

	return nil
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
