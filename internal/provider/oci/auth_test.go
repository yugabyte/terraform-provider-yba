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
	"os"
	"path/filepath"
	"testing"
	"time"

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

	if missing, err := readPrivateKey("", filepath.Join(dir, "missing.pem")); err == nil {
		t.Fatalf("a missing file must fail, got key %q", missing)
	}

	t.Setenv("HOME", dir)
	if got, err := readPrivateKey("", "~/api.pem"); err != nil || got != "pem-body" {
		t.Fatalf("~/api.pem: read = %q, err = %v", got, err)
	}
	if got, _ := expandHome("~"); got != dir {
		t.Fatalf("~ = %q, want %q", got, dir)
	}
	if got, _ := expandHome("~user/k.pem"); got != "~user/k.pem" {
		t.Fatalf("~user must not expand, got %q", got)
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

// An imported provider has empty key fields in state, and ignore_changes keeps
// them empty in the plan. An auth update still sends the configured key.
func TestOCIUpdateUsesConfiguredKeyAfterImport(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "api.pem")
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

// The plan checks see the configured key arguments, not the key in state, and
// need the key file only when the apply reads it.
func TestOCIPlanKeyChecks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "api.pem")
	if err := os.WriteFile(path, []byte("pem"), 0o600); err != nil {
		t.Fatal(err)
	}
	attrs := map[string]string{
		"id": "prov", "name": "oci", "compartment_id": "c", "default_region": "r",
		"use_instance_principal": "false", "tenancy_id": "t", "user_id": "u",
		"fingerprint": "f", "regions.#": "1", "regions.0.code": "r",
		"regions.0.vcn_id": "v", "regions.0.zones.#": "1", "regions.0.zones.0.code": "r-AD-1",
		"regions.0.zones.0.subnet": "s", "yba_managed_image_bundles.#": "1",
		"yba_managed_image_bundles.0.arch": "x86_64",
	}
	config := func(key string, value string) *terraform.ResourceConfig {
		raw := map[string]interface{}{
			"name": "oci", "compartment_id": "c", "default_region": "r",
			"use_instance_principal": false, "tenancy_id": "t", "user_id": "u",
			"fingerprint": "f",
			"regions": []interface{}{map[string]interface{}{
				"code": "r", "vcn_id": "v",
				"zones": []interface{}{map[string]interface{}{"code": "r-AD-1", "subnet": "s"}},
			}},
			"yba_managed_image_bundles": []interface{}{map[string]interface{}{"arch": "x86_64"}},
		}
		if key != "" {
			raw[key] = value
		}
		return terraform.NewResourceConfigRaw(raw)
	}
	diff := func(stateKey, stateValue string, c *terraform.ResourceConfig) error {
		s := map[string]string{}
		for k, v := range attrs {
			s[k] = v
		}
		s[stateKey] = stateValue
		_, err := ResourceOCIProvider().Diff(context.Background(),
			&terraform.InstanceState{ID: "prov", Attributes: s}, c, nil)
		return err
	}
	if err := diff("private_key_content", "pem",
		config("private_key_file_path", path)); err != nil {
		t.Fatalf("content to path: %v", err)
	}
	if err := diff("private_key_file_path", path,
		config("private_key_content", "pem")); err != nil {
		t.Fatalf("path to content: %v", err)
	}
	if err := diff("private_key_content", "pem", config("", "")); err == nil {
		t.Fatal("a plan that clears the key must fail")
	}
	// Terraform sends the plan its configuration after ignore_changes, so an
	// imported key under ignore_changes arrives empty, as no key does. The plan
	// lets an existing provider through, also when another auth field changes;
	// the apply reads the configured key.
	if err := diff("private_key_content", "", config("", "")); err != nil {
		t.Fatalf("imported provider, no auth change: %v", err)
	}
	if err := diff("fingerprint", "old", config("", "")); err != nil {
		t.Fatalf("imported provider, fingerprint change: %v", err)
	}
	if err := diff("use_instance_principal", "true", config("", "")); err == nil {
		t.Fatal("a switch to API-key auth with no key must fail")
	}
	missing := filepath.Join(t.TempDir(), "missing.pem")
	if err := diff("private_key_file_path", missing,
		config("private_key_file_path", missing)); err != nil {
		t.Fatalf("a plan that leaves auth alone needs no key file: %v", err)
	}
	if err := diff("private_key_content", "pem",
		config("private_key_file_path", missing)); err == nil {
		t.Fatal("a plan that sends the key must find the file")
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
	if err := validateOCIAuthInput(mixed); err == nil {
		t.Fatal("instance principal with API-key fields must fail")
	}
	missing := ok
	missing.Fingerprint = ""
	if err := validateOCIAuthInput(missing); err == nil {
		t.Fatal("API-key auth without a fingerprint must fail")
	}
	both := ok
	both.PrivateKeyFilePath = "/tmp/k.pem"
	if err := validateOCIAuthInput(both); err == nil {
		t.Fatal("expected exactly-one key error")
	}
}

func TestSSHNameConflict(t *testing.T) {
	name := "my-key"
	existing := []client.AccessKey{{
		KeyInfo: client.KeyInfo{KeyPairName: &name},
	}}
	// YBA would store the rotated key as my-key-<timestamp>.
	if err := sshNameConflict(name, name, existing); err == nil {
		t.Fatal("a content rotation under a stored name must fail")
	}
	if err := sshNameConflict("", name, existing); err != nil {
		t.Fatalf("first apply after import: %v", err)
	}
	if err := sshNameConflict(name, "other-key", existing); err != nil {
		t.Fatalf("new name: %v", err)
	}
	// Custom key, then YBA-managed keys, then the old custom name again.
	managed := "yb-managed-key"
	older, newer := time.Now().Add(-time.Hour), time.Now()
	switched := []client.AccessKey{
		{KeyInfo: client.KeyInfo{KeyPairName: &name}, CreationDate: &older},
		{KeyInfo: client.KeyInfo{KeyPairName: &managed}, CreationDate: &newer},
	}
	if err := sshNameConflict("", name, switched); err == nil {
		t.Fatal("an older key name must fail, or YBA renames it silently")
	}
}
