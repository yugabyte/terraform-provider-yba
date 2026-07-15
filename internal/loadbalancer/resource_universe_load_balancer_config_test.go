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

package loadbalancer

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	client "github.com/yugabyte/platform-go-client"
	clientv2 "github.com/yugabyte/platform-go-client/v2"

	"github.com/yugabyte/terraform-provider-yba/internal/api"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

const (
	testCustomer = "cust"
	testUniverse = "0b8f6f4e-6f36-4b19-9d0c-7c3d1d5d1111"
	testTask     = "5a3f0f9e-1111-4b19-9d0c-7c3d1d5d2222"
)

// updateLBRequest mirrors the JSON the resource PUTs to update_lb_config.
type updateLBRequest struct {
	UniverseUUID string           `json:"universeUUID"`
	Clusters     []client.Cluster `json:"clusters"`
}

// fakeYBA is a minimal YBA stand-in: it serves universe details, accepts
// update_lb_config PUTs (persisting the sent clusters so later GETs reflect
// them), and reports the dispatched task as succeeded.
type fakeYBA struct {
	mu              sync.Mutex
	clusters        []client.Cluster
	lbPuts          []updateLBRequest
	universeMissing bool
	// missingStatus/missingBody override the 404 shape when universeMissing is
	// set, to mimic YBA reporting a gone universe through non-404 responses.
	missingStatus int
	missingBody   string
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func (f *fakeYBA) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		path := strings.TrimSuffix(r.URL.Path, "/")
		base := "/api/v1/customers/" + testCustomer
		switch {
		case r.Method == http.MethodGet && path == base+"/universes/"+testUniverse:
			if f.universeMissing {
				status, body := f.missingStatus, f.missingBody
				if status == 0 {
					status = http.StatusNotFound
					body = `{"error":"Cannot find universe ` + testUniverse + `"}`
				}
				w.WriteHeader(status)
				_, _ = w.Write([]byte(body))
				return
			}
			writeJSON(w, map[string]interface{}{
				"universeUUID": testUniverse,
				"name":         "test-universe",
				"universeDetails": map[string]interface{}{
					"clusters": f.clusters,
				},
			})
		case r.Method == http.MethodPut &&
			path == base+"/universes/"+testUniverse+"/update_lb_config":
			var req updateLBRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":"bad update_lb_config body"}`))
				return
			}
			f.lbPuts = append(f.lbPuts, req)
			f.clusters = req.Clusters
			writeJSON(w, map[string]string{
				"taskUUID": testTask, "resourceUUID": testUniverse,
			})
		case r.Method == http.MethodGet && path == base+"/tasks/"+testTask:
			writeJSON(w, map[string]interface{}{
				"title":   "UpdateLoadBalancerConfig",
				"percent": 100.0,
				"status":  "Success",
				"details": map[string]interface{}{"taskDetails": []interface{}{}},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"unhandled ` + r.Method + " " + path + `"}`))
		}
	}
}

func newTestClient(t *testing.T, f *fakeYBA) *api.APIClient {
	t.Helper()
	srv := httptest.NewServer(f.handler())
	t.Cleanup(srv.Close)
	addr := strings.TrimPrefix(srv.URL, "http://")

	cfg := client.NewConfiguration()
	cfg.Scheme = "http"
	cfg.Host = addr
	cfgV2 := clientv2.NewConfiguration()
	cfgV2.Scheme = "http"
	cfgV2.Host = addr

	return &api.APIClient{
		VanillaClient: &api.VanillaClient{
			Client: srv.Client(), Host: addr, EnableHTTPS: false,
		},
		YugawareClient:   client.NewAPIClient(cfg),
		YugawareClientV2: clientv2.NewAPIClient(cfgV2),
		CustomerID:       testCustomer,
		APIKey:           "tok",
	}
}

// primaryCluster returns a universe primary cluster placed in us-west-2
// across two AZs, with no load balancer attached.
func primaryCluster() client.Cluster {
	return client.Cluster{
		Uuid:        utils.GetStringPointer("cl-primary"),
		ClusterType: "PRIMARY",
		UserIntent: client.UserIntent{
			UniverseName: utils.GetStringPointer("test-universe"),
		},
		PlacementInfo: &client.PlacementInfo{
			CloudList: []client.PlacementCloud{{
				Code: utils.GetStringPointer("aws"),
				Uuid: utils.GetStringPointer("prov-1"),
				RegionList: []client.PlacementRegion{{
					Code: utils.GetStringPointer("us-west-2"),
					Uuid: utils.GetStringPointer("reg-1"),
					AzList: []client.PlacementAZ{
						{
							Name: utils.GetStringPointer("us-west-2a"),
							Uuid: utils.GetStringPointer("az-1"),
						},
						{
							Name: utils.GetStringPointer("us-west-2b"),
							Uuid: utils.GetStringPointer("az-2"),
						},
					},
				}},
			}},
		},
	}
}

// TestResourceGuardrails pins the schema contract: these properties ship into
// customer state files and must not regress silently.
func TestResourceGuardrails(t *testing.T) {
	res := ResourceUniverseLoadBalancerConfig()

	if res.Importer == nil {
		t.Error("resource must support import")
	}
	if res.Description == "" {
		t.Error("resource description is required")
	}
	if _, ok := res.Schema["customer_uuid"]; ok {
		t.Error("customer_uuid is a noise field and must not be exposed")
	}

	uu := res.Schema["universe_uuid"]
	if !uu.Required || !uu.ForceNew {
		t.Error("universe_uuid must be Required and ForceNew")
	}

	lb := res.Schema["load_balancer"]
	if !lb.Required {
		t.Error("load_balancer must be Required")
	}
	if lb.ForceNew {
		t.Error("load_balancer must be updatable in place, not ForceNew")
	}
	if res.UpdateContext == nil {
		t.Error("resource must support in-place update")
	}

	for name, s := range res.Schema {
		if s.Description == "" {
			t.Errorf("field %s is missing a Description", name)
		}
	}
	for name, s := range lb.Elem.(*schema.Resource).Schema {
		if s.Description == "" {
			t.Errorf("load_balancer field %s is missing a Description", name)
		}
	}

	for op, timeout := range map[string]*time.Duration{
		"create": res.Timeouts.Create,
		"update": res.Timeouts.Update,
		"delete": res.Timeouts.Delete,
	} {
		if timeout == nil || *timeout != loadBalancerTaskTimeout {
			t.Errorf("%s timeout must default to loadBalancerTaskTimeout", op)
		}
	}
}

// attachedPrimaryCluster returns primaryCluster with an LB already attached:
// enableLB set, every AZ in us-west-2 pointing at lbName, region FQDN set.
func attachedPrimaryCluster(lbName, lbFQDN string) client.Cluster {
	cl := primaryCluster()
	cl.UserIntent.EnableLB = utils.GetBoolPointer(true)
	region := &cl.PlacementInfo.CloudList[0].RegionList[0]
	if lbFQDN != "" {
		region.LbFQDN = utils.GetStringPointer(lbFQDN)
	}
	for a := range region.AzList {
		region.AzList[a].LbName = utils.GetStringPointer(lbName)
	}
	return cl
}

func TestReadReflectsAttachedLoadBalancers(t *testing.T) {
	f := &fakeYBA{clusters: []client.Cluster{
		attachedPrimaryCluster("my-nlb", "nlb.example.com"),
	}}
	apiClient := newTestClient(t, f)

	res := ResourceUniverseLoadBalancerConfig()
	// Simulate a fresh import: only the ID is known.
	d := schema.TestResourceDataRaw(t, res.Schema, map[string]interface{}{})
	d.SetId(testUniverse)

	if diags := res.ReadContext(context.Background(), d, apiClient); diags.HasError() {
		t.Fatalf("read returned error: %v", diags)
	}
	if got := d.Get("universe_uuid").(string); got != testUniverse {
		t.Errorf("universe_uuid = %q, want %q", got, testUniverse)
	}

	blocks := d.Get("load_balancer").(*schema.Set).List()
	if len(blocks) != 1 {
		t.Fatalf("load_balancer blocks = %d, want 1", len(blocks))
	}
	block := blocks[0].(map[string]interface{})
	if block["region"] != "us-west-2" {
		t.Errorf("region = %q, want us-west-2", block["region"])
	}
	if block["lb_name"] != "my-nlb" {
		t.Errorf("lb_name = %q, want my-nlb", block["lb_name"])
	}
	if block["lb_fqdn"] != "nlb.example.com" {
		t.Errorf("lb_fqdn = %q, want nlb.example.com", block["lb_fqdn"])
	}
	if block["read_replica"] != false {
		t.Errorf("read_replica = %v, want false", block["read_replica"])
	}
	if overrides := block["az_overrides"].(map[string]interface{}); len(overrides) != 0 {
		t.Errorf("az_overrides = %v, want empty (all AZs share the region LB)", overrides)
	}
}

func TestReadMissingUniverseRemovesFromState(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
	}{
		{name: "404"},
		{
			name:   "400 with missing marker",
			status: http.StatusBadRequest,
			body:   `{"error":"Cannot find universe ` + testUniverse + `"}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeYBA{
				universeMissing: true,
				missingStatus:   tc.status,
				missingBody:     tc.body,
			}
			apiClient := newTestClient(t, f)

			res := ResourceUniverseLoadBalancerConfig()
			d := schema.TestResourceDataRaw(t, res.Schema, map[string]interface{}{})
			d.SetId(testUniverse)

			if diags := res.ReadContext(context.Background(), d, apiClient); diags.HasError() {
				t.Fatalf("read of missing universe should not error: %v", diags)
			}
			if d.Id() != "" {
				t.Fatalf("expected resource removed from state, ID = %q", d.Id())
			}
		})
	}
}

func TestDeleteDisablesLoadBalancers(t *testing.T) {
	f := &fakeYBA{clusters: []client.Cluster{
		attachedPrimaryCluster("my-nlb", "nlb.example.com"),
	}}
	apiClient := newTestClient(t, f)

	res := ResourceUniverseLoadBalancerConfig()
	d := schema.TestResourceDataRaw(t, res.Schema, map[string]interface{}{
		"universe_uuid": testUniverse,
	})
	d.SetId(testUniverse)

	if diags := res.DeleteContext(context.Background(), d, apiClient); diags.HasError() {
		t.Fatalf("delete returned error: %v", diags)
	}
	if d.Id() != "" {
		t.Errorf("resource ID after delete = %q, want empty", d.Id())
	}

	if len(f.lbPuts) != 1 {
		t.Fatalf("update_lb_config PUTs = %d, want 1", len(f.lbPuts))
	}
	cl := f.lbPuts[0].Clusters[0]
	if cl.UserIntent.EnableLB == nil || *cl.UserIntent.EnableLB {
		t.Error("payload must carry an explicit enableLB=false")
	}
	region := cl.PlacementInfo.CloudList[0].RegionList[0]
	if region.GetLbFQDN() != "" {
		t.Errorf("region lbFQDN = %q, want cleared", region.GetLbFQDN())
	}
	for _, az := range region.AzList {
		if az.GetLbName() != "" {
			t.Errorf("az %s lbName = %q, want cleared", az.GetName(), az.GetLbName())
		}
	}
}

func TestDeleteMissingUniverseIsIdempotent(t *testing.T) {
	f := &fakeYBA{universeMissing: true}
	apiClient := newTestClient(t, f)

	res := ResourceUniverseLoadBalancerConfig()
	d := schema.TestResourceDataRaw(t, res.Schema, map[string]interface{}{
		"universe_uuid": testUniverse,
	})
	d.SetId(testUniverse)

	if diags := res.DeleteContext(context.Background(), d, apiClient); diags.HasError() {
		t.Fatalf("delete of missing universe should not error: %v", diags)
	}
	if len(f.lbPuts) != 0 {
		t.Errorf("update_lb_config PUTs = %d, want 0 for a gone universe", len(f.lbPuts))
	}
}

func TestCreateHonorsAZOverrides(t *testing.T) {
	f := &fakeYBA{clusters: []client.Cluster{primaryCluster()}}
	apiClient := newTestClient(t, f)

	res := ResourceUniverseLoadBalancerConfig()
	d := schema.TestResourceDataRaw(t, res.Schema, map[string]interface{}{
		"universe_uuid": testUniverse,
		"load_balancer": []interface{}{
			map[string]interface{}{
				"region":  "us-west-2",
				"lb_name": "region-nlb",
				"az_overrides": map[string]interface{}{
					"us-west-2b": "zonal-nlb",
				},
			},
		},
	})

	if diags := res.CreateContext(context.Background(), d, apiClient); diags.HasError() {
		t.Fatalf("create returned error: %v", diags)
	}

	if len(f.lbPuts) != 1 {
		t.Fatalf("update_lb_config PUTs = %d, want 1", len(f.lbPuts))
	}
	region := f.lbPuts[0].Clusters[0].PlacementInfo.CloudList[0].RegionList[0]
	want := map[string]string{"us-west-2a": "region-nlb", "us-west-2b": "zonal-nlb"}
	for _, az := range region.AzList {
		if az.GetLbName() != want[az.GetName()] {
			t.Errorf("az %s lbName = %q, want %q",
				az.GetName(), az.GetLbName(), want[az.GetName()])
		}
	}
}

func TestReadRoundTripsAZOverrides(t *testing.T) {
	cl := attachedPrimaryCluster("region-nlb", "")
	// us-west-2b diverges from the region default.
	cl.PlacementInfo.CloudList[0].RegionList[0].AzList[1].LbName =
		utils.GetStringPointer("zonal-nlb")
	f := &fakeYBA{clusters: []client.Cluster{cl}}
	apiClient := newTestClient(t, f)

	res := ResourceUniverseLoadBalancerConfig()
	d := schema.TestResourceDataRaw(t, res.Schema, map[string]interface{}{})
	d.SetId(testUniverse)

	if diags := res.ReadContext(context.Background(), d, apiClient); diags.HasError() {
		t.Fatalf("read returned error: %v", diags)
	}

	blocks := d.Get("load_balancer").(*schema.Set).List()
	if len(blocks) != 1 {
		t.Fatalf("load_balancer blocks = %d, want 1", len(blocks))
	}
	block := blocks[0].(map[string]interface{})
	if block["lb_name"] != "region-nlb" {
		t.Errorf("lb_name = %q, want region-nlb (the majority name)", block["lb_name"])
	}
	overrides := block["az_overrides"].(map[string]interface{})
	if len(overrides) != 1 || overrides["us-west-2b"] != "zonal-nlb" {
		t.Errorf("az_overrides = %v, want {us-west-2b: zonal-nlb}", overrides)
	}
}

// readReplicaCluster returns an ASYNC cluster placed in us-west-2 with one AZ.
func readReplicaCluster() client.Cluster {
	return client.Cluster{
		Uuid:        utils.GetStringPointer("cl-rr"),
		ClusterType: "ASYNC",
		UserIntent: client.UserIntent{
			UniverseName: utils.GetStringPointer("test-universe"),
		},
		PlacementInfo: &client.PlacementInfo{
			CloudList: []client.PlacementCloud{{
				Code: utils.GetStringPointer("aws"),
				Uuid: utils.GetStringPointer("prov-1"),
				RegionList: []client.PlacementRegion{{
					Code: utils.GetStringPointer("us-west-2"),
					Uuid: utils.GetStringPointer("reg-1"),
					AzList: []client.PlacementAZ{{
						Name: utils.GetStringPointer("us-west-2c"),
						Uuid: utils.GetStringPointer("az-rr-1"),
					}},
				}},
			}},
		},
	}
}

func TestCreateAttachesToReadReplicaCluster(t *testing.T) {
	f := &fakeYBA{clusters: []client.Cluster{primaryCluster(), readReplicaCluster()}}
	apiClient := newTestClient(t, f)

	res := ResourceUniverseLoadBalancerConfig()
	d := schema.TestResourceDataRaw(t, res.Schema, map[string]interface{}{
		"universe_uuid": testUniverse,
		"load_balancer": []interface{}{
			map[string]interface{}{
				"region":       "us-west-2",
				"lb_name":      "rr-nlb",
				"read_replica": true,
			},
		},
	})

	if diags := res.CreateContext(context.Background(), d, apiClient); diags.HasError() {
		t.Fatalf("create returned error: %v", diags)
	}

	if len(f.lbPuts) != 1 {
		t.Fatalf("update_lb_config PUTs = %d, want 1", len(f.lbPuts))
	}
	for _, cl := range f.lbPuts[0].Clusters {
		switch cl.ClusterType {
		case "ASYNC":
			if !cl.UserIntent.GetEnableLB() {
				t.Error("read replica enableLB not set")
			}
			az := cl.PlacementInfo.CloudList[0].RegionList[0].AzList[0]
			if az.GetLbName() != "rr-nlb" {
				t.Errorf("read replica az lbName = %q, want rr-nlb", az.GetLbName())
			}
		case "PRIMARY":
			if cl.UserIntent.GetEnableLB() {
				t.Error("primary enableLB must stay false when only the RR is configured")
			}
			for _, az := range cl.PlacementInfo.CloudList[0].RegionList[0].AzList {
				if az.GetLbName() != "" {
					t.Errorf("primary az %s lbName = %q, want empty",
						az.GetName(), az.GetLbName())
				}
			}
		}
	}
}

func TestCreateFailsWhenUniverseHasNoReadReplica(t *testing.T) {
	f := &fakeYBA{clusters: []client.Cluster{primaryCluster()}}
	apiClient := newTestClient(t, f)

	res := ResourceUniverseLoadBalancerConfig()
	d := schema.TestResourceDataRaw(t, res.Schema, map[string]interface{}{
		"universe_uuid": testUniverse,
		"load_balancer": []interface{}{
			map[string]interface{}{
				"region":       "us-west-2",
				"lb_name":      "rr-nlb",
				"read_replica": true,
			},
		},
	})

	diags := res.CreateContext(context.Background(), d, apiClient)
	if !diags.HasError() {
		t.Fatal("create must fail when read_replica is set but the universe has no ASYNC cluster")
	}
	if len(f.lbPuts) != 0 {
		t.Errorf("update_lb_config PUTs = %d, want 0 on validation failure", len(f.lbPuts))
	}
	if d.Id() != "" {
		t.Errorf("resource ID = %q, want empty after failed create", d.Id())
	}
}

func TestUpdateRemapsLoadBalancers(t *testing.T) {
	f := &fakeYBA{clusters: []client.Cluster{
		attachedPrimaryCluster("my-nlb", "nlb.example.com"),
	}}
	apiClient := newTestClient(t, f)

	res := ResourceUniverseLoadBalancerConfig()
	// Config now points at a different LB and drops the FQDN.
	d := schema.TestResourceDataRaw(t, res.Schema, map[string]interface{}{
		"universe_uuid": testUniverse,
		"load_balancer": []interface{}{
			map[string]interface{}{"region": "us-west-2", "lb_name": "new-nlb"},
		},
	})
	d.SetId(testUniverse)

	if diags := res.UpdateContext(context.Background(), d, apiClient); diags.HasError() {
		t.Fatalf("update returned error: %v", diags)
	}

	if len(f.lbPuts) != 1 {
		t.Fatalf("update_lb_config PUTs = %d, want 1", len(f.lbPuts))
	}
	cl := f.lbPuts[0].Clusters[0]
	if !cl.UserIntent.GetEnableLB() {
		t.Error("enableLB must stay true after remap")
	}
	region := cl.PlacementInfo.CloudList[0].RegionList[0]
	for _, az := range region.AzList {
		if az.GetLbName() != "new-nlb" {
			t.Errorf("az %s lbName = %q, want new-nlb", az.GetName(), az.GetLbName())
		}
	}
	if region.GetLbFQDN() != "" {
		t.Errorf("lbFQDN = %q, want cleared (config no longer sets it)", region.GetLbFQDN())
	}
}

// twoRegionAttachedPrimary returns a primary cluster attached in both
// us-west-2 (west-nlb) and us-east-1 (east-nlb).
func twoRegionAttachedPrimary() client.Cluster {
	cl := attachedPrimaryCluster("west-nlb", "")
	cl.PlacementInfo.CloudList[0].RegionList = append(
		cl.PlacementInfo.CloudList[0].RegionList,
		client.PlacementRegion{
			Code: utils.GetStringPointer("us-east-1"),
			Uuid: utils.GetStringPointer("reg-2"),
			AzList: []client.PlacementAZ{{
				Name:   utils.GetStringPointer("us-east-1a"),
				Uuid:   utils.GetStringPointer("az-3"),
				LbName: utils.GetStringPointer("east-nlb"),
			}},
		},
	)
	return cl
}

func TestUpdateClearsRegionsRemovedFromConfig(t *testing.T) {
	f := &fakeYBA{clusters: []client.Cluster{twoRegionAttachedPrimary()}}
	apiClient := newTestClient(t, f)

	res := ResourceUniverseLoadBalancerConfig()
	// Config keeps only us-west-2; us-east-1 must be detached.
	d := schema.TestResourceDataRaw(t, res.Schema, map[string]interface{}{
		"universe_uuid": testUniverse,
		"load_balancer": []interface{}{
			map[string]interface{}{"region": "us-west-2", "lb_name": "west-nlb"},
		},
	})
	d.SetId(testUniverse)

	if diags := res.UpdateContext(context.Background(), d, apiClient); diags.HasError() {
		t.Fatalf("update returned error: %v", diags)
	}

	if len(f.lbPuts) != 1 {
		t.Fatalf("update_lb_config PUTs = %d, want 1", len(f.lbPuts))
	}
	regions := f.lbPuts[0].Clusters[0].PlacementInfo.CloudList[0].RegionList
	for _, region := range regions {
		for _, az := range region.AzList {
			want := ""
			if region.GetCode() == "us-west-2" {
				want = "west-nlb"
			}
			if az.GetLbName() != want {
				t.Errorf("region %s az %s lbName = %q, want %q",
					region.GetCode(), az.GetName(), az.GetLbName(), want)
			}
		}
	}
}

func TestCreateAttachesLoadBalancerToPrimaryCluster(t *testing.T) {
	f := &fakeYBA{clusters: []client.Cluster{primaryCluster()}}
	apiClient := newTestClient(t, f)

	res := ResourceUniverseLoadBalancerConfig()
	d := schema.TestResourceDataRaw(t, res.Schema, map[string]interface{}{
		"universe_uuid": testUniverse,
		"load_balancer": []interface{}{
			map[string]interface{}{
				"region":  "us-west-2",
				"lb_name": "my-nlb",
				"lb_fqdn": "nlb.example.com",
			},
		},
	})

	if diags := res.CreateContext(context.Background(), d, apiClient); diags.HasError() {
		t.Fatalf("create returned error: %v", diags)
	}
	if d.Id() != testUniverse {
		t.Fatalf("resource ID = %q, want universe UUID %q", d.Id(), testUniverse)
	}

	if len(f.lbPuts) != 1 {
		t.Fatalf("update_lb_config PUTs = %d, want 1", len(f.lbPuts))
	}
	sent := f.lbPuts[0]
	if sent.UniverseUUID != testUniverse {
		t.Errorf("payload universeUUID = %q, want %q", sent.UniverseUUID, testUniverse)
	}
	if len(sent.Clusters) != 1 {
		t.Fatalf("payload clusters = %d, want 1", len(sent.Clusters))
	}
	cl := sent.Clusters[0]
	if !cl.UserIntent.GetEnableLB() {
		t.Error("primary cluster enableLB not set in payload")
	}
	region := cl.PlacementInfo.CloudList[0].RegionList[0]
	if region.GetLbFQDN() != "nlb.example.com" {
		t.Errorf("region lbFQDN = %q, want %q", region.GetLbFQDN(), "nlb.example.com")
	}
	for _, az := range region.AzList {
		if az.GetLbName() != "my-nlb" {
			t.Errorf("az %s lbName = %q, want %q", az.GetName(), az.GetLbName(), "my-nlb")
		}
	}
}
