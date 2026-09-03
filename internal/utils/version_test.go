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

import "testing"

// The two lines the provider must tell apart, plus the shapes YBA's own
// comparison accepts: a custom build without -bN and a downstream suffix after
// the second dash.
func TestIsVersionStable(t *testing.T) {
	cases := map[string]bool{
		"2024.2.0.0-b1":       true,
		"2026.1.2.0-b84":      true,
		"2026.1.2.0-b84-ybm3": true,
		"2.20.0.0-b1":         true,
		"2.31.0.0-b386":       false,
		"2.31.0.0-custom":     false,
		"2.31.0.0-b395-ybm7":  false,
		"2.29.0.0-b622":       false,
		"garbage":             false,
	}
	for v, want := range cases {
		if got := IsVersionStable(v); got != want {
			t.Errorf("IsVersionStable(%q) = %v, want %v", v, got, want)
		}
	}
}

func TestCompareYbVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"2.31.0.0-b386", "2.31.0.0-b385", 1},
		{"2.31.0.0-b164", "2.31.0.0-b386", -1},
		{"2.31.0.0-b386", "2.31.0.0-b386", 0},
		{"2026.1.2.0-b84", "2026.1.1.0-b91", 1},
		{"2026.2.0.0-b1", "2026.1.2.0-b84", 1},
		// A build without a numeric -bN compares equal within its release.
		{"2.31.0.0-custom", "2.31.0.0-b386", 0},
		{"2.29.0.0-custom", "2.31.0.0-b386", -1},
		// Everything after the second dash is ignored.
		{"2.31.0.0-b395-ybm7", "2.31.0.0-b386", 1},
		{"2.31.0.0-b100-ybm7", "2.31.0.0-b386", -1},
	}
	for _, c := range cases {
		got, err := CompareYbVersions(c.a, c.b)
		if err != nil {
			t.Errorf("CompareYbVersions(%q, %q): %v", c.a, c.b, err)
			continue
		}
		if got != c.want {
			t.Errorf("CompareYbVersions(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
	if _, err := CompareYbVersions("dev-build", "2.31.0.0-b1"); err == nil {
		t.Error("expected an error for an unparseable version")
	}
}

func TestIsExperimentalPatchVersion(t *testing.T) {
	cases := map[string]bool{
		"2.31.0.4263-b4": true, // YBM internal build
		"2.31.0.3183-b4": true,
		"2.31.0.1900":    true, // bare experimental branch name
		"2.31.0.2083":    true,
		"2.31.0.1000-b1": true, // boundary: the tooling reserves >999
		"2.31.0.999-b1":  false,
		"2.31.0.0-b386":  false,
		"2025.2.2.2-b11": false, // released line with a small revision
		"2026.1.2.0-b84": false,
		"2.31.0":         false,
		"garbage":        false,
	}
	for v, want := range cases {
		if got := IsExperimentalPatchVersion(v); got != want {
			t.Errorf("IsExperimentalPatchVersion(%q) = %v, want %v", v, got, want)
		}
	}
}

func TestMeetsMinimum(t *testing.T) {
	minimum := YBAMinimumVersion{Stable: "2026.1.2.0-b84", Preview: "2.31.0.0-b386"}
	cases := []struct {
		version     string
		want        bool
		wantApplied string
	}{
		{"2.31.0.0-b164", false, "2.31.0.0-b386"},
		{"2.31.0.0-b386", true, "2.31.0.0-b386"},
		{"2.31.0.0-b395", true, "2.31.0.0-b386"},
		{"2.33.0.0-b1", true, "2.31.0.0-b386"},
		{"2026.1.1.0-b91", false, "2026.1.2.0-b84"},
		{"2026.1.2.0-b84", true, "2026.1.2.0-b84"},
		{"2026.2.0.0-b1", true, "2026.1.2.0-b84"},
		// A stable build is never held to the preview bound, and vice versa:
		// 2026.1.2.0 is numerically far above 2.31 but on its own line.
		{"2026.1.1.0-b91", false, "2026.1.2.0-b84"},
		// Custom builds pass once their release is new enough.
		{"2.31.0.0-custom", true, "2.31.0.0-b386"},
		{"2.29.0.0-custom", false, "2.31.0.0-b386"},
		{"2026.1.2.0-custom", true, "2026.1.2.0-b84"},
		// Downstream suffixes keep the build number.
		{"2.31.0.0-b395-ybm7", true, "2.31.0.0-b386"},
		{"2.31.0.0-b100-ybm7", false, "2.31.0.0-b386"},
		{"2026.1.2.0-b90-ybm", true, "2026.1.2.0-b84"},
		// Experimental patch builds (fourth part >= 1000) are assumed to meet
		// every minimum: their base and cherry-picks are not derivable from the
		// string, so even an older-looking line passes.
		{"2.31.0.4263-b4", true, "2.31.0.0-b386"},
		{"2.29.0.9999-b1", true, "2.31.0.0-b386"},
		{"2.31.0.1900", true, "2.31.0.0-b386"},
	}
	for _, c := range cases {
		got, applied, err := MeetsMinimum(c.version, minimum)
		if err != nil {
			t.Errorf("MeetsMinimum(%q): %v", c.version, err)
			continue
		}
		if got != c.want || applied != c.wantApplied {
			t.Errorf("MeetsMinimum(%q) = (%v, %q), want (%v, %q)",
				c.version, got, applied, c.want, c.wantApplied)
		}
	}
	if _, _, err := MeetsMinimum("dev-build", minimum); err == nil {
		t.Error("expected an error for an unparseable version")
	}
}

func TestValidateProviderMinimumVersion(t *testing.T) {
	for _, v := range []string{"2024.2.0.0-b1", "2026.1.2.0-b84", "2.23.1.0-b1", "2.31.0.0-b164"} {
		if err := ValidateProviderMinimumVersion(v); err != nil {
			t.Errorf("%s should be supported: %v", v, err)
		}
	}
	for _, v := range []string{"2024.1.0.0-b129", "2.23.0.0-b416", "dev-build"} {
		if err := ValidateProviderMinimumVersion(v); err == nil {
			t.Errorf("%s should be rejected", v)
		}
	}
}
