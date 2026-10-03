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
)

// Allowed values shared by the schema validators and the spec validation, so
// they cannot drift apart.
var (
	releasePlatforms     = []string{platformLinux, platformKubernetes}
	releaseArchitectures = []string{"x86_64", "aarch64"}
	releaseTypes         = []string{"LTS", "STS", "PREVIEW"}
	releaseStates        = []string{"ACTIVE", "DISABLED"}
	// Architecture values accepted by the yba_release_version data source's
	// deployment_type filter (YBA's ?deployment_type= query parameter).
	releaseDeploymentTypes = []string{"x86_64", "aarch64", "kubernetes"}
)

// artifactSpec is the flattened form of one `artifact` block.
type artifactSpec struct {
	Platform      string
	Architecture  string
	LocalFile     string
	PackageURL    string
	PackageFileID string
	Sha256        string
}

// key identifies the artifact within its release: YBA allows at most one
// artifact per (platform, architecture) pair and matches update requests to
// existing artifacts by this pair.
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

// validateArtifactSpecs enforces the invariants YBA's /ybdb_release endpoints
// reject: exactly one source per artifact, at most one artifact per
// (platform, architecture) pair, an architecture on every LINUX artifact and
// none on KUBERNETES artifacts.
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

// classifyArtifactChanges compares the prior and planned artifact sets.
// removed reports whether any (platform, architecture) key disappears, and
// flipped holds the keys whose source switches between local_file and
// package_url. Both classes delete an artifact server-side (flips are
// delete-then-recreate: YBA's matched update cannot clear the previous source
// field, and a leftover package URL takes precedence over the uploaded file
// when provisioning universes), which YBA rejects on in-use releases.
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
	flipped := map[string]bool{}
	for _, spec := range planSpecs {
		old, existed := oldByKey[spec.key()]
		if existed &&
			((old.LocalFile != "" && spec.PackageURL != "") ||
				(old.PackageURL != "" && spec.LocalFile != "")) {
			flipped[spec.key()] = true
		}
	}
	return removed, flipped
}

func hasLinuxArtifact(specs []artifactSpec) bool {
	for _, spec := range specs {
		if spec.Platform == platformLinux {
			return true
		}
	}
	return false
}

// toClientArtifacts builds the artifact list for the create request. The
// generated model always serializes every field; YBA's create handler picks
// package_file_id over package_url when both are present, and empty strings
// coerce to null server-side, so this is safe for create (unlike update — see
// api.ReleaseUpdateArtifact).
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

// newReleaseAPIVersionCheck gates operations on the /ybdb_release (new
// release management) endpoints. Mirrors yba-cli's minimum versions for the
// same endpoints.
func newReleaseAPIVersionCheck(ctx context.Context, c *client.APIClient) error {
	minVersions := utils.YBAMinimumVersion{
		Stable:  utils.YBANewReleaseAPIMinStableVersion,
		Preview: utils.YBANewReleaseAPIMinPreviewVersion,
	}
	allowed, version, err := utils.CheckValidYBAVersion(ctx, c, minVersions)
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
	return nil
}

// uploadArtifactFile streams the spec's local_file to YBA, reads the metadata
// YBA extracted from it, cross-validates that metadata against the declared
// version/platform/architecture, and fills in PackageFileID and Sha256.
// Returns the metadata so callers can infer release_type/release date.
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
