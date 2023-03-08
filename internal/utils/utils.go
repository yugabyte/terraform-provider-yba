package utils

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	client "github.com/yugabyte/platform-go-client"
)

// StringSlice accepts array of interface and returns a pointer to slice of string
func StringSlice(in []interface{}) *[]string {
	var out []string
	for _, v := range in {
		out = append(out, v.(string))
	}
	return &out
}

// StringMap accepts a string -> interface map and returns pointer to string -> string map
func StringMap(in map[string]interface{}) *map[string]string {
	out := make(map[string]string)
	for k, v := range in {
		out[k] = v.(string)
	}
	return &out
}

// MapFromSingletonList returns a map of string -> interface from a slice of interface
func MapFromSingletonList(in []interface{}) map[string]interface{} {
	if len(in) == 0 {
		return make(map[string]interface{})
	}
	return in[0].(map[string]interface{})
}

// GetBoolPointer returns a pointer to bool value
func GetBoolPointer(in bool) *bool {
	if !in {
		return nil
	}
	return &in
}

// GetStringPointer returns a pointer to string value
func GetStringPointer(in string) *string {
	if in == "" {
		return nil
	}
	return &in
}

// GetInt32Pointer returns a pointer to int32 value
func GetInt32Pointer(in int32) *int32 {
	if in == 0 {
		return nil
	}
	return &in
}

// GetInt64Pointer returns a pointer to int64 value
func GetInt64Pointer(in int64) *int64 {
	if in == 0 {
		return nil
	}
	return &in
}

// GetFloat64Pointer returns a pointer to float64 type
func GetFloat64Pointer(in float64) *float64 {
	if in == 0 {
		return nil
	}
	return &in
}

// CreateSingletonList returns a list of single entry from an interface
func CreateSingletonList(in interface{}) []interface{} {
	return []interface{}{in}
}

var (
	// PendingTaskStates lists incomplete task states
	PendingTaskStates = []string{"Created", "Initializing", "Running"}
	// SuccessTaskStates lists successful task states
	SuccessTaskStates = []string{"Success"}
)

// WaitForTask waits for State change for a YBA task
func WaitForTask(ctx context.Context, tUUID string, cUUID string, c *client.APIClient,
	timeout time.Duration) error {
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
