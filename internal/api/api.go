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

package api

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	client "github.com/yugabyte/platform-go-client"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// APIClient struct to handle API calls
type APIClient struct {
	VanillaClient  *VanillaClient
	YugawareClient *client.APIClient
	APIKey         string
	CustomerID     string
}

// NewAPIClient creates a wrapper for public and non-public APIs
func NewAPIClient(enableHTTPS bool, host, apiKey string) (*APIClient, error) {
	// create swagger go client
	cfg := client.NewConfiguration()
	cfg.Host = host
	if enableHTTPS {
		cfg.Scheme = "https"
		tr := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
		cfg.HTTPClient = &http.Client{Transport: tr}
	} else {
		cfg.Scheme = "http"
	}
	if apiKey != "" {
		cfg.DefaultHeader = map[string]string{"X-AUTH-YW-API-TOKEN": apiKey}
	}
	ywc := client.NewAPIClient(cfg)

	// create vanilla client for non-public APIs
	vc := &VanillaClient{
		Client:      &http.Client{Timeout: 30 * time.Second},
		Host:        host,
		EnableHTTPS: enableHTTPS,
	}

	// create wrapper client
	c := &APIClient{
		VanillaClient:  vc,
		YugawareClient: ywc,
		APIKey:         apiKey,
	}

	// authenticate if api token is provided
	if apiKey != "" {
		r, response, err := c.YugawareClient.SessionManagementApi.GetSessionInfo(
			context.Background()).Execute()
		if err != nil {
			errMessage := utils.ErrorFromHTTPResponse(response, err, "Provider Creation",
				"NewAPIClient", "Get Session Info")
			return nil, errMessage
		}
		if !r.HasCustomerUUID() {
			return nil, errors.New("could not retrieve customer id")
		}
		c.CustomerID = *r.CustomerUUID
	}
	return c, nil
}

// VanillaClient struct used for accessing non-public APIs
type VanillaClient struct {
	Client      *http.Client
	Host        string
	EnableHTTPS bool
}

func (c VanillaClient) makeRequest(method string, url string, body io.Reader, apiKey string) (
	*http.Response, error) {
	var req *http.Request
	var err error
	if c.EnableHTTPS {
		http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
		req, err = http.NewRequest(method, fmt.Sprintf("https://%s/%s", c.Host, url), body)
		if err != nil {
			return nil, err
		}
	} else {
		req, err = http.NewRequest(method, fmt.Sprintf("http://%s/%s", c.Host, url), body)
		if err != nil {
			return nil, err
		}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-AUTH-YW-API-TOKEN", apiKey)

	r, err := c.Client.Do(req)
	if err != nil {
		return nil, err
	}
	return r, err
}
