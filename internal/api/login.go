package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
)

//LoginRequest to handle request body of REST API
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// LoginResponse to handle response body of REST API
type LoginResponse struct {
	AuthToken    string `json:"authToken"`
	CustomerUUID string `json:"customerUUID"`
	UserUUID     string `json:"userUUID"`
}

// Login calls REST API for logging into YBA
func (vc *VanillaClient) Login(ctx context.Context, email string, password string) (
	*LoginResponse, error) {
	req := LoginRequest{Email: email, Password: password}
	reqBytes, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	reqBuf := bytes.NewBuffer(reqBytes)

	res, err := vc.makeRequest(http.MethodPost, "api/v1/login", reqBuf, "")
	if err != nil {
		return nil, err
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	loginResp := LoginResponse{}
	err = json.Unmarshal(body, &loginResp)
	if err != nil {
		return nil, err
	}
	return &loginResp, nil
}
