package cloud_provider

import (
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func ConfigSchema() *schema.Schema {
	return &schema.Schema{
		Type:     schema.TypeMap,
		Elem:     &schema.Schema{Type: schema.TypeString},
		ForceNew: true,
		Optional: true,
	}
}

func ComputedConfigSchema() *schema.Schema {
	return &schema.Schema{
		Type:     schema.TypeMap,
		Elem:     &schema.Schema{Type: schema.TypeString},
		Optional: true,
		ForceNew: true,
		Computed: true,
	}
}
