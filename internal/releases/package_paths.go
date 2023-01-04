package releases

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func PackagePathsSchema() *schema.Resource {
	return &schema.Resource{
		Schema: map[string]*schema.Schema{
			"x86_64": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "Path to x86_64 package",
			},
			"x86_64_checksum": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Checksum for x86_64 package",
			},
		},
	}
}

func formatInputPaths(ctx context.Context, paths interface{}) map[string]interface{}{
	
	path := make(map[string]interface{}) 
	for _, p := range paths.([]interface{}){
		path = p.(map[string]interface{})
	}
	return path
}
