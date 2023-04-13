package releases

import (
	"context"
	"sort"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/api"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/utils"
)

// ReleaseVersion data spurce keeps track of the imported releases on current YBA
func ReleaseVersion() *schema.Resource {
	return &schema.Resource{
		Description: "Retrieve release version",

		ReadContext: dataSourceReleaseVersionRead,

		Schema: map[string]*schema.Schema{
			"version": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Release version given by user",
			},
			"selected_version": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "Selected release version. If version is empty, use lastest version available",
			},
			"version_list": {
				Type:     schema.TypeList,
				Elem:     &schema.Schema{Type: schema.TypeString},
				Computed: true,
				Description: "List of releases matching the selected release. If selected_version " +
					"is not provided, returns entire list",
			},
		},
	}
}

func dataSourceReleaseVersionRead(ctx context.Context, d *schema.ResourceData, meta interface{}) (
	diag.Diagnostics) {
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

	versions := make([]string, 0)
	for v := range r {
		versions = append(versions, v)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(versions)))

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
