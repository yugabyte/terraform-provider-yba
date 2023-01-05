package releases

import (
	"context"
	"sort"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/api"
)

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
				Type:        schema.TypeList,
				Elem:        &schema.Schema{Type: schema.TypeString},
				Computed:    true,
				Description: "List of releases matching the selected release. If selected_version is not provided, returns entire list",
			},
		},
	}
}

func dataSourceReleaseVersionRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	c := meta.(*api.ApiClient).YugawareClient
	cUUID := meta.(*api.ApiClient).CustomerId

	r, _, err := c.ReleaseManagementApi.GetListOfReleases(ctx, cUUID).IncludeMetadata(true).Execute()
	if err != nil {
		return diag.FromErr(err)
	}

	versions := make([]string, 0)
	for v := range r {
		versions = append(versions, v)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(versions)))

	if d.Get("version").(string) == "" {
		d.Set("version_list", versions)
	} else {
		matched_versions := make([]string, 0)
		for _, version := range versions {
			if strings.HasPrefix(version, d.Get("version").(string)) {
				matched_versions = append(matched_versions, version)
			}
		}
		d.Set("version_list", matched_versions)
	}

	list_versions := d.Get("version_list").([]interface{})
	for _, version := range list_versions {
		d.SetId(version.(string))
		d.Set("selected_version", version.(string))
		break
	}
	return diags
}
