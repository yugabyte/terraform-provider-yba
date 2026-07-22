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

// Package acctest provides shared helpers used by acceptance tests.
package acctest

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"testing"

	sdkacctest "github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

	"github.com/yugabyte/terraform-provider-yba/internal/api"
	"github.com/yugabyte/terraform-provider-yba/internal/provider"
)

const (
	ybProviderName = "yba"

	// Some GCP resources (e.g. yba_cloud_provider) read the SA key from the file
	// at GOOGLE_APPLICATION_CREDENTIALS instead of inline. SetupGCPCredentialsFile
	// materializes testGCPCredentials into a file and points this at it.
	googleAppCredsEnv = "GOOGLE_APPLICATION_CREDENTIALS"

	// TF_VAR_* env variables for GCP - used by Terraform to populate variables
	testGCPCredentials = "TF_VAR_GCP_CREDENTIALS"
	testGCPProject     = "TF_VAR_GCP_PROJECT_ID"
	testGCPVPCNetwork  = "TF_VAR_GCP_VPC_NETWORK"
	testGCPRegion      = "TF_VAR_GCP_REGION"
	testGCPSubnetwork  = "TF_VAR_GCP_SUBNETWORK"
	testGCPImage       = "TF_VAR_GCP_IMAGE"

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
	// APIClient is the shared YBA client (YBA_HOST), used by storage-config,
	// user, and customer tests. Provider tests use APIClientForCloud instead.
	APIClient *api.APIClient
	// sharedClientErr records why APIClient could not be built; surfaced by
	// TestAccPreCheck so only shared-YBA tests fail, not every package.
	sharedClientErr error

	// cloudClients caches one YBA client per cloud, keyed by cloud code.
	cloudClients   = map[string]*api.APIClient{}
	cloudClientsMu sync.Mutex
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

// CloudYBAHost returns TF_VAR_<CLOUD>_YBA_HOST (cloud is the upper-case code,
// e.g. "AWS"). Each cloud has its own YBA. Provider tests must target the YBA
// running on that cloud, because use_iam_instance_profile authenticates with
// the YBA host's own instance role, which only exists on a same-cloud YBA.
func CloudYBAHost(cloud string) string { return os.Getenv("TF_VAR_" + cloud + "_YBA_HOST") }

// CloudYBAAPIKey returns TF_VAR_<CLOUD>_YBA_API_KEY. See CloudYBAHost.
func CloudYBAAPIKey(cloud string) string { return os.Getenv("TF_VAR_" + cloud + "_YBA_API_KEY") }

// APIClientForCloud builds and caches the YBA client for a cloud. Check and
// Destroy helpers use it to read back state from the YBA the test wrote to.
func APIClientForCloud(cloud string) (*api.APIClient, error) {
	cloudClientsMu.Lock()
	defer cloudClientsMu.Unlock()
	if c, ok := cloudClients[cloud]; ok {
		return c, nil
	}
	c, err := api.NewAPIClient(true, CloudYBAHost(cloud), CloudYBAAPIKey(cloud))
	if err != nil {
		return nil, fmt.Errorf("%s fixture YBA (TF_VAR_%s_YBA_HOST=%s) is unreachable: %w",
			cloud, cloud, CloudYBAHost(cloud), err)
	}
	cloudClients[cloud] = c
	return c, nil
}

// YBAProviderBlock returns an HCL provider block plus its variable declarations,
// pointing the provider at the cloud's YBA. Prepend it to a cloud's test config.
//
// The endpoint lives in the config rather than a process-global YBA_HOST. The
// provider reads its endpoint at configure time, so a shared env var would race
// across parallel tests targeting different YBAs.
func YBAProviderBlock(cloud string) string {
	return fmt.Sprintf(`
variable "%[1]s_YBA_HOST" {
  type = string
}

variable "%[1]s_YBA_API_KEY" {
  type      = string
  sensitive = true
}

provider "yba" {
  host         = var.%[1]s_YBA_HOST
  api_token    = var.%[1]s_YBA_API_KEY
  enable_https = true
}
`, cloud)
}

// testPrefix isolates concurrent runs on a shared YBA. Honors TF_ACCTEST_PREFIX;
// otherwise derives "acc-<branch>" from CI env (GITHUB_HEAD_REF on PRs, where
// HEAD is detached; GITHUB_REF_NAME on pushes) or the local git branch. Only the
// branch's last path segment is used ("user/feature" -> "feature") to keep names
// short — they flow into YBA-derived identifiers stored in varchar(100) columns.
func testPrefix() string {
	if p := os.Getenv("TF_ACCTEST_PREFIX"); p != "" {
		return p
	}
	branch := getEnvMulti("GITHUB_HEAD_REF", "GITHUB_REF_NAME")
	if branch == "" {
		out, _ := exec.CommandContext(
			context.Background(), "git", "symbolic-ref", "--short", "-q", "HEAD").Output()
		branch = string(out)
	}
	if i := strings.LastIndex(branch, "/"); i >= 0 {
		branch = branch[i+1:]
	}
	re := regexp.MustCompile(`[^a-z0-9]+`)
	slug := strings.Trim(re.ReplaceAllString(strings.ToLower(branch), "-"), "-")
	if slug == "" {
		return "acc"
	}
	return "acc-" + slug
}

// maxNameLen bounds the length of names produced by RandomName. Acceptance-test
// names flow into YBA-derived identifiers (e.g. cloud-provider access-key codes)
// that YBA stores in varchar(100) columns; an over-long name fails apply with
// "value too long for type character varying(100)". Capping the whole name keeps
// those identifiers comfortably under 100 regardless of branch-name length, so
// contributors never have to keep branch names short by hand.
const maxNameLen = 40

// RandomName builds a unique acceptance-test resource name of the form
// <prefix>-<kind>-<random>, capped at maxNameLen. The random suffix avoids
// collisions within a run and testPrefix isolates concurrent runs by different
// branches against the same YBA; when a long branch prefix would overflow the
// cap, only the prefix is trimmed (uniqueness still comes from the suffix).
func RandomName(kind string) string {
	tail := fmt.Sprintf("%s-%s", kind, sdkacctest.RandString(12))
	prefix := testPrefix()

	// Reserve one char for the separator between prefix and tail.
	if budget := maxNameLen - len(tail) - 1; budget < len(prefix) {
		if budget < 0 {
			budget = 0
		}
		prefix = strings.TrimRight(prefix[:budget], "-")
	}
	if prefix == "" {
		return tail
	}
	return fmt.Sprintf("%s-%s", prefix, tail)
}

func init() {
	ProviderFactories = map[string]func() (*schema.Provider, error){
		ybProviderName: func() (*schema.Provider, error) { return provider.New(), nil },
	}
	// Build the shared client only when a shared YBA is set, so importing this
	// package never panics on an empty YBA_HOST. Tests that use the shared client
	// gate on TestAccPreCheck (which fails fast on an empty host or a dead
	// client), so they never hit a nil client.
	if TestHost() == "" {
		return
	}
	c, err := api.NewAPIClient(true, TestHost(), TestAPIKey())
	if err != nil {
		// Never panic here: that kills every test binary importing this
		// package, including per-cloud provider tests that talk only to their
		// own YBA (TF_VAR_<CLOUD>_YBA_HOST) and never touch the shared one.
		sharedClientErr = err
		return
	}
	APIClient = c
}

// SetupGCPCredentialsFile materializes the inline GCP SA key (TF_VAR_GCP_CREDENTIALS)
// to a 0600 file and points GOOGLE_APPLICATION_CREDENTIALS at it, for resources
// that read the key from a file path rather than inline. It returns a cleanup
// that removes the file; call both from a package TestMain so the file lives for
// the whole run and is deleted after. No-op when the key isn't set or the env
// var is already provided.
//
// The file gets a unique name per test binary. `go test ./...` runs packages
// concurrently, so a fixed shared path would let one package's cleanup delete
// the file while another package is still mid-apply.
func SetupGCPCredentialsFile() (func(), error) {
	creds := os.Getenv(testGCPCredentials)
	if creds == "" || os.Getenv(googleAppCredsEnv) != "" {
		return func() {}, nil
	}
	f, err := os.CreateTemp("", "yba-acctest-gcp-creds-*.json")
	if err != nil {
		return func() {}, err
	}
	path := f.Name()
	if _, err := f.WriteString(creds); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return func() {}, err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return func() {}, err
	}
	if err := os.Setenv(googleAppCredsEnv, path); err != nil {
		_ = os.Remove(path)
		return func() {}, err
	}
	return func() { _ = os.Remove(path) }, nil
}

// TestAccPreCheckGCP Preflight checks for GCP acceptance tests
func TestAccPreCheckGCP(t *testing.T) {
	requiredVars := []string{
		testGCPCredentials,
		testGCPProject,
		testGCPVPCNetwork,
		testGCPRegion,
		testGCPSubnetwork,
		testGCPImage,
	}
	for _, v := range requiredVars {
		if os.Getenv(v) == "" {
			t.Skipf("%s not set; skipping GCP acceptance tests", v)
		}
	}
}

// TestAccPreCheckCloudYBA skips unless the cloud's YBA endpoint is set
// (TF_VAR_<CLOUD>_YBA_HOST and _API_KEY). Without a same-cloud fixture YBA the
// tests would fail at apply, so they skip instead.
func TestAccPreCheckCloudYBA(t *testing.T, cloud string) {
	for _, suffix := range []string{"_YBA_HOST", "_YBA_API_KEY"} {
		v := "TF_VAR_" + cloud + suffix
		if os.Getenv(v) == "" {
			t.Skipf("%s not set; skipping %s tests that target the %s YBA", v, cloud, cloud)
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
			t.Skipf("%s not set; skipping AWS acceptance tests", v)
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
			t.Skipf("%s not set; skipping multi-zone AWS acceptance tests", v)
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
			t.Skipf("%s not set; skipping Azure acceptance tests", v)
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
	if sharedClientErr != nil {
		t.Fatalf("shared YBA (YBA_HOST=%s) is unreachable%s: %v",
			TestHost(), sharedClientErr)
	}
}

// IsResourceNotFoundError function
func IsResourceNotFoundError(err error) bool {
	return strings.Contains(err.Error(), "404")
}
