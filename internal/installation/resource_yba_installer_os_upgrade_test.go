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

// TestAccLong_YBA_GCP_OSImageUpgrade exercises the data-disk rehydration flow
// end-to-end on a throwaway YBA stand it deploys itself (NOT the shared standing
// fixture YBA, which other tests depend on and must not be reimaged).
//
// OLD and NEW are resolved automatically as the two most recent AlmaLinux 9
// images (NEW = the family head, OLD = the next most recent), so nothing pins a
// specific image. Both are AlmaLinux 9 on purpose: YBA installs the same way on
// each, whereas the alma8 family ships an older Python that YBA's preflight
// rejects.
//
// Flow:
//  1. Stand up a fresh GCP VM (boot image = OLD) with a separate persistent data
//     disk mounted at /opt/yugabyte/data, install YBA over SSH via yba_installer,
//     register the first customer, and create a yba_gcp_provider through that YBA.
//  2. Flip the VM boot image OLD -> NEW. GCP can't reimage in place, so the VM is
//     destroyed and recreated; the data disk is NOT recreated and re-attaches to
//     the new VM, the startup script re-mounts /opt/yugabyte/data, and the
//     installer (wired with replace_triggered_by on the VM) re-runs. Because the
//     data dir is now populated it installs with --without-data, rehydrating YBA
//     from the surviving disk.
//
// Assertions:
//   - The VM was actually reimaged (its server-assigned instance_id changed).
//   - The yba_gcp_provider was NOT recreated (its UUID is unchanged) AND it still
//     exists in the rehydrated YBA — i.e. no YBA state was lost across the host
//     OS-image upgrade.
//
// This is a long test (~30 min: two YBA installs on real GCP). It reuses the
// standing fixture's network/project/credentials but stands up its OWN VM, data
// disk, YBA install, and cloud provider — none of the standing fixture's
// resources are modified — and tears them all down at the end (resource.Test
// runs destroy even on failure). It needs only the standing fixture env,
// TF_VAR_GCP_YBA_VERSION, and the YBA license at the repo root; any missing ->
// the test skips, not fails.
func TestAccLong_YBA_GCP_OSImageUpgrade(t *testing.T) {
	ybaVersion := os.Getenv("TF_VAR_GCP_YBA_VERSION")
	licensePath := repoPath("yugabyte_anywhere.lic")
	settingsPath := repoPath("acctest", "resources", "yba-ctl.yml")
	bootScriptPath := repoPath("acctest", "resources", "gcp-bootscript.sh")

	// Captured from Terraform state between the two steps.
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
		// yba comes from the in-process factory; the infra providers are pulled
		// from the registry for the duration of the test.
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

// checkProviderStillOnYBA queries the rehydrated YBA directly (using the host and
// API token from state, not the shared fixture YBA) and confirms the provider
// created before the upgrade is still present — the direct proof that
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

// captureAttr records a resource attribute from state into out for cross-step
// comparison. The synthetic attribute name "id" reads Primary.ID.
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

# Resolve the two most recent AlmaLinux 9 images so the upgrade is always between
# real, current releases and nobody has to pin image names. new = the family
# head (newest); old = the most recent almalinux-9 whose name differs from new,
# i.e. the second newest.
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

# Authenticated YBA provider: its token comes from the customer created above,
# so Terraform configures it only after the customer exists and YBA is up. Used
# to create the cloud provider whose survival this test asserts.
provider "yba" {
  host         = google_compute_address.yba.address
  api_token    = yba_customer_resource.customer.api_token
  enable_https = true
}

resource "tls_private_key" "yba" {
  algorithm = "RSA"
  rsa_bits  = 4096
}

# Static external IP so the YBA host/API address stays constant across the VM
# reimage (the VM is replaced; this address is reused).
resource "google_compute_address" "yba" {
  name         = "%[1]s"
  region       = var.GCP_REGION
  address_type = "EXTERNAL"
}

# Persistent YBA state on a separate disk. It is NOT recreated when the boot
# image changes, so /opt/yugabyte/data outlives the host reimage; that survival
# is the whole point of the test.
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

  # Changing this image forces the VM to be replaced (GCP can't reimage a boot
  # disk in place); that replacement is the "OS upgrade" under test.
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

# The data disk is mounted at /opt/yugabyte/data by the VM startup script.
# Block the install until the mount actually exists.
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

  # When the boot image changes the VM is replaced; re-run the installer on the
  # new host. The reinstall sees the re-attached, pre-populated
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

// repoPath builds an absolute path to a repo-relative file from this test file's
// compiled location (internal/installation), so file() references and the
// license check resolve regardless of the test's working directory.
func repoPath(parts ...string) string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return filepath.Join(parts...)
	}
	root := filepath.Join(filepath.Dir(thisFile), "..", "..")
	return filepath.Join(append([]string{root}, parts...)...)
}

var gcpNameInvalid = regexp.MustCompile(`[^a-z0-9-]`)

// gcpSafeName coerces an acctest random name into a valid GCP resource name:
// lowercase, only [a-z0-9-], starting with a letter and ending with a letter
// or digit.
func gcpSafeName(s string) string {
	s = gcpNameInvalid.ReplaceAllString(strings.ToLower(s), "-")
	s = strings.Trim(s, "-")
	if s == "" || s[0] < 'a' || s[0] > 'z' {
		s = "yba-" + s
	}
	return s
}
