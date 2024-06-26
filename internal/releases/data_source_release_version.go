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

package releases

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/yugabyte/terraform-provider-yba/internal/api"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
	"golang.org/x/exp/slices"
)

// ReleaseVersion data spurce keeps track of the imported releases on current YBA
func ReleaseVersion() *schema.Resource {
	return &schema.Resource{
		Description: "Retrieve release version.",

		ReadContext: dataSourceReleaseVersionRead,

		Schema: map[string]*schema.Schema{
			"version": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Release version given by user.",
			},
			"selected_version": {
				Type:     schema.TypeString,
				Computed: true,
				Description: "Selected release version. If version is empty, use " +
					"lastest version available.",
			},
			"track": {
				Type:     schema.TypeString,
				Optional: true,
				Description: "YugabyteDB release verion track. Allowed values: stable, preview." +
					" Uses the latest/user given version from the corresponding track.",
			},
			"version_list": {
				Type:     schema.TypeList,
				Elem:     &schema.Schema{Type: schema.TypeString},
				Computed: true,
				Description: "List of releases matching the selected release. " +
					"If selected_version is not provided, returns entire list.",
			},
		},
	}
}

func dataSourceReleaseVersionRead(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID

	_, response, err := c.ReleaseManagementApi.Refresh(ctx, cUUID).Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.DataSourceEntity,
			"Release Version", "Read - Refresh")
		return diag.FromErr(errMessage)
	}

	r, response, err := c.ReleaseManagementApi.GetListOfReleases(ctx, cUUID).IncludeMetadata(
		true).Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.DataSourceEntity,
			"Release Version", "Read")
		return diag.FromErr(errMessage)
	}

	versionsStable := make([]string, 0)
	versionsPreview := make([]string, 0)
	for v := range r {
		isStable, err := utils.IsVersionStable(v)
		if err != nil {
			diag.FromErr(err)
		}
		if isStable {
			versionsStable = append(versionsStable, v)
		} else {
			versionsPreview = append(versionsPreview, v)
		}
	}
	// the function as described in the documentation is the less function,
	// but for the purpose of getting the latest release, it's described as
	// a function returning the greater of the 2 versions
	slices.SortStableFunc(versionsStable, func(x, y string) bool {
		compare, err := utils.CompareYbVersions(x, y)
		if err != nil {
			return false
		}
		if compare == 0 || compare == -1 {
			return false
		}
		return true
	})

	slices.SortStableFunc(versionsPreview, func(x, y string) bool {
		compare, err := utils.CompareYbVersions(x, y)
		if err != nil {
			return false
		}
		if compare == 0 || compare == -1 {
			return false
		}
		return true
	})

	var versions []string
	releaseTrack := d.Get("track").(string)
	if len(releaseTrack) != 0 {
		if strings.Compare("stable", releaseTrack) == 0 {
			versions = versionsStable
		} else {
			versions = versionsPreview
		}
	} else {
		versions = append(versionsStable, versionsPreview...)
	}

	if d.Get("version").(string) == "" {
		d.Set("version_list", versions)
	} else {
		matchedVersions := make([]string, 0)
		for _, version := range versions {
			if strings.HasPrefix(version, d.Get("version").(string)) {
				matchedVersions = append(matchedVersions, version)
			}
		}
		d.Set("version_list", matchedVersions)
	}

	listVersions := d.Get("version_list").([]interface{})
	for _, version := range listVersions {
		d.SetId(version.(string))
		d.Set("selected_version", version.(string))
		break
	}
	return diags
}
