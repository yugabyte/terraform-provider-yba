package utils

import (
	"context"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"reflect"
)

func ComputedValueDiff(key string) schema.CustomizeDiffFunc {
	return func(_ context.Context, diff *schema.ResourceDiff, _ interface{}) error {
		o, n := diff.GetChange(key)
		if !reflect.DeepEqual(o, n) {
			if err := diff.SetNewComputed(key); err != nil {
				return err
			}
		}
		return nil
	}
}
