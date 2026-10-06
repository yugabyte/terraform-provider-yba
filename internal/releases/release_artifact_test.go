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

// Every source change replaces the artifact (delete, then add): YBA 2025.2 and
// older cannot change a source in place.
func TestClassifyArtifactChanges(t *testing.T) {
	// Old specs come from state, where an upload has a package_file_id.
	x86Uploaded := artifactSpec{
		Platform: "LINUX", Architecture: "x86_64", LocalFile: "/tmp/x.tar.gz",
		PackageFileID: "file-1",
	}
	x86Imported := artifactSpec{Platform: "LINUX", Architecture: "x86_64", PackageFileID: "file-1"}
	x86File := artifactSpec{Platform: "LINUX", Architecture: "x86_64", LocalFile: "/tmp/x.tar.gz"}
	x86FileMoved := artifactSpec{
		Platform: "LINUX", Architecture: "x86_64", LocalFile: "/tmp/other.tar.gz",
	}
	armFile := artifactSpec{Platform: "LINUX", Architecture: "aarch64", LocalFile: "/tmp/a.tar.gz"}
	x86URL := artifactSpec{
		Platform: "LINUX", Architecture: "x86_64", PackageURL: "https://example.com/x.tar.gz",
	}
	x86URLMoved := artifactSpec{
		Platform: "LINUX", Architecture: "x86_64", PackageURL: "https://example.com/y.tar.gz",
	}
	chart := artifactSpec{Platform: "KUBERNETES", PackageURL: "https://example.com/helm.tgz"}
	chartCached := chart
	chartCached.PackageFileID = "chart-1"

	cases := []struct {
		name         string
		old          []artifactSpec
		plan         []artifactSpec
		wantRemoved  bool
		wantReplaced []string
	}{
		{name: "no change", old: []artifactSpec{x86Uploaded}, plan: []artifactSpec{x86File}},
		{
			name: "cached chart is unchanged",
			old:  []artifactSpec{chartCached}, plan: []artifactSpec{chart},
		},
		{
			name: "aarch64 added",
			old:  []artifactSpec{x86Uploaded}, plan: []artifactSpec{x86File, armFile},
		},
		{
			name: "artifact removed",
			old:  []artifactSpec{x86Uploaded, armFile}, plan: []artifactSpec{x86File},
			wantRemoved: true,
		},
		{
			name: "file path change",
			old:  []artifactSpec{x86Uploaded}, plan: []artifactSpec{x86FileMoved},
			wantReplaced: []string{x86File.key()},
		},
		{
			name: "url change",
			old:  []artifactSpec{x86URL}, plan: []artifactSpec{x86URLMoved},
			wantReplaced: []string{x86URL.key()},
		},
		{
			name: "file to url",
			old:  []artifactSpec{x86Uploaded}, plan: []artifactSpec{x86URL},
			wantReplaced: []string{x86URL.key()},
		},
		{
			name: "url to file",
			old:  []artifactSpec{x86URL}, plan: []artifactSpec{x86File},
			wantReplaced: []string{x86File.key()},
		},
		{
			name: "local_file declared after import",
			old:  []artifactSpec{x86Imported}, plan: []artifactSpec{x86File},
			wantReplaced: []string{x86File.key()},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			removed, replaced := classifyArtifactChanges(tc.old, tc.plan)
			if removed != tc.wantRemoved {
				t.Errorf("removed = %v, want %v", removed, tc.wantRemoved)
			}
			if len(replaced) != len(tc.wantReplaced) {
				t.Fatalf("replaced = %v, want keys %v", replaced, tc.wantReplaced)
			}
			for _, key := range tc.wantReplaced {
				if !replaced[key] {
					t.Errorf("expected key %s replaced, got %v", key, replaced)
				}
			}
		})
	}
}
