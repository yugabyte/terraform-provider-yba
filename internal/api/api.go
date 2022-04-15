package api

import (
	"context"
	"errors"
	"fmt"
	client "github.com/yugabyte/platform-go-client"
	"io"
	"net/http"
)

type ApiClient struct {
	VanillaClient  *VanillaClient
	YugawareClient *client.APIClient
	ApiKey         string
	CustomerId     string
}

func (c ApiClient) Authenticate() error {
	r, _, err := c.YugawareClient.SessionManagementApi.GetSessionInfo(context.Background()).Execute()
	if err != nil {
		return err
	}
	if !r.HasCustomerUUID() {
		return errors.New("could not retrieve customer id")
	}
	c.CustomerId = *r.CustomerUUID
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
