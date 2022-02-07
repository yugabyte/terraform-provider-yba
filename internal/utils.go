package utils

import (
	"context"
	"github.com/go-openapi/strfmt"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/yugabyte/yb-tools/yugaware-client/pkg/client"
	"github.com/yugabyte/yb-tools/yugaware-client/pkg/client/swagger/client/customer_tasks"
	"time"
)

func StringSlice(in []interface{}) (out []string) {
	for _, v := range in {
		out = append(out, v.(string))
	}
	return out
}

func UUIDSlice(in []interface{}) (out []strfmt.UUID) {
	for _, v := range in {
		out = append(out, strfmt.UUID(v.(string)))
	}
	return out
}

func StringMap(in map[string]interface{}) map[string]string {
	out := make(map[string]string)
	for k, v := range in {
		out[k] = v.(string)
	}
	return out
}

var PendingTaskStates = []string{"Created", "Initializing", "Running"}
var SuccessTaskStates = []string{"Success"}

func WaitForTask(ctx context.Context, tUUID strfmt.UUID, c *client.YugawareClient, timeout time.Duration) error {
	wait := &resource.StateChangeConf{
		Delay:   1 * time.Second,
		Pending: PendingTaskStates,
		Target:  SuccessTaskStates,
		Timeout: timeout,

		Refresh: func() (result interface{}, state string, err error) {
			r, err := c.PlatformAPIs.CustomerTasks.TaskStatus(&customer_tasks.TaskStatusParams{
				CUUID:      c.CustomerUUID(),
				TUUID:      tUUID,
				Context:    ctx,
				HTTPClient: c.Session(),
			},
				c.SwaggerAuth,
			)
			if err != nil {
				return nil, "", err
			}

			s := r.Payload["status"].(string)
			return s, s, nil
		},
	}

	if _, err := wait.WaitForStateContext(ctx); err != nil {
		return err
	}

	return nil
}
