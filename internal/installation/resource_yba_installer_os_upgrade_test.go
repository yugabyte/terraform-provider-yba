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

package installation_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"

	"github.com/yugabyte/terraform-provider-yba/internal/acctest"
	"github.com/yugabyte/terraform-provider-yba/internal/api"
)

// TestAccLong_YBA_GCP_OSImageUpgrade proves YBA survives a host reimage. It
// stands up its own throwaway YBA on a fresh GCP VM (never the shared standing
// fixture, which must not be reimaged) with /opt/yugabyte/data on a separate
// persistent disk, creates a yba_gcp_provider, then flips the boot image so
// the VM is destroyed and recreated. The reinstall must rehydrate from the
// surviving data disk: instance_id changes, provider UUID doesn't. Both images
// are AlmaLinux 9 — alma8's older Python fails YBA preflight. ~30 min (two
// real YBA installs); missing TF_VAR_GCP_YBA_VERSION or repo-root license
// skips the test rather than failing it.
func TestAccLong_YBA_GCP_OSImageUpgrade(t *testing.T) {
	ybaVersion := os.Getenv("TF_VAR_GCP_YBA_VERSION")
	licensePath := repoPath("yugabyte_anywhere.lic")
	settingsPath := repoPath("acctest", "resources", "yba-ctl.yml")
	bootScriptPath := repoPath("acctest", "resources", "gcp-bootscript.sh")

	var providerUUIDBefore, providerUUIDAfter string
	var instanceIDBefore, instanceIDAfter string

	name := gcpSafeName(acctest.RandomName("ybaosup"))

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			acctest.TestAccPreCheckGCP(t)
			if ybaVersion == "" {
				t.Skip("TF_VAR_GCP_YBA_VERSION not set; skipping YBA OS-image-upgrade test")
			}
			if _, err := os.Stat(licensePath); err != nil {
				t.Skipf("YBA license not found at %s; skipping YBA OS-image-upgrade test",
					licensePath)
			}
		},
		ProviderFactories: acctest.ProviderFactories,
		ExternalProviders: map[string]resource.ExternalProvider{
			"google": {Source: "hashicorp/google", VersionConstraint: ">= 5.0"},
			"tls":    {Source: "hashicorp/tls", VersionConstraint: ">= 4.0"},
			"random": {Source: "hashicorp/random", VersionConstraint: ">= 3.0"},
			"null":   {Source: "hashicorp/null", VersionConstraint: ">= 3.0"},
		},
		Steps: []resource.TestStep{
			{
				Config: osUpgradeGCPConfig(name,
					"data.google_compute_image.old.self_link", ybaVersion,
					bootScriptPath, licensePath, settingsPath),
				Check: resource.ComposeTestCheckFunc(
					captureAttr("yba_gcp_provider.test", "id", &providerUUIDBefore),
					captureAttr("google_compute_instance.yba", "instance_id",
						&instanceIDBefore),
				),
			},
			{
				Config: osUpgradeGCPConfig(name,
					"data.google_compute_image.new.self_link", ybaVersion,
					bootScriptPath, licensePath, settingsPath),
				Check: resource.ComposeTestCheckFunc(
					captureAttr("yba_gcp_provider.test", "id", &providerUUIDAfter),
					captureAttr("google_compute_instance.yba", "instance_id",
						&instanceIDAfter),
					checkOSUpgradePreservedProvider(t,
						&instanceIDBefore, &instanceIDAfter,
						&providerUUIDBefore, &providerUUIDAfter),
					checkProviderStillOnYBA(t, &providerUUIDAfter),
				),
			},
		},
	})
}

// checkOSUpgradePreservedProvider asserts the host was actually reimaged and the
// cloud provider object was not recreated in the process.
func checkOSUpgradePreservedProvider(t *testing.T,
	instBefore, instAfter, provBefore, provAfter *string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		if *instBefore == "" || *instAfter == "" {
			return errors.New("instance_id was not captured in both steps")
		}
		if *instBefore == *instAfter {
			return fmt.Errorf(
				"expected the YBA host VM to be reimaged (instance_id change), "+
					"but it stayed %s — the boot image change did not replace the VM",
				*instBefore)
		}
		if *provBefore == "" || *provAfter == "" {
			return errors.New("yba_gcp_provider id was not captured in both steps")
		}
		if *provBefore != *provAfter {
			return fmt.Errorf(
				"cloud provider was recreated across the OS upgrade (uuid %s -> %s); "+
					"YBA state did not survive the host reimage",
				*provBefore, *provAfter)
		}
		oldImg := stateAttr(s, "data.google_compute_image.old", "self_link")
		newImg := stateAttr(s, "data.google_compute_image.new", "self_link")
		t.Logf("VERIFIED OS image upgrade: YBA host VM reimaged from %s to %s "+
			"(instance_id %s -> %s)", oldImg, newImg, *instBefore, *instAfter)
		t.Logf("VERIFIED provider preserved: yba_gcp_provider %s was not recreated "+
			"across the reimage (same UUID before and after)", *provAfter)
		return nil
	}
}

// checkProviderStillOnYBA asks the rehydrated YBA (host/token from state, not
// the shared fixture YBA) for the pre-upgrade provider — direct proof that
// /opt/yugabyte/data survived.
func checkProviderStillOnYBA(t *testing.T, providerUUID *string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		host := stateAttr(s, "google_compute_address.yba", "address")
		token := stateAttr(s, "yba_customer_resource.customer", "api_token")
		if host == "" || token == "" {
			return errors.New("could not read YBA host/api_token from state")
		}

		c, err := api.NewAPIClient(true, host, token)
		if err != nil {
			return fmt.Errorf("connecting to rehydrated YBA at %s: %w", host, err)
		}

		providers, _, err := c.YugawareClient.CloudProvidersAPI.
			GetListOfProviders(context.Background(), c.CustomerID).Execute()
		if err != nil {
			return fmt.Errorf("listing providers on rehydrated YBA: %w", err)
		}
		for _, p := range providers {
			if p.GetUuid() == *providerUUID {
				t.Logf("VERIFIED data survived: cloud provider %s (name=%q code=%q) "+
					"is still present on the rehydrated YBA at %s after the OS image "+
					"upgrade — /opt/yugabyte/data persisted across the host reimage",
					*providerUUID, p.GetName(), p.GetCode(), host)
				return nil
			}
		}
		return fmt.Errorf(
			"cloud provider %s is gone from YBA after the OS upgrade; data was lost",
			*providerUUID)
	}
}

// captureAttr saves a state attribute into out for cross-step comparison;
// attr "id" reads Primary.ID, not a real attribute.
func captureAttr(resourceName, attr string, out *string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("resource not found in state: %s", resourceName)
		}
		if attr == "id" {
			*out = rs.Primary.ID
		} else {
			*out = rs.Primary.Attributes[attr]
		}
		if *out == "" {
			return fmt.Errorf("%s.%s is empty in state", resourceName, attr)
		}
		return nil
	}
}

func stateAttr(s *terraform.State, resourceName, attr string) string {
	rs, ok := s.RootModule().Resources[resourceName]
	if !ok {
		return ""
	}
	return rs.Primary.Attributes[attr]
}

// osUpgradeGCPConfig renders the throwaway-YBA-stand config. bootImageRef is an
// HCL reference to the boot-image data source for this step (old then new);
// everything else is constant so the only planned change between steps is the
// host reimage.
func osUpgradeGCPConfig(
	name, bootImageRef, ybaVersion, bootScript, license, settings string) string {
	return fmt.Sprintf(`
variable "GCP_PROJECT_ID" { type = string }
variable "GCP_REGION"     { type = string }
variable "GCP_VPC_NETWORK" { type = string }
variable "GCP_SUBNETWORK"  { type = string }

variable "GCP_CREDENTIALS" {
  type      = string
  sensitive = true
}

provider "google" {
  project     = var.GCP_PROJECT_ID
  region      = var.GCP_REGION
  credentials = var.GCP_CREDENTIALS
}

# Two most recent AlmaLinux 9 images, so nothing pins image names: new = family
# head, old = newest almalinux-9 whose name differs from new (second newest).
data "google_compute_image" "new" {
  project     = "almalinux-cloud"
  family      = "almalinux-9"
  most_recent = true
}

data "google_compute_image" "old" {
  project     = "almalinux-cloud"
  most_recent = true
  filter      = "(family = \"almalinux-9\") (name != \"${data.google_compute_image.new.name}\")"

  lifecycle {
    postcondition {
      condition     = self.self_link != data.google_compute_image.new.self_link
      error_message = "could not resolve a second-most-recent almalinux-9 image distinct from the newest"
    }
  }
}

# Bootstrap (tokenless) YBA provider: installs YBA and registers the first
# customer on the freshly-created VM, before any API token exists.
provider "yba" {
  alias = "bootstrap"
  host  = google_compute_address.yba.address

  api_token = ""
}

# Authenticated YBA provider: token comes from the customer above, so it
# configures only after YBA is up; creates the provider whose survival is asserted.
provider "yba" {
  host         = google_compute_address.yba.address
  api_token    = yba_customer_resource.customer.api_token
  enable_https = true
}

resource "tls_private_key" "yba" {
  algorithm = "RSA"
  rsa_bits  = 4096
}

# Static external IP so the YBA address stays constant across the VM reimage.
resource "google_compute_address" "yba" {
  name         = "%[1]s"
  region       = var.GCP_REGION
  address_type = "EXTERNAL"
}

# NOT recreated when the boot image changes: /opt/yugabyte/data outliving the
# reimage is the point of the test.
resource "google_compute_disk" "data" {
  name = "%[1]s-data"
  zone = "${var.GCP_REGION}-a"
  type = "pd-balanced"
  size = 250
}

resource "google_compute_instance" "yba" {
  name         = "%[1]s"
  machine_type = "n2-standard-4"
  zone         = "${var.GCP_REGION}-a"

  allow_stopping_for_update = true

  # Changing this image replaces the VM (GCP can't reimage a boot disk in
  # place); that replacement is the "OS upgrade" under test.
  boot_disk {
    initialize_params {
      image = %[2]s
      size  = 100
      type  = "pd-balanced"
    }
  }

  # device_name must match the VM hostname; the startup script resolves the data
  # disk at /dev/disk/by-id/google-$(hostname -s).
  attached_disk {
    source      = google_compute_disk.data.self_link
    device_name = "%[1]s"
  }

  network_interface {
    network    = var.GCP_VPC_NETWORK
    subnetwork = var.GCP_SUBNETWORK

    access_config {
      nat_ip = google_compute_address.yba.address
    }
  }

  metadata = {
    ssh-keys       = "yugabyte:${tls_private_key.yba.public_key_openssh}"
    startup-script = file("%[3]s")
  }

  service_account {
    email  = jsondecode(var.GCP_CREDENTIALS).client_email
    scopes = ["cloud-platform"]
  }
}

resource "random_password" "customer" {
  length           = 16
  min_upper        = 1
  min_lower        = 1
  min_numeric      = 1
  min_special      = 1
  override_special = "!#$%%*-_"
}

# Block the install until the startup script has mounted /opt/yugabyte/data.
resource "null_resource" "wait_for_data_mount" {
  triggers = {
    instance = google_compute_instance.yba.instance_id
  }

  connection {
    type        = "ssh"
    host        = google_compute_address.yba.address
    user        = "yugabyte"
    private_key = tls_private_key.yba.private_key_openssh
    timeout     = "10m"
  }

  provisioner "remote-exec" {
    inline = [
      "for i in $(seq 1 120); do mountpoint -q /opt/yugabyte/data && exit 0; sleep 5; done; echo 'timed out waiting for /opt/yugabyte/data to mount' >&2; exit 1",
    ]
  }
}

resource "yba_installer" "install" {
  provider = yba.bootstrap

  ssh_host_ip               = google_compute_address.yba.address
  ssh_user                  = "yugabyte"
  ssh_private_key           = tls_private_key.yba.private_key_openssh
  yba_license_file          = "%[4]s"
  application_settings_file = "%[5]s"
  yba_version               = "%[6]s"
  host_os                   = "linux"
  host_architecture         = "x86_64"

  # Reinstall on the replaced VM: sees the re-attached, pre-populated
  # /opt/yugabyte/data and runs 'install --without-data', rehydrating YBA.
  lifecycle {
    replace_triggered_by = [google_compute_instance.yba]
  }

  depends_on = [null_resource.wait_for_data_mount]
}

resource "yba_customer_resource" "customer" {
  provider = yba.bootstrap

  code     = "admin"
  email    = "admin@example.com"
  name     = "admin"
  password = random_password.customer.result

  lifecycle {
    ignore_changes = [password]
  }

  depends_on = [yba_installer.install]
}

resource "yba_gcp_provider" "test" {
  name        = "%[1]s-provider"
  credentials = var.GCP_CREDENTIALS
  project_id  = var.GCP_PROJECT_ID
  network     = var.GCP_VPC_NETWORK
  regions {
    code          = var.GCP_REGION
    shared_subnet = var.GCP_SUBNETWORK
  }
  yba_managed_image_bundles {
    arch = "x86_64"
  }
  air_gap_install = false

  depends_on = [yba_customer_resource.customer]
}
`, name, bootImageRef, bootScript, license, settings, ybaVersion)
}

// repoPath resolves repo-relative paths via runtime.Caller so file() references
// and the license check work regardless of the test's working directory.
func repoPath(parts ...string) string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return filepath.Join(parts...)
	}
	root := filepath.Join(filepath.Dir(thisFile), "..", "..")
	return filepath.Join(append([]string{root}, parts...)...)
}

var gcpNameInvalid = regexp.MustCompile(`[^a-z0-9-]`)

// gcpSafeName coerces a random name to GCP naming rules: lowercase [a-z0-9-],
// starting with a letter.
func gcpSafeName(s string) string {
	s = gcpNameInvalid.ReplaceAllString(strings.ToLower(s), "-")
	s = strings.Trim(s, "-")
	if s == "" || s[0] < 'a' || s[0] > 'z' {
		s = "yba-" + s
	}
	return s
}
