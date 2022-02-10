package cloud_provider

import (
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func ConfigSchema() *schema.Schema {
	return &schema.Schema{
		Type:     schema.TypeMap,
		Elem:     &schema.Schema{Type: schema.TypeString},
		Optional: true,
	}
}

func ComputedConfigSchema() *schema.Schema {
	return &schema.Schema{
		Type:     schema.TypeMap,
		Elem:     &schema.Schema{Type: schema.TypeString},
		Optional: true,
		Computed: true,
	}
}

//func ConfigDiff(_ context.Context, diff *schema.ResourceDiff, meta interface{}) error {
//	oldConfig, newConfig := diff.GetChange("config")
//}
