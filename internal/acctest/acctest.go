package acctest

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/api"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/provider"
	"os"
	"testing"
)

const (
	testHost        = "TF_ACC_TEST_HOST"
	testApiKey      = "TF_ACC_TEST_API_KEY"
	testCloudConfig = "TF_ACC_TEST_CLOUD_CONFIG"
	YBProviderName  = "yb"
)

var (
	ProviderFactories map[string]func() (*schema.Provider, error)
	YBProvider        *schema.Provider
)

func init() {
	YBProvider = provider.New()
	ProviderFactories = map[string]func() (*schema.Provider, error){
		YBProviderName: func() (*schema.Provider, error) { return provider.New(), nil },
	}
}

func TestHost() string {
	return os.Getenv(testHost)
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

func ConfigWithYBProvider(config string) string {
	return fmt.Sprintf(`
terraform {
  required_providers {
    yb = {
      version = "~> 0.1.0"
      source  = "terraform.yugabyte.com/platform/yugabyte-platform"
    }
  }
}

provider "yb" {
  host = "%s"
}

%s
`, TestHost(), config)
}

func GetCtxWithConnectionInfo(s *terraform.InstanceState) (context.Context, string) {
	ctx := context.Background()
	key := s.Attributes["connection_info.api_token"]
	cUUID := s.Attributes["connection_info.cuuid"]
	return api.SetContextApiKey(ctx, key), cUUID
}
