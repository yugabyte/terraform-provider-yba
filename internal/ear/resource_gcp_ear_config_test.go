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
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

// The two auth modes map onto YBA's mutually exclusive keys: the key file goes
// as a JSON object (YBA reads project_id out of it), host identity as the flag
// alone, and unset optional settings stay out of the body so YBA applies its
// own defaults.
func TestGCPBuildAuthModes(t *testing.T) {
	base := map[string]interface{}{
		"name": "ring", "location_id": "global", "key_ring_id": "kr", "crypto_key_id": "ck",
	}
	withKey := map[string]interface{}{
		"credentials": `{"type": "service_account", "project_id": "p"}`,
	}
	withHost := map[string]interface{}{"use_gcp_iam": true}
	for k, v := range base {
		withKey[k] = v
		withHost[k] = v
	}

	body, err := gcpBuildCreate(
		schema.TestResourceDataRaw(t, ResourceGCPEARConfig().Schema, withKey),
	)
	if err != nil {
		t.Fatalf("key file build: %v", err)
	}
	if key, ok := body[gcpKeyConfig].(map[string]interface{}); !ok || key["project_id"] != "p" {
		t.Errorf("GCP_CONFIG = %v: want the key file as a JSON object", body[gcpKeyConfig])
	}
	if _, has := body[gcpKeyUseIAM]; has {
		t.Error("USE_GCP_IAM must be absent in key-file mode")
	}

	body, err = gcpBuildCreate(
		schema.TestResourceDataRaw(t, ResourceGCPEARConfig().Schema, withHost),
	)
	if err != nil {
		t.Fatalf("host identity build: %v", err)
	}
	if body[gcpKeyUseIAM] != true {
		t.Errorf("USE_GCP_IAM = %v want true", body[gcpKeyUseIAM])
	}
	for _, absent := range []string{gcpKeyConfig, gcpKeyProtectionLevel, gcpKeyKMSEndpoint,
		gcpKeyProjectID, "name"} {
		if _, has := body[absent]; has {
			t.Errorf("%s must not be in the body", absent)
		}
	}

	// gcpBuildAuth checks use_gcp_iam first and would drop the key file, so the
	// plan must reject both modes at once.
	both := map[string]interface{}{"credentials": withKey["credentials"]}
	for k, v := range withHost {
		both[k] = v
	}
	diags := ResourceGCPEARConfig().Validate(terraform.NewResourceConfigRaw(both))
	if !diags.HasError() {
		t.Error("want a plan error when credentials and use_gcp_iam are both set")
	}
}

// A build without host-identity support stores the flag next to the key it
// keeps using; Read must report that instead of a clean host-identity state.
func TestGCPFlattenRejectsKeyFileNextToHostIdentity(t *testing.T) {
	d := ResourceGCPEARConfig().TestResourceData()
	d.SetId(testConfigUUID)
	err := gcpFlatten(d, map[string]interface{}{
		gcpKeyUseIAM: true, gcpKeyConfig: map[string]interface{}{"private_key": "***"},
	})
	if err == nil {
		t.Fatal("want an error for USE_GCP_IAM stored next to GCP_CONFIG")
	}
	// The flag alone, as a supporting build stores it, is the host identity.
	if err := gcpFlatten(d, map[string]interface{}{gcpKeyUseIAM: "true"}); err != nil {
		t.Fatalf("flatten: %v", err)
	}
	if d.Get("use_gcp_iam") != true {
		t.Error("use_gcp_iam must be read back from USE_GCP_IAM")
	}
}
