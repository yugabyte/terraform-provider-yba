package acctest

import (
	"context"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	client "github.com/yugabyte/platform-go-client"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/api"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/provider"
	"os"
	"testing"
)

const (
	testHost                = "YB_HOST"
	testApiKey              = "TF_ACC_TEST_API_KEY"
	testGCPConfig           = "TF_ACC_TEST_GCP_CONFIG"
	testAWSAccessKey        = "AWS_ACCESS_KEY_ID"
	testAWSSecretAccessKey  = "AWS_SECRET_ACCESS_KEY"
	testAzureSubscriptionID = "TF_ACC_TEST_AZURE_SUBSCRIPTION_ID"
	testAzureResourceGroup  = "TF_ACC_TEST_AZURE_RESOURCE_GROUP"
	testAzureTenantID       = "TF_ACC_TEST_AZURE_TENANT_ID"
	testAzureClientID       = "TF_ACC_TEST_CLIENT_ID"
	testAzureClientSecret   = "TF_ACC_TEST_CLIENT_SECRET"
	YBProviderName          = "yb"
)

var (
	ProviderFactories map[string]func() (*schema.Provider, error)
	YWClient          *client.APIClient
)

func init() {
	YWClient = api.NewYugawareClient(os.Getenv(testHost), "http")
	ProviderFactories = map[string]func() (*schema.Provider, error){
		YBProviderName: func() (*schema.Provider, error) { return provider.New(), nil },
	}
}

func TestApiKey() string {
	return os.Getenv(testApiKey)
}

func TestGCPConfig() string {
	return os.Getenv(testGCPConfig)
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

func TestAccPreCheck(t *testing.T) {
	if v := os.Getenv(testHost); v == "" {
		t.Fatal(testHost + " must be set for acceptance tests")
	}
	if v := os.Getenv(testApiKey); v == "" {
		t.Fatal(testApiKey + " must be set for acceptance tests")
	}
	if v := os.Getenv(testGCPConfig); v == "" {
		t.Fatal(testGCPConfig + " must be set for acceptance tests")
	}
	if v := os.Getenv(testAWSAccessKey); v == "" {
		t.Fatal(testAWSAccessKey + " must be set for acceptance tests")
	}
	if v := os.Getenv(testAWSSecretAccessKey); v == "" {
		t.Fatal(testAWSSecretAccessKey + " must be set for acceptance tests")
	}
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

func GetCtxWithConnectionInfo(s *terraform.InstanceState) (context.Context, string) {
	ctx := context.Background()
	key := s.Attributes["connection_info.0.api_token"]
	cUUID := s.Attributes["connection_info.0.cuuid"]
	return api.SetContextApiKey(ctx, key), cUUID
}
