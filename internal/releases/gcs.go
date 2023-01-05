package releases

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func GcsSchema() *schema.Resource {
	return &schema.Resource{
		Schema: map[string]*schema.Schema{
			"credentials_json": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "GCS Credentials in from json file",
			},
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

func formatInputGcs(ctx context.Context, data []interface{}) map[string]interface{} {

	gcs := make(map[string]interface{})
	for _, v := range data {
		gcs = v.(map[string]interface{})
		gcs["credentialsJson"] = gcs["credentials_json"]
		delete(gcs, "credentials_json")
		gcs["paths"] = formatInputPaths(ctx, gcs["paths"])

	}
	return gcs
}

func formatOutputGcs(ctx context.Context, gcs map[string]interface{}) []map[string]interface{} {

	gcs["credentials_json"] = gcs["credentialsJson"]
	delete(gcs, "credentialsJson")
	mapSlice := []map[string]interface{}{}
	gcs["paths"] = append(mapSlice, gcs["paths"].(map[string]interface{}))

	gcs_formatted := []map[string]interface{}{}
	gcs_formatted = append(gcs_formatted, gcs)
	return gcs_formatted
}
