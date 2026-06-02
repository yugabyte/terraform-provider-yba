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
	"strings"
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

func TestIsGAVersion(t *testing.T) {
	tests := []struct {
		name     string
		version  string
		expected bool
	}{
		{
			name:     "GA version 2024.1.0.0",
			version:  "2024.1.0.0",
			expected: true,
		},
		{
			name:     "GA version with build number",
			version:  "2024.1.0.0-b129",
			expected: true,
		},
		{
			name:     "GA version 2024.2.3.0",
			version:  "2024.2.3.0",
			expected: true,
		},
		{
			name:     "GA version 2025.1.0.0",
			version:  "2025.1.0.0-b50",
			expected: true,
		},
		{
			name:     "Pre-release version 2.25.0.0",
			version:  "2.25.0.0",
			expected: false,
		},
		{
			name:     "Pre-release version with build number",
			version:  "2.25.0.0-b300",
			expected: false,
		},
		{
			name:     "Pre-release version 2.31.0.0",
			version:  "2.31.0.0-b23",
			expected: false,
		},
		{
			name:     "Pre-release version 2.20.0.0",
			version:  "2.20.0.0-b1",
			expected: false,
		},
		{
			name:     "Empty string is not GA",
			version:  "",
			expected: false,
		},
		{
			name:     "Year without trailing dot is not GA",
			version:  "2024",
			expected: false,
		},
		{
			name:     "Pre-2000 year is not GA",
			version:  "1999.1.0.0",
			expected: false,
		},
		{
			name:     "Year must be at the start",
			version:  "v2024.1.0.0",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsGAVersion(tt.version)
			if result != tt.expected {
				t.Errorf("IsGAVersion(%q) = %v, expected %v", tt.version, result, tt.expected)
			}
		})
	}
}

func TestGetYBAInstallerPackageNames(t *testing.T) {
	tests := []struct {
		name           string
		version        string
		os             string
		arch           string
		expectedFolder string
		expectedBundle string
		expectedV      string
	}{
		{
			name:           "GA version 2024.x uses linux",
			version:        "2024.1.0.0-b129",
			os:             "linux",
			arch:           "x86_64",
			expectedFolder: "yba_installer_full-2024.1.0.0-b129",
			expectedBundle: "yba_installer_full-2024.1.0.0-b129-linux-x86_64",
			expectedV:      "2024.1.0.0",
		},
		{
			name:           "GA version 2025.x uses linux",
			version:        "2025.2.2.2-b11",
			os:             "linux",
			arch:           "x86_64",
			expectedFolder: "yba_installer_full-2025.2.2.2-b11",
			expectedBundle: "yba_installer_full-2025.2.2.2-b11-linux-x86_64",
			expectedV:      "2025.2.2.2",
		},
		{
			name:           "Pre-release version converts linux to centos",
			version:        "2.25.0.0-b300",
			os:             "linux",
			arch:           "x86_64",
			expectedFolder: "yba_installer_full-2.25.0.0-b300",
			expectedBundle: "yba_installer_full-2.25.0.0-b300-centos-x86_64",
			expectedV:      "2.25.0.0",
		},
		{
			name:           "Pre-release version with explicit centos",
			version:        "2.25.0.0-b300",
			os:             "centos",
			arch:           "x86_64",
			expectedFolder: "yba_installer_full-2.25.0.0-b300",
			expectedBundle: "yba_installer_full-2.25.0.0-b300-centos-x86_64",
			expectedV:      "2.25.0.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			folder, bundle, v := getYBAInstallerPackageNames(tt.version, tt.os, tt.arch)
			if folder != tt.expectedFolder {
				t.Errorf("folder = %q, expected %q", folder, tt.expectedFolder)
			}
			if bundle != tt.expectedBundle {
				t.Errorf("bundle = %q, expected %q", bundle, tt.expectedBundle)
			}
			if v != tt.expectedV {
				t.Errorf("v = %q, expected %q", v, tt.expectedV)
			}
		})
	}
}

func TestGetYBAInstallerDownloadURL(t *testing.T) {
	tests := []struct {
		name     string
		version  string
		os       string
		arch     string
		expected string
	}{
		{
			name:     "GA version 2024.x uses downloads.yugabyte.com with build-stripped path and linux",
			version:  "2024.1.0.0-b129",
			os:       "linux",
			arch:     "x86_64",
			expected: "https://downloads.yugabyte.com/releases/2024.1.0.0/yba_installer_full-2024.1.0.0-b129-linux-x86_64.tar.gz",
		},
		{
			name:     "GA version 2025.x uses downloads.yugabyte.com with build-stripped path and linux",
			version:  "2025.2.2.2-b11",
			os:       "linux",
			arch:     "x86_64",
			expected: "https://downloads.yugabyte.com/releases/2025.2.2.2/yba_installer_full-2025.2.2.2-b11-linux-x86_64.tar.gz",
		},
		{
			name:     "GA version without build number keeps full version in path",
			version:  "2024.1.0.0",
			os:       "linux",
			arch:     "x86_64",
			expected: "https://downloads.yugabyte.com/releases/2024.1.0.0/yba_installer_full-2024.1.0.0-linux-x86_64.tar.gz",
		},
		{
			name:     "Pre-release version uses releases.yugabyte.com with full version path and centos",
			version:  "2.25.0.0-b300",
			os:       "linux",
			arch:     "x86_64",
			expected: "https://releases.yugabyte.com/2.25.0.0-b300/yba_installer_full-2.25.0.0-b300-centos-x86_64.tar.gz",
		},
		{
			name:     "GA version propagates aarch64 architecture into the URL",
			version:  "2025.2.2.2-b11",
			os:       "linux",
			arch:     "aarch64",
			expected: "https://downloads.yugabyte.com/releases/2025.2.2.2/yba_installer_full-2025.2.2.2-b11-linux-aarch64.tar.gz",
		},
		{
			name:     "Pre-release version propagates aarch64 architecture into the URL",
			version:  "2.25.0.0-b300",
			os:       "linux",
			arch:     "aarch64",
			expected: "https://releases.yugabyte.com/2.25.0.0-b300/yba_installer_full-2.25.0.0-b300-centos-aarch64.tar.gz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getYBAInstallerDownloadURL(tt.version, tt.os, tt.arch)
			if got != tt.expected {
				t.Errorf("getYBAInstallerDownloadURL(%q, %q, %q) = %q, expected %q",
					tt.version, tt.os, tt.arch, got, tt.expected)
			}
		})
	}
}

func TestGetBundleDownloadCommands(t *testing.T) {
	// getBundleDownloadCommands wraps the download URL in a curl command and appends
	// the matching tar extraction; the URL routing itself is covered by
	// TestGetYBAInstallerDownloadURL.
	folder, commands := getBundleDownloadCommands("2.25.0.0-b300", "linux", "x86_64")
	if folder != "yba_installer_full-2.25.0.0-b300" {
		t.Errorf("folder = %q", folder)
	}
	if len(commands) != 2 {
		t.Fatalf("expected 2 commands (curl + tar), got %d: %v", len(commands), commands)
	}
	wantCurl := "curl -O " + getYBAInstallerDownloadURL("2.25.0.0-b300", "linux", "x86_64")
	if commands[0] != wantCurl {
		t.Errorf("curl command = %q, expected %q", commands[0], wantCurl)
	}
	if commands[1] != "tar -xf yba_installer_full-2.25.0.0-b300-centos-x86_64.tar.gz" {
		t.Errorf("tar command = %q", commands[1])
	}
}

func TestGetInstallCommandsWithConfig(t *testing.T) {
	cmds := getInstallCommands("2024.1.0.0-b129", "linux", "x86_64", true, nil)
	// Expected order: curl, tar, license add, mkdir /opt/yba-ctl,
	// mv settings.yml, combined install (bash if-else).
	if len(cmds) != 6 {
		t.Fatalf("expected 6 commands when config=true, got %d: %v", len(cmds), cmds)
	}
	if !strings.Contains(cmds[3], "mkdir -p /opt/yba-ctl") {
		t.Errorf("expected /opt/yba-ctl mkdir at index 3, got %q", cmds[3])
	}
	if !strings.Contains(cmds[4], "mv /tmp/settings.yml /opt/yba-ctl/yba-ctl.yml") {
		t.Errorf("expected settings move at index 4, got %q", cmds[4])
	}
	last := cmds[len(cmds)-1]
	if !strings.Contains(last, "yba-ctl install -f") {
		t.Errorf("expected combined install command, got %q", last)
	}
	if !strings.Contains(last, "--without-data") {
		t.Errorf("expected combined install to include --without-data branch, got %q", last)
	}
}

// TestGetInstallCommandsWithoutConfig verifies that when no
// application_settings input is supplied, the function does not stage
// /opt/yba-ctl/yba-ctl.yml. The combined install must still be the last
// command so callers that grep cmds[len-1] for the install flags keep
// working.
func TestGetInstallCommandsWithoutConfig(t *testing.T) {
	cmds := getInstallCommands("2024.1.0.0-b129", "linux", "x86_64", false, nil)
	if len(cmds) != 4 {
		t.Fatalf("expected 4 commands when config=false, got %d: %v", len(cmds), cmds)
	}
	for i, cmd := range cmds {
		if strings.Contains(cmd, "mkdir -p /opt/yba-ctl") {
			t.Errorf("did not expect /opt/yba-ctl mkdir when config=false, found at %d: %q",
				i, cmd)
		}
		if strings.Contains(cmd, "mv /tmp/settings.yml") {
			t.Errorf("did not expect settings move when config=false, found at %d: %q",
				i, cmd)
		}
	}
	last := cmds[len(cmds)-1]
	if !strings.Contains(last, "yba-ctl install -f") {
		t.Errorf("expected install -f in last command, got %q", last)
	}
}

// TestGetInstallCommandsDataDirBranch locks in the bash structure that
// picks between fresh install and --without-data install. A regression
// here would silently re-initialise storage on a host that booted with
// a pre-populated /opt/yugabyte/data disk attached.
func TestGetInstallCommandsDataDirBranch(t *testing.T) {
	skip := []string{"diskAvailability"}
	cmds := getInstallCommands("2024.1.0.0-b129", "linux", "x86_64", true, &skip)
	last := cmds[len(cmds)-1]

	wantSubstrings := []string{
		`if [ -d /opt/yugabyte/data ]`,
		`[ -n "$(sudo ls -A /opt/yugabyte/data 2>/dev/null)" ]`,
		`yba-ctl install -f --without-data -s diskAvailability`,
		`/opt/yba-ctl/yba-ctl start`,
		`else sudo ./yba_installer_full-2024.1.0.0-b129/yba-ctl install -f -s diskAvailability;`,
		`fi`,
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(last, want) {
			t.Errorf("combined install missing %q, got %q", want, last)
		}
	}
}

// TestGetInstallCommandsDataDirBranchPreRelease confirms YBA_MODE=dev
// propagates into both branches and the yba-ctl start fallback.
func TestGetInstallCommandsDataDirBranchPreRelease(t *testing.T) {
	cmds := getInstallCommands("2.25.0.0-b300", "linux", "x86_64", false, nil)
	last := cmds[len(cmds)-1]

	wantSubstrings := []string{
		`sudo YBA_MODE=dev ./yba_installer_full-2.25.0.0-b300/yba-ctl install -f --without-data`,
		`sudo YBA_MODE=dev /opt/yba-ctl/yba-ctl start`,
		`else sudo YBA_MODE=dev ./yba_installer_full-2.25.0.0-b300/yba-ctl install -f;`,
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(last, want) {
			t.Errorf("combined install missing %q, got %q", want, last)
		}
	}
}

func TestGetInstallCommandsSkipPreflight(t *testing.T) {
	tests := []struct {
		name     string
		skip     []string
		expected string
	}{
		{"single check", []string{"diskAvailability"}, "-s diskAvailability"},
		{"multiple checks joined with comma", []string{"a", "b", "c"}, "-s a,b,c"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmds := getInstallCommands("2024.1.0.0", "linux", "x86_64", false, &tt.skip)
			last := cmds[len(cmds)-1]
			if !strings.Contains(last, tt.expected) {
				t.Errorf("install command = %q, expected to contain %q", last, tt.expected)
			}
		})
	}
}

func TestGetInstallCommandsNilOrEmptySkipPreflight(t *testing.T) {
	empty := []string{}
	for _, skip := range []*[]string{nil, &empty} {
		cmds := getInstallCommands("2024.1.0.0", "linux", "x86_64", false, skip)
		last := cmds[len(cmds)-1]
		if strings.Contains(last, "-s ") {
			t.Errorf("install command should not contain -s flag, got %q", last)
		}
	}
}

func TestGetReconfigureCommands(t *testing.T) {
	tests := []struct {
		name        string
		version     string
		expectedEnv bool
	}{
		{"GA version", "2024.1.0.0-b129", false},
		{"Pre-release version", "2.25.0.0-b300", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmds := getReconfigureCommands(tt.version)
			if len(cmds) != 2 {
				t.Fatalf("expected 2 commands, got %d: %v", len(cmds), cmds)
			}
			if !strings.Contains(cmds[0], "mv /tmp/settings.yml /opt/yba-ctl/yba-ctl.yml") {
				t.Errorf("expected settings move first, got %q", cmds[0])
			}
			if !strings.Contains(cmds[1], "/opt/yba-ctl/yba-ctl reconfigure -f") {
				t.Errorf("expected reconfigure second, got %q", cmds[1])
			}
			hasEnv := strings.Contains(cmds[1], "YBA_MODE=dev")
			if hasEnv != tt.expectedEnv {
				t.Errorf("YBA_MODE=dev presence = %v, expected %v in %q",
					hasEnv, tt.expectedEnv, cmds[1])
			}
		})
	}
}

func TestGetUpgradeCommands(t *testing.T) {
	tests := []struct {
		name        string
		version     string
		os          string
		expectedEnv bool
		bundleOS    string
	}{
		{"GA version uses linux bundle, no YBA_MODE", "2024.1.0.0-b129", "linux", false, "linux"},
		{"Pre-release uses centos bundle, with YBA_MODE", "2.25.0.0-b300", "linux", true, "centos"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmds := getUpgradeCommands(tt.version, tt.os, "x86_64", nil)
			if len(cmds) != 3 {
				t.Fatalf("expected 3 commands (curl, tar, upgrade), got %d: %v", len(cmds), cmds)
			}
			if !strings.Contains(cmds[0], "-"+tt.bundleOS+"-x86_64") {
				t.Errorf("curl command should reference %q bundle, got %q", tt.bundleOS, cmds[0])
			}
			upgrade := cmds[2]
			if !strings.Contains(upgrade, "yba-ctl upgrade -f") {
				t.Errorf("expected upgrade command, got %q", upgrade)
			}
			hasEnv := strings.Contains(upgrade, "YBA_MODE=dev")
			if hasEnv != tt.expectedEnv {
				t.Errorf("YBA_MODE=dev presence = %v, expected %v in %q",
					hasEnv, tt.expectedEnv, upgrade)
			}
		})
	}
}

func TestYbaCtlSudo(t *testing.T) {
	if got := ybaCtlSudo("2024.1.0.0-b129"); got != "sudo" {
		t.Errorf("ybaCtlSudo(GA) = %q, expected %q", got, "sudo")
	}
	if got := ybaCtlSudo("2.25.0.0-b300"); got != "sudo YBA_MODE=dev" {
		t.Errorf("ybaCtlSudo(pre-release) = %q, expected %q", got, "sudo YBA_MODE=dev")
	}
}

func TestGetAddLicenseCommandFormat(t *testing.T) {
	if got := getAddLicenseCommand("2024.1.0.0-b129", "myfolder"); got !=
		"sudo ./myfolder/yba-ctl license add -l /tmp/license.lic" {
		t.Errorf("GA license command = %q", got)
	}
	if got := getAddLicenseCommand("2.25.0.0-b300", "myfolder"); got !=
		"sudo YBA_MODE=dev ./myfolder/yba-ctl license add -l /tmp/license.lic" {
		t.Errorf("pre-release license command = %q", got)
	}
}

func TestGetInstallCommandsPreRelease(t *testing.T) {
	// Pre-release install pulls the centos bundle from releases.yugabyte.com and
	// runs yba-ctl in dev mode. Expected order: curl, tar, license add, install.
	cmds := getInstallCommands("2.31.0.0-b14", "linux", "x86_64", false, nil)
	if len(cmds) != 4 {
		t.Fatalf("expected 4 commands when config=false, got %d: %v", len(cmds), cmds)
	}
	if !strings.HasPrefix(cmds[0],
		"curl -O https://releases.yugabyte.com/2.31.0.0-b14/"+
			"yba_installer_full-2.31.0.0-b14-centos-x86_64") {
		t.Errorf("curl command = %q", cmds[0])
	}
	if !strings.Contains(cmds[2], "YBA_MODE=dev") ||
		!strings.Contains(cmds[2], "yba-ctl license add") {
		t.Errorf("license command = %q", cmds[2])
	}
	if !strings.Contains(cmds[3], "YBA_MODE=dev ./") ||
		!strings.Contains(cmds[3], "yba-ctl install -f") {
		t.Errorf("install command = %q", cmds[3])
	}
}

func TestGetUpgradeCommandsSkipPreflight(t *testing.T) {
	skip := []string{"diskAvailability", "memoryAvailability"}
	cmds := getUpgradeCommands("2024.1.0.0", "linux", "x86_64", &skip)
	last := cmds[len(cmds)-1]
	if !strings.Contains(last, "-s diskAvailability,memoryAvailability") {
		t.Errorf("upgrade command should contain joined skip flags, got %q", last)
	}
}

func TestGetDeleteCommands(t *testing.T) {
	tests := []struct {
		name        string
		version     string
		expectedEnv bool
	}{
		{"GA version", "2024.1.0.0-b129", false},
		{"Pre-release version", "2.25.0.0-b300", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmds := getDeleteCommands(tt.version)
			if len(cmds) != 2 {
				t.Fatalf("expected 2 commands (clean, rm tmp), got %d: %v",
					len(cmds), cmds)
			}
			if !strings.Contains(cmds[0], "/opt/yba-ctl/yba-ctl clean") {
				t.Errorf("expected clean command first, got %q", cmds[0])
			}
			// `--all` would wipe /opt/yugabyte/data which must outlive
			// the resource when a separate data disk is mounted there.
			if strings.Contains(cmds[0], "--all") {
				t.Errorf("clean must not pass --all, got %q", cmds[0])
			}
			hasEnv := strings.Contains(cmds[0], "YBA_MODE=dev")
			if hasEnv != tt.expectedEnv {
				t.Errorf("YBA_MODE=dev presence = %v, expected %v in %q",
					hasEnv, tt.expectedEnv, cmds[0])
			}
			if !strings.Contains(cmds[1], "/tmp/server.crt") ||
				!strings.Contains(cmds[1], "/tmp/server.key") ||
				!strings.Contains(cmds[1], "/tmp/license.lic") ||
				!strings.Contains(cmds[1], "/tmp/settings.yml") {
				t.Errorf("expected rm of all tmp files, got %q", cmds[1])
			}
			// Regression: an earlier version of this function ran
			// `sudo rm -rf /opt/yugabyte`, which would wipe the data
			// disk on every destroy/replace cycle.
			for i, cmd := range cmds {
				if strings.Contains(cmd, "rm -rf /opt/yugabyte") {
					t.Errorf("delete commands must not wipe /opt/yugabyte, found at %d: %q",
						i, cmd)
				}
			}
		})
	}
}
