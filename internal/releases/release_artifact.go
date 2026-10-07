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

package releases

import (
	"context"
	"fmt"
	"strings"

	client "github.com/yugabyte/platform-go-client"

	"github.com/yugabyte/terraform-provider-yba/internal/api"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

const (
	platformLinux         = "LINUX"
	platformKubernetes    = "KUBERNETES"
	metadataStatusSuccess = "success"
	releasesRedesignKey   = "yb.releases.use_redesign"
)

var (
	releasePlatforms       = []string{platformLinux, platformKubernetes}
	releaseArchitectures   = []string{"x86_64", "aarch64"}
	releaseTypes           = []string{"LTS", "STS", "PREVIEW"}
	releaseStates          = []string{"ACTIVE", "DISABLED"}
	releaseDeploymentTypes = []string{"x86_64", "aarch64", "kubernetes"}
)

type artifactSpec struct {
	Platform      string
	Architecture  string
	LocalFile     string
	PackageURL    string
	PackageFileID string
	Sha256        string
}

// YBA allows one artifact per (platform, architecture).
func (a artifactSpec) key() string {
	return fmt.Sprintf("%s/%s", strings.ToUpper(a.Platform), strings.ToLower(a.Architecture))
}

func expandArtifactSpecs(raw []interface{}) []artifactSpec {
	specs := make([]artifactSpec, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		specs = append(specs, artifactSpec{
			Platform:      m["platform"].(string),
			Architecture:  m["architecture"].(string),
			LocalFile:     m["local_file"].(string),
			PackageURL:    m["package_url"].(string),
			PackageFileID: m["package_file_id"].(string),
			Sha256:        m["sha256"].(string),
		})
	}
	return specs
}

func flattenArtifactSpecs(specs []artifactSpec) []map[string]interface{} {
	flat := make([]map[string]interface{}, 0, len(specs))
	for _, spec := range specs {
		flat = append(flat, map[string]interface{}{
			"platform":        spec.Platform,
			"architecture":    spec.Architecture,
			"local_file":      spec.LocalFile,
			"package_url":     spec.PackageURL,
			"package_file_id": spec.PackageFileID,
			"sha256":          spec.Sha256,
		})
	}
	return flat
}

// Mirrors YBA's own artifact checks so a bad config fails at plan.
func validateArtifactSpecs(specs []artifactSpec) error {
	seen := map[string]int{}
	for i, spec := range specs {
		if (spec.LocalFile == "") == (spec.PackageURL == "") {
			return fmt.Errorf(
				"artifact %d: exactly one of local_file or package_url must be set", i)
		}
		switch spec.Platform {
		case platformLinux:
			if spec.Architecture == "" {
				return fmt.Errorf(
					"artifact %d: architecture is required when platform is %s", i, platformLinux)
			}
		case platformKubernetes:
			if spec.Architecture != "" {
				return fmt.Errorf(
					"artifact %d: architecture must not be set when platform is %s",
					i, platformKubernetes)
			}
		}
		key := spec.key()
		if first, dup := seen[key]; dup {
			return fmt.Errorf(
				"artifacts %d and %d both use platform/architecture %s; "+
					"YBA allows one artifact per pair per release", first, i, key)
		}
		seen[key] = i
	}
	return nil
}

// Any source change replaces the artifact: delete it, then add it again. YBA
// cannot change a source in place on every supported version (2025.2 and
// older match update artifacts by package_file_id or package_url, and no
// version clears an old source). Removals and replacements both delete an
// artifact, which YBA rejects on an in-use release.
func classifyArtifactChanges(oldSpecs, planSpecs []artifactSpec) (bool, map[string]bool) {
	planKeys := map[string]bool{}
	for _, spec := range planSpecs {
		planKeys[spec.key()] = true
	}
	removed := false
	for _, old := range oldSpecs {
		if !planKeys[old.key()] {
			removed = true
		}
	}
	oldByKey := map[string]artifactSpec{}
	for _, old := range oldSpecs {
		oldByKey[old.key()] = old
	}
	replaced := map[string]bool{}
	for _, spec := range planSpecs {
		if old, existed := oldByKey[spec.key()]; existed && !sameArtifactSource(old, spec) {
			replaced[spec.key()] = true
		}
	}
	return removed, replaced
}

// An imported artifact has no local_file, so declaring one uploads it.
func sameArtifactSource(old, plan artifactSpec) bool {
	if plan.LocalFile != "" {
		return plan.LocalFile == old.LocalFile && old.PackageFileID != ""
	}
	return plan.PackageURL == old.PackageURL && old.LocalFile == ""
}

func hasLinuxArtifact(specs []artifactSpec) bool {
	for _, spec := range specs {
		if spec.Platform == platformLinux {
			return true
		}
	}
	return false
}

// The generated model sends empty fields. Safe on create only: YBA stores
// package_file_id alone when set. See api.ReleaseUpdateArtifact.
func toClientArtifacts(specs []artifactSpec) []client.Artifact {
	artifacts := make([]client.Artifact, 0, len(specs))
	for _, spec := range specs {
		artifacts = append(artifacts, client.Artifact{
			Architecture:  spec.Architecture,
			PackageFileId: spec.PackageFileID,
			PackageUrl:    spec.PackageURL,
			Platform:      spec.Platform,
			Sha256:        spec.Sha256,
		})
	}
	return artifacts
}

func toReleaseUpdateArtifacts(specs []artifactSpec) []api.ReleaseUpdateArtifact {
	artifacts := make([]api.ReleaseUpdateArtifact, 0, len(specs))
	for _, spec := range specs {
		artifacts = append(artifacts, api.ReleaseUpdateArtifact{
			Platform:      spec.Platform,
			Architecture:  spec.Architecture,
			PackageFileID: spec.PackageFileID,
			PackageURL:    spec.PackageURL,
			Sha256:        spec.Sha256,
		})
	}
	return artifacts
}

func releaseAPICheck(ctx context.Context, c *api.APIClient) error {
	minVersions := utils.YBAMinimumVersion{
		Stable:  utils.YBANewReleaseAPIMinStableVersion,
		Preview: utils.YBANewReleaseAPIMinPreviewVersion,
	}
	version, err := c.AppVersion(ctx)
	if err != nil {
		return err
	}
	allowed, _, err := utils.MeetsMinimum(version, minVersions)
	if err != nil {
		return err
	}
	if !allowed {
		return fmt.Errorf(
			"the release management API requires YugabyteDB Anywhere version %s (stable) or "+
				"%s (preview) and above; current version: %s",
			utils.YBANewReleaseAPIMinStableVersion,
			utils.YBANewReleaseAPIMinPreviewVersion,
			version)
	}
	// With the flag off, /ybdb_release still accepts writes, but universes read
	// releases from the legacy store and cannot see them.
	enabled, err := utils.GetGlobalRuntimeConfigBool(
		ctx, c.YugawareClient, c.CustomerID, releasesRedesignKey)
	if err != nil {
		return err
	}
	if !enabled {
		return fmt.Errorf(
			"the release management API requires global runtime config %s=true (%s=false)",
			releasesRedesignKey, releasesRedesignKey)
	}
	return nil
}

func uploadArtifactFile(
	ctx context.Context,
	vc *api.VanillaClient,
	cUUID string,
	apiKey string,
	version string,
	index int,
	spec *artifactSpec,
) (*client.ResponseExtractMetadata, error) {
	if err := utils.FileExist(spec.LocalFile); err != nil {
		return nil, fmt.Errorf("artifact %d: %w", index, err)
	}
	fileUUID, err := vc.UploadReleaseFile(ctx, cUUID, apiKey, spec.LocalFile)
	if err != nil {
		return nil, fmt.Errorf("artifact %d (%s): %w", index, spec.LocalFile, err)
	}
	metadata, err := vc.GetUploadedReleaseMetadata(ctx, cUUID, apiKey, fileUUID)
	if err != nil {
		return nil, fmt.Errorf("artifact %d (%s): %w", index, spec.LocalFile, err)
	}
	if metadata.Status != metadataStatusSuccess {
		return nil, fmt.Errorf(
			"artifact %d (%s): metadata extraction returned status %q",
			index, spec.LocalFile, metadata.Status)
	}
	if metadata.Version != version {
		return nil, fmt.Errorf(
			"artifact %d (%s): tarball version %q does not match release version %q",
			index, spec.LocalFile, metadata.Version, version)
	}
	if !strings.EqualFold(metadata.Platform, spec.Platform) {
		return nil, fmt.Errorf(
			"artifact %d (%s): tarball platform %q does not match declared platform %q",
			index, spec.LocalFile, metadata.Platform, spec.Platform)
	}
	if !strings.EqualFold(metadata.Architecture, spec.Architecture) {
		return nil, fmt.Errorf(
			"artifact %d (%s): tarball architecture %q does not match declared architecture %q",
			index, spec.LocalFile, metadata.Architecture, spec.Architecture)
	}
	spec.PackageFileID = fileUUID
	spec.Sha256 = metadata.Sha256
	return metadata, nil
}
