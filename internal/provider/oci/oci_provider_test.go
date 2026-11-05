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
	pathKey := fmt.Sprintf("private_key_file_path = %q", keyPath)
	// Provider.getMaskProviderDetails masks ociPrivateKeyContent on every
	// provider response, and YBA does not store the local path. Read does not
	// set the SSH fields: API responses redact sshPrivateKeyContent
	// (RedactingService.SECRET_PATHS_FOR_APIS), and keyPairName is YBA's key
	// code, which AccessKeyHandler.doEdit renames with a timestamp
	// (AccessKey.getNewKeyCode), so it need not be the configured name. The
	// import check skips all four; the plan after import ignores the three
	// that this config sets.
	imported := ociProviderConfigAPIKey(rName, pathKey+`
  lifecycle {
    ignore_changes = [private_key_file_path, ssh_keypair_name, ssh_private_key_content]
  }`)
	resource.ParallelTest(t, resource.TestCase{
		ProviderFactories: acctest.ProviderFactories,
		CheckDestroy:      testAccCheckDestroyOCIProvider,
		Steps: append([]resource.TestStep{
			{
				Config: ociProviderConfigAPIKey(rName,
					"private_key_content = base64decode(var.OCI_PRIVATE_KEY_B64)"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("yba_oci_provider.test", "name", rName),
					resource.TestCheckResourceAttr(
						"yba_oci_provider.test", "use_instance_principal", "false"),
				),
			},
			{
				Config: ociProviderConfigAPIKey(rName, pathKey),
				Check: resource.TestCheckResourceAttr(
					"yba_oci_provider.test", "private_key_file_path", keyPath),
			},
		}, acctest.ImportPlanSteps(t, "yba_oci_provider.test", imported, acctest.ImportPlanOptions{
			// The move to the path leaves private_key_content "" in state; import
			// never sets it, because OCICloudInfo.withSensitiveDataMasked masks it.
			VerifyIgnore: []string{
				"private_key_content", "private_key_file_path",
				"ssh_keypair_name", "ssh_private_key_content",
			},
		})...),
	})
}

func TestAccOCIProvider_InstancePrincipal(t *testing.T) {
	acctest.TestAccPreCheckOCI(t)
	rName := acctest.RandomName("oci-ip")
	config := ociProviderConfigInstancePrincipal(rName)
	resource.ParallelTest(t, resource.TestCase{
		ProviderFactories: acctest.ProviderFactories,
		CheckDestroy:      testAccCheckDestroyOCIProvider,
		Steps: append([]resource.TestStep{{
			Config: config,
			Check: resource.TestCheckResourceAttr(
				"yba_oci_provider.test", "use_instance_principal", "true"),
		}}, acctest.ImportPlanSteps(
			t, "yba_oci_provider.test", config, acctest.ImportPlanOptions{})...),
	})
}

// TestAccOCIProvider_Update adds a region with its own image and a reordered
// zone list, changes the NTP servers to an unsorted list, then removes the
// region. The create step also reads the regions back through
// yba_provider_regions.
func TestAccOCIProvider_Update(t *testing.T) {
	acctest.TestAccPreCheckOCIMultiRegion(t)
	keyPath := writeOCIKeyFile(t)
	rName := acctest.RandomName("oci-upd")
	const res = "yba_oci_provider.test"
	resource.ParallelTest(t, resource.TestCase{
		ProviderFactories: acctest.ProviderFactories,
		CheckDestroy:      testAccCheckDestroyOCIProvider,
		Steps: []resource.TestStep{
			{
				Config: ociProviderConfigUpdate(rName, keyPath, false) + `
data "yba_provider_regions" "test" {
  provider_id = yba_oci_provider.test.id
}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(res, "regions.#", "1"),
					resource.TestCheckResourceAttr(res, "regions.0.zones.#", "2"),
					resource.TestCheckTypeSetElemNestedAttrs(
						"data.yba_provider_regions.test", "regions.*", map[string]string{
							"code":              os.Getenv("TF_VAR_OCI_REGION"),
							"vnet_name":         os.Getenv("TF_VAR_OCI_VCN_ID"),
							"instance_template": os.Getenv("TF_VAR_OCI_INSTANCE_CONFIGURATION_ID"),
						}),
				),
			},
			{
				Config: ociProviderConfigUpdate(rName, keyPath, true),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(res, "regions.#", "2"),
					resource.TestCheckResourceAttr(res, "ntp_servers.#", "2"),
					resource.TestCheckResourceAttr(res, "ntp_servers.0", "169.254.169.254"),
					resource.TestCheckResourceAttr(res, "ntp_servers.1", "10.0.0.1"),
				),
			},
			{
				Config: ociProviderConfigUpdate(rName, keyPath, false),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(res, "regions.#", "1"),
					resource.TestCheckResourceAttr(res, "ntp_servers.#", "1"),
				),
			},
		},
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

// ociProviderConfigAPIKey is an API-key provider; key sets the private key
// argument and any lifecycle block.
func ociProviderConfigAPIKey(name, key string) string {
	return acctest.YBAProviderBlock("OCI") + fmt.Sprintf(`
variable "OCI_TENANCY_ID" { type = string }
variable "OCI_USER_ID" { type = string }
variable "OCI_FINGERPRINT" { type = string }
variable "OCI_PRIVATE_KEY_B64" {
  type      = string
  sensitive = true
}
variable "OCI_COMPARTMENT_ID" { type = string }
variable "OCI_REGION" { type = string }
variable "OCI_VCN_ID" { type = string }
variable "OCI_SUBNET_ID" { type = string }

resource "yba_oci_provider" "test" {
  name                   = "%[1]s"
  compartment_id         = var.OCI_COMPARTMENT_ID
  default_region         = var.OCI_REGION
  use_instance_principal = false
  tenancy_id             = var.OCI_TENANCY_ID
  user_id                = var.OCI_USER_ID
  fingerprint            = var.OCI_FINGERPRINT
  # A custom SSH key: the fixture's RSA API key PEM, reused.
  ssh_keypair_name        = "%[1]s-key"
  ssh_private_key_content = base64decode(var.OCI_PRIVATE_KEY_B64)
  %[2]s
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
`, name, key)
}

func ociProviderConfigInstancePrincipal(name string) string {
	return acctest.YBAProviderBlock("OCI") + fmt.Sprintf(`
variable "OCI_COMPARTMENT_ID" { type = string }
variable "OCI_REGION" { type = string }
variable "OCI_VCN_ID" { type = string }
variable "OCI_SUBNET_ID" { type = string }

resource "yba_oci_provider" "test" {
  name                   = "%s"
  compartment_id         = var.OCI_COMPARTMENT_ID
  default_region         = var.OCI_REGION
  use_instance_principal = true
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
`, name)
}

// ociProviderConfigUpdate is a two-zone API-key provider with a custom image.
// second adds a region with its own image, lists the first region's zones in
// reverse order, and sets two NTP servers out of sorted order.
func ociProviderConfigUpdate(name, keyPath string, second bool) string {
	zones := `
    zones {
      code   = "${var.OCI_REGION}-AD-1"
      subnet = var.OCI_SUBNET_ID
    }
    zones {
      code   = "${var.OCI_REGION}-AD-2"
      subnet = var.OCI_SUBNET_ID_2
    }`
	ntp := `["169.254.169.254"]`
	secondRegion, secondImage := "", ""
	if second {
		zones = `
    zones {
      code   = "${var.OCI_REGION}-AD-2"
      subnet = var.OCI_SUBNET_ID_2
    }
    zones {
      code   = "${var.OCI_REGION}-AD-1"
      subnet = var.OCI_SUBNET_ID
    }`
		ntp = `["169.254.169.254", "10.0.0.1"]`
		secondRegion = `
  regions {
    code   = var.OCI_REGION_2
    vcn_id = var.OCI_VCN_ID_2
    zones {
      code   = "${var.OCI_REGION_2}-AD-1"
      subnet = var.OCI_SUBNET_REGION_2
    }
  }`
		secondImage = `
        (var.OCI_REGION_2) = var.OCI_IMAGE_ID_2`
	}
	return acctest.YBAProviderBlock("OCI") + fmt.Sprintf(`
variable "OCI_TENANCY_ID" { type = string }
variable "OCI_USER_ID" { type = string }
variable "OCI_FINGERPRINT" { type = string }
variable "OCI_COMPARTMENT_ID" { type = string }
variable "OCI_REGION" { type = string }
variable "OCI_VCN_ID" { type = string }
variable "OCI_SUBNET_ID" { type = string }
variable "OCI_SUBNET_ID_2" { type = string }
variable "OCI_IMAGE_ID" { type = string }
variable "OCI_INSTANCE_CONFIGURATION_ID" { type = string }
variable "OCI_REGION_2" { type = string }
variable "OCI_VCN_ID_2" { type = string }
variable "OCI_SUBNET_REGION_2" { type = string }
variable "OCI_IMAGE_ID_2" { type = string }

resource "yba_oci_provider" "test" {
  name                   = "%[1]s"
  compartment_id         = var.OCI_COMPARTMENT_ID
  default_region         = var.OCI_REGION
  use_instance_principal = false
  tenancy_id             = var.OCI_TENANCY_ID
  user_id                = var.OCI_USER_ID
  fingerprint            = var.OCI_FINGERPRINT
  private_key_file_path  = %[2]q
  set_up_chrony          = true
  ntp_servers            = %[3]s
  regions {
    code                      = var.OCI_REGION
    vcn_id                    = var.OCI_VCN_ID
    instance_configuration_id = var.OCI_INSTANCE_CONFIGURATION_ID%[4]s
  }%[5]s
  yba_managed_image_bundles {
    arch           = "x86_64"
    use_as_default = true
  }
  image_bundles {
    name = "%[1]s-alma9"
    details {
      arch     = "x86_64"
      ssh_user = "opc"
      region_overrides = {
        (var.OCI_REGION) = var.OCI_IMAGE_ID%[6]s
      }
    }
  }
}
`, name, keyPath, ntp, zones, secondRegion, secondImage)
}
