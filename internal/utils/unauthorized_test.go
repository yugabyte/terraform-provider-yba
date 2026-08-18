// Licensed to YugabyteDB, Inc. under one or more contributor license
// agreements. See the NOTICE file distributed with this work for
// additional information regarding copyright ownership. Yugabyte
// licenses this file to you under the Mozilla License, Version 2.0
// (the "License"); you may not use this file except in compliance
// with the License.  You may obtain a copy of the License at
// http://mozilla.org/MPL/2.0/.
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package utils

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func httpErrorResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestErrorFromHTTPResponse401WrapsSentinel(t *testing.T) {
	resp := httpErrorResponse(http.StatusUnauthorized, `{"error":"Invalid token"}`)
	defer func() { _ = resp.Body.Close() }()
	err := ErrorFromHTTPResponse(
		resp, errors.New("401 Unauthorized"), ResourceEntity, "Customer", "Read")
	if !IsHTTPUnauthorizedError(err) {
		t.Errorf("expected 401 error to wrap ErrHTTPUnauthorized, got: %v", err)
	}
	if !strings.Contains(err.Error(), "authentication failed (HTTP 401)") {
		t.Errorf("unexpected message: %v", err)
	}
}

func TestIsHTTPUnauthorizedErrorRejectsOthers(t *testing.T) {
	resp := httpErrorResponse(http.StatusInternalServerError, `{"error":"boom"}`)
	defer func() { _ = resp.Body.Close() }()
	err := ErrorFromHTTPResponse(
		resp, errors.New("500"), ResourceEntity, "Customer", "Read")
	if IsHTTPUnauthorizedError(err) {
		t.Errorf("500 error must not match ErrHTTPUnauthorized: %v", err)
	}
	if IsHTTPUnauthorizedError(nil) {
		t.Error("nil must not match ErrHTTPUnauthorized")
	}
}
