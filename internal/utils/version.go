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

package utils

import (
	"fmt"
	"regexp"
)

// ybmExperimentalVersionRegex matches YBM's internal builds of YBA
// ("2.31.0.2083"): four numeric parts, no -bN or other suffix, and a nonzero
// fourth part, which is YBM's internal build counter rather than a release
// revision. A released YBA always reports a -bN build, so this shape only
// comes from YBM's internally built YBAs.
var ybmExperimentalVersionRegex = regexp.MustCompile(`^\d+\.\d+\.\d+\.[1-9]\d*$`)

// IsYBMExperimentalVersion reports whether version is an internal YBM build
// of YBA. See ybmExperimentalVersionRegex for the shape.
func IsYBMExperimentalVersion(version string) bool {
	return ybmExperimentalVersionRegex.MatchString(version)
}

// MinimumFor returns the bound that applies to version: Stable when the build
// is on the stable line (2024.1.x, 2.20.x), Preview otherwise (2.31.x). This is
// the rule YBA itself applies in Util.compareYBVersions, so the provider and
// the server agree on which line a build belongs to.
func (m YBAMinimumVersion) MinimumFor(version string) string {
	if IsVersionStable(version) {
		return m.Stable
	}
	return m.Preview
}

// MeetsMinimum reports whether version is at least the minimum for its release
// line, and returns the minimum it was compared against. Comparison follows
// YBA's own rules (CompareYbVersions): the four numeric parts first, then the
// -bN build number. A build without a numeric -bN (a custom or local build)
// compares equal to any build of the same release, so it passes once the
// release itself is new enough. Anything after the second dash is ignored, so
// a downstream suffix on a YBA build ("2.31.0.0-b386-<tag>") keeps its build
// number. Returns an error when version cannot be parsed; callers decide
// whether that blocks (the provider floor) or is logged and let through (a
// feature gate, where the server's own error remains the backstop).
func MeetsMinimum(version string, minimum YBAMinimumVersion) (bool, string, error) {
	applied := minimum.MinimumFor(version)
	// A YBM internal build's counter has no fixed relation to the -bN builds
	// on the public lines and its branch point (with cherry-picks) is not
	// derivable from the string. Only Yugabyte-run tooling targets these
	// builds, so they are assumed to meet every minimum; the server's own
	// error is the backstop.
	if IsYBMExperimentalVersion(version) {
		return true, applied, nil
	}
	cmp, err := CompareYbVersions(version, applied)
	if err != nil {
		return false, applied, err
	}
	return cmp >= 0, applied, nil
}

// ValidateProviderMinimumVersion rejects a YBA build older than the provider's
// floor (YBATerraformProviderMinStableVersion / ...PreviewVersion). Matches
// yba-cli's IsCLISupported pattern in internal/client/client.go.
func ValidateProviderMinimumVersion(version string) error {
	floor := YBAMinimumVersion{
		Stable:  YBATerraformProviderMinStableVersion,
		Preview: YBATerraformProviderMinPreviewVersion,
	}
	ok, _, err := MeetsMinimum(version, floor)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf(
			"YugabyteDB Anywhere Terraform provider is not supported for YugabyteDB Anywhere "+
				"Host version %s. Please use a version greater than or equal to "+
				"Stable: %s, Preview: %s",
			version, floor.Stable, floor.Preview)
	}
	return nil
}
