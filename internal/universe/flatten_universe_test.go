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

package universe

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	client "github.com/yugabyte/platform-go-client"
)

// YBA returns no PerProcessFlags when both maps are empty. What Read writes
// from that must plan clean against an empty per_process block in config, and
// against a config without one.
func TestFlattenSpecificGFlagsEmptyPerProcess(t *testing.T) {
	r := &schema.Resource{Schema: map[string]*schema.Schema{
		"specific_gflags": userIntentSchema().Schema["specific_gflags"],
	}}
	d := r.TestResourceData()
	d.SetId("x")
	if err := d.Set(
		"specific_gflags",
		flattenSpecificGFlags(client.NewSpecificGFlags()),
	); err != nil {
		t.Fatal(err)
	}
	for name, sg := range map[string]map[string]interface{}{
		"empty per_process": {"per_process": []interface{}{map[string]interface{}{
			"master_gflags":  map[string]interface{}{},
			"tserver_gflags": map[string]interface{}{},
		}}},
		"no per_process": {"gflag_groups": []interface{}{}},
	} {
		cfg := terraform.NewResourceConfigRaw(map[string]interface{}{
			"specific_gflags": []interface{}{sg},
		})
		diff, err := r.Diff(context.Background(), d.State(), cfg, nil)
		if err != nil {
			t.Fatal(err)
		}
		if diff != nil && len(diff.Attributes) > 0 {
			t.Errorf("%s: unexpected diff: %v", name, diff.Attributes)
		}
	}
}
