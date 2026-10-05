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

// MeetsMinimum picks the bound from the build's release line and compares the
// way YBA does: a build without -bN equals any build of its release, text after
// the second dash is ignored, and an experimental patch build (fourth part
// >= 1000) meets every minimum.
func TestMeetsMinimum(t *testing.T) {
	minimum := YBAMinimumVersion{Stable: "2026.1.2.0-b84", Preview: "2.31.0.0-b386"}
	cases := []struct {
		version     string
		want        bool
		wantApplied string
	}{
		{"2.31.0.0-b385", false, "2.31.0.0-b386"},
		{"2.31.0.0-b386", true, "2.31.0.0-b386"},
		{"2026.1.1.0-b91", false, "2026.1.2.0-b84"},
		{"2026.1.2.0-b84", true, "2026.1.2.0-b84"},
		{"2.31.0.0-custom", true, "2.31.0.0-b386"},
		{"2.29.0.0-custom", false, "2.31.0.0-b386"},
		{"2.31.0.0-b395-ybm7", true, "2.31.0.0-b386"},
		{"2.31.0.0-b100-ybm7", false, "2.31.0.0-b386"},
		{"2.29.0.1000-b1", true, "2.31.0.0-b386"},
		{"2.29.0.999-b1", false, "2.31.0.0-b386"},
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

// Unlike a feature gate, the provider floor rejects a version it cannot parse.
func TestValidateProviderMinimumVersion(t *testing.T) {
	for _, v := range []string{"2024.2.0.0-b1", "2.23.1.0-b1"} {
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
