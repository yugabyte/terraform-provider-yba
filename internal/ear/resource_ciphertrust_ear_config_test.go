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

package ear

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

// The plan rejects credentials that do not match auth_type, and a missing
// secret when the apply would send it. After an import, the state holds no
// secret, and a plan that leaves the secret and auth_type unchanged sends
// nothing, so it must pass. A create body repeats the checks, for values that
// were unknown at plan time: TestResourceDataRaw skips the plan, as an
// unknown value does.
func TestCipherTrustAuthTypeGuard(t *testing.T) {
	imported := withState(ctState, map[string]string{"password": ""})
	cases := []struct {
		name    string
		state   map[string]string // nil: create
		raw     map[string]interface{}
		wantErr bool
	}{
		{
			name:    "create without the password",
			raw:     ctConfig(map[string]interface{}{"username": "yba"}),
			wantErr: true,
		},
		{
			name: "user name with a refresh token",
			raw: ctConfig(map[string]interface{}{
				"auth_type": "REFRESH_TOKEN", "refresh_token": "rt", "username": "yba",
			}),
			wantErr: true,
		},
		{
			name:  "unchanged secret after an import",
			state: imported,
			raw:   ctConfig(map[string]interface{}{"username": "yba"}),
		},
		{
			name:    "switch to a refresh token without one",
			state:   imported,
			raw:     ctConfig(map[string]interface{}{"auth_type": "REFRESH_TOKEN"}),
			wantErr: true,
		},
	}
	res := ResourceCipherTrustEARConfig()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var state *terraform.InstanceState
			if tc.state != nil {
				state = &terraform.InstanceState{
					ID: testConfigUUID, Attributes: map[string]string{"id": testConfigUUID},
				}
				for k, v := range tc.state {
					state.Attributes[k] = v
				}
			}
			_, err := res.Diff(context.Background(), state,
				terraform.NewResourceConfigRaw(tc.raw), nil)
			if (err != nil) != tc.wantErr {
				t.Errorf("plan error = %v, want an error: %t", err, tc.wantErr)
			}
			if tc.state == nil {
				_, err = ctBuildCreate(schema.TestResourceDataRaw(t, res.Schema, tc.raw))
				if (err != nil) != tc.wantErr {
					t.Errorf("create body error = %v, want an error: %t", err, tc.wantErr)
				}
			}
		})
	}
}
