package releases

import (
	"context"
	"sort"

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
				Required:    true,
				Description: "Release Version",
			},
		},
	}
}

type ReleaseResponse struct {
	Version     string `json:"version"`
	Data 		interface{} `json:"metadata"`
}

func dataSourceReleaseVersionRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	c := meta.(*api.ApiClient).YugawareClient
	cUUID := meta.(*api.ApiClient).CustomerId

	r, _, err := c.ReleaseManagementApi.GetListOfReleases(ctx, cUUID).IncludeMetadata(true).Execute()
	if err != nil {
		return diag.FromErr(err)
	}

	if d.Get("version").(string) ==  "" {

		versions := make([]string, 0, len(r))
 
    	for v := range r{
        	versions = append(versions, v)
    	}
    	sort.Sort(sort.Reverse(sort.StringSlice(versions)))

		for _, version := range versions {
			d.SetId(version)
			d.Set("version", version)
			break
		}
	} else {
		d.SetId(d.Get("version").(string))
	}
	return diags
}
