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

func TestGetYBAInstallerPackageString(t *testing.T) {
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
			folder, bundle, v := getYBAInstallerPackageString(tt.version, tt.os, tt.arch)
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

func TestYbaCtlCommands(t *testing.T) {
	tests := []struct {
		name    string
		version string
		hasEnv  bool
	}{
		{"GA version 2024.x", "2024.1.0.0-b129", false},
		{"GA version 2025.x", "2025.2.2.2-b11", false},
		{"Pre-release 2.31", "2.31.0.0-b14", true},
		{"Pre-release 2.25", "2.25.0.0-b300", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test getAddLicenseCommand
			license := getAddLicenseCommand(tt.version, "test-folder")
			if tt.hasEnv && !strings.Contains(license, "YBA_MODE=dev") {
				t.Errorf("getAddLicenseCommand(%q) should contain YBA_MODE=dev", tt.version)
			}
			if !tt.hasEnv && strings.Contains(license, "YBA_MODE=dev") {
				t.Errorf("getAddLicenseCommand(%q) should not contain YBA_MODE=dev", tt.version)
			}

			// Test getInstallCommands
			install := getInstallCommands(tt.version, "linux", "x86_64", false, nil)
			lastCmd := install[len(install)-1]
			if tt.hasEnv && !strings.Contains(lastCmd, "YBA_MODE=dev") {
				t.Errorf("getInstallCommands(%q) should contain YBA_MODE=dev", tt.version)
			}

			// Test getReconfigureCommands
			reconf := getReconfigureCommands(tt.version)
			lastCmd = reconf[len(reconf)-1]
			if tt.hasEnv && !strings.Contains(lastCmd, "YBA_MODE=dev") {
				t.Errorf("getReconfigureCommands(%q) should contain YBA_MODE=dev", tt.version)
			}
		})
	}
}

func TestGetYBAInstallerBundle(t *testing.T) {
	tests := []struct {
		name               string
		version            string
		os                 string
		arch               string
		expectedFolder     string
		expectedCurlPrefix string
	}{
		{
			name:               "GA version 2024.x uses downloads.yugabyte.com with linux",
			version:            "2024.1.0.0-b129",
			os:                 "linux",
			arch:               "x86_64",
			expectedFolder:     "yba_installer_full-2024.1.0.0-b129",
			expectedCurlPrefix: "curl -O https://downloads.yugabyte.com/releases/2024.1.0.0/yba_installer_full-2024.1.0.0-b129-linux-x86_64",
		},
		{
			name:               "GA version 2025.x uses downloads.yugabyte.com with linux",
			version:            "2025.2.2.2-b11",
			os:                 "linux",
			arch:               "x86_64",
			expectedFolder:     "yba_installer_full-2025.2.2.2-b11",
			expectedCurlPrefix: "curl -O https://downloads.yugabyte.com/releases/2025.2.2.2/yba_installer_full-2025.2.2.2-b11-linux-x86_64",
		},
		{
			name:               "Pre-release version uses releases.yugabyte.com with centos",
			version:            "2.25.0.0-b300",
			os:                 "linux",
			arch:               "x86_64",
			expectedFolder:     "yba_installer_full-2.25.0.0-b300",
			expectedCurlPrefix: "curl -O https://releases.yugabyte.com/2.25.0.0-b300/yba_installer_full-2.25.0.0-b300-centos-x86_64",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			folder, commands := getYBAInstallerBundle(tt.version, tt.os, tt.arch)
			if folder != tt.expectedFolder {
				t.Errorf("folder = %q, expected %q", folder, tt.expectedFolder)
			}
			if len(commands) != 2 {
				t.Fatalf("expected 2 commands (curl + tar), got %d: %v", len(commands), commands)
			}
			if !strings.HasPrefix(commands[0], tt.expectedCurlPrefix) {
				t.Errorf("curl command = %q, expected prefix %q", commands[0], tt.expectedCurlPrefix)
			}
			if !strings.HasPrefix(commands[1], "tar -xf ") {
				t.Errorf("second command should be tar extraction, got %q", commands[1])
			}
		})
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

func TestGetUpgradeCommandsSkipPreflight(t *testing.T) {
	skip := []string{"diskAvailability", "memoryAvailability"}
	cmds := getUpgradeCommands("2024.1.0.0", "linux", "x86_64", &skip)
	last := cmds[len(cmds)-1]
	if !strings.Contains(last, "-s diskAvailability,memoryAvailability") {
		t.Errorf("upgrade command should contain joined skip flags, got %q", last)
	}
}

// newInstallerResourceData builds a *schema.ResourceData using the live
// yba_installer schema and a map of attribute values. It is the
// standard pattern in the Terraform SDK for unit-testing schema
// helpers without spinning up a full provider.
func newInstallerResourceData(t *testing.T, raw map[string]interface{}) *schema.ResourceData {
	t.Helper()
	res := ResourceYBAInstaller()
	d := schema.TestResourceDataRaw(t, res.Schema, raw)
	return d
}

func TestResolveInstallerInputContent(t *testing.T) {
	d := newInstallerResourceData(t, map[string]interface{}{
		"ssh_host_ip":          "1.2.3.4",
		"ssh_user":             "user",
		"ssh_private_key":      "ssh-key-content",
		"yba_license":          "license-content",
		"yba_version":          "2024.1.0.0-b1",
		"tls_certificate":      "cert-content",
		"tls_key":              "key-content",
		"application_settings": "settings: yes\n",
	})

	cases := []struct {
		spec     installerFileSpec
		expected string
	}{
		{sshPrivateKeySpec, "ssh-key-content"},
		{licenseSpec, "license-content"},
		{tlsCertificateSpec, "cert-content"},
		{tlsKeySpec, "key-content"},
		{applicationSettingsSpec, "settings: yes\n"},
	}
	for _, tc := range cases {
		got, err := resolveInstallerInput(d, tc.spec)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", tc.spec.contentAttr, err)
		}
		if got != tc.expected {
			t.Errorf("%s: got %q, expected %q", tc.spec.contentAttr, got, tc.expected)
		}
	}
}

func TestResolveInstallerInputFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "license.lic")
	if err := os.WriteFile(path, []byte("from-file"), 0o600); err != nil {
		t.Fatalf("write tmp file: %v", err)
	}

	d := newInstallerResourceData(t, map[string]interface{}{
		"ssh_host_ip":               "1.2.3.4",
		"ssh_user":                  "user",
		"ssh_private_key_file_path": path,
		"yba_license_file":          path,
		"yba_version":               "2024.1.0.0-b1",
	})

	got, err := resolveInstallerInput(d, licenseSpec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "from-file" {
		t.Errorf("expected file contents, got %q", got)
	}

	got, err = resolveInstallerInput(d, sshPrivateKeySpec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "from-file" {
		t.Errorf("expected file contents from ssh path, got %q", got)
	}
}

func TestResolveInstallerInputUnset(t *testing.T) {
	d := newInstallerResourceData(t, map[string]interface{}{
		"ssh_host_ip":     "1.2.3.4",
		"ssh_user":        "user",
		"ssh_private_key": "k",
		"yba_license":     "l",
		"yba_version":     "2024.1.0.0-b1",
	})
	got, err := resolveInstallerInput(d, tlsCertificateSpec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("expected empty string for unset spec, got %q", got)
	}
}

func TestInstallerInputProvided(t *testing.T) {
	d := newInstallerResourceData(t, map[string]interface{}{
		"ssh_host_ip":     "1.2.3.4",
		"ssh_user":        "user",
		"ssh_private_key": "key",
		"yba_license":     "lic",
		"yba_version":     "2024.1.0.0-b1",
	})
	if !installerInputProvided(d, sshPrivateKeySpec) {
		t.Error("expected ssh_private_key to be reported as provided")
	}
	if !installerInputProvided(d, licenseSpec) {
		t.Error("expected yba_license to be reported as provided")
	}
	if installerInputProvided(d, tlsCertificateSpec) {
		t.Error("expected tls_certificate to be reported as not provided")
	}
}

func TestResolveSSHPrivateKeyRequiresInput(t *testing.T) {
	// Schema validation prevents this in practice (ExactlyOneOf), but
	// the helper itself should still surface a clear error if called
	// against an empty resource (e.g. during legacy state migration).
	res := ResourceYBAInstaller()
	d := schema.TestResourceDataRaw(t, res.Schema, map[string]interface{}{})
	if _, err := resolveSSHPrivateKey(d); err == nil {
		t.Fatal("expected error when neither ssh_private_key nor file path is set")
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
