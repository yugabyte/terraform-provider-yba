package releases

import (
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func PackageSchema() *schema.Schema {
	return &schema.Schema{
		Type:     schema.TypeList,
		ForceNew: true,
		Optional: true,
		Computed: true,
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				"path": {
					Type:        schema.TypeString,
					Computed:    true,
					Optional:    true,
					Description: "Path",
				},
				"arch": {
					Type:        schema.TypeString,
					Computed:    true,
					Optional:    true,
					Description: "Architecture",
				},
			},
		},
	}

}
