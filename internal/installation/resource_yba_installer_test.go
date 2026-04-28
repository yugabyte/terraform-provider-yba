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

func TestYbaCtlCommand(t *testing.T) {
	tests := []struct {
		name     string
		version  string
		path     string
		args     string
		expected string
	}{
		{
			name:     "GA version does not add YBA_MODE",
			version:  "2024.1.0.0-b129",
			path:     "./yba-ctl",
			args:     "install -f",
			expected: "sudo ./yba-ctl install -f",
		},
		{
			name:     "GA version 2025.x does not add YBA_MODE",
			version:  "2025.2.2.2-b11",
			path:     "./yba-ctl",
			args:     "install -f",
			expected: "sudo ./yba-ctl install -f",
		},
		{
			name:     "Pre-release version adds YBA_MODE=dev after sudo",
			version:  "2.31.0.0-b14",
			path:     "./yba-ctl",
			args:     "install -f",
			expected: "sudo YBA_MODE=dev ./yba-ctl install -f",
		},
		{
			name:     "Pre-release version 2.25 adds YBA_MODE=dev after sudo",
			version:  "2.25.0.0-b300",
			path:     "/opt/yba-ctl/yba-ctl",
			args:     "reconfigure -f",
			expected: "sudo YBA_MODE=dev /opt/yba-ctl/yba-ctl reconfigure -f",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ybaCtlCommand(tt.version, tt.path, tt.args)
			if result != tt.expected {
				t.Errorf("ybaCtlCommand(%q, %q, %q) = %q, expected %q",
					tt.version, tt.path, tt.args, result, tt.expected)
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
			if len(commands) < 1 {
				t.Fatalf("expected at least 1 command, got %d", len(commands))
			}
			curlCmd := commands[0]
			if len(curlCmd) < len(tt.expectedCurlPrefix) ||
				curlCmd[:len(tt.expectedCurlPrefix)] != tt.expectedCurlPrefix {
				t.Errorf("curl command = %q, expected prefix %q", curlCmd, tt.expectedCurlPrefix)
			}
		})
	}
}
