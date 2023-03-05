package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// ReleaseResponse handles the resturn value of the releases endpoint
type ReleaseResponse struct {
	Success bool `json:"success"`
}

// ReleaseImport uses REST API to call import release functionality
func (vc *VanillaClient) ReleaseImport(ctx context.Context, cUUID string, version string,
	s3 map[string]interface{}, gcs map[string]interface{}, https map[string]interface{},
	token string) (bool, error) {
	mapping := make(map[string]interface{})

	if len(s3) != 0 {
		mapping = map[string]interface{}{
			version: map[string]interface{}{
				"s3": s3,
			},
		}
	} else if len(gcs) != 0 {
		mapping = map[string]interface{}{
			version: map[string]interface{}{
				"gcs": gcs,
			},
		}
	} else if len(https) != 0 {
		mapping = map[string]interface{}{
			version: map[string]interface{}{
				"http": https,
			},
		}
	} else {
		return false, fmt.Errorf("Request body empty")
	}

	reqBytes, err := json.Marshal(mapping)
	if err != nil {
		return false, err
	}

	reqBuf := bytes.NewBuffer(reqBytes)

	req, err := http.NewRequest("POST", fmt.Sprintf("http://%s/api/v1/customers/%s/releases",
		vc.Host, cUUID), reqBuf)

	if err != nil {
		return false, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-AUTH-YW-API-TOKEN", token)

	r, err := vc.Client.Do(req)
	if err != nil {
		return false, err
	}

	_, err = io.ReadAll(r.Body)

	if err != nil {
		tflog.Info(ctx, fmt.Sprint("ERROR: "+err.Error()))
		return false, err
	}
	return true, nil
}
