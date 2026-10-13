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
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

// earResources lists every per-provider EAR resource with its secrets, the
// only arguments that are Sensitive.
var earResources = map[string]struct {
	spec    func() earSpec
	secrets []string
}{
	"yba_aws_ear_config":   {awsEARSpec, []string{"access_key_id", "secret_access_key"}},
	"yba_azure_ear_config": {azureEARSpec, []string{"client_secret"}},
	"yba_gcp_ear_config":   {gcpEARSpec, []string{"credentials"}},
}

// Shared guardrails for every EAR resource. The shared docs note says that
// only the arguments whose description says so can change in place, and that
// a change to any other argument forces replacement; ForceNew must agree.
func TestEARConfigGuardrails(t *testing.T) {
	for name, tc := range earResources {
		t.Run(name, func(t *testing.T) {
			res := earResource(tc.spec())
			if err := res.InternalValidate(nil, true); err != nil {
				t.Fatalf("schema failed InternalValidate: %v", err)
			}
			if res.Description == "" {
				t.Error("resource description is required")
			}
			if res.Importer == nil {
				t.Error("the resource must be importable")
			}
			if _, ok := res.Schema["customer_uuid"]; ok {
				t.Error("customer_uuid is a noise field and must not be exposed")
			}
			for op, timeout := range map[string]*time.Duration{
				"create": res.Timeouts.Create,
				"update": res.Timeouts.Update,
				"delete": res.Timeouts.Delete,
			} {
				if timeout == nil || *timeout != earTaskTimeout {
					t.Errorf("%s timeout must default to earTaskTimeout", op)
				}
			}
			secrets := map[string]bool{}
			for _, f := range tc.secrets {
				secrets[f] = true
			}
			for field, s := range res.Schema {
				if s.Description == "" {
					t.Errorf("%s needs a Description: it renders into the docs", field)
				}
				if s.Sensitive != secrets[field] {
					t.Errorf("%s: Sensitive = %t want %t: only secrets are Sensitive",
						field, s.Sensitive, secrets[field])
				}
				if !s.Optional && !s.Required {
					continue
				}
				inPlace := strings.Contains(s.Description, "change in place")
				replaces := strings.Contains(s.Description, "forces replacement")
				if s.ForceNew != replaces || s.ForceNew == inPlace {
					t.Errorf("%s: ForceNew = %t, but the Description says it can change in "+
						"place: %t, forces replacement: %t", field, s.ForceNew, inPlace, replaces)
				}
			}
		})
	}
}

// azureState is a service principal configuration with the secret "old".
var azureState = map[string]string{
	"name": "vault", "client_id": "app", "tenant_id": "tenant", "client_secret": "old",
	"vault_url": "https://v.vault.azure.net/", "key_name": "yb-key", "key_size": "2048",
}

// azureConfig is the configuration of azureState with clientID and secret;
// an empty secret leaves client_secret unset.
func azureConfig(clientID, secret string) map[string]interface{} {
	raw := map[string]interface{}{
		"name": "vault", "client_id": clientID, "tenant_id": "tenant",
		"vault_url": "https://v.vault.azure.net/", "key_name": "yb-key",
	}
	if secret != "" {
		raw["client_secret"] = secret
	}
	return raw
}

// plannedData returns the ResourceData that Update gets for a plan from the
// state attrs to the configuration raw.
func plannedData(
	t *testing.T, res *schema.Resource, attrs map[string]string, raw map[string]interface{},
) *schema.ResourceData {
	t.Helper()
	state := &terraform.InstanceState{
		ID: testConfigUUID, Attributes: map[string]string{"id": testConfigUUID},
	}
	for k, v := range attrs {
		state.Attributes[k] = v
	}
	diff, err := res.Diff(context.Background(), state, terraform.NewResourceConfigRaw(raw), nil)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	d, err := schema.InternalMap(res.Schema).Data(state, diff)
	if err != nil {
		t.Fatalf("data: %v", err)
	}
	return d
}

// absent marks a key that the edit body must leave out.
var absent = struct{}{}

// YBA's AWS and AZU edit merges keep a stored key that the body leaves out.
// So a secret goes only when the configuration changes it, and a configuration
// without a secret clears it with "". For Azure, "" goes on every edit without
// a configured secret: imported state has no secret, so a change does not
// show, while YBA may still store one.
func TestEARBuildEditSecretRule(t *testing.T) {
	awsState := map[string]string{
		"name": "kms", "region": "us-west-2", "cmk_id": "key", "access_key_id": "AKIA",
		"secret_access_key": "old", "kms_endpoint": "kms-a.example.com",
	}
	awsConfig := func(keys bool, endpoint string) map[string]interface{} {
		raw := map[string]interface{}{"name": "kms", "region": "us-west-2", "cmk_id": "key"}
		if keys {
			raw["access_key_id"] = "AKIA"
			raw["secret_access_key"] = "old"
		}
		if endpoint != "" {
			raw["kms_endpoint"] = endpoint
		}
		return raw
	}
	importedAzureState := map[string]string{}
	for k, v := range azureState {
		importedAzureState[k] = v
	}
	importedAzureState["client_secret"] = ""

	cases := []struct {
		name  string
		spec  earSpec
		state map[string]string
		raw   map[string]interface{}
		want  map[string]interface{}
	}{
		{
			name: "azure client_id change keeps the stored secret", spec: azureEARSpec(),
			state: azureState, raw: azureConfig("app-2", "old"),
			want: map[string]interface{}{
				azureKeyClientID: "app-2", azureKeyTenantID: "tenant", azureKeyClientSecret: absent,
			},
		},
		{
			name: "azure rotated secret", spec: azureEARSpec(),
			state: azureState, raw: azureConfig("app", "new"),
			want: map[string]interface{}{azureKeyClientSecret: "new"},
		},
		{
			name: "azure switch to the managed identity", spec: azureEARSpec(),
			state: azureState, raw: azureConfig("mi", ""),
			want: map[string]interface{}{azureKeyClientID: "mi", azureKeyClientSecret: ""},
		},
		{
			name: "azure switch on imported state", spec: azureEARSpec(),
			state: importedAzureState, raw: azureConfig("mi", ""),
			want: map[string]interface{}{azureKeyClientID: "mi", azureKeyClientSecret: ""},
		},
		{
			name: "aws endpoint change keeps the stored keys", spec: awsEARSpec(),
			state: awsState, raw: awsConfig(true, "kms-b.example.com"),
			want: map[string]interface{}{
				awsKeyKMSEndpoint: "kms-b.example.com",
				awsKeyAccessKeyID: absent, awsKeySecretAccessKey: absent,
			},
		},
		{
			name: "aws switch to the host identity", spec: awsEARSpec(),
			state: awsState, raw: awsConfig(false, ""),
			want: map[string]interface{}{
				awsKeyKMSEndpoint: "", awsKeyAccessKeyID: "", awsKeySecretAccessKey: "",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := plannedData(t, earResource(tc.spec), tc.state, tc.raw)
			body, err := tc.spec.buildEdit(d)
			if err != nil {
				t.Fatalf("build: %v", err)
			}
			for key, want := range tc.want {
				got, has := body[key]
				if want == absent {
					if has {
						t.Errorf("%s = %v: want it left out", key, got)
					}
					continue
				}
				if !has || got != want {
					t.Errorf("%s = %v (present %t) want %v", key, got, has, want)
				}
			}
		})
	}
}

// YBA stores AZU_KEY_SIZE as the create body sent it: a JSON number from the
// UI, yba-cli and this provider, or a string from another client.
func TestAzureFlattenKeySize(t *testing.T) {
	for in, want := range map[interface{}]int{float64(4096): 4096, "3072": 3072} {
		d := ResourceAzureEARConfig().TestResourceData()
		if err := azureFlatten(d, map[string]interface{}{azureKeySize: in}); err != nil {
			t.Fatalf("flatten %#v: %v", in, err)
		}
		if got := d.Get("key_size"); got != want {
			t.Errorf("key_size from %#v = %v want %d", in, got, want)
		}
	}
	d := ResourceAzureEARConfig().TestResourceData()
	if err := azureFlatten(d, map[string]interface{}{azureKeySize: "large"}); err == nil {
		t.Error("want an error for a key size that is not an integer")
	}
}
