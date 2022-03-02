package utils

import (
	"context"
	"github.com/go-openapi/strfmt"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	client "github.com/yugabyte/platform-go-client"
	"time"
)

func StringSlice(in []interface{}) *[]string {
	var out []string
	for _, v := range in {
		out = append(out, v.(string))
	}
	return &out
}

func UUIDSlice(in []interface{}) (out []strfmt.UUID) {
	for _, v := range in {
		out = append(out, strfmt.UUID(v.(string)))
	}
	return out
}

func StringMap(in map[string]interface{}) *map[string]string {
	out := make(map[string]string)
	for k, v := range in {
		out[k] = v.(string)
	}
	return &out
}

func MapFromSingletonList(in []interface{}) map[string]interface{} {
	if len(in) == 0 {
		return make(map[string]interface{})
	}
	return in[0].(map[string]interface{})
}

func GetBoolPointer(in bool) *bool {
	return &in
}

func GetStringPointer(in string) *string {
	return &in
}

func GetInt32Pointer(in int32) *int32 {
	return &in
}

func CreateSingletonList(in interface{}) []interface{} {
	return []interface{}{in}
}

func GetUUIDPointer(in string) *strfmt.UUID {
	out := strfmt.UUID(in)
	return &out
}

var PendingTaskStates = []string{"Created", "Initializing", "Running"}
var SuccessTaskStates = []string{"Success"}

func WaitForTask(ctx context.Context, tUUID string, cUUID string, c *client.APIClient, timeout time.Duration) error {
	wait := &resource.StateChangeConf{
		Delay:   1 * time.Second,
		Pending: PendingTaskStates,
		Target:  SuccessTaskStates,
		Timeout: timeout,

		Refresh: func() (result interface{}, state string, err error) {
			r, _, err := c.CustomerTasksApi.TaskStatus(ctx, cUUID, tUUID).Execute()
			if err != nil {
				return nil, "", err
			}

			// TODO: figure out why this is a nested map
			s := r["body"]["status"].(string)
			return s, s, nil
		},
	}

	if _, err := wait.WaitForStateContext(ctx); err != nil {
		return err
	}

	return nil
}
