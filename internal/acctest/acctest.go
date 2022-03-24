package acctest

import (
	"context"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	client "github.com/yugabyte/platform-go-client"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/api"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/provider"
	"os"
	"strings"
	"testing"
)

const (
	// env variables/other constants for yugabyte provider
	testHost              = "YB_HOST"
	testApiKey            = "TF_ACC_TEST_API_KEY"
	testYBSoftwareVersion = "TF_ACC_TEST_YB_SOFTWARE_VERSION"
	ybProviderName        = "yb"

	// env variables for gcp provider
	testGCPCredentials = "GOOGLE_CREDENTIALS"
	testGCPProject     = "GOOGLE_PROJECT"
	testGCPRegion      = "GOOGLE_REGION"
	testGCPZone        = "GOOGLE_ZONE"

	// env variables for aws provider
	testAWSAccessKey       = "AWS_ACCESS_KEY_ID"
	testAWSSecretAccessKey = "AWS_SECRET_ACCESS_KEY"

	// env variables for azure provider
	testAzureSubscriptionID = "ARM_SUBSCRIPTION_ID"
	testAzureResourceGroup  = "ARM_RESOURCE_GROUP"
	testAzureTenantID       = "ARM_TENANT_ID"
	testAzureClientID       = "ARM_CLIENT_ID"
	testAzureClientSecret   = "ARM_CLIENT_SECRET"
)

var (
	ProviderFactories map[string]func() (*schema.Provider, error)
	YWClient          *client.APIClient
)

func init() {
	YWClient = api.NewYugawareClient(os.Getenv(testHost), "http")
	ProviderFactories = map[string]func() (*schema.Provider, error){
		ybProviderName: func() (*schema.Provider, error) { return provider.New(), nil },
	}
}

func TestApiKey() string {
	return os.Getenv(testApiKey)
}

func TestGCPCredentials() string {
	return os.Getenv(testGCPCredentials)
}

func TestAWSAccessKey() string {
	return os.Getenv(testAWSAccessKey)
}

func TestAWSSecretAccessKey() string {
	return os.Getenv(testAWSSecretAccessKey)
}

func TestAzureClientID() string {
	return os.Getenv(testAzureClientID)
}

func TestAzureSubscriptionID() string {
	return os.Getenv(testAzureSubscriptionID)
}

func TestAzureResourceGroup() string {
	return os.Getenv(testAzureResourceGroup)
}

func TestAzureTenantID() string {
	return os.Getenv(testAzureTenantID)
}

func TestAzureClientSecret() string {
	return os.Getenv(testAzureClientSecret)
}

func TestYBSoftwareVersion() string {
	return os.Getenv(testYBSoftwareVersion)
}

func TestAccPreCheckGCP(t *testing.T) {
	if v := os.Getenv(testGCPCredentials); v == "" {
		t.Fatal(testGCPCredentials + " must be set for acceptance tests")
	}
	if v := os.Getenv(testGCPProject); v == "" {
		t.Fatal(testGCPProject + " must be set for acceptance tests")
	}
	if v := os.Getenv(testGCPRegion); v == "" {
		t.Fatal(testGCPRegion + " must be set for acceptance tests")
	}
	if v := os.Getenv(testGCPZone); v == "" {
		t.Fatal(testGCPZone + " must be set for acceptance tests")
	}
}

func TestAccPreCheckAWS(t *testing.T) {
	if v := os.Getenv(testAWSAccessKey); v == "" {
		t.Fatal(testAWSAccessKey + " must be set for acceptance tests")
	}
	if v := os.Getenv(testAWSSecretAccessKey); v == "" {
		t.Fatal(testAWSSecretAccessKey + " must be set for acceptance tests")
	}
}

func TestAccPreCheckAzure(t *testing.T) {
	if v := os.Getenv(testAzureClientID); v == "" {
		t.Fatal(testAzureClientID + " must be set for acceptance tests")
	}
	if v := os.Getenv(testAzureSubscriptionID); v == "" {
		t.Fatal(testAzureSubscriptionID + " must be set for acceptance tests")
	}
	if v := os.Getenv(testAzureResourceGroup); v == "" {
		t.Fatal(testAzureResourceGroup + " must be set for acceptance tests")
	}
	if v := os.Getenv(testAzureTenantID); v == "" {
		t.Fatal(testAzureTenantID + " must be set for acceptance tests")
	}
	if v := os.Getenv(testAzureClientSecret); v == "" {
		t.Fatal(testAzureClientSecret + " must be set for acceptance tests")
	}
}

func TestAccPreCheck(t *testing.T) {
	if v := os.Getenv(testHost); v == "" {
		t.Fatal(testHost + " must be set for acceptance tests")
	}
	if v := os.Getenv(testApiKey); v == "" {
		t.Fatal(testApiKey + " must be set for acceptance tests")
	}
	if v := os.Getenv(testYBSoftwareVersion); v == "" {
		t.Fatal(testYBSoftwareVersion + " must be set for acceptance tests")
	}
}

func GetCtxWithConnectionInfo(s *terraform.InstanceState) (context.Context, string) {
	ctx := context.Background()
	key := s.Attributes["connection_info.0.api_token"]
	cUUID := s.Attributes["connection_info.0.cuuid"]
	return api.SetContextApiKey(ctx, key), cUUID
}

func IsResourceNotFoundError(err error) bool {
	if strings.Contains(err.Error(), "404") {
		return true
	}
	return false
}
