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

// FindProvider finds a provider by UUID from a list of providers.
// Returns utils.ErrResourceNotFound if the provider is not in the list.
func FindProvider(providers []client.Provider, uuid string) (*client.Provider, error) {
	for _, p := range providers {
		if *p.Uuid == uuid {
			return &p, nil
		}
	}
	return nil, utils.ResourceNotFoundError("provider", uuid)
}

// IsProviderNotFoundError checks if an error indicates a provider was not found.
// This is used by Read functions to detect when a provider has been deleted
// outside of Terraform, allowing the resource to be removed from state.
func IsProviderNotFoundError(err error) bool {
	return utils.IsResourceNotFoundError(err)
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

// GetAPIClient extracts the API client from Terraform meta interface
func GetAPIClient(meta interface{}) (*client.APIClient, string) {
	apiClient := meta.(*api.APIClient)
	return apiClient.YugawareClient, apiClient.CustomerID
}

// LatestAccessKey returns the most recently created access key from the list.
// YBA never deletes old access keys on rotation - it appends new ones - and
// allAccessKeys has no server-side ordering guarantee. We select by max
// CreationDate to match what YBA's own getLatestKey() does
// (ORDER BY creation_date DESC LIMIT 1).
func LatestAccessKey(keys []client.AccessKey) *client.AccessKey {
	if len(keys) == 0 {
		return nil
	}
	latest := &keys[0]
	for i := 1; i < len(keys); i++ {
		if keys[i].HasCreationDate() {
			if !latest.HasCreationDate() ||
				keys[i].GetCreationDate().After(latest.GetCreationDate()) {
				latest = &keys[i]
			}
		}
	}
	return latest
}

// ValidateSSHKeypairNameUnique returns an error if newName matches the
// KeyPairName of any existing access key on the provider. YBA versions keys on
// every update by appending a timestamp (e.g. "my-key-2026-03-18-10-01-29")
// when a key with the requested name already exists, so reusing a base name
// silently produces a renamed version rather than surfacing the conflict.
// This check fails fast instead.
func ValidateSSHKeypairNameUnique(existing []client.AccessKey, newName string) error {
	for _, k := range existing {
		info := k.GetKeyInfo()
		if info.GetKeyPairName() == newName {
			return fmt.Errorf(
				"ssh_keypair_name %q already exists on this provider; choose "+
					"a unique name, or omit ssh_keypair_name and "+
					"ssh_private_key_content to let YugabyteDB Anywhere "+
					"manage the key pair",
				newName,
			)
		}
	}
	return nil
}
