package utils

import "github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

func ResourceSetIfExists(dest map[string]interface{}, src *schema.ResourceData, getKey string, setKey string) {
	if v, exists := src.GetOk(getKey); exists {
		dest[setKey] = v
	}
}

func MapSetIfExists(dest map[string]interface{}, src map[string]interface{}, getKey string, setKey string) {
	if v, exists := src[getKey]; exists {
		dest[setKey] = v
	}
}
