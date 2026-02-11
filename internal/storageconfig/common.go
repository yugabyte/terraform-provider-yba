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

package storageconfig

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	client "github.com/yugabyte/platform-go-client"
	"github.com/yugabyte/terraform-provider-yba/internal/api"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// findStorageConfig finds a storage config by UUID and type
func findStorageConfig(configs []client.CustomerConfigUI, uuid string, configType string) (
	*client.CustomerConfigUI, error) {
	for _, c := range configs {
		if *c.ConfigUUID == uuid {
			if c.GetName() != configType {
				return nil, fmt.Errorf("storage config %s is not of type %s", uuid, configType)
			}
			return &c, nil
		}
	}
	return nil, fmt.Errorf("could not find storage config with id %s", uuid)
}

// resourceStorageConfigDelete is the common delete function for all storage configs
func resourceStorageConfigDelete(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{}) diag.Diagnostics {

	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID

	_, response, err := c.CustomerConfigurationAPI.DeleteCustomerConfig(ctx, cUUID, d.Id()).
		Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"Storage Config", "Delete")
		return diag.FromErr(errMessage)
	}

	d.SetId("")
	return nil
}
