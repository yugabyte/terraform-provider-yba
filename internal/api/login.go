package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
)

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginResponse struct {
	AuthToken    string `json:"authToken"`
	CustomerUUID string `json:"customerUUID"`
	UserUUID     string `json:"userUUID"`
}

func (vc *VanillaClient) Login(ctx context.Context, email string, password string) (error, *LoginResponse) {
	req := LoginRequest{Email: email, Password: password}
	reqBytes, err := json.Marshal(req)
	if err != nil {
		return err, nil
	}
	reqBuf := bytes.NewBuffer(reqBytes)

	res, err := vc.MakeRequest(http.MethodPost, "api/v1/login", reqBuf, "")
	if err != nil {
		return err, nil
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return err, nil
	}
	loginResp := LoginResponse{}
	err = json.Unmarshal(body, &loginResp)
	if err != nil {
		return err, nil
	}
	return nil, &loginResp
}
