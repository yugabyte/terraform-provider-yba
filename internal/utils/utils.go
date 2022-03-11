package utils

import (
	"context"
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

func StringMap(in map[string]interface{}) *map[string]string {
	out := make(map[string]string)
	for k, v := range in {
		out[k] = v.(string)
	}
	return &out
}

func GetStringMap(in *map[string]string) map[string]string {
	if in != nil {
		return *in
	}
	return map[string]string{}
}

func MapFromSingletonList(in []interface{}) map[string]interface{} {
	if len(in) == 0 {
		return make(map[string]interface{})
	}
	return in[0].(map[string]interface{})
}

func GetBoolPointer(in bool) *bool {
	if !in {
		return nil
	}
	return &in
}

func GetStringPointer(in string) *string {
	if in == "" {
		return nil
	}
	return &in
}

func GetInt32Pointer(in int32) *int32 {
	if in == 0 {
		return nil
	}
	return &in
}

func GetInt64Pointer(in int64) *int64 {
	if in == 0 {
		return nil
	}
	return &in
}

func CreateSingletonList(in interface{}) []interface{} {
	return []interface{}{in}
}

var (
	PendingTaskStates = []string{"Created", "Initializing", "Running"}
	SuccessTaskStates = []string{"Success"}
)

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

			s := r["status"].(string)
			return s, s, nil
		},
	}

	if _, err := wait.WaitForStateContext(ctx); err != nil {
		return err
	}

	return nil
}
