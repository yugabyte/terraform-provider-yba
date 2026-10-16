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

package oci

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	client "github.com/yugabyte/platform-go-client"
)

func TestExpandHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	got, err := expandHome("~/keys/api.pem")
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join(home, "keys/api.pem") {
		t.Fatalf("expandHome(~) = %q", got)
	}
	got, err = expandHome("/etc/passwd")
	if err != nil {
		t.Fatal(err)
	}
	if got != "/etc/passwd" {
		t.Fatalf("absolute path changed to %q", got)
	}
}

func TestReadPrivateKeyFileAndTilde(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "api.pem")
	if err := os.WriteFile(path, []byte("pem-body"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := readPrivateKey("", path)
	if err != nil {
		t.Fatal(err)
	}
	if got != "pem-body" {
		t.Fatalf("read = %q", got)
	}

	missing, err := readPrivateKey("", filepath.Join(dir, "missing.pem"))
	if err == nil || !strings.Contains(err.Error(), "private_key_file_path") {
		t.Fatalf("missing file err = %v, key %q", err, missing)
	}
}

func TestReadPrivateKeyExactlyOne(t *testing.T) {
	if _, err := readPrivateKey("", ""); err == nil {
		t.Fatal("expected error when neither key input is set")
	}
	if _, err := readPrivateKey("inline", "/tmp/also.pem"); err == nil {
		t.Fatal("expected error when both key inputs are set")
	}
	got, err := readPrivateKey("inline-pem", "")
	if err != nil || got != "inline-pem" {
		t.Fatalf("inline = %q, %v", got, err)
	}
}

func TestSetOCIAuthForCreateInstancePrincipalOmitsAPIKey(t *testing.T) {
	info := &client.OCICloudInfo{}
	err := setOCIAuthForCreate(info, ociCloudInput{UseInstancePrincipal: true})
	if err != nil {
		t.Fatal(err)
	}
	if info.GetOciAuthType() != ociAuthInstancePrincipal {
		t.Fatalf("auth = %q", info.GetOciAuthType())
	}
	if info.OciTenancyId != nil || info.OciUserId != nil ||
		info.OciFingerprint != nil || info.OciPrivateKeyContent != nil {
		t.Fatalf("instance principal set an API-key field: %+v", info)
	}
}

func TestSetOCIAuthForCreateAPIKey(t *testing.T) {
	info := &client.OCICloudInfo{}
	in := ociCloudInput{
		TenancyID: "ocid1.tenancy.oc1..t", UserID: "ocid1.user.oc1..u",
		Fingerprint: "ab:cd", PrivateKeyContent: "pem",
	}
	if err := setOCIAuthForCreate(info, in); err != nil {
		t.Fatal(err)
	}
	if info.GetOciAuthType() != ociAuthAPIKey || info.GetOciTenancyId() != in.TenancyID ||
		info.GetOciUserId() != in.UserID || info.GetOciFingerprint() != in.Fingerprint ||
		info.GetOciPrivateKeyContent() != "pem" {
		t.Fatalf("api key payload = %+v", info)
	}
}

func TestSetOCIAuthForUpdateKeepsMaskedKey(t *testing.T) {
	info := &client.OCICloudInfo{}
	info.SetOciPrivateKeyContent("p***m")
	in := ociCloudInput{
		TenancyID: "ocid1.tenancy.oc1..t", UserID: "ocid1.user.oc1..u",
		Fingerprint: "ab:cd",
	}
	if err := setOCIAuthForUpdate(info, in, false); err != nil {
		t.Fatal(err)
	}
	if info.GetOciPrivateKeyContent() != "p***m" {
		t.Fatalf("stored key replaced with %q", info.GetOciPrivateKeyContent())
	}
}

func TestSetOCIAuthForUpdateClearsAPIKeyOnInstancePrincipal(t *testing.T) {
	info := &client.OCICloudInfo{}
	info.SetOciTenancyId("ocid1.tenancy.oc1..t")
	info.SetOciPrivateKeyContent("p***m")
	err := setOCIAuthForUpdate(info, ociCloudInput{UseInstancePrincipal: true}, false)
	if err != nil {
		t.Fatal(err)
	}
	if info.GetOciAuthType() != ociAuthInstancePrincipal ||
		info.GetOciTenancyId() != "" || info.GetOciPrivateKeyContent() != "" {
		t.Fatalf("fields not cleared: %+v", info)
	}
}

func TestValidateOCIAuthInput(t *testing.T) {
	ok := ociCloudInput{
		UseInstancePrincipal: false,
		TenancyID:            "t", UserID: "u", Fingerprint: "f", PrivateKeyContent: "pem",
	}
	if err := validateOCIAuthInput(ok); err != nil {
		t.Fatal(err)
	}
	if err := validateOCIAuthInput(ociCloudInput{UseInstancePrincipal: true}); err != nil {
		t.Fatal(err)
	}
	mixed := ociCloudInput{UseInstancePrincipal: true, TenancyID: "t"}
	if err := validateOCIAuthInput(mixed); err == nil ||
		!strings.Contains(err.Error(), "use_instance_principal is true") {
		t.Fatalf("mixed mode err = %v", err)
	}
	missing := ok
	missing.Fingerprint = ""
	if err := validateOCIAuthInput(missing); err == nil ||
		!strings.Contains(err.Error(), "fingerprint") {
		t.Fatalf("err = %v", err)
	}
	both := ok
	both.PrivateKeyFilePath = "/tmp/k.pem"
	if err := validateOCIAuthInput(both); err == nil {
		t.Fatal("expected exactly-one key error")
	}
}
