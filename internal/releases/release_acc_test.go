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

package releases_test

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	sdkacctest "github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"

	"github.com/yugabyte/terraform-provider-yba/internal/acctest"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// YBA's metadata extraction reads only version_metadata.json and derives the
// version as <version_number>-b<build_number>.
func writeSyntheticReleaseTarball(
	t *testing.T, dir, versionNumber, buildNumber, arch string) string {
	t.Helper()
	metadata := fmt.Sprintf(`{
		"version_number": %q,
		"build_number": %q,
		"platform": "linux",
		"architecture": %q,
		"release_type": "PREVIEW"
	}`, versionNumber, buildNumber, arch)

	version := fmt.Sprintf("%s-b%s", versionNumber, buildNumber)
	path := filepath.Join(dir, fmt.Sprintf("yugabyte-%s-linux-%s.tar.gz", version, arch))
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create tarball: %v", err)
	}
	defer func() { _ = f.Close() }()
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	entry := fmt.Sprintf("yugabyte-%s/version_metadata.json", version)
	if err := tw.WriteHeader(&tar.Header{
		Name: entry,
		Mode: 0600,
		Size: int64(len(metadata)),
	}); err != nil {
		t.Fatalf("write tar header: %v", err)
	}
	if _, err := tw.Write([]byte(metadata)); err != nil {
		t.Fatalf("write tar entry: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar writer: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip writer: %v", err)
	}
	return path
}

// releaseConfig keeps the uploaded x86_64 tarball at index 0 in every step,
// because ignore_changes matches blocks by position. arm is the source line of
// a LINUX aarch64 block in front of the KUBERNETES block, or "" for none. YBA
// lists an artifact that an update adds after the earlier ones, so with arm
// set the config order differs from YBA's.
func releaseConfig(version, tag, x86Tarball, arm string) string {
	armBlock := ""
	if arm != "" {
		armBlock = fmt.Sprintf(`
  artifact {
    platform     = "LINUX"
    architecture = "aarch64"
    %s
  }
`, arm)
	}
	return fmt.Sprintf(`
resource "yba_ybdb_release" "test" {
  version       = %[1]q
  release_tag   = %[2]q
  release_notes = <<-EOT
    Acceptance test release: héllo ✓
    Second line.
  EOT

  artifact {
    platform     = "LINUX"
    architecture = "x86_64"
    local_file   = %[3]q
  }
%[4]s
  artifact {
    platform    = "KUBERNETES"
    package_url = "https://Charts.Example.COM/yugabyte/yugabyte-%[1]s.tgz?v=1"
  }

  # YBA does not return local_file: the GET lists only the path of the file on
  # the YBA host (ReleasesController.releaseToResponseRelease), so an import
  # cannot read it back.
  lifecycle {
    ignore_changes = [artifact[0].local_file]
  }
}
`, version, tag, x86Tarball, armBlock)
}

const releaseARMLookup = `
data "yba_release_version" "aarch64" {
  version         = yba_ybdb_release.test.version
  deployment_type = "aarch64"
}
`

func TestAccRelease(t *testing.T) {
	// YBA allows one release per version and rejects versions newer than itself.
	versionNumber := fmt.Sprintf("2.21.%d.%d",
		sdkacctest.RandIntRange(100, 999), sdkacctest.RandIntRange(1, 99))
	version := versionNumber + "-b1"
	dir := t.TempDir()
	x86Tarball := writeSyntheticReleaseTarball(t, dir, versionNumber, "1", "x86_64")
	armTarball := writeSyntheticReleaseTarball(t, dir, versionNumber, "1", "aarch64")
	resourceName := "yba_ybdb_release.test"
	var fixtureVersion string
	armURL := fmt.Sprintf(
		`package_url = "https://Downloads.Example.COM/yugabyte-%s-linux-aarch64.tar.gz?v=1"`,
		version)
	multiArch := releaseConfig(version, "Acc Multi-Arch", x86Tarball, armURL)

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { acctest.TestAccPreCheck(t) },
		ProviderFactories: acctest.ProviderFactories,
		CheckDestroy:      testAccCheckReleaseDestroy,
		Steps: append(append([]resource.TestStep{
			{
				Config: releaseConfig(version, "Acc X86", x86Tarball, ""),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckReleaseExists(resourceName, 2),
					resource.TestCheckResourceAttr(resourceName, "version", version),
					resource.TestCheckResourceAttr(resourceName, "release_type", "PREVIEW"),
					resource.TestCheckResourceAttr(resourceName, "state", "ACTIVE"),
					resource.TestCheckResourceAttrSet(
						resourceName, "artifact.0.package_file_id"),
					resource.TestCheckResourceAttrSet(resourceName, "artifact.0.sha256"),
				),
			},
			{
				// YBA lists the added aarch64 artifact last, after KUBERNETES.
				Config: multiArch,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckReleaseExists(resourceName, 3),
					resource.TestCheckResourceAttr(
						resourceName, "release_tag", "Acc Multi-Arch"),
					resource.TestCheckResourceAttr(
						resourceName, "artifact.1.architecture", "aarch64"),
				),
			},
		}, acctest.ImportPlanSteps(t, resourceName, multiArch, acctest.ImportPlanOptions{
			// YBA returns neither local_file nor sha256 (see releaseConfig;
			// ResponseRelease.Artifact has no sha256).
			VerifyIgnore: []string{"artifact.0.local_file", "artifact.0.sha256"},
		})...),
			resource.TestStep{
				// Replacing the aarch64 URL with a tarball uploads it on update.
				Config: releaseConfig(version, "Acc Multi-Arch", x86Tarball,
					fmt.Sprintf("local_file   = %q", armTarball)) + releaseARMLookup,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckReleaseExists(resourceName, 3),
					resource.TestCheckResourceAttrSet(
						resourceName, "artifact.1.package_file_id"),
					resource.TestCheckResourceAttrSet(resourceName, "artifact.1.sha256"),
					resource.TestCheckResourceAttr(
						"data.yba_release_version.aarch64", "selected_version", version),
				),
			},
			resource.TestStep{
				// A release registered outside Terraform. The step does not
				// persist the import, so the test never destroys the release. Its
				// config drops the aarch64 lookup, which matches nothing here.
				Config:       releaseConfig(version, "Acc X86", x86Tarball, ""),
				ResourceName: resourceName,
				ImportState:  true,
				ImportStateIdFunc: func(*terraform.State) (string, error) {
					uuid, v, err := fixtureRelease()
					fixtureVersion = v
					return uuid, err
				},
				ImportStateCheck: func(states []*terraform.InstanceState) error {
					if len(states) != 1 {
						return fmt.Errorf("imported %d resources, want 1", len(states))
					}
					a := states[0].Attributes
					if a["version"] != fixtureVersion || a["artifact.#"] == "0" ||
						a["artifact.0.local_file"] != "" ||
						a["artifact.0.package_file_id"]+a["artifact.0.package_url"] == "" {
						return fmt.Errorf("imported release %s reads back as %v", fixtureVersion, a)
					}
					return nil
				},
			}),
	})
}

// fixtureRelease returns the UUID and version of the fixture YBA's release of
// its own version, which exists before any test runs.
func fixtureRelease() (string, string, error) {
	c := acctest.APIClient
	ctx := context.Background()
	appVersion, err := c.AppVersion(ctx)
	if err != nil {
		return "", "", err
	}
	releases, response, err := c.YugawareClient.NewReleaseManagementAPI.ListNewReleases(
		ctx, c.CustomerID).Execute()
	if err != nil {
		return "", "", utils.ErrorFromHTTPResponse(response, err, utils.TestEntity,
			"Releases", "List")
	}
	for _, r := range releases {
		if r.Version == appVersion {
			return r.ReleaseUuid, r.Version, nil
		}
	}
	return "", "", fmt.Errorf("no release for YBA version %s", appVersion)
}

func testAccCheckReleaseExists(n string, wantArtifacts int) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("release %q not found in state", n)
		}
		if rs.Primary.ID == "" {
			return fmt.Errorf("release %q has no id", n)
		}
		c := acctest.APIClient
		r, response, err := c.YugawareClient.NewReleaseManagementAPI.GetNewRelease(
			context.Background(), c.CustomerID, rs.Primary.ID).Execute()
		if err != nil {
			return utils.ErrorFromHTTPResponse(response, err, utils.TestEntity,
				rs.Primary.ID, "Get Release")
		}
		if len(r.Artifacts) != wantArtifacts {
			return fmt.Errorf("release %s has %d artifacts, want %d",
				rs.Primary.ID, len(r.Artifacts), wantArtifacts)
		}
		return nil
	}
}

func testAccCheckReleaseDestroy(s *terraform.State) error {
	c := acctest.APIClient
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "yba_ybdb_release" {
			continue
		}
		_, response, err := c.YugawareClient.NewReleaseManagementAPI.GetNewRelease(
			context.Background(), c.CustomerID, rs.Primary.ID).Execute()
		if err == nil {
			return fmt.Errorf("release %s still exists", rs.Primary.ID)
		}
		if !utils.IsReleaseNotFound(response, err) {
			return utils.ErrorFromHTTPResponse(response, err, utils.TestEntity,
				rs.Primary.ID, "Destroy Check")
		}
	}
	return nil
}
