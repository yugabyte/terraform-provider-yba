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
	"encoding/json"
	"reflect"
	"testing"
)

// YBA's update saves every non-null field: an empty package_url on a file
// artifact would replace its null URL, and an omitted tag would never clear.
func TestReleaseUpdateRequestSendsOnlySetFields(t *testing.T) {
	req := ReleaseUpdateRequest{
		Artifacts: []ReleaseUpdateArtifact{
			{Platform: "LINUX", Architecture: "x86_64", PackageFileID: "file-1", Sha256: "sha-1"},
			{Platform: "KUBERNETES", PackageURL: "https://example.com/helm.tgz"},
		},
	}

	raw, err := json.Marshal(req)

	if err != nil {
		t.Fatal(err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]interface{}{
		"artifacts": []interface{}{
			map[string]interface{}{
				"platform": "LINUX", "architecture": "x86_64",
				"package_file_id": "file-1", "sha256": "sha-1",
			},
			map[string]interface{}{
				"platform": "KUBERNETES", "package_url": "https://example.com/helm.tgz",
			},
		},
		"release_notes": "",
		"release_tag":   "",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("body = %s\nwant %v", raw, want)
	}
}
