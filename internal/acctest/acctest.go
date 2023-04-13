package acctest

import (
	"os"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/api"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/provider"
)

const (
	// env variables/other constants for yugabyte provider
	testHost              = "YB_HOST"
	testAPIKey            = "YB_API_KEY"
	testYBSoftwareVersion = "YB_SOFTWARE_VERSION"
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

	// ProviderFactories maps schema.Provider to errors generated
	ProviderFactories map[string]func() (*schema.Provider, error)
	// APIClient variable
	APIClient *api.APIClient
)

func init() {
	c, err := api.NewAPIClient(os.Getenv(testHost), os.Getenv(testAPIKey))
	if err != nil {
		panic(err)
	}
	APIClient = c
	ProviderFactories = map[string]func() (*schema.Provider, error){
		ybProviderName: func() (*schema.Provider, error) { return provider.New(), nil },
	}
}

// TestAPIKey getter
func TestAPIKey() string {
	return os.Getenv(testAPIKey)
}

// TestGCPCredentials getter
func TestGCPCredentials() string {
	return os.Getenv(testGCPCredentials)
}

// TestAWSAccessKey getter
func TestAWSAccessKey() string {
	return os.Getenv(testAWSAccessKey)
}

// TestAWSSecretAccessKey getter
func TestAWSSecretAccessKey() string {
	return os.Getenv(testAWSSecretAccessKey)
}

// TestAzureClientID getter
func TestAzureClientID() string {
	return os.Getenv(testAzureClientID)
}

// TestAzureSubscriptionID getter
func TestAzureSubscriptionID() string {
	return os.Getenv(testAzureSubscriptionID)
}

// TestAzureResourceGroup getter
func TestAzureResourceGroup() string {
	return os.Getenv(testAzureResourceGroup)
}

// TestAzureTenantID getter
func TestAzureTenantID() string {
	return os.Getenv(testAzureTenantID)
}

// TestAzureClientSecret getter
func TestAzureClientSecret() string {
	return os.Getenv(testAzureClientSecret)
}

// TestYBSoftwareVersion getter
func TestYBSoftwareVersion() string {
	return os.Getenv(testYBSoftwareVersion)
}

// TestAccPreCheckGCP Preflight checks for acceptance tests
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

// TestAccPreCheckAWS Preflight checks for acceptance tests
func TestAccPreCheckAWS(t *testing.T) {
	if v := os.Getenv(testAWSAccessKey); v == "" {
		t.Fatal(testAWSAccessKey + " must be set for acceptance tests")
	}
	if v := os.Getenv(testAWSSecretAccessKey); v == "" {
		t.Fatal(testAWSSecretAccessKey + " must be set for acceptance tests")
	}
}

// TestAccPreCheckAzure Preflight checks for acceptance tests
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

// TestAccPreCheck Preflight checks for acceptance tests
func TestAccPreCheck(t *testing.T) {
	if v := os.Getenv(testHost); v == "" {
		t.Fatal(testHost + " must be set for acceptance tests")
	}
	if v := os.Getenv(testAPIKey); v == "" {
		t.Fatal(testAPIKey + " must be set for acceptance tests")
	}
	if v := os.Getenv(testYBSoftwareVersion); v == "" {
		t.Fatal(testYBSoftwareVersion + " must be set for acceptance tests")
	}
}

// IsResourceNotFoundError function
func IsResourceNotFoundError(err error) bool {
	if strings.Contains(err.Error(), "404") {
		return true
	}
	return false
}
