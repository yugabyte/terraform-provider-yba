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

// Package releases provides resources and data sources related to YugabyteDB
// releases and version metadata.
package releases

import (
	"context"
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"

	"github.com/yugabyte/terraform-provider-yba/internal/api"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
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
				ValidateDiagFunc: validation.ToDiagFunc(
					validation.StringInSlice([]string{"stable", "preview"}, false)),
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

	_, response, err := c.ReleaseManagementAPI.Refresh(ctx, cUUID).Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.DataSourceEntity,
			"Release Version", "Read - Refresh")
		return diag.FromErr(errMessage)
	}

	r, response, err := c.ReleaseManagementAPI.GetListOfReleases(ctx, cUUID).IncludeMetadata(
		true).Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.DataSourceEntity,
			"Release Version", "Read")
		return diag.FromErr(errMessage)
	}

	versionsStable := make([]string, 0)
	versionsPreview := make([]string, 0)
	for v := range r {
		if utils.IsVersionStable(v) {
			versionsStable = append(versionsStable, v)
		} else {
			versionsPreview = append(versionsPreview, v)
		}
	}
	// SortStableFunc expects a comparison function (negative when x should sort
	// before y). To get the latest release first we sort descending, so we
	// return the negated version comparison and treat errors as "equal".
	sortDescending := func(x, y string) int {
		compare, err := utils.CompareYbVersions(x, y)
		if err != nil {
			return 0
		}
		return -compare
	}
	slices.SortStableFunc(versionsStable, sortDescending)
	slices.SortStableFunc(versionsPreview, sortDescending)

	var versions []string
	releaseTrack := d.Get("track").(string)
	if len(releaseTrack) != 0 {
		if strings.Compare("stable", releaseTrack) == 0 {
			versions = versionsStable
		} else {
			versions = versionsPreview
		}
	} else {
		versions = make([]string, 0, len(versionsStable)+len(versionsPreview))
		versions = append(versions, versionsStable...)
		versions = append(versions, versionsPreview...)
	}

	if d.Get("version").(string) == "" {
		if err := d.Set("version_list", versions); err != nil {
			return diag.FromErr(err)
		}
	} else {
		matchedVersions := make([]string, 0)
		for _, version := range versions {
			if strings.HasPrefix(version, d.Get("version").(string)) {
				matchedVersions = append(matchedVersions, version)
			}
		}
		if err := d.Set("version_list", matchedVersions); err != nil {
			return diag.FromErr(err)
		}
	}

	listVersions := d.Get("version_list").([]interface{})
	if len(listVersions) > 0 {
		version := listVersions[0].(string)
		d.SetId(version)
		if err := d.Set("selected_version", version); err != nil {
			return diag.FromErr(err)
		}
	}
	return diags
}
