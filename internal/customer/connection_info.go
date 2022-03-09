package customer

import (
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func ConnectionInfoSchema() *schema.Schema {
	return &schema.Schema{
		Type:     schema.TypeList,
		MaxItems: 1,
		Required: true,
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				"api_token": {
					Type:     schema.TypeString,
					Required: true,
				},
				"cuuid": {
					Type:     schema.TypeString,
					Required: true,
					ForceNew: true,
				},
			},
		},
	}
}
