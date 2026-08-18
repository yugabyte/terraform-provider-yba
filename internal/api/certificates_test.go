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

package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

// The generated client's CreateSelfSignedCert marshals the request body as a
// bare JSON string, which YBA rejects; the vanilla wrapper exists to send the
// {"label": ...} object the backend actually parses. This test locks that
// contract.
func TestCreateSelfSignedCertificateSendsLabelObject(t *testing.T) {
	var gotPath string
	var gotBody map[string]string
	vc, _ := newStubVanillaClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		if err := json.Unmarshal(body, &gotBody); err != nil {
			t.Fatalf("request body is not a JSON object: %s", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`"6db4c6f2-2c1b-4a3e-8a9f-3d3f2a1b0c9d"`))
	})

	uuid, err := vc.CreateSelfSignedCertificate(
		context.Background(), "cust-uuid", "token", "my-ca")
	if err != nil {
		t.Fatalf("CreateSelfSignedCertificate: %v", err)
	}
	if uuid != "6db4c6f2-2c1b-4a3e-8a9f-3d3f2a1b0c9d" {
		t.Errorf("uuid = %q, want bare-string UUID from response", uuid)
	}
	wantPath := "/api/v1/customers/cust-uuid/certificates/create_self_signed_cert"
	if gotPath != wantPath {
		t.Errorf("path = %q, want %q", gotPath, wantPath)
	}
	if gotBody["label"] != "my-ca" {
		t.Errorf(`body = %v, want {"label": "my-ca"}`, gotBody)
	}
}

func TestCreateSelfSignedCertificateSurfacesHTTPError(t *testing.T) {
	vc, _ := newStubVanillaClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"Certificate with name - my-ca already exists"}`))
	})

	_, err := vc.CreateSelfSignedCertificate(
		context.Background(), "cust-uuid", "token", "my-ca")
	if err == nil {
		t.Fatal("expected error for HTTP 400, got nil")
	}
}
