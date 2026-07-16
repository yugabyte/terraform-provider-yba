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

	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// UpdateLoadBalancerConfig PUTs desired LB state to update_lb_config and
// returns the queued task UUID. Hand-rolled: the generated client marshals the
// payload as a query param, not a JSON body.
func (vc *VanillaClient) UpdateLoadBalancerConfig(
	ctx context.Context,
	cUUID string,
	uniUUID string,
	params interface{},
	token string,
) (string, *http.Response, error) {

	reqBytes, err := json.Marshal(params)
	if err != nil {
		return "", nil, fmt.Errorf("marshal update_lb_config request: %w", err)
	}

	path := fmt.Sprintf("api/v1/customers/%s/universes/%s/update_lb_config", cUUID, uniUUID)

	res, err := vc.makeRequest(ctx, http.MethodPut, path, bytes.NewBuffer(reqBytes), token)
	if err != nil {
		return "", nil, fmt.Errorf("update_lb_config request failed: %w", err)
	}
	defer func() { _ = res.Body.Close() }()

	if httpErr := utils.CheckHTTPError(res, "UpdateLoadBalancerConfig"); httpErr != nil {
		return "", res, httpErr
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return "", res, fmt.Errorf("error reading update_lb_config response: %w", err)
	}

	var task struct {
		TaskUUID string `json:"taskUUID"`
	}
	if err := json.Unmarshal(body, &task); err != nil {
		return "", res, fmt.Errorf(
			"error parsing update_lb_config response (status %d): %w", res.StatusCode, err)
	}

	return task.TaskUUID, res, nil
}
