package api

import (
	"fmt"
	"github.com/yugabyte/yb-tools/yugaware-client/pkg/client"
	"io"
	"net/http"
)

type ApiClient struct {
	VanillaClient  *VanillaClient
	YugawareClient *client.YugawareClient
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
