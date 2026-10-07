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

package universe

import (
	"testing"

	client "github.com/yugabyte/platform-go-client"

	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// A valid on-prem cluster must pass: a check that always fails blocks every
// plan for an on-prem universe. Each on-prem rule fails on its own, and other
// provider types skip the rules.
func TestValidateOnPremUserIntent(t *testing.T) {
	intent := func(
		providerType string, tags map[string]string, mountPoints, storageType string,
	) client.UserIntent {
		di := &client.DeviceInfo{}
		if mountPoints != "" {
			di.MountPoints = utils.GetStringPointer(mountPoints)
		}
		if storageType != "" {
			di.StorageType = utils.GetStringPointer(storageType)
		}
		return client.UserIntent{
			ProviderType: utils.GetStringPointer(providerType),
			InstanceTags: &tags,
			DeviceInfo:   di,
		}
	}
	cases := []struct {
		name    string
		ui      client.UserIntent
		wantErr bool
	}{
		{"valid onprem", intent("onprem", nil, "/mnt/d0", ""), false},
		{"onprem with instance tags", intent("onprem", map[string]string{"k": "v"}, "/mnt/d0", ""),
			true},
		{"onprem without mount points", intent("onprem", nil, "", ""), true},
		{"onprem with storage type", intent("onprem", nil, "/mnt/d0", "GP3"), true},
		{
			"aws skips the onprem rules",
			intent("aws", map[string]string{"k": "v"}, "", "GP3"),
			false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateOnPremUserIntent(c.ui, "primary")
			if (err != nil) != c.wantErr {
				t.Fatalf("validateOnPremUserIntent() error = %v, wantErr %v", err, c.wantErr)
			}
		})
	}
}
