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
	"errors"

	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// AppVersion returns the YBA build this client talks to ("2.31.0.0-b395"),
// read once from GET /api/v1/app_version and cached for the life of the
// terraform run so every version gate shares one call. It is "" when the
// client has no server behind it: bootstrap mode without an api_token, or a
// bare client in unit tests.
func (c *APIClient) AppVersion(ctx context.Context) (string, error) {
	c.appVersionMu.Lock()
	defer c.appVersionMu.Unlock()
	if c.appVersion != "" || c.YugawareClient == nil {
		return c.appVersion, nil
	}
	r, resp, err := c.YugawareClient.SessionManagementAPI.AppVersion(ctx).Execute()
	if err != nil {
		return "", utils.ErrorFromHTTPResponse(resp, err, "Validation",
			"YBA Version", "Get App Version")
	}
	if r["version"] == "" {
		return "", errors.New("YugabyteDB Anywhere app_version response has no version")
	}
	c.appVersion = r["version"]
	return c.appVersion, nil
}

// SetAppVersion seeds the cache. Unit tests use it to pin a YBA build without
// a server.
func (c *APIClient) SetAppVersion(version string) {
	c.appVersionMu.Lock()
	defer c.appVersionMu.Unlock()
	c.appVersion = version
}
