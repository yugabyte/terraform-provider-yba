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

// Package providerutil contains shared utilities for cloud provider resources
// following patterns from yba-cli
package providerutil

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	client "github.com/yugabyte/platform-go-client"
	"github.com/yugabyte/terraform-provider-yba/internal/api"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// Provider codes matching yba-cli patterns
const (
	AWSProviderCode    = "aws"
	GCPProviderCode    = "gcp"
	AzureProviderCode  = "azu"
	OnPremProviderCode = "onprem"
	K8sProviderCode    = "kubernetes"
)

// DefaultTimeouts for provider operations
var DefaultTimeouts = &schema.ResourceTimeout{
	Create: schema.DefaultTimeout(30 * time.Minute),
	Update: schema.DefaultTimeout(30 * time.Minute),
	Delete: schema.DefaultTimeout(15 * time.Minute),
}

// WaitForProviderTask waits for a provider-related task to complete
// This mirrors yba-cli's WaitForCreateProviderTask/WaitForUpdateProviderTask patterns
func WaitForProviderTask(
	ctx context.Context,
	taskUUID string,
	providerName string,
	operation string,
	c *client.APIClient,
	cUUID string,
	timeout time.Duration,
) error {
	if taskUUID == "" {
		return nil
	}

	tflog.Info(ctx, fmt.Sprintf("Waiting for provider %s to be %s", providerName, operation))
	err := utils.WaitForTask(ctx, taskUUID, cUUID, c, timeout)
	if err != nil {
		return fmt.Errorf("provider %s %s failed: %w", providerName, operation, err)
	}
	tflog.Info(ctx, fmt.Sprintf("Provider %s has been %s successfully", providerName, operation))
	return nil
}

// FindProvider finds a provider by UUID from a list of providers
func FindProvider(providers []client.Provider, uuid string) (*client.Provider, error) {
	for _, p := range providers {
		if *p.Uuid == uuid {
			return &p, nil
		}
	}
	return nil, fmt.Errorf("could not find provider %s", uuid)
}

// GetProvider fetches the current state of a provider
func GetProvider(
	ctx context.Context,
	c *client.APIClient,
	cUUID string,
	pUUID string,
) (*client.Provider, error) {
	providers, response, err := c.CloudProvidersAPI.GetListOfProviders(ctx, cUUID).Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"Provider", "Read")
		return nil, errMessage
	}
	return FindProvider(providers, pUUID)
}

// ProviderYBAVersionCheck checks if the YBA version supports provider operations
// Mirrors yba-cli's NewProviderYBAVersionCheck
func ProviderYBAVersionCheck(ctx context.Context, c *client.APIClient) (bool, string, error) {
	allowedVersions := utils.YBAMinimumVersion{
		Stable:  utils.YBAAllowEditProviderMinVersion,
		Preview: utils.YBAAllowEditProviderMinVersion,
	}
	return utils.CheckValidYBAVersion(ctx, c, allowedVersions)
}

// ImageBundlesYBAVersionCheck checks if YBA version supports image bundles
func ImageBundlesYBAVersionCheck(ctx context.Context, c *client.APIClient) (bool, string, error) {
	allowedVersions := utils.YBAMinimumVersion{
		Stable:  utils.YBAAllowImageBundlesMinVersion,
		Preview: utils.YBAAllowImageBundlesMinVersion,
	}
	return utils.CheckValidYBAVersion(ctx, c, allowedVersions)
}

// GetAPIClient extracts the API client from Terraform meta interface
func GetAPIClient(meta interface{}) (*client.APIClient, string) {
	apiClient := meta.(*api.APIClient)
	return apiClient.YugawareClient, apiClient.CustomerID
}
