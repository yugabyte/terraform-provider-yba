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
	"io"
	"net/http"
)

// CreateSelfSignedCertificate asks YBA to mint a new self-signed root certificate
// with the given label and returns the new certificate's UUID.
//
// The generated client's CertificateInfoAPI.CreateSelfSignedCert cannot be used:
// it marshals the request body as the bare label string, while the backend reads
// the label from a JSON object ({"label": "..."}), so every call through the
// generated method fails with "Certificate label can't be null".
func (vc *VanillaClient) CreateSelfSignedCertificate(
	ctx context.Context, cUUID string, token string, label string,
) (string, error) {
	url := fmt.Sprintf("api/v1/customers/%s/certificates/create_self_signed_cert", cUUID)
	body, err := json.Marshal(map[string]string{"label": label})
	if err != nil {
		return "", err
	}
	resp, err := vc.makeRequest(ctx, http.MethodPost, url, bytes.NewBuffer(body), token)
	if err != nil {
		return "", fmt.Errorf("create self signed certificate request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if httpErr := vanillaHTTPError(resp, "Certificate", "Create"); httpErr != nil {
		return "", httpErr
	}
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	// The endpoint returns the new certificate UUID as a bare JSON string.
	var certUUID string
	if err = json.Unmarshal(respBytes, &certUUID); err != nil {
		return "", fmt.Errorf("unmarshal self signed certificate response: %w", err)
	}
	return certUUID, nil
}
