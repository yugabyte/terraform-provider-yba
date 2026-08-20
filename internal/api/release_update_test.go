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

package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// Guards the omitempty contract: YBA's update handler treats any present
// field as "change this", so a stray `"package_url": ""` on a file-based
// artifact would overwrite its null URL and break provisioning from it.
func TestUpdateReleaseOmitsEmptyArtifactFields(t *testing.T) {
	var (
		gotMethod string
		gotPath   string
		gotBody   map[string]interface{}
	)
	vc, _ := newStubVanillaClient(t,
		func(w http.ResponseWriter, r *http.Request) {
			gotMethod = r.Method
			gotPath = r.URL.Path
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &gotBody)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true}`))
		})

	err := vc.UpdateRelease(context.Background(), "cust-1", "token", "rel-1",
		ReleaseUpdateRequest{
			Artifacts: []ReleaseUpdateArtifact{
				{
					Platform:      "LINUX",
					Architecture:  "x86_64",
					PackageFileID: "file-uuid-1",
					Sha256:        "abc123",
				},
				{
					Platform:   "KUBERNETES",
					PackageURL: "https://example.com/helm.tgz",
				},
			},
			ReleaseNotes: "",
			ReleaseTag:   "tag-1",
		})
	if err != nil {
		t.Fatalf("update error: %v", err)
	}
	if gotMethod != http.MethodPut {
		t.Errorf("expected PUT, got %s", gotMethod)
	}
	if !strings.HasSuffix(gotPath, "/customers/cust-1/ybdb_release/rel-1") {
		t.Errorf("unexpected path: %s", gotPath)
	}

	artifacts, ok := gotBody["artifacts"].([]interface{})
	if !ok || len(artifacts) != 2 {
		t.Fatalf("expected 2 artifacts in body, got %+v", gotBody["artifacts"])
	}
	fileArtifact := artifacts[0].(map[string]interface{})
	if fileArtifact["package_file_id"] != "file-uuid-1" || fileArtifact["sha256"] != "abc123" {
		t.Errorf("file artifact fields lost: %+v", fileArtifact)
	}
	if _, present := fileArtifact["package_url"]; present {
		t.Errorf("file artifact must not carry package_url: %+v", fileArtifact)
	}
	urlArtifact := artifacts[1].(map[string]interface{})
	if urlArtifact["package_url"] != "https://example.com/helm.tgz" {
		t.Errorf("url artifact fields lost: %+v", urlArtifact)
	}
	for _, key := range []string{"package_file_id", "sha256", "architecture"} {
		if _, present := urlArtifact[key]; present {
			t.Errorf("url/kubernetes artifact must not carry %s: %+v", key, urlArtifact)
		}
	}

	// Tag and notes always present (clearing them must reach YBA); empty
	// state and zero date omitted (YBA must leave them untouched).
	if gotBody["release_tag"] != "tag-1" {
		t.Errorf("release_tag lost: %+v", gotBody)
	}
	if _, present := gotBody["release_notes"]; !present {
		t.Errorf("release_notes must be sent even when empty: %+v", gotBody)
	}
	for _, key := range []string{"state", "release_date"} {
		if _, present := gotBody[key]; present {
			t.Errorf("empty %s must be omitted: %+v", key, gotBody)
		}
	}
}

func TestUpdateReleaseNeverSendsNullArtifacts(t *testing.T) {
	var gotBody map[string]json.RawMessage
	vc, _ := newStubVanillaClient(t,
		func(w http.ResponseWriter, r *http.Request) {
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &gotBody)
			_, _ = w.Write([]byte(`{"success":true}`))
		})

	if err := vc.UpdateRelease(context.Background(), "cust", "token", "rel",
		ReleaseUpdateRequest{ReleaseTag: "t"}); err != nil {
		t.Fatalf("update error: %v", err)
	}
	// A null artifacts field makes YBA delete every artifact on the release.
	if string(gotBody["artifacts"]) != "[]" {
		t.Errorf("nil artifacts must serialize as [], got %s", gotBody["artifacts"])
	}
}

func TestUpdateReleaseSurfacesErrors(t *testing.T) {
	vc, _ := newStubVanillaClient(t,
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"cannot remove artifacts from in-use release"}`))
		})

	err := vc.UpdateRelease(context.Background(), "cust", "token", "rel",
		ReleaseUpdateRequest{Artifacts: []ReleaseUpdateArtifact{{Platform: "LINUX"}}})
	if err == nil {
		t.Fatal("expected error on 400")
	}
	if !strings.Contains(err.Error(), "cannot remove artifacts") {
		t.Errorf("error did not surface response body: %v", err)
	}
}
