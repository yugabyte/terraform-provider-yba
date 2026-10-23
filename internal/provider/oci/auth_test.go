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
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hashicorp/go-cty/cty"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	client "github.com/yugabyte/platform-go-client"
)

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

func TestSetOCIAuthForUpdateKeepsMaskedKey(t *testing.T) {
	info := &client.OCICloudInfo{}
	info.SetOciPrivateKeyContent("p***m")
	in := ociCloudInput{
		TenancyID: "ocid1.tenancy.oc1..t", UserID: "ocid1.user.oc1..u",
		Fingerprint: "ab:cd", PrivateKeyContent: "pem",
	}
	if err := setOCIAuthForUpdate(info, in, false); err != nil {
		t.Fatal(err)
	}
	if info.GetOciPrivateKeyContent() != "p***m" {
		t.Fatalf("stored key replaced with %q", info.GetOciPrivateKeyContent())
	}
}

func TestSetOCIAuthForUpdateRejectsEmptyIdentity(t *testing.T) {
	info := &client.OCICloudInfo{}
	info.SetOciPrivateKeyContent("p***m")
	err := setOCIAuthForUpdate(info, ociCloudInput{
		Fingerprint: "ab:cd", PrivateKeyContent: "pem",
	}, true)
	if err == nil {
		t.Fatal("empty tenancy and user were accepted")
	}
	if info.GetOciPrivateKeyContent() != "p***m" {
		t.Fatal("rejected update wrote the key")
	}
}

func TestSetOCIAuthForUpdateRereadsFileWhenPathUnchanged(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "api.pem")
	if err := os.WriteFile(path, []byte("new-pem\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	info := &client.OCICloudInfo{}
	info.SetOciPrivateKeyContent("p***m")
	err := setOCIAuthForUpdate(info, ociCloudInput{
		TenancyID: "t", UserID: "u", Fingerprint: "new:fp",
		PrivateKeyFilePath: path,
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if info.GetOciPrivateKeyContent() != "new-pem\n" || info.GetOciFingerprint() != "new:fp" {
		t.Fatalf("info = %+v", info)
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

func TestOCIContentKeptForFilePath(t *testing.T) {
	if !ociContentKeptForFilePath("pem", "", "/tmp/k.pem") {
		t.Fatal("file-path config must keep the PEM Read stored")
	}
	if ociContentKeptForFilePath("pem", "other", "/tmp/k.pem") {
		t.Fatal("a configured PEM must not be suppressed")
	}
	if ociContentKeptForFilePath("", "pem", "") {
		t.Fatal("empty state must not be suppressed")
	}
	if ociContentKeptForFilePath("pem", "", "") {
		t.Fatal("content removal without a path must not be suppressed")
	}
}

func TestSuppressOCIPrivateKeyFilePathMatchesBody(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "api.pem")
	pem := "-----BEGIN PRIVATE KEY-----\nYQ==\n-----END PRIVATE KEY-----\n"
	if err := os.WriteFile(path, []byte(pem), 0o600); err != nil {
		t.Fatal(err)
	}
	if !ociPathMatchesStoredPEM("", path, pem) {
		t.Fatal("matching file must suppress the path")
	}
	if ociPathMatchesStoredPEM("", path, pem+"x") {
		t.Fatal("different PEM must not suppress the path")
	}
	if ociPathMatchesStoredPEM(path, path, pem) {
		t.Fatal("a path already in state must not be suppressed")
	}
}

func TestReadClearOCIPrivateKey(t *testing.T) {
	pem := "-----BEGIN PRIVATE KEY-----\nYQ==\n-----END PRIVATE KEY-----\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/customers/cust/providers/prov" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"details":{"cloudInfo":{"oci":{"ociPrivateKeyContent":%q}}}}`, pem)
	}))
	defer srv.Close()
	cfg := client.NewConfiguration()
	cfg.Servers = client.ServerConfigurations{{URL: srv.URL}}
	cfg.HTTPClient = srv.Client()
	got, err := readClearOCIPrivateKey(
		context.Background(), client.NewAPIClient(cfg), "cust", "prov")
	if err != nil {
		t.Fatal(err)
	}
	if got != pem {
		t.Fatalf("key = %q", got)
	}
}

func TestOCIUpdateUsesConfiguredKeyWhenStateHoldsPEM(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "api.pem")
	pem := "-----BEGIN PRIVATE KEY-----\nYQ==\n-----END PRIVATE KEY-----\n"
	if err := os.WriteFile(path, []byte("new-pem\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	raw := cty.ObjectVal(map[string]cty.Value{
		"private_key_content":    cty.NullVal(cty.String),
		"private_key_file_path":  cty.StringVal(path),
		"tenancy_id":             cty.StringVal("t"),
		"user_id":                cty.StringVal("u"),
		"fingerprint":            cty.StringVal("new:fp"),
		"use_instance_principal": cty.False,
	})
	state := &terraform.InstanceState{
		ID: "prov",
		Attributes: map[string]string{
			"id":                     "prov",
			"private_key_content":    pem,
			"tenancy_id":             "t",
			"user_id":                "u",
			"fingerprint":            "new:fp",
			"use_instance_principal": "false",
		},
		RawConfig: raw,
	}
	in := ociCloudInputFromResource(ResourceOCIProvider().Data(state))
	if in.PrivateKeyContent != "" || in.PrivateKeyFilePath != path {
		t.Fatalf("config input = content %q path %q", in.PrivateKeyContent, in.PrivateKeyFilePath)
	}
	info := &client.OCICloudInfo{}
	info.SetOciPrivateKeyContent("p***m")
	if err := setOCIAuthForUpdate(info, in, false); err != nil {
		t.Fatal(err)
	}
	if info.GetOciPrivateKeyContent() != "new-pem\n" || info.GetOciFingerprint() != "new:fp" {
		t.Fatalf("info = %+v", info)
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

func TestStoreableOCIPrivateKey(t *testing.T) {
	pem := "-----BEGIN PRIVATE KEY-----\nYQ==\n-----END PRIVATE KEY-----"
	if got, ok := storeableOCIPrivateKey(pem); !ok || got != pem {
		t.Fatalf("pem ok=%v got=%q", ok, got)
	}
	if _, ok := storeableOCIPrivateKey("p***m"); ok {
		t.Fatal("masked key was stored")
	}
	if _, ok := storeableOCIPrivateKey(""); ok {
		t.Fatal("empty key was stored")
	}
}

func TestSSHNameConflictSkipsUnchangedName(t *testing.T) {
	name := "my-key"
	existing := []client.AccessKey{{
		KeyInfo: client.KeyInfo{KeyPairName: &name},
	}}
	if err := sshNameConflict(false, name, existing); err != nil {
		t.Fatalf("content rotation: %v", err)
	}
	if err := sshNameConflict(true, "other-key", existing); err != nil {
		t.Fatalf("new name: %v", err)
	}
	if err := sshNameConflict(true, name, existing); err == nil {
		t.Fatal("renaming onto an existing key must fail")
	}
}
