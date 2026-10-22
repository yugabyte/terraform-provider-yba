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

package oci_test

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"

	"github.com/yugabyte/terraform-provider-yba/internal/acctest"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

func TestAccOCIProvider_APIKey(t *testing.T) {
	acctest.TestAccPreCheckOCI(t)
	keyPath := writeOCIKeyFile(t)
	rName := acctest.RandomName("oci")
	config := ociProviderConfigAPIKey(rName, keyPath)
	resource.ParallelTest(t, resource.TestCase{
		ProviderFactories: acctest.ProviderFactories,
		CheckDestroy:      testAccCheckDestroyOCIProvider,
		Steps: append([]resource.TestStep{{
			Config: config,
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr("yba_oci_provider.test", "name", rName),
				resource.TestCheckResourceAttr(
					"yba_oci_provider.test", "use_instance_principal", "false"),
			),
		}}, acctest.ImportPlanSteps(t, "yba_oci_provider.test", config, acctest.ImportPlanOptions{
			// YBA does not receive the local path. Read stores the PEM instead.
			VerifyIgnore: []string{"private_key_file_path"},
		})...),
	})
}

func writeOCIKeyFile(t *testing.T) string {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(os.Getenv("TF_VAR_OCI_PRIVATE_KEY_B64"))
	if err != nil {
		t.Fatalf("TF_VAR_OCI_PRIVATE_KEY_B64: %v", err)
	}
	path := filepath.Join(t.TempDir(), "oci_api_key.pem")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func testAccCheckDestroyOCIProvider(s *terraform.State) error {
	apiClient, err := acctest.APIClientForCloud("OCI")
	if err != nil {
		return err
	}
	for _, r := range s.RootModule().Resources {
		if r.Type != "yba_oci_provider" {
			continue
		}
		time.Sleep(60 * time.Second)
		res, response, err := apiClient.YugawareClient.CloudProvidersAPI.
			GetListOfProviders(context.Background(), apiClient.CustomerID).Execute()
		if err != nil {
			return utils.ErrorFromHTTPResponse(
				response, err, utils.TestEntity, "OCI Provider Test", "Read")
		}
		for _, p := range res {
			if p.GetUuid() == r.Primary.ID {
				return errors.New("OCI provider is not destroyed")
			}
		}
	}
	return nil
}

func ociProviderConfigAPIKey(name, keyPath string) string {
	return acctest.YBAProviderBlock("OCI") + fmt.Sprintf(`
variable "OCI_TENANCY_ID" { type = string }
variable "OCI_USER_ID" { type = string }
variable "OCI_FINGERPRINT" { type = string }
variable "OCI_COMPARTMENT_ID" { type = string }
variable "OCI_REGION" { type = string }
variable "OCI_VCN_ID" { type = string }
variable "OCI_SUBNET_ID" { type = string }

resource "yba_oci_provider" "test" {
  name                   = "%s"
  compartment_id         = var.OCI_COMPARTMENT_ID
  default_region         = var.OCI_REGION
  use_instance_principal = false
  tenancy_id             = var.OCI_TENANCY_ID
  user_id                = var.OCI_USER_ID
  fingerprint            = var.OCI_FINGERPRINT
  private_key_file_path  = %q
  regions {
    code   = var.OCI_REGION
    vcn_id = var.OCI_VCN_ID
    zones {
      code   = "${var.OCI_REGION}-AD-1"
      subnet = var.OCI_SUBNET_ID
    }
  }
  yba_managed_image_bundles { arch = "x86_64" }
}
`, name, keyPath)
}
