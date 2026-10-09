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

	"github.com/hashicorp/go-cty/cty"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	client "github.com/yugabyte/platform-go-client"
)

// YBA drops PerProcessFlags when both maps are empty. An empty per_process
// block the customer authored (raw config, on apply) or that state already
// tracks (prior state, on refresh) must survive Read, otherwise the plan shows
// a never-ending `+ per_process` diff whose apply is a no-op.
func TestRestoreEmptyPerProcess(t *testing.T) {
	emptyPP := []interface{}{map[string]interface{}{
		"master_gflags":  map[string]interface{}{},
		"tserver_gflags": map[string]interface{}{},
	}}
	stateCluster := func(pp []interface{}) []interface{} {
		return []interface{}{map[string]interface{}{
			"uuid": "c1",
			"user_intent": []interface{}{map[string]interface{}{
				"specific_gflags": []interface{}{map[string]interface{}{
					"per_process": pp,
				}},
			}},
		}}
	}
	ppObj := cty.ObjectVal(map[string]cty.Value{
		"master_gflags": cty.MapValEmpty(cty.String),
	})
	configWithPP := cty.ObjectVal(map[string]cty.Value{
		"clusters": cty.ListVal([]cty.Value{cty.ObjectVal(map[string]cty.Value{
			"user_intent": cty.ListVal([]cty.Value{cty.ObjectVal(map[string]cty.Value{
				"specific_gflags": cty.ListVal([]cty.Value{cty.ObjectVal(map[string]cty.Value{
					"per_process": cty.ListVal([]cty.Value{ppObj}),
				})}),
			})}),
		})}),
	})

	cases := []struct {
		name      string
		old       []interface{}
		rawConfig cty.Value
		apiFlags  map[string]map[string]string
		wantPP    int
	}{
		{"authored in config", nil, configWithPP, nil, 1},
		{"tracked in prior state", stateCluster(emptyPP), cty.NilVal, nil, 1},
		{"untracked stays absent", stateCluster([]interface{}{}), cty.NilVal, nil, 0},
		{
			"server flags left as returned", stateCluster(emptyPP), cty.NilVal,
			map[string]map[string]string{"TSERVER": {"v": "1"}}, 1,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sg := client.NewSpecificGFlags()
			if tc.apiFlags != nil {
				sg.SetPerProcessFlags(*client.NewPerProcessFlags(tc.apiFlags))
			}
			newClusters := []map[string]interface{}{{
				"uuid": "c1",
				"user_intent": []interface{}{map[string]interface{}{
					"specific_gflags": flattenSpecificGFlags(sg),
				}},
			}}
			restoreEmptyPerProcess(newClusters, tc.old, tc.rawConfig)
			got := specificGFlagsFromState(newClusters[0])
			pp, _ := got["per_process"].([]interface{})
			if len(pp) != tc.wantPP {
				t.Fatalf("per_process = %v, want %d block(s)", pp, tc.wantPP)
			}
			if tc.apiFlags != nil {
				tm, _ := pp[0].(map[string]interface{})["tserver_gflags"].(map[string]string)
				if tm["v"] != "1" {
					t.Fatalf("server tserver_gflags overwritten: %v", pp[0])
				}
			}
		})
	}

	// What Read writes after an apply of the reported config (YBA returns no
	// PerProcessFlags, no prior block, config authors one) must plan clean.
	newClusters := []map[string]interface{}{{
		"uuid": "c1",
		"user_intent": []interface{}{map[string]interface{}{
			"specific_gflags": flattenSpecificGFlags(client.NewSpecificGFlags()),
		}},
	}}
	restoreEmptyPerProcess(newClusters, nil, configWithPP)
	r := &schema.Resource{Schema: map[string]*schema.Schema{
		"specific_gflags": userIntentSchema().Schema["specific_gflags"],
	}}
	d := r.TestResourceData()
	d.SetId("x")
	if err := d.Set("specific_gflags", []interface{}{
		specificGFlagsFromState(newClusters[0]),
	}); err != nil {
		t.Fatal(err)
	}
	cfg := terraform.NewResourceConfigRaw(map[string]interface{}{
		"specific_gflags": []interface{}{map[string]interface{}{"per_process": emptyPP}},
	})
	diff, err := r.Diff(context.Background(), d.State(), cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	if diff != nil && len(diff.Attributes) > 0 {
		t.Fatalf("unexpected diff: %v", diff.Attributes)
	}
}
