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

// writeSyntheticReleaseTarball builds a tiny yet valid YBDB release tarball:
// YBA's metadata extraction only needs a version_metadata.json entry with
// version_number, build_number, platform, and (for linux) architecture. YBA
// derives the release version as <version_number>-b<build_number>.
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

func releaseConfig(version, tag, x86Tarball string) string {
	return fmt.Sprintf(`
resource "yba_release" "test" {
  version     = %q
  release_tag = %q

  artifact {
    platform     = "LINUX"
    architecture = "x86_64"
    local_file   = %q
  }
}
`, version, tag, x86Tarball)
}

func releaseConfigMultiArch(version, tag, x86Tarball, armTarball string) string {
	return fmt.Sprintf(`
resource "yba_release" "test" {
  version     = %q
  release_tag = %q

  artifact {
    platform     = "LINUX"
    architecture = "x86_64"
    local_file   = %q
  }

  artifact {
    platform     = "LINUX"
    architecture = "aarch64"
    local_file   = %q
  }
}

data "yba_release_version" "aarch64" {
  version         = yba_release.test.version
  deployment_type = "aarch64"
}
`, version, tag, x86Tarball, armTarball)
}

func TestAccRelease(t *testing.T) {
	// Randomized version: YBA allows a single release per version, and the
	// 2.21 preview line stays safely older than any test YBA's own version.
	versionNumber := fmt.Sprintf("2.21.%d.%d",
		sdkacctest.RandIntRange(100, 999), sdkacctest.RandIntRange(1, 99))
	version := versionNumber + "-b1"
	dir := t.TempDir()
	x86Tarball := writeSyntheticReleaseTarball(t, dir, versionNumber, "1", "x86_64")
	armTarball := writeSyntheticReleaseTarball(t, dir, versionNumber, "1", "aarch64")
	resourceName := "yba_release.test"

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { acctest.TestAccPreCheck(t) },
		ProviderFactories: acctest.ProviderFactories,
		CheckDestroy:      testAccCheckReleaseDestroy,
		Steps: []resource.TestStep{
			{
				Config: releaseConfig(version, "acc-x86", x86Tarball),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckReleaseExists(resourceName, 1),
					resource.TestCheckResourceAttr(resourceName, "version", version),
					resource.TestCheckResourceAttr(resourceName, "release_type", "PREVIEW"),
					resource.TestCheckResourceAttr(resourceName, "state", "ACTIVE"),
					resource.TestCheckResourceAttrSet(
						resourceName, "artifact.0.package_file_id"),
					resource.TestCheckResourceAttrSet(resourceName, "artifact.0.sha256"),
				),
			},
			{
				// PLAT-20985: add the aarch64 artifact to the existing release
				// and find the version through the deployment_type filter.
				Config: releaseConfigMultiArch(version, "acc-multi", x86Tarball, armTarball),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckReleaseExists(resourceName, 2),
					resource.TestCheckResourceAttr(resourceName, "release_tag", "acc-multi"),
					resource.TestCheckResourceAttr(
						resourceName, "artifact.1.architecture", "aarch64"),
					resource.TestCheckResourceAttrSet(
						resourceName, "artifact.1.package_file_id"),
					resource.TestCheckResourceAttrSet(resourceName, "artifact.1.sha256"),
					resource.TestCheckResourceAttr(
						"data.yba_release_version.aarch64", "selected_version", version),
				),
			},
			{
				// local_file and sha256 never reach YBA's GET response, and
				// the artifact block order after import follows YBA, not the
				// config, so artifact attributes are not verifiable.
				ResourceName:            resourceName,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"artifact"},
			},
		},
	})
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
		if rs.Type != "yba_release" {
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
