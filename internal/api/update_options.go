package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// UniverseUpdateOptions fetches the required update options for an EDIT operation
// by calling the universe_configure endpoint.
func (vc *VanillaClient) UniverseUpdateOptions(
	ctx context.Context,
	cUUID string,
	taskParams interface{},
	token string,
) ([]string, error) {

	reqBytes, err := json.Marshal(taskParams)
	if err != nil {
		return nil, fmt.Errorf("marshal universe_configure request: %w", err)
	}
	reqBuf := bytes.NewBuffer(reqBytes)

	// Target the universe_configure endpoint instead
	path := fmt.Sprintf("api/v1/customers/%s/universe_configure", cUUID)

	res, err := vc.makeRequest(http.MethodPost, path, reqBuf, token)
	if err != nil {
		return nil, fmt.Errorf("universe_configure request failed: %w", err)
	}
	defer res.Body.Close()

	if httpErr := utils.CheckHTTPError(res, "UniverseConfigureOptions"); httpErr != nil {
		return nil, httpErr
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading universe_configure response: %w", err)
	}

	// Unmarshal just the updateOptions array from the larger response object
	var configureResp struct {
		UpdateOptions []string `json:"updateOptions"`
	}
	if err := json.Unmarshal(body, &configureResp); err != nil {
		return nil, fmt.Errorf("error parsing universe_configure response (status %d): %w", res.StatusCode, err)
	}

	return configureResp.UpdateOptions, nil
}
