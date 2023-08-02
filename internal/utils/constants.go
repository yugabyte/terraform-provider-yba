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

package utils

// Entities
const (

	// ResourceEntity identifies resources
	ResourceEntity = "Resource"

	// DataSourceEntity identifies data sources
	DataSourceEntity = "Data Source"

	// TestEntity identifies tests
	TestEntity = "Test"
)

// Environment variable fields
const (
	// GCPCredentialsEnv env variable name for gcp provider/storage config/releases
	GCPCredentialsEnv = "GOOGLE_APPLICATION_CREDENTIALS"
	// GCSCredentialsJSON field name to denote in Json request
	GCSCredentialsJSON = "GCS_CREDENTIALS_JSON"

	// AWSAccessKeyEnv env variable name for aws provider/storage config/releases
	AWSAccessKeyEnv = "AWS_ACCESS_KEY_ID"
	// AWSSecretAccessKeyEnv env variable name for aws provider/storage config/releases
	AWSSecretAccessKeyEnv = "AWS_SECRET_ACCESS_KEY"

	// AzureSubscriptionIDEnv env variable name for azure provider
	AzureSubscriptionIDEnv = "AZURE_SUBSCRIPTION_ID"
	// AzureRGEnv env variable name for azure provider
	AzureRGEnv = "AZURE_RG"
	// AzureTenantIDEnv env variable name for azure provider
	AzureTenantIDEnv = "AZURE_TENANT_ID"
	// AzureClientIDEnv env variable name for azure provider
	AzureClientIDEnv = "AZURE_CLIENT_ID"
	// AzureClientSecretEnv env variable name for azure provider
	AzureClientSecretEnv = "AZURE_CLIENT_SECRET"

	// AzureStorageSasTokenEnv env variable name azure storage config
	AzureStorageSasTokenEnv = "AZURE_STORAGE_SAS_TOKEN"
)

// Minimum YugabyteDB Anywhere versions to support operation
const (

	// YBAAllowUniverseMinVersion specifies minimum version
	// required to use Universe resource via YBA Terraform
	YBAAllowUniverseMinVersion = "2.17.1.0-b371"

	// YBAAllowBackupMinVersion specifies minimum version
	// required to use Scheduled Backup resource via YBA Terraform
	YBAAllowBackupMinVersion = "2.18.1.0-b20"

	// YBAAllowEditProviderMinVersion specifies minimum version
	// required to Edit a Provider (onprem or cloud) resource
	// via YBA Terraform
	YBAAllowEditProviderMinVersion = "2.18.0.0-b65"

	// YBAAllowFailureSubTaskListMinVersion specifies minimum version
	// required to fetch failed subtask message from YugabyteDB Anywhere
	YBAAllowFailureSubTaskListMinVersion = "2.18.1.0-b68"
)

// YugabyteDB Anywhere versions >= the minimum listed versions for operations
// that need to be restricted

// YBARestrictBackupVersions are certain YugabyteDB Anywhere versions >= min
// version for backups that would not support the operation
func YBARestrictBackupVersions() []string {
	return []string{"2.19.0.0"}
}

// YBARestrictFailedSubtasksVersions are certain YugabyteDB Anywhere versions >= min
// version for of fetching failed subtask lists that would not support the operation
func YBARestrictFailedSubtasksVersions() []string {
	return []string{"2.19.0.0"}
}
