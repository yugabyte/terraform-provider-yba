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
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// ReleaseResponse handles the resturn value of the releases endpoint
type ReleaseResponse struct {
	Success bool `json:"success"`
}

// ReleaseImport uses REST API to call import release functionality
func (vc *VanillaClient) ReleaseImport(ctx context.Context, cUUID string, version string,
	s3 map[string]interface{}, gcs map[string]interface{}, https map[string]interface{},
	token string) (bool, error) {
	mapping := make(map[string]interface{})

	if len(s3) != 0 {
		mapping = map[string]interface{}{
			version: map[string]interface{}{
				"s3": s3,
			},
		}
	} else if len(gcs) != 0 {
		mapping = map[string]interface{}{
			version: map[string]interface{}{
				"gcs": gcs,
			},
		}
	} else if len(https) != 0 {
		mapping = map[string]interface{}{
			version: map[string]interface{}{
				"http": https,
			},
		}
	} else {
		return false, fmt.Errorf("Request body empty")
	}

	reqBytes, err := json.Marshal(mapping)
	if err != nil {
		return false, err
	}

	reqBuf := bytes.NewBuffer(reqBytes)

	var req *http.Request
	if vc.EnableHTTPS {
		http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
		req, err = http.NewRequest("POST", fmt.Sprintf("https://%s/api/v1/customers/%s/releases",
			vc.Host, cUUID), reqBuf)
	} else {
		req, err = http.NewRequest("POST", fmt.Sprintf("http://%s/api/v1/customers/%s/releases",
			vc.Host, cUUID), reqBuf)
	}
	if err != nil {
		return false, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-AUTH-YW-API-TOKEN", token)

	r, err := vc.Client.Do(req)
	if err != nil {
		err = fmt.Errorf("Error occured during Post call for Import Release %s", err.Error())
		return false, err
	}

	var body []byte
	body, err = io.ReadAll(r.Body)
	if err != nil {
		err = fmt.Errorf("Error reading Import Release response body %s", err.Error())
		return false, err
	}

	responseBody := utils.YbaStructuredError{}
	if err = json.Unmarshal(body, &responseBody); err != nil {
		return false, fmt.Errorf("%s %s",
			"Failed unmarshalling Import Release Response body", err.Error())
	}

	if *responseBody.Success {
		return true, nil
	}

	errorMessage := utils.ErrorFromResponseBody(responseBody)
	return false, fmt.Errorf("Error importing release: %s", errorMessage)

}
