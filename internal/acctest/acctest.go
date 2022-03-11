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
	testHost        = "YB_HOST"
	testApiKey      = "TF_ACC_TEST_API_KEY"
	testCloudConfig = "TF_ACC_TEST_CLOUD_CONFIG"
	YBProviderName  = "yb"
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

func TestCloudConfig() string {
	return os.Getenv(testCloudConfig)
}

func TestAccPreCheck(t *testing.T) {
	if v := os.Getenv(testHost); v == "" {
		t.Fatal(testHost + " must be set for acceptance tests")
	}
	if v := os.Getenv(testApiKey); v == "" {
		t.Fatal(testApiKey + " must be set for acceptance tests")
	}
	if v := os.Getenv(testCloudConfig); v == "" {
		t.Fatal(testCloudConfig + " must be set for acceptance tests")
	}
}

func GetCtxWithConnectionInfo(s *terraform.InstanceState) (context.Context, string) {
	ctx := context.Background()
	key := s.Attributes["connection_info.0.api_token"]
	cUUID := s.Attributes["connection_info.0.cuuid"]
	return api.SetContextApiKey(ctx, key), cUUID
}
