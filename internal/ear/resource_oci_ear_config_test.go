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

package ear

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"

	"github.com/yugabyte/terraform-provider-yba/internal/api"
)

func ociBase() map[string]interface{} {
	return map[string]interface{}{
		"name":           "kms",
		"compartment_id": "ocid1.compartment.oc1..c",
		"vault_id":       "ocid1.vault.oc1..v",
		"region":         "us-ashburn-1",
		"key_name":       "yb-master",
	}
}

func ociAPIKeyConfig() map[string]interface{} {
	raw := ociBase()
	raw["tenancy_id"] = "ocid1.tenancy.oc1..t"
	raw["user_id"] = "ocid1.user.oc1..u"
	raw["fingerprint"] = "aa:bb:cc:dd:ee"
	raw["private_key_content"] = "-----BEGIN PRIVATE KEY-----\nold\n-----END PRIVATE KEY-----"
	return raw
}

// The two auth modes map onto YBA's ociAuthType. API-key auth sends the four
// signing-key fields. Instance principal sends the auth type and nothing else
// from that set. The cached key OCID stays out of the create body.
func TestOCIBuildCreateAuthModes(t *testing.T) {
	withKey := ociAPIKeyConfig()
	body, err := ociBuildCreate(
		schema.TestResourceDataRaw(t, ResourceOCIEARConfig().Schema, withKey),
	)
	if err != nil {
		t.Fatalf("api key build: %v", err)
	}
	if body[ociKeyAuthType] != ociAuthAPIKey {
		t.Errorf("ociAuthType = %v want %s", body[ociKeyAuthType], ociAuthAPIKey)
	}
	if body[ociKeyPrivateKey] != withKey["private_key_content"] {
		t.Errorf("private key = %v", body[ociKeyPrivateKey])
	}
	if body[ociKeyTenancyID] != withKey["tenancy_id"] || body[ociKeyKeyName] != "yb-master" {
		t.Errorf("create body = %v", body)
	}
	if _, has := body[ociKeyKeyOCID]; has {
		t.Error("ociKeyOcid must not be in the create body")
	}
	if _, has := body["name"]; has {
		t.Error("name is added by the shared create helper, not the OCI builder")
	}

	withHost := ociBase()
	withHost["use_instance_principal"] = true
	body, err = ociBuildCreate(
		schema.TestResourceDataRaw(t, ResourceOCIEARConfig().Schema, withHost))
	if err != nil {
		t.Fatalf("instance principal build: %v", err)
	}
	if body[ociKeyAuthType] != ociAuthInstancePrincipal {
		t.Errorf("ociAuthType = %v want %s", body[ociKeyAuthType], ociAuthInstancePrincipal)
	}
	for _, absentKey := range []string{
		ociKeyTenancyID, ociKeyUserID, ociKeyFingerprint, ociKeyPrivateKey, ociKeyKeyOCID,
	} {
		if _, has := body[absentKey]; has {
			t.Errorf("%s must not be in the instance-principal body", absentKey)
		}
	}

}

func TestOCIPlanRequiresAPIKeyFields(t *testing.T) {
	raw := ociBase()
	raw["tenancy_id"] = "ocid1.tenancy.oc1..t"
	raw["user_id"] = "ocid1.user.oc1..u"
	raw["fingerprint"] = "aa:bb"
	_, err := ResourceOCIEARConfig().Diff(
		context.Background(), nil, terraform.NewResourceConfigRaw(raw), nil)
	if err == nil || !strings.Contains(err.Error(), "private_key_content") {
		t.Fatalf("plan error = %v, want one that names private_key_content", err)
	}

	_, err = ResourceOCIEARConfig().Diff(
		context.Background(), nil, terraform.NewResourceConfigRaw(ociAPIKeyConfig()), nil)
	if err != nil {
		t.Fatalf("complete api-key plan: %v", err)
	}
	host := ociBase()
	host["use_instance_principal"] = true
	_, err = ResourceOCIEARConfig().Diff(
		context.Background(), nil, terraform.NewResourceConfigRaw(host), nil)
	if err != nil {
		t.Fatalf("instance-principal plan: %v", err)
	}

	explicitFalse := ociAPIKeyConfig()
	explicitFalse["use_instance_principal"] = false
	_, err = ResourceOCIEARConfig().Diff(
		context.Background(), nil, terraform.NewResourceConfigRaw(explicitFalse), nil)
	if err != nil {
		t.Fatalf("explicit use_instance_principal = false with the signing key: %v", err)
	}
	// ConflictsWith rejects a set bool, including false. Diff does not run
	// that check; Validate does.
	if diags := ResourceOCIEARConfig().Validate(
		terraform.NewResourceConfigRaw(explicitFalse)); diags.HasError() {
		t.Fatalf("explicit use_instance_principal = false must validate: %v", diags)
	}

	both := ociAPIKeyConfig()
	both["use_instance_principal"] = true
	_, err = ResourceOCIEARConfig().Diff(
		context.Background(), nil, terraform.NewResourceConfigRaw(both), nil)
	if err == nil || !strings.Contains(err.Error(), "use_instance_principal") {
		t.Fatalf("mixed mode plan error = %v, want one that names use_instance_principal", err)
	}
}

// YBA's OCI edit merge keeps a stored editable field the body leaves out, and
// rejects a change to the auth type, vault, region, key name or key OCID.
func TestOCIBuildEditKeepsStoredPrivateKey(t *testing.T) {
	state := map[string]string{
		"name": "kms", "use_instance_principal": "false",
		"tenancy_id": "ocid1.tenancy.oc1..t", "user_id": "ocid1.user.oc1..u",
		"fingerprint":         "aa:bb:cc:dd:ee",
		"private_key_content": "-----BEGIN PRIVATE KEY-----\nold\n-----END PRIVATE KEY-----",
		"compartment_id":      "ocid1.compartment.oc1..c",
		"vault_id":            "ocid1.vault.oc1..v",
		"region":              "us-ashburn-1",
		"key_name":            "yb-master",
	}
	res := ResourceOCIEARConfig()

	fingerprint := ociAPIKeyConfig()
	fingerprint["fingerprint"] = "11:22:33:44:55"
	body, err := ociBuildEdit(plannedData(t, res, state, fingerprint))
	if err != nil {
		t.Fatalf("fingerprint edit: %v", err)
	}
	if body[ociKeyFingerprint] != "11:22:33:44:55" {
		t.Errorf("fingerprint = %v", body[ociKeyFingerprint])
	}
	if _, has := body[ociKeyPrivateKey]; has {
		t.Error("a fingerprint change must leave the stored private key out of the body")
	}
	for _, locked := range []string{ociKeyAuthType, ociKeyVaultID, ociKeyRegion, ociKeyKeyName,
		ociKeyKeyOCID} {
		if _, has := body[locked]; has {
			t.Errorf("%s is not editable and must stay out of the edit body", locked)
		}
	}

	rotated := ociAPIKeyConfig()
	rotated["private_key_content"] = "-----BEGIN PRIVATE KEY-----\nnew\n-----END PRIVATE KEY-----"
	body, err = ociBuildEdit(plannedData(t, res, state, rotated))
	if err != nil {
		t.Fatalf("key rotation: %v", err)
	}
	if body[ociKeyPrivateKey] != rotated["private_key_content"] {
		t.Errorf("private key = %v", body[ociKeyPrivateKey])
	}

	moved := ociAPIKeyConfig()
	moved["compartment_id"] = "ocid1.compartment.oc1..d"
	body, err = ociBuildEdit(plannedData(t, res, state, moved))
	if err != nil {
		t.Fatalf("compartment edit: %v", err)
	}
	if body[ociKeyCompartmentID] != "ocid1.compartment.oc1..d" {
		t.Errorf("compartment = %v", body[ociKeyCompartmentID])
	}
	if _, has := body[ociKeyPrivateKey]; has {
		t.Error("a compartment change must leave the stored private key out of the body")
	}
}

// The list masks ociKeyName, ociKeyOcid and ociPrivateKeyContent. Flatten
// keeps them out of state. A blank ociAuthType is API-key auth.
func TestOCIFlattenSkipsMaskedKeyFields(t *testing.T) {
	d := ResourceOCIEARConfig().TestResourceData()
	d.SetId(testConfigUUID)
	err := ociFlatten(d, map[string]interface{}{
		ociKeyAuthType:      "",
		ociKeyRegion:        "us-ashburn-1",
		ociKeyVaultID:       "ocid1.vault.oc1..v",
		ociKeyCompartmentID: "ocid1.compartment.oc1..c",
		ociKeyTenancyID:     "ocid1.tenancy.oc1..t",
		ociKeyKeyName:       "yb************er",
		ociKeyKeyOCID:       "oc************aa",
		ociKeyPrivateKey:    "-----BEGIN P*****Y-----",
	})
	if err != nil {
		t.Fatalf("flatten: %v", err)
	}
	if d.Get("use_instance_principal") != false {
		t.Error("a blank ociAuthType is API-key auth")
	}
	if d.Get("region") != "us-ashburn-1" || d.Get("tenancy_id") != "ocid1.tenancy.oc1..t" {
		t.Errorf("clear settings were not written: region=%v tenancy=%v",
			d.Get("region"), d.Get("tenancy_id"))
	}
	if d.Get("key_name") != "" || d.Get("key_id") != "" || d.Get("private_key_content") != "" {
		t.Error("flatten must not write the masked key name, key OCID or private key")
	}

	if err := ociFlatten(d, map[string]interface{}{
		ociKeyAuthType: ociAuthInstancePrincipal, ociKeyRegion: "us-phoenix-1",
	}); err != nil {
		t.Fatalf("flatten instance principal: %v", err)
	}
	if d.Get("use_instance_principal") != true {
		t.Error("INSTANCE_PRINCIPAL must be read back as use_instance_principal")
	}
}

// key_name and key_id are read from the by-UUID GET, which returns them in
// clear. The private key in that response stays out of state.
func TestOCIReadKeyUsesClearGet(t *testing.T) {
	f := &fakeYBA{savedConfig: `{
		"ociKeyName": "yb-master",
		"ociKeyOcid": "ocid1.key.oc1.iad.aaaa",
		"ociPrivateKeyContent": "-----BEGIN PRIVATE KEY-----\nsecret\n-----END PRIVATE KEY-----"
	}`}
	d := ResourceOCIEARConfig().TestResourceData()
	d.SetId(testConfigUUID)
	if err := ociReadKey(context.Background(), d, f.apiClient(t)); err != nil {
		t.Fatalf("read key: %v", err)
	}
	if d.Get("key_name") != "yb-master" {
		t.Errorf("key_name = %v", d.Get("key_name"))
	}
	if d.Get("key_id") != "ocid1.key.oc1.iad.aaaa" {
		t.Errorf("key_id = %v", d.Get("key_id"))
	}
	if d.Get("private_key_content") != "" {
		t.Error("the private key from the clear GET must not enter state")
	}

	if err := d.Set("key_name", "yb-master"); err != nil {
		t.Fatal(err)
	}
	if err := d.Set("key_id", "ocid1.key.oc1.iad.aaaa"); err != nil {
		t.Fatal(err)
	}
	f.savedConfig = ""
	if err := ociReadKey(context.Background(), d, f.apiClient(t)); err != nil {
		t.Fatalf("refresh with the key already in state: %v", err)
	}
	if d.Get("key_name") != "yb-master" || d.Get("key_id") != "ocid1.key.oc1.iad.aaaa" {
		t.Errorf("refresh rewrote the key: name=%v id=%v", d.Get("key_name"), d.Get("key_id"))
	}
	if d.Get("private_key_content") != "" {
		t.Error("refresh must not write the private key")
	}
}

func TestOCIVersionAllowed(t *testing.T) {
	cases := []struct {
		version string
		wantErr string
	}{
		{version: "2026.1.2.0-b45", wantErr: "2026.1.2.0-b49"},
		{version: "2026.1.2.0-b49"},
		{version: "2026.1.3.0-b69"},
		{version: "2.31.0.0-b243", wantErr: "2.31.0.0-b244"},
		{version: "2.31.0.0-b244"},
		{version: "2.31.0.0-b386"},
		{version: "2.31.0.4263-b4"},
	}
	for _, tc := range cases {
		t.Run(tc.version, func(t *testing.T) {
			err := ociVersionAllowed(tc.version)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("allowed version: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) ||
				!strings.Contains(err.Error(), "yba_oci_ear_config") {
				t.Fatalf("error = %v, want it to name yba_oci_ear_config and %q", err, tc.wantErr)
			}
		})
	}

	c := &api.APIClient{}
	c.SetAppVersion("not-a-version")
	if err := ociRequireVersion(context.Background(), c); err != nil {
		t.Fatalf("an unreadable version must pass: %v", err)
	}
}
