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

package acctest

import (
	"os"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/yugabyte/terraform-provider-yba/internal/api"
	"github.com/yugabyte/terraform-provider-yba/internal/provider"
)

const (
	ybProviderName = "yba"

	// TF_VAR_* env variables for GCP - used by Terraform to populate variables
	testGCPCredentials = "TF_VAR_GCP_CREDENTIALS"
	testGCPProject     = "TF_VAR_GCP_PROJECT_ID"
	testGCPVPCNetwork  = "TF_VAR_GCP_VPC_NETWORK"

	// TF_VAR_* env variables for AWS - used by Terraform to populate variables
	testAWSAccessKey       = "TF_VAR_AWS_ACCESS_KEY_ID"
	testAWSSecretAccessKey = "TF_VAR_AWS_SECRET_ACCESS_KEY"
	testAWSSGID            = "TF_VAR_AWS_SG_ID"
	testAWSVPCID           = "TF_VAR_AWS_VPC_ID"
	testAWSZoneSubnetID    = "TF_VAR_AWS_ZONE_SUBNET_ID"
	testAWSZoneSubnetID2   = "TF_VAR_AWS_ZONE_SUBNET_ID_2"
	testAWSAMIID           = "TF_VAR_AWS_AMI_ID"

	// TF_VAR_* env variables for Azure - used by Terraform to populate variables
	testAzureSubscriptionID = "TF_VAR_AZURE_SUBSCRIPTION_ID"
	testAzureResourceGroup  = "TF_VAR_AZURE_RG"
	testAzureTenantID       = "TF_VAR_AZURE_TENANT_ID"
	testAzureClientID       = "TF_VAR_AZURE_CLIENT_ID"
	testAzureClientSecret   = "TF_VAR_AZURE_CLIENT_SECRET"
	testAzureVnetID         = "TF_VAR_AZURE_VNET_ID"
	testAzureSubnetID       = "TF_VAR_AZURE_SUBNET_ID"
)

var (
	// ProviderFactories maps schema.Provider to errors generated
	ProviderFactories map[string]func() (*schema.Provider, error)
	// APIClient variable
	APIClient *api.APIClient
)

// getEnvMulti returns the first non-empty value from the given env var names
func getEnvMulti(names ...string) string {
	for _, name := range names {
		if v := os.Getenv(name); v != "" {
			return v
		}
	}
	return ""
}

// TestHost returns YBA_HOST or YB_HOST
func TestHost() string {
	return getEnvMulti("YBA_HOST", "YB_HOST")
}

// TestAPIKey returns YBA_API_KEY or YB_API_KEY
func TestAPIKey() string {
	return getEnvMulti("YBA_API_KEY", "YB_API_KEY")
}

func init() {
	c, err := api.NewAPIClient(true, TestHost(), TestAPIKey())
	if err != nil {
		panic(err)
	}
	APIClient = c
	ProviderFactories = map[string]func() (*schema.Provider, error){
		ybProviderName: func() (*schema.Provider, error) { return provider.New(), nil },
	}
}

// TestAccPreCheckGCP Preflight checks for GCP acceptance tests
func TestAccPreCheckGCP(t *testing.T) {
	requiredVars := []string{
		testGCPCredentials,
		testGCPProject,
		testGCPVPCNetwork,
	}
	for _, v := range requiredVars {
		if os.Getenv(v) == "" {
			t.Fatalf("%s must be set for GCP acceptance tests", v)
		}
	}
}

// TestAccPreCheckAWS Preflight checks for AWS acceptance tests
func TestAccPreCheckAWS(t *testing.T) {
	requiredVars := []string{
		testAWSAccessKey,
		testAWSSecretAccessKey,
		testAWSSGID,
		testAWSVPCID,
		testAWSZoneSubnetID,
	}
	for _, v := range requiredVars {
		if os.Getenv(v) == "" {
			t.Fatalf("%s must be set for AWS acceptance tests", v)
		}
	}
}

// TestAccPreCheckAWSMultiZone Preflight checks for multi-zone AWS acceptance tests
func TestAccPreCheckAWSMultiZone(t *testing.T) {
	requiredVars := []string{
		testAWSZoneSubnetID2,
		testAWSAMIID,
	}
	for _, v := range requiredVars {
		if os.Getenv(v) == "" {
			t.Fatalf("%s must be set for multi-zone AWS acceptance tests", v)
		}
	}
}

// TestAccPreCheckAzure Preflight checks for Azure acceptance tests
func TestAccPreCheckAzure(t *testing.T) {
	requiredVars := []string{
		testAzureSubscriptionID,
		testAzureResourceGroup,
		testAzureTenantID,
		testAzureClientID,
		testAzureClientSecret,
		testAzureVnetID,
		testAzureSubnetID,
	}
	for _, v := range requiredVars {
		if os.Getenv(v) == "" {
			t.Fatalf("%s must be set for Azure acceptance tests", v)
		}
	}
}

// TestAccPreCheck Preflight checks for all acceptance tests (YBA connection)
func TestAccPreCheck(t *testing.T) {
	if TestHost() == "" {
		t.Fatal("YBA_HOST or YB_HOST must be set for acceptance tests")
	}
	if TestAPIKey() == "" {
		t.Fatal("YBA_API_KEY or YB_API_KEY must be set for acceptance tests")
	}
}

// IsResourceNotFoundError function
func IsResourceNotFoundError(err error) bool {
	if strings.Contains(err.Error(), "404") {
		return true
	}
	return false
}
