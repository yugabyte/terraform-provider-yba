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

package perfadvisor

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	clientv2 "github.com/yugabyte/platform-go-client/v2"
)

// YBA reads passwords back masked. Without carrying the configured value over,
// every plan would report a diff from the real password to "********" and an
// apply would write the mask back as the credential.
func TestFlattenAuthKeepsTheConfiguredPasswordWhenTheServerMasksIt(t *testing.T) {
	configured := []interface{}{
		map[string]interface{}{
			"type":     "BASIC",
			"username": "writer",
			"password": "s3cret",
		},
	}
	fromServer := &clientv2.PerfAdvisorEndpointAuth{
		Type:     "BASIC",
		Username: strPtr("writer"),
		Password: strPtr(maskedPassword),
	}

	got := flattenAuth(fromServer, configured)

	block := got[0].(map[string]interface{})
	if block["password"] != "s3cret" {
		t.Errorf("expected the configured password to be kept, got %q", block["password"])
	}
	if block["username"] != "writer" {
		t.Errorf("expected the username to be refreshed, got %q", block["username"])
	}
}

// A value that is not the mask is a real one, and reconciling it is how an
// out-of-band change shows up as drift instead of being silently kept.
func TestFlattenAuthReconcilesAnUnmaskedPassword(t *testing.T) {
	configured := []interface{}{
		map[string]interface{}{"type": "BASIC", "username": "writer", "password": "stale"},
	}
	fromServer := &clientv2.PerfAdvisorEndpointAuth{
		Type:     "BASIC",
		Username: strPtr("writer"),
		Password: strPtr("rotated"),
	}

	block := flattenAuth(fromServer, configured)[0].(map[string]interface{})

	if block["password"] != "rotated" {
		t.Errorf("expected the server value to win, got %q", block["password"])
	}
}

// No auth on the server means the block is absent, not empty: Terraform has to
// clear it rather than keep a stale credential in state.
func TestFlattenAuthReturnsNilForNoAuth(t *testing.T) {
	if got := flattenAuth(nil, nil); got != nil {
		t.Errorf("expected nil, got %#v", got)
	}
}

// The YBM identifiers are optional, and an empty string has to stay unset -
// sending "" would pin an empty header rather than omitting it.
func TestBuildEndpointSpecOmitsUnsetYbmIdentifiers(t *testing.T) {
	d := schema.TestResourceDataRaw(
		t,
		ResourcePerfAdvisorEndpoint().Schema,
		map[string]interface{}{
			"name":                "byoc-prod",
			"type":                "BYOC",
			"collection_endpoint": "https://byoc.cloud.yugabyte.com",
			"metrics_endpoint":    "https://byoc.cloud.yugabyte.com/api/v1/otlp/metrics",
			"metrics_type":        "otlphttp",
		})

	spec := buildEndpointSpec(d)

	if spec.YbmAccountId != nil {
		t.Errorf("expected no account id, got %q", *spec.YbmAccountId)
	}
	if spec.YbmProjectId != nil {
		t.Errorf("expected no project id, got %q", *spec.YbmProjectId)
	}
	if spec.Name != "byoc-prod" {
		t.Errorf("unexpected name %q", spec.Name)
	}
	if string(spec.MetricsType) != "otlphttp" {
		t.Errorf("unexpected metrics type %q", spec.MetricsType)
	}
}

func strPtr(in string) *string {
	return &in
}
