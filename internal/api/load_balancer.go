package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// UpdateLBConfig calls PUT /universes/{uuid}/update_lb_config with the
// modified universeDetails payload. Returns the async task UUID.
func (vc *VanillaClient) UpdateLBConfig(
	cUUID, uUUID string,
	universeDetails interface{},
	token string,
) (string, error) {
	reqBytes, err := json.Marshal(universeDetails)
	if err != nil {
		return "", fmt.Errorf("marshal update_lb_config request: %w", err)
	}

	path := fmt.Sprintf("api/v1/customers/%s/universes/%s/update_lb_config", cUUID, uUUID)
	res, err := vc.makeRequest(http.MethodPut, path, bytes.NewBuffer(reqBytes), token)
	if err != nil {
		return "", fmt.Errorf("update_lb_config request failed: %w", err)
	}
	defer res.Body.Close()

	if httpErr := utils.CheckHTTPError(res, "UpdateLBConfig"); httpErr != nil {
		return "", httpErr
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return "", fmt.Errorf("error reading update_lb_config response: %w", err)
	}

	var resp struct {
		TaskUUID string `json:"taskUUID"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("error parsing update_lb_config response: %w", err)
	}

	return resp.TaskUUID, nil
}
