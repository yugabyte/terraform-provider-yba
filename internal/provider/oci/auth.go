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

// Package oci provides the Terraform resource for an Oracle Cloud
// Infrastructure provider in YugabyteDB Anywhere.
package oci

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	client "github.com/yugabyte/platform-go-client"

	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

const (
	ociAuthAPIKey            = "API_KEY"
	ociAuthInstancePrincipal = "INSTANCE_PRINCIPAL"
)

// ociCloudInput is the auth and identity values read from configuration.
type ociCloudInput struct {
	CompartmentID        string
	DefaultRegion        string
	HostedZoneID         string
	UseInstancePrincipal bool
	TenancyID            string
	UserID               string
	Fingerprint          string
	PrivateKeyContent    string
	PrivateKeyFilePath   string
}

func apiKeyFieldsSet(in ociCloudInput) bool {
	return in.TenancyID != "" || in.UserID != "" || in.Fingerprint != "" ||
		in.PrivateKeyContent != "" || in.PrivateKeyFilePath != ""
}

// expandHome expands a leading "~" or "~/" to the user's home directory.
// Other paths, including "~user", are returned unchanged.
func expandHome(path string) (string, error) {
	if path == "" || !strings.HasPrefix(path, "~") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("private_key_file_path: cannot expand ~: %w", err)
	}
	if path == "~" {
		return home, nil
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, path[2:]), nil
	}
	return path, nil
}

// readPrivateKey returns inline content, or the file at filePath after
// expandHome. Exactly one of the two inputs must be set.
func readPrivateKey(content, filePath string) (string, error) {
	switch {
	case content != "" && filePath != "":
		return "", fmt.Errorf(
			"private_key_content and private_key_file_path: set exactly one")
	case content != "":
		return content, nil
	case filePath == "":
		return "", fmt.Errorf(
			"private_key_content or private_key_file_path must be set")
	}
	expanded, err := expandHome(filePath)
	if err != nil {
		return "", err
	}
	if err = utils.FileExist(expanded); err != nil {
		return "", fmt.Errorf("private_key_file_path: %w", err)
	}
	data, err := os.ReadFile(expanded)
	if err != nil {
		return "", fmt.Errorf("private_key_file_path: %w", err)
	}
	return string(data), nil
}

// validateOCIAuthInput checks API-key completeness. Instance principal
// carries no API-key fields. The file is not opened here.
//
// The instance-principal check uses the bool value. An explicit false is
// API-key auth and is valid next to the API-key fields.
func validateOCIAuthInput(in ociCloudInput) error {
	if in.UseInstancePrincipal {
		if apiKeyFieldsSet(in) {
			return fmt.Errorf(
				"tenancy_id, user_id, fingerprint, private_key_content, and " +
					"private_key_file_path must be omitted when use_instance_principal is true")
		}
		return nil
	}
	if in.TenancyID == "" || in.UserID == "" || in.Fingerprint == "" {
		return fmt.Errorf(
			"tenancy_id, user_id, and fingerprint are required when " +
				"use_instance_principal is false")
	}
	if (in.PrivateKeyContent == "") == (in.PrivateKeyFilePath == "") {
		return fmt.Errorf(
			"private_key_content and private_key_file_path: set exactly one")
	}
	return nil
}

// setOCIAuthForCreate writes auth onto a new provider. Instance principal
// leaves the API-key pointers nil so they are omitted from the request.
func setOCIAuthForCreate(info *client.OCICloudInfo, in ociCloudInput) error {
	if err := validateOCIAuthInput(in); err != nil {
		return err
	}
	if in.UseInstancePrincipal {
		info.SetOciAuthType(ociAuthInstancePrincipal)
		return nil
	}
	key, err := readPrivateKey(in.PrivateKeyContent, in.PrivateKeyFilePath)
	if err != nil {
		return err
	}
	info.SetOciAuthType(ociAuthAPIKey)
	info.SetOciTenancyId(in.TenancyID)
	info.SetOciUserId(in.UserID)
	info.SetOciFingerprint(in.Fingerprint)
	info.SetOciPrivateKeyContent(key)
	return nil
}

// setOCIAuthForUpdate writes auth onto an edit. Switching to instance
// principal clears the API-key fields. When keyChanged is false and no
// private_key_file_path is set, the key already on info stays: YBA restores
// its stored key when the incoming value contains '*'. A set path is read
// and stored even when keyChanged is false.
func setOCIAuthForUpdate(info *client.OCICloudInfo, in ociCloudInput, keyChanged bool) error {
	if in.UseInstancePrincipal {
		if apiKeyFieldsSet(in) {
			return fmt.Errorf(
				"tenancy_id, user_id, fingerprint, private_key_content, and " +
					"private_key_file_path must be omitted when use_instance_principal is true")
		}
		info.SetOciAuthType(ociAuthInstancePrincipal)
		info.SetOciTenancyId("")
		info.SetOciUserId("")
		info.SetOciFingerprint("")
		info.SetOciPrivateKeyContent("")
		return nil
	}
	if err := validateOCIAuthInput(in); err != nil {
		return err
	}
	info.SetOciAuthType(ociAuthAPIKey)
	info.SetOciTenancyId(in.TenancyID)
	info.SetOciUserId(in.UserID)
	info.SetOciFingerprint(in.Fingerprint)
	// A path has no content diff. Re-read it on every auth update so a new
	// fingerprint is not stored with the previous PEM. Content is re-sent
	// only when keyChanged is true; otherwise YBA still holds the key and
	// restores it when the request contains '*'.
	if !keyChanged && in.PrivateKeyFilePath == "" {
		return nil
	}
	key, err := readPrivateKey(in.PrivateKeyContent, in.PrivateKeyFilePath)
	if err != nil {
		return err
	}
	info.SetOciPrivateKeyContent(key)
	return nil
}

// storeableOCIPrivateKey reports a PEM from the by-UUID provider GET.
// The list route masks the key as the first character, '*', and the last
// character. A masked value must not be written into state.
func storeableOCIPrivateKey(key string) (string, bool) {
	if key == "" || strings.Contains(key, "*") || !strings.Contains(key, "PRIVATE KEY") {
		return "", false
	}
	return key, true
}
