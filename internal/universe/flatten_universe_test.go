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

// What Read writes for specific_gflags must plan clean against the config
// whenever YBA holds the same flags in another form, and must show a diff
// when the flags changed outside Terraform.
func TestReadSpecificGFlagsPlansClean(t *testing.T) {
	flags := func(kv ...string) map[string]interface{} {
		m := map[string]interface{}{}
		for i := 0; i < len(kv); i += 2 {
			m[kv[i]] = kv[i+1]
		}
		return m
	}
	emptyPP := func() map[string]interface{} {
		return map[string]interface{}{"per_process": []interface{}{map[string]interface{}{
			"master_gflags": flags(), "tserver_gflags": flags(),
		}}}
	}
	perAZ := func(entries ...map[string]interface{}) func() map[string]interface{} {
		return func() map[string]interface{} {
			l := make([]interface{}, len(entries))
			for i, e := range entries {
				l[i] = e
			}
			return map[string]interface{}{"per_az": l}
		}
	}
	az := func(uuid string, master, tserver map[string]interface{}) map[string]interface{} {
		return map[string]interface{}{
			"az_uuid": uuid, "master_gflags": master, "tserver_gflags": tserver,
		}
	}
	apiPerAZ := func(azs map[string]map[string]map[string]string) *client.SpecificGFlags {
		sg := client.NewSpecificGFlags()
		m := map[string]client.PerProcessFlags{}
		for uuid, v := range azs {
			m[uuid] = *client.NewPerProcessFlags(v)
		}
		sg.SetPerAZ(m)
		return sg
	}
	twoAZs := perAZ(
		az("az-b", flags(), flags("b", "1")),
		az("az-a", flags(), flags("a", "1")),
	)

	cases := []struct {
		name   string
		config func() map[string]interface{}
		// prior is the block in state before Read; nil means import.
		prior    func() map[string]interface{}
		api      *client.SpecificGFlags
		wantDiff bool
	}{
		{"empty per_process, imported", emptyPP, nil, client.NewSpecificGFlags(), false},
		{
			"no per_process in config, imported",
			func() map[string]interface{} { return map[string]interface{}{} },
			nil, client.NewSpecificGFlags(), false,
		},
		{
			"empty per_process, state from an earlier release", emptyPP,
			func() map[string]interface{} {
				return map[string]interface{}{"per_process": []interface{}{}}
			},
			client.NewSpecificGFlags(), false,
		},
		{
			// Every field was unknown at plan, so the SDK reads the block as nil.
			"empty per_process, first Read after create", emptyPP,
			func() map[string]interface{} {
				return map[string]interface{}{"per_process": []interface{}{nil}}
			},
			client.NewSpecificGFlags(), false,
		},
		{
			"per_az in another order than YBA's", twoAZs, twoAZs,
			apiPerAZ(map[string]map[string]map[string]string{
				"az-a": {"TSERVER": {"a": "1"}}, "az-b": {"TSERVER": {"b": "1"}},
			}), false,
		},
		{
			"per_az imported, config in sorted order",
			perAZ(az("az-a", flags(), flags("a", "1")), az("az-b", flags(), flags("b", "1"))),
			nil, apiPerAZ(map[string]map[string]map[string]string{
				"az-a": {"TSERVER": {"a": "1"}}, "az-b": {"TSERVER": {"b": "1"}},
			}), false,
		},
		{
			"per_az entry with no flags",
			perAZ(az("az-a", flags(), flags())), perAZ(az("az-a", flags(), flags())),
			client.NewSpecificGFlags(), false,
		},
		{
			"YBA returns an empty process map", twoAZs, twoAZs,
			apiPerAZ(map[string]map[string]map[string]string{
				"az-a": {"TSERVER": {"a": "1"}, "MASTER": {}}, "az-b": {"TSERVER": {"b": "1"}},
			}), false,
		},
		{
			"flags changed outside Terraform", twoAZs, twoAZs,
			apiPerAZ(map[string]map[string]map[string]string{
				"az-a": {"TSERVER": {"a": "2"}}, "az-b": {"TSERVER": {"b": "1"}},
			}), true,
		},
	}

	r := &schema.Resource{Schema: map[string]*schema.Schema{
		"specific_gflags": userIntentSchema().Schema["specific_gflags"],
	}}
	cluster := func(sg []interface{}) map[string]interface{} {
		return map[string]interface{}{
			"uuid":        "c1",
			"user_intent": []interface{}{map[string]interface{}{"specific_gflags": sg}},
		}
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			newClusters := []map[string]interface{}{cluster(flattenSpecificGFlags(tc.api))}
			var oldClusters []interface{}
			if tc.prior != nil {
				oldClusters = []interface{}{cluster([]interface{}{tc.prior()})}
			}
			keepEquivalentSpecificGFlags(newClusters, oldClusters,
				[]client.Cluster{{UserIntent: client.UserIntent{SpecificGFlags: tc.api}}})

			d := r.TestResourceData()
			d.SetId("x")
			ui := newClusters[0]["user_intent"].([]interface{})[0].(map[string]interface{})
			if err := d.Set("specific_gflags", ui["specific_gflags"]); err != nil {
				t.Fatal(err)
			}
			cfg := terraform.NewResourceConfigRaw(map[string]interface{}{
				"specific_gflags": []interface{}{tc.config()},
			})
			diff, err := r.Diff(context.Background(), d.State(), cfg, nil)
			if err != nil {
				t.Fatal(err)
			}
			if got := diff != nil && len(diff.Attributes) > 0; got != tc.wantDiff {
				t.Fatalf("diff = %v, want diff %v", diff, tc.wantDiff)
			}
		})
	}
}
