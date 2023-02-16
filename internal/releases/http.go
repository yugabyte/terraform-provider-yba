package releases

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func HttpSchema() *schema.Resource {
	return &schema.Resource{
		Schema: map[string]*schema.Schema{
			"paths": {
				Type:        schema.TypeList,
				MaxItems:    1,
				Required:    true,
				Elem:        PackagePathsSchema(),
				Description: "Package path and checksum",
			},
		},
	}
}

func formatInputHttp(ctx context.Context, data []interface{}) map[string]interface{} {

	http := make(map[string]interface{})
	for _, v := range data {
		http = v.(map[string]interface{})
		http["paths"] = formatInputPaths(ctx, http["paths"])

	}
	return http
}

func formatOutputHttp(ctx context.Context, http map[string]interface{}) []map[string]interface{} {

	mapSlice := []map[string]interface{}{}
	paths_formatted := formatOutputPaths(ctx, http["paths"].(map[string]interface{}))
	http["paths"] = append(mapSlice, paths_formatted)

	http_formatted := []map[string]interface{}{}
	http_formatted = append(http_formatted, http)
	return http_formatted
}
