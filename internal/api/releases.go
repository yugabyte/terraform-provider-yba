package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/hashicorp/terraform-plugin-log/tflog"
)

type ReleaseResponse struct {
	Success bool `json:"success"`
}

func (vc *VanillaClient) ReleaseImport(ctx context.Context, cUUID string, version string, s3 map[string]interface{}, gcs map[string]interface{}, https map[string]interface{}, token string) (error, bool) {
	mapping := make(map[string]interface{})

	if len(s3) != 0 {
		mapping = map[string]interface{}{
			version : map[string]interface{}{
				"s3": s3,
			},
		}
	} else if len(gcs) != 0 {
		mapping = map[string]interface{}{
			version : map[string]interface{}{
				"gcs": gcs,
			},
		}
	} else if len(https) != 0 {
		mapping = map[string]interface{}{
			version : map[string]interface{}{
				"http": https,
			},
		}
	} else {
		return errors.New(fmt.Sprintf("Request body empty")), false;
	}

	reqBytes, err := json.Marshal(mapping)
	if err != nil {
		return err, false
	}

	reqBuf := bytes.NewBuffer(reqBytes)
	
	req, err := http.NewRequest("POST", fmt.Sprintf("http://%s/api/v1/customers/%s/releases", vc.Host, cUUID), reqBuf)
	
	if err != nil {
		return err, false
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-AUTH-YW-API-TOKEN", token)

	r, err := vc.Client.Do(req)
	if err != nil {
		return err, false
	}
	
	
	_, err = io.ReadAll(r.Body)

	if err != nil {
		tflog.Info(ctx, fmt.Sprint("ERROR: "+ err.Error()))
		return err, false
	}
	return nil, true
}
