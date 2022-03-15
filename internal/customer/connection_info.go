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
					Type:        schema.TypeString,
					Required:    true,
					Description: "The API Token for the customer. This can be found in the YugabyteDB Anywhere Portal and is also returned when a customer resource is created",
				},
				"cuuid": {
					Type:        schema.TypeString,
					Required:    true,
					ForceNew:    true,
					Description: "UUID for the customer associated with the resource/data source.",
				},
			},
		},
	}
}
