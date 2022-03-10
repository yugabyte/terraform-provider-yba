package acctest

import (
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/provider"
	"os"
	"testing"
)

func TestAccPreCheck(t *testing.T) {
	if v := os.Getenv("TF_ACC_TEST_HOST"); v == "" {
		t.Fatal("TF_ACC_TEST_HOST must be set for acceptance tests")
	}
	if v := os.Getenv("TF_ACC_TEST_API_KEY"); v == "" {
		t.Fatal("TF_ACC_TEST_API_KEY must be set for acceptance tests")
	}
}

var TestAccProviders = map[string]func() (*schema.Provider, error){
	"openapi": func() (*schema.Provider, error) {
		return provider.New()(), nil
	},
}
