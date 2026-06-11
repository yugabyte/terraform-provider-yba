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

package installation

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// newInstallerData builds a *schema.ResourceData from the real resource
// schema so the tests exercise the same GetOk semantics the runtime sees.
func newInstallerData(t *testing.T, raw map[string]interface{}) *schema.ResourceData {
	t.Helper()
	return schema.TestResourceDataRaw(t, ResourceYBAInstaller().Schema, raw)
}

// TestSchemaIsValid guards the schema wiring (ConflictsWith / ExactlyOneOf
// references, types, etc.). A typo in an attribute name referenced by
// ConflictsWith would only surface at provider start otherwise.
func TestSchemaIsValid(t *testing.T) {
	if err := ResourceYBAInstaller().InternalValidate(nil, true); err != nil {
		t.Fatalf("installer schema failed InternalValidate: %v", err)
	}
}

// TestResolveInstallerInput covers the inline-vs-file precedence and the
// "neither set" case for the content/file pair.
func TestResolveInstallerInput(t *testing.T) {
	dir := t.TempDir()
	licPath := filepath.Join(dir, "license.lic")
	if err := os.WriteFile(licPath, []byte("from-file"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	tests := []struct {
		name    string
		raw     map[string]interface{}
		want    string
		wantErr bool
	}{
		{
			name: "inline content used when only inline is set",
			raw: map[string]interface{}{
				"yba_license": "inline-license",
			},
			want: "inline-license",
		},
		{
			// ConflictsWith normally prevents both being set, but the
			// resolver still documents inline-over-file precedence.
			name: "inline content takes precedence over file",
			raw: map[string]interface{}{
				"yba_license":      "inline-license",
				"yba_license_file": licPath,
			},
			want: "inline-license",
		},
		{
			name: "falls back to file when no inline content",
			raw: map[string]interface{}{
				"yba_license_file": licPath,
			},
			want: "from-file",
		},
		{
			name: "neither set returns empty without error",
			raw:  map[string]interface{}{},
			want: "",
		},
		{
			name: "missing file surfaces an error",
			raw: map[string]interface{}{
				"yba_license_file": filepath.Join(dir, "does-not-exist.lic"),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newInstallerData(t, tt.raw)
			got, err := resolveInstallerInput(d, licenseSpec)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got content %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// TestResolveSSHPrivateKey checks that either form satisfies the SSH key,
// and that supplying neither is an error.
func TestResolveSSHPrivateKey(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "id_rsa")
	if err := os.WriteFile(keyPath, []byte("key-from-file"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	tests := []struct {
		name    string
		raw     map[string]interface{}
		want    string
		wantErr bool
	}{
		{
			name: "inline key",
			raw:  map[string]interface{}{"ssh_private_key": "inline-key"},
			want: "inline-key",
		},
		{
			name: "file key",
			raw:  map[string]interface{}{"ssh_private_key_file_path": keyPath},
			want: "key-from-file",
		},
		{
			name:    "neither is an error",
			raw:     map[string]interface{}{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newInstallerData(t, tt.raw)
			got, err := resolveSSHPrivateKey(d)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// TestInstallerInputProvided checks the "is this input set in either form"
// predicate used by CustomizeDiff and the reconfigure flow.
func TestInstallerInputProvided(t *testing.T) {
	tests := []struct {
		name string
		raw  map[string]interface{}
		spec installerFileSpec
		want bool
	}{
		{
			name: "inline form provided",
			raw:  map[string]interface{}{"tls_certificate": "cert"},
			spec: tlsCertificateSpec,
			want: true,
		},
		{
			name: "file form provided",
			raw:  map[string]interface{}{"tls_certificate_file": "/tmp/c.crt"},
			spec: tlsCertificateSpec,
			want: true,
		},
		{
			name: "neither provided",
			raw:  map[string]interface{}{},
			spec: tlsCertificateSpec,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newInstallerData(t, tt.raw)
			if got := installerInputProvided(d, tt.spec); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

// fakeChangeDetector lets us drive installerInputHasChange without a real
// plan diff; *schema.ResourceData / *schema.ResourceDiff also satisfy the
// changeDetector interface in production.
type fakeChangeDetector struct {
	changed map[string]bool
}

func (f fakeChangeDetector) HasChange(key string) bool { return f.changed[key] }

// TestInstallerInputHasChange verifies a change on either the inline or the
// file attribute is reported, and that unrelated changes are ignored.
func TestInstallerInputHasChange(t *testing.T) {
	tests := []struct {
		name    string
		changed map[string]bool
		spec    installerFileSpec
		want    bool
	}{
		{
			name:    "inline attr changed",
			changed: map[string]bool{"application_settings": true},
			spec:    applicationSettingsSpec,
			want:    true,
		},
		{
			name:    "file attr changed",
			changed: map[string]bool{"application_settings_file": true},
			spec:    applicationSettingsSpec,
			want:    true,
		},
		{
			name:    "unrelated change ignored",
			changed: map[string]bool{"yba_version": true},
			spec:    applicationSettingsSpec,
			want:    false,
		},
		{
			name:    "nothing changed",
			changed: map[string]bool{},
			spec:    applicationSettingsSpec,
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := fakeChangeDetector{changed: tt.changed}
			if got := installerInputHasChange(d, tt.spec); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

// TestInstallerSpecSets guards the static spec groupings: the license must
// not leak into the reconfigure-only set (the bug the rebased commit fixed),
// and the install set must be the union of both with a fresh backing array.
func TestInstallerSpecSets(t *testing.T) {
	for _, spec := range reconfigurationYBAInstallerSpecs() {
		if spec.contentAttr == licenseSpec.contentAttr {
			t.Fatalf("license spec leaked into the reconfigure-only set")
		}
	}

	install := installationYBAInstallerSpecs()
	want := len(reconfigurationYBAInstallerSpecs()) + len(licenseYBAInstallerSpecs())
	if len(install) != want {
		t.Fatalf("install set has %d specs, want %d", len(install), want)
	}

	// Mutating the returned slice must not affect a subsequent call.
	install[0] = installerFileSpec{}
	if installationYBAInstallerSpecs()[0] == (installerFileSpec{}) {
		t.Fatalf("installationYBAInstallerSpecs returned a shared/mutable slice")
	}
}
