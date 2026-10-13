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
	"fmt"
	"io"
	"net/http"
)

// GetKMSConfig returns the stored settings of an encryption-at-rest
// configuration, with credentials in clear.
//
// The generated client's EncryptionAtRestAPI.GetKMSConfig cannot be used: it
// decodes the response as map[string]map[string]interface{}, while YBA returns
// the flat settings object ({"CLIENT_ID": "...", ...}), so the decode fails on
// the first string setting.
func (vc *VanillaClient) GetKMSConfig(
	ctx context.Context, cUUID, configUUID, token string,
) (map[string]interface{}, error) {
	url := fmt.Sprintf("api/v1/customers/%s/kms_configs/%s", cUUID, configUUID)
	resp, err := vc.makeRequest(ctx, http.MethodGet, url, nil, token)
	if err != nil {
		return nil, fmt.Errorf("get KMS config request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if httpErr := vanillaHTTPError(resp, "Encryption At Rest Config", "Get"); httpErr != nil {
		return nil, httpErr
	}
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var settings map[string]interface{}
	if err = json.Unmarshal(respBytes, &settings); err != nil {
		return nil, fmt.Errorf("unmarshal KMS config response: %w", err)
	}
	return settings, nil
}
