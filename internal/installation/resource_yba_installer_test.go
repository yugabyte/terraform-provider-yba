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
	"strings"
	"testing"
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
	// Expected order: curl, tar, license add, mv settings.yml, install
	if len(cmds) != 5 {
		t.Fatalf("expected 5 commands when config=true, got %d: %v", len(cmds), cmds)
	}
	if !strings.Contains(cmds[3], "mv /tmp/settings.yml /opt/yba-ctl/yba-ctl.yml") {
		t.Errorf("expected settings move at index 3, got %q", cmds[3])
	}
	if !strings.Contains(cmds[4], "yba-ctl install -f") {
		t.Errorf("expected install command at index 4, got %q", cmds[4])
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
			if len(cmds) != 3 {
				t.Fatalf("expected 3 commands (clean, rm yugabyte, rm tmp), got %d: %v",
					len(cmds), cmds)
			}
			if !strings.Contains(cmds[0], "/opt/yba-ctl/yba-ctl clean") {
				t.Errorf("expected clean command first, got %q", cmds[0])
			}
			hasEnv := strings.Contains(cmds[0], "YBA_MODE=dev")
			if hasEnv != tt.expectedEnv {
				t.Errorf("YBA_MODE=dev presence = %v, expected %v in %q",
					hasEnv, tt.expectedEnv, cmds[0])
			}
			if cmds[1] != "sudo rm -rf /opt/yugabyte" {
				t.Errorf("expected rm -rf /opt/yugabyte, got %q", cmds[1])
			}
			if !strings.Contains(cmds[2], "/tmp/server.crt") ||
				!strings.Contains(cmds[2], "/tmp/server.key") ||
				!strings.Contains(cmds[2], "/tmp/license.lic") ||
				!strings.Contains(cmds[2], "/tmp/settings.yml") {
				t.Errorf("expected rm of all tmp files, got %q", cmds[2])
			}
		})
	}
}
