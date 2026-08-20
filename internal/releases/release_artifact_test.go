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
	"strings"
	"testing"
)

func TestValidateArtifactSpecs(t *testing.T) {
	cases := []struct {
		name    string
		specs   []artifactSpec
		wantErr string
	}{
		{
			name: "valid multi-arch set",
			specs: []artifactSpec{
				{Platform: "LINUX", Architecture: "x86_64", LocalFile: "/tmp/x86.tar.gz"},
				{Platform: "LINUX", Architecture: "aarch64", LocalFile: "/tmp/arm.tar.gz"},
				{Platform: "KUBERNETES", PackageURL: "https://example.com/helm.tgz"},
			},
		},
		{
			name: "both sources set",
			specs: []artifactSpec{{
				Platform: "LINUX", Architecture: "x86_64",
				LocalFile: "/tmp/x.tar.gz", PackageURL: "https://example.com/x.tar.gz",
			}},
			wantErr: "exactly one of local_file or package_url",
		},
		{
			name:    "no source set",
			specs:   []artifactSpec{{Platform: "LINUX", Architecture: "x86_64"}},
			wantErr: "exactly one of local_file or package_url",
		},
		{
			name:    "linux without architecture",
			specs:   []artifactSpec{{Platform: "LINUX", LocalFile: "/tmp/x.tar.gz"}},
			wantErr: "architecture is required",
		},
		{
			name: "kubernetes with architecture",
			specs: []artifactSpec{{
				Platform: "KUBERNETES", Architecture: "x86_64",
				PackageURL: "https://example.com/helm.tgz",
			}},
			wantErr: "architecture must not be set",
		},
		{
			name: "duplicate platform and architecture",
			specs: []artifactSpec{
				{Platform: "LINUX", Architecture: "x86_64", LocalFile: "/tmp/a.tar.gz"},
				{Platform: "LINUX", Architecture: "x86_64", PackageURL: "https://example.com/b"},
			},
			wantErr: "one artifact per pair",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateArtifactSpecs(tc.specs)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("expected valid specs, got %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestClassifyArtifactChanges(t *testing.T) {
	x86File := artifactSpec{Platform: "LINUX", Architecture: "x86_64", LocalFile: "/tmp/x.tar.gz"}
	armFile := artifactSpec{Platform: "LINUX", Architecture: "aarch64", LocalFile: "/tmp/a.tar.gz"}
	x86URL := artifactSpec{
		Platform: "LINUX", Architecture: "x86_64", PackageURL: "https://example.com/x.tar.gz",
	}
	x86FileMoved := artifactSpec{
		Platform: "LINUX", Architecture: "x86_64", LocalFile: "/tmp/other.tar.gz",
	}

	cases := []struct {
		name        string
		old         []artifactSpec
		plan        []artifactSpec
		wantRemoved bool
		wantFlipped []string
	}{
		{name: "no change", old: []artifactSpec{x86File}, plan: []artifactSpec{x86File}},
		{
			name: "aarch64 added",
			old:  []artifactSpec{x86File}, plan: []artifactSpec{x86File, armFile},
		},
		{
			name: "artifact removed",
			old:  []artifactSpec{x86File, armFile}, plan: []artifactSpec{x86File},
			wantRemoved: true,
		},
		{
			name: "file to url flip",
			old:  []artifactSpec{x86File}, plan: []artifactSpec{x86URL},
			wantFlipped: []string{x86URL.key()},
		},
		{
			name: "url to file flip",
			old:  []artifactSpec{x86URL}, plan: []artifactSpec{x86File},
			wantFlipped: []string{x86File.key()},
		},
		{
			name: "file path change is not a flip",
			old:  []artifactSpec{x86File}, plan: []artifactSpec{x86FileMoved},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			removed, flipped := classifyArtifactChanges(tc.old, tc.plan)
			if removed != tc.wantRemoved {
				t.Errorf("removed = %v, want %v", removed, tc.wantRemoved)
			}
			if len(flipped) != len(tc.wantFlipped) {
				t.Fatalf("flipped = %v, want keys %v", flipped, tc.wantFlipped)
			}
			for _, key := range tc.wantFlipped {
				if !flipped[key] {
					t.Errorf("expected key %s flipped, got %v", key, flipped)
				}
			}
		})
	}
}

func TestHasLinuxArtifact(t *testing.T) {
	if hasLinuxArtifact([]artifactSpec{{Platform: "KUBERNETES"}}) {
		t.Error("kubernetes-only set must not report a LINUX artifact")
	}
	if !hasLinuxArtifact([]artifactSpec{{Platform: "KUBERNETES"}, {Platform: "LINUX"}}) {
		t.Error("expected LINUX artifact to be found")
	}
}
