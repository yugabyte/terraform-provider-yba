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
	"slices"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	client "github.com/yugabyte/platform-go-client"

	"github.com/yugabyte/terraform-provider-yba/internal/utils"
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

// YBA returns the secrets in LDAP gflags as REDACTED. Read must write the state
// value that YBA's value stands for, so the config plans clean, and Update must
// send a GFlags upgrade only when the config changes the secret or YBA's value
// changed outside Terraform: YBA rejects an upgrade that changes nothing. Where
// state holds REDACTED (after an import, or from an earlier release), Update
// must send the config, which may hold a new secret.
func TestRedactedGFlagsPlanAndUpgrade(t *testing.T) {
	const bind = "dummy-bind-pw"
	const hba = `"host all all 0.0.0.0/0 ldap ldapserver=ldap.example.com ` +
		`ldapbindpasswd=""dummy hba,pw"" ldaptls=0",local all yugabyte trust`
	const hbaRedacted = `"host all all 0.0.0.0/0 ldap ldapserver=ldap.example.com ` +
		`ldapbindpasswd=""REDACTED"" ldaptls=0",local all yugabyte trust`
	const hbaMovedRedacted = `"host all all 0.0.0.0/0 ldap ldapserver=ldap2.example.com ` +
		`ldapbindpasswd=""REDACTED"" ldaptls=0",local all yugabyte trust`

	specific := func(key string, entry map[string]interface{}) map[string]interface{} {
		return map[string]interface{}{"specific_gflags": []interface{}{
			map[string]interface{}{key: []interface{}{entry}},
		}}
	}
	perProcess := func(k, v string) map[string]interface{} {
		return specific("per_process", map[string]interface{}{
			"master_gflags":  map[string]interface{}{},
			"tserver_gflags": map[string]interface{}{k: v},
		})
	}
	perAZ := func(k, v string) map[string]interface{} {
		return specific("per_az", map[string]interface{}{
			"az_uuid": "az-a", "tserver_gflags": map[string]interface{}{k: v},
		})
	}
	flat := func(k, v string) map[string]interface{} {
		return map[string]interface{}{"tserver_gflags": map[string]interface{}{k: v}}
	}
	apiPerProcess := func(k, v string) func() client.UserIntent {
		return func() client.UserIntent {
			sg := client.NewSpecificGFlags()
			sg.SetPerProcessFlags(*client.NewPerProcessFlags(
				map[string]map[string]string{"TSERVER": {k: v}}))
			return client.UserIntent{SpecificGFlags: sg}
		}
	}
	apiPerAZ := func(k, v string) func() client.UserIntent {
		return func() client.UserIntent {
			sg := client.NewSpecificGFlags()
			sg.SetPerAZ(map[string]client.PerProcessFlags{
				"az-a": *client.NewPerProcessFlags(map[string]map[string]string{"TSERVER": {k: v}}),
			})
			return client.UserIntent{SpecificGFlags: sg}
		}
	}
	apiFlat := func(k, v string) func() client.UserIntent {
		return func() client.UserIntent {
			return client.UserIntent{TserverGFlags: &map[string]string{k: v}}
		}
	}

	cases := []struct {
		name   string
		config map[string]interface{}
		// prior is the user_intent in state before Read; nil means import.
		prior        map[string]interface{}
		api          func() client.UserIntent
		wantPlanDiff bool
		wantUpgrade  bool
	}{
		{
			"ycql_ldap_bind_passwd in per_process",
			perProcess("ycql_ldap_bind_passwd", bind), perProcess("ycql_ldap_bind_passwd", bind),
			apiPerProcess("ycql_ldap_bind_passwd", ybaRedacted), false, false,
		},
		{
			"ysql_hba_conf_csv in per_process",
			perProcess("ysql_hba_conf_csv", hba), perProcess("ysql_hba_conf_csv", hba),
			apiPerProcess("ysql_hba_conf_csv", hbaRedacted), false, false,
		},
		{
			"ysql_hba_conf_csv in per_az",
			perAZ("ysql_hba_conf_csv", hba), perAZ("ysql_hba_conf_csv", hba),
			apiPerAZ("ysql_hba_conf_csv", hbaRedacted), false, false,
		},
		{
			"ycql_ldap_bind_passwd in the deprecated tserver_gflags",
			flat("ycql_ldap_bind_passwd", bind), flat("ycql_ldap_bind_passwd", bind),
			apiFlat("ycql_ldap_bind_passwd", ybaRedacted), false, false,
		},
		{
			"secret changed in the config",
			perProcess("ycql_ldap_bind_passwd", "dummy-new-pw"),
			perProcess("ycql_ldap_bind_passwd", bind),
			apiPerProcess("ycql_ldap_bind_passwd", ybaRedacted), true, true,
		},
		{
			"ysql_hba_conf_csv changed outside Terraform",
			perProcess("ysql_hba_conf_csv", hba), perProcess("ysql_hba_conf_csv", hba),
			apiPerProcess("ysql_hba_conf_csv", hbaMovedRedacted), true, true,
		},
		{
			"imported, or state from an earlier release",
			perProcess("ysql_hba_conf_csv", hba), nil,
			apiPerProcess("ysql_hba_conf_csv", hbaRedacted), true, true,
		},
	}

	r := &schema.Resource{Schema: map[string]*schema.Schema{}}
	for _, f := range []string{"master_gflags", "tserver_gflags", "specific_gflags"} {
		r.Schema[f] = userIntentSchema().Schema[f]
	}
	cluster := func(ui map[string]interface{}) map[string]interface{} {
		return map[string]interface{}{"uuid": "c1", "user_intent": []interface{}{ui}}
	}
	apiClusters := func(ui client.UserIntent) []client.Cluster {
		return []client.Cluster{{Uuid: utils.GetStringPointer("c1"), UserIntent: ui}}
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var oldClusters []interface{}
			if tc.prior != nil {
				oldClusters = []interface{}{cluster(tc.prior)}
			}
			api := restoreClustersRedactedGFlags(apiClusters(tc.api()), oldClusters)
			ui := api[0].UserIntent
			newClusters := []map[string]interface{}{cluster(map[string]interface{}{
				"master_gflags":   masterFromIntent(ui),
				"tserver_gflags":  tserverFromIntent(ui),
				"specific_gflags": flattenSpecificGFlags(ui.SpecificGFlags),
			})}
			keepEquivalentSpecificGFlags(newClusters, oldClusters, api)

			d := r.TestResourceData()
			d.SetId("x")
			for f, v := range userIntentMap(newClusters[0]) {
				if err := d.Set(f, v); err != nil {
					t.Fatal(err)
				}
			}
			diff, err := r.Diff(context.Background(), d.State(),
				terraform.NewResourceConfigRaw(tc.config), nil)
			if err != nil {
				t.Fatal(err)
			}
			if got := diff != nil && len(diff.Attributes) > 0; got != tc.wantPlanDiff {
				t.Errorf("plan diff = %v, want %v", diff, tc.wantPlanDiff)
			}

			state := cluster(map[string]interface{}{
				"master_gflags":   d.Get("master_gflags"),
				"tserver_gflags":  d.Get("tserver_gflags"),
				"specific_gflags": d.Get("specific_gflags"),
			})
			api = restoreClustersRedactedGFlags(apiClusters(tc.api()), []interface{}{state})
			got := gflagsChanged(api[0].UserIntent, gflagsOf(cluster(tc.config)))
			if got != tc.wantUpgrade {
				t.Errorf("upgrade = %v, want %v", got, tc.wantUpgrade)
			}
		})
	}
}

// Update builds the GFlags upgrade payload from YBA's clusters. A cluster that
// the upgrade does not change must still go back with REDACTED, so that YBA
// keeps the secret it stores rather than the one in state.
func TestRestoreRedactedGFlagsKeepsYBAClusters(t *testing.T) {
	const k = "ycql_ldap_bind_passwd"
	procs := func(v string) map[string]map[string]string {
		return map[string]map[string]string{"TSERVER": {k: v}}
	}
	sg := client.NewSpecificGFlags()
	sg.SetPerProcessFlags(*client.NewPerProcessFlags(procs(ybaRedacted)))
	sg.SetPerAZ(map[string]client.PerProcessFlags{"az-a": *client.NewPerProcessFlags(
		procs(ybaRedacted))})
	api := []client.Cluster{{UserIntent: client.UserIntent{
		MasterGFlags:   &map[string]string{k: ybaRedacted},
		TserverGFlags:  &map[string]string{k: ybaRedacted},
		SpecificGFlags: sg,
	}}}
	flags := map[string]interface{}{k: "dummy-bind-pw"}
	state := map[string]interface{}{"user_intent": []interface{}{map[string]interface{}{
		"master_gflags":  flags,
		"tserver_gflags": flags,
		"specific_gflags": []interface{}{map[string]interface{}{
			"per_process": []interface{}{map[string]interface{}{"tserver_gflags": flags}},
			"per_az": []interface{}{
				map[string]interface{}{"az_uuid": "az-a", "tserver_gflags": flags},
			},
		}},
	}}}
	values := func(ui client.UserIntent) []string {
		return []string{
			ui.GetMasterGFlags()[k],
			ui.GetTserverGFlags()[k],
			ui.SpecificGFlags.PerProcessFlags.Value["TSERVER"][k],
			ui.SpecificGFlags.GetPerAZ()["az-a"].Value["TSERVER"][k],
		}
	}

	restored := restoreClustersRedactedGFlags(api, []interface{}{state})

	for i, v := range values(restored[0].UserIntent) {
		if v != "dummy-bind-pw" {
			t.Errorf("restored value %d = %q, want the state value", i, v)
		}
	}
	for i, v := range values(api[0].UserIntent) {
		if v != ybaRedacted {
			t.Errorf("YBA's value %d = %q, want %q", i, v, ybaRedacted)
		}
	}
}

// The Read after Update writes the configured secret to state only for a
// cluster whose gflags YBA got or confirmed. A secret that never reached YBA
// keeps the prior value, so the next plan still shows the change.
func TestRedactedGFlagsAfterUpdate(t *testing.T) {
	const k = "ycql_ldap_bind_passwd"
	cluster := func(pw string) map[string]interface{} {
		return map[string]interface{}{"uuid": "c1", "user_intent": []interface{}{
			map[string]interface{}{"tserver_gflags": map[string]interface{}{k: pw}},
		}}
	}
	prior := []interface{}{cluster("dummy-old-pw")}
	planned := []interface{}{cluster("dummy-new-pw")}
	cases := []struct {
		name string
		sent map[int]bool
		want string
	}{
		{"YBA got or confirmed the gflags", map[int]bool{0: true}, "dummy-new-pw"},
		{"apply failed first, or Update does not send them", map[int]bool{}, "dummy-old-pw"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			api := []client.Cluster{{
				Uuid:       utils.GetStringPointer("c1"),
				UserIntent: client.UserIntent{TserverGFlags: &map[string]string{k: ybaRedacted}},
			}}

			got := restoreClustersRedactedGFlags(
				api, gflagsKnownAfterUpdate(prior, planned, tc.sent))

			if v := got[0].UserIntent.GetTserverGFlags()[k]; v != tc.want {
				t.Errorf("state value = %q, want %q", v, tc.want)
			}
		})
	}
}

func TestRedactedGFlagMatches(t *testing.T) {
	cases := []struct {
		redacted, value string
		want            bool
	}{
		{"REDACTED", "dummy-pw", true},
		{"plain", "plain", true},
		{"plain", "other", false},
		{`ldapbindpasswd=""REDACTED"" ldaptls=0`, `ldapbindpasswd=""a b"" ldaptls=0`, true},
		// YBA quotes the token, and redacts the comment after an unquoted #.
		{`ldap ldapbindpasswd="REDACTED"#REDACTED`, `ldap ldapbindpasswd=""pw#x""`, true},
		{`ldapserver=a ldapbindpasswd=REDACTED`, `ldapserver=b ldapbindpasswd=pw`, false},
		{`a=REDACTED b=REDACTED`, `a=x b=y`, true},
		{`a=REDACTED b=REDACTED`, `a=x c=y`, false},
		{"abREDACTEDba", "aba", false},
	}
	for _, c := range cases {
		if got := redactedGFlagMatches(c.redacted, c.value); got != c.want {
			t.Errorf("redactedGFlagMatches(%q, %q) = %v, want %v", c.redacted, c.value, got, c.want)
		}
	}
}

// YBA returns the nodes in a new order on every GET. Read must list them by
// node_idx, so that node_details_set[0] names the same node after each refresh.
func TestFlattenNodeDetailsSetOrder(t *testing.T) {
	node := func(idx int32, name string) client.NodeDetailsResp {
		return client.NodeDetailsResp{
			NodeIdx: &idx, NodeName: &name, CloudInfo: &client.CloudSpecificInfo{},
		}
	}
	var got []string
	for _, n := range flattenNodeDetailsSet([]client.NodeDetailsResp{
		node(10, "n10"), node(2, "n2"), node(1, "n1"),
	}) {
		got = append(got, *n.(map[string]interface{})["node_name"].(*string))
	}
	if want := []string{"n1", "n2", "n10"}; !slices.Equal(got, want) {
		t.Errorf("node order = %v, want %v", got, want)
	}
}
