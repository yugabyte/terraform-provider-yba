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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// ReleaseUpdateArtifact is one artifact in a ReleaseUpdateRequest. Not the
// generated model: it always sends `"package_url": ""`, YBA's update saves any
// non-null field, and a non-null URL wins over the uploaded file. omitempty
// leaves a field unchanged.
type ReleaseUpdateArtifact struct {
	Platform      string `json:"platform"`
	Architecture  string `json:"architecture,omitempty"`
	PackageFileID string `json:"package_file_id,omitempty"`
	PackageURL    string `json:"package_url,omitempty"`
	Sha256        string `json:"sha256,omitempty"`
}

// ReleaseUpdateRequest is the PUT /ybdb_release/{rUUID} body. YBA deletes
// every artifact missing from Artifacts. ReleaseDate is in seconds (create
// takes msecs); zero keeps the stored date. Tag and notes are always sent so
// clearing them in config reaches YBA.
type ReleaseUpdateRequest struct {
	Artifacts    []ReleaseUpdateArtifact `json:"artifacts"`
	ReleaseDate  int64                   `json:"release_date,omitempty"`
	ReleaseNotes string                  `json:"release_notes"`
	ReleaseTag   string                  `json:"release_tag"`
	State        string                  `json:"state,omitempty"`
}

// UpdateRelease replaces a release's fields and artifact set.
func (vc *VanillaClient) UpdateRelease(
	ctx context.Context,
	cUUID string,
	apiKey string,
	releaseUUID string,
	request ReleaseUpdateRequest,
) error {
	reqBytes, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("marshal release update request: %w", err)
	}

	path := fmt.Sprintf("api/v1/customers/%s/ybdb_release/%s", cUUID, releaseUUID)
	r, err := vc.makeRequest(ctx, http.MethodPut, path, bytes.NewReader(reqBytes), apiKey)
	if err != nil {
		return fmt.Errorf("release update request failed: %w", err)
	}
	defer func() { _ = r.Body.Close() }()

	return utils.CheckHTTPError(r, "Update Release")
}
