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

// ReleaseUpdateArtifact is one artifact in a ReleaseUpdateRequest. The
// generated client's UpdateRelease model cannot be used for this endpoint:
// its Artifact type always serializes every field, and YBA's update handler
// treats any non-null value as "change this" — an always-present
// `"package_url": ""` overwrites the null package URL of a file-based
// artifact, after which YBA prefers the (now empty) URL over the stored file
// when provisioning universes. omitempty keeps absent fields absent, which
// YBA reads as "leave unchanged".
type ReleaseUpdateArtifact struct {
	Platform      string `json:"platform"`
	Architecture  string `json:"architecture,omitempty"` // absent for KUBERNETES (must be null)
	PackageFileID string `json:"package_file_id,omitempty"`
	PackageURL    string `json:"package_url,omitempty"`
	Sha256        string `json:"sha256,omitempty"`
}

// ReleaseUpdateRequest is the body of PUT /ybdb_release/{rUUID}. Artifacts is
// the complete desired set: YBA deletes any existing artifact whose
// (platform, architecture) pair is missing from it — including ALL artifacts
// when the field is null, which is why it carries no omitempty and callers
// must always populate it. ReleaseDate is in SECONDS (create uses msecs) and
// is omitted when zero so a date-less release keeps its null date instead of
// being reset to the epoch. Tag and notes are always sent so that clearing
// them in config clears them server-side. State is omitted when empty so YBA
// leaves it untouched.
type ReleaseUpdateRequest struct {
	Artifacts    []ReleaseUpdateArtifact `json:"artifacts"`
	ReleaseDate  int64                   `json:"release_date,omitempty"`
	ReleaseNotes string                  `json:"release_notes"`
	ReleaseTag   string                  `json:"release_tag"`
	State        string                  `json:"state,omitempty"`
}

// UpdateRelease calls PUT /api/v1/customers/{cUUID}/ybdb_release/{rUUID}.
func (vc *VanillaClient) UpdateRelease(
	ctx context.Context,
	cUUID string,
	apiKey string,
	releaseUUID string,
	request ReleaseUpdateRequest,
) error {
	if request.Artifacts == nil {
		// A null artifacts field makes YBA delete every artifact on the release.
		request.Artifacts = []ReleaseUpdateArtifact{}
	}
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
