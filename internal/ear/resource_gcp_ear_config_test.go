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
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func TestGCPEARConfigSchemaSanity(t *testing.T) {
	res := ResourceGCPEARConfig()
	if err := res.InternalValidate(nil, true); err != nil {
		t.Fatalf("schema failed InternalValidate: %v", err)
	}
	s := res.Schema
	// YBA rejects edits to every key-location field ("cannot be changed").
	for _, f := range []string{"name", "project_id", "location_id", "key_ring_id",
		"crypto_key_id", "protection_level", "kms_endpoint"} {
		if !s[f].ForceNew {
			t.Errorf("%s must be ForceNew: YBA does not allow editing it", f)
		}
	}
	// Credentials are the one thing YBA lets an edit change.
	for _, f := range []string{"credentials", "use_gcp_iam"} {
		if s[f].ForceNew {
			t.Errorf("%s must stay editable in place", f)
		}
	}
	if !s["credentials"].Sensitive {
		t.Error("credentials must be Sensitive")
	}
	if got := s["credentials"].ConflictsWith; len(got) != 1 || got[0] != "use_gcp_iam" {
		t.Errorf("credentials.ConflictsWith = %v want [use_gcp_iam]", got)
	}
}

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
}

func TestGCPFlattenRejectsKeyFileNextToHostIdentity(t *testing.T) {
	d := ResourceGCPEARConfig().TestResourceData()
	d.SetId(testConfigUUID)
	err := gcpFlatten(d, map[string]interface{}{
		gcpKeyUseIAM: true, gcpKeyConfig: map[string]interface{}{"private_key": "***"},
	})
	if err == nil || !strings.Contains(err.Error(), "does not support host identity") {
		t.Fatalf("want the unsupported-build error, got %v", err)
	}
}
