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
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// writeTempReleaseFile creates a multi-MB file so that buffering or
// truncation bugs in the streaming body would surface as content mismatches.
func writeTempReleaseFile(t *testing.T) (string, []byte) {
	t.Helper()
	content := bytes.Repeat([]byte("yugabyte-release-payload-0123456789"), 100000) // ~3.4MB
	path := filepath.Join(t.TempDir(), "yugabyte-2.21.0.0-b1-linux-x86_64.tar.gz")
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatalf("write temp release file: %v", err)
	}
	return path, content
}

func TestUploadReleaseFile(t *testing.T) {
	path, content := writeTempReleaseFile(t)

	var (
		gotMethod        string
		gotPath          string
		gotToken         string
		gotContentLength int64
		gotChunked       bool
		gotFormName      string
		gotFileName      string
		gotFileContent   []byte
		gotExtraPart     bool
	)
	vc, _ := newStubVanillaClient(t,
		func(w http.ResponseWriter, r *http.Request) {
			gotMethod = r.Method
			gotPath = r.URL.Path
			gotToken = r.Header.Get("X-AUTH-YW-API-TOKEN")
			gotContentLength = r.ContentLength
			gotChunked = len(r.TransferEncoding) > 0
			mr, err := r.MultipartReader()
			if err != nil {
				t.Errorf("multipart reader: %v", err)
				return
			}
			part, err := mr.NextPart()
			if err != nil {
				t.Errorf("first multipart part: %v", err)
				return
			}
			gotFormName = part.FormName()
			gotFileName = part.FileName()
			gotFileContent, err = io.ReadAll(part)
			if err != nil {
				t.Errorf("read multipart part: %v", err)
				return
			}
			if _, err := mr.NextPart(); err != io.EOF {
				gotExtraPart = true
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"resourceUUID":"file-uuid-1"}`))
		})

	fileUUID, err := vc.UploadReleaseFile(context.Background(), "cust-1", "token", path)
	if err != nil {
		t.Fatalf("upload error: %v", err)
	}
	if fileUUID != "file-uuid-1" {
		t.Errorf("expected file UUID file-uuid-1, got %s", fileUUID)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("expected POST, got %s", gotMethod)
	}
	if gotPath != "/api/v1/customers/cust-1/ybdb_release/upload" {
		t.Errorf("unexpected path: %s", gotPath)
	}
	if gotToken != "token" {
		t.Errorf("auth token not sent, got %q", gotToken)
	}
	// Fixed-length streaming contract: a Content-Length larger than the file
	// (multipart framing included) and no chunked transfer encoding.
	if gotChunked {
		t.Error("request used chunked transfer encoding; expected fixed Content-Length")
	}
	if gotContentLength <= int64(len(content)) {
		t.Errorf("Content-Length %d not larger than file size %d", gotContentLength, len(content))
	}
	if gotFormName != "file" {
		t.Errorf("expected multipart form field name 'file', got %q", gotFormName)
	}
	if gotFileName != filepath.Base(path) {
		t.Errorf("expected filename %q, got %q", filepath.Base(path), gotFileName)
	}
	if !bytes.Equal(gotFileContent, content) {
		t.Errorf("uploaded content mismatch: got %d bytes, want %d bytes",
			len(gotFileContent), len(content))
	}
	if gotExtraPart {
		t.Error("expected exactly one multipart part")
	}
}

func TestUploadReleaseFileServerError(t *testing.T) {
	path, _ := writeTempReleaseFile(t)
	vc, _ := newStubVanillaClient(t,
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"no 'file' found in body"}`))
		})

	fileUUID, err := vc.UploadReleaseFile(context.Background(), "cust", "token", path)
	if fileUUID != "" {
		t.Errorf("expected empty file UUID on error, got %s", fileUUID)
	}
	var httpErr *utils.HTTPResponseError
	if !errors.As(err, &httpErr) {
		t.Fatalf("expected *utils.HTTPResponseError, got %v", err)
	}
	if httpErr.StatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", httpErr.StatusCode)
	}
	if !strings.Contains(err.Error(), "no 'file' found in body") {
		t.Errorf("error did not surface response body: %v", err)
	}
}

func TestUploadReleaseFileMissingLocalFile(t *testing.T) {
	vc, _ := newStubVanillaClient(t,
		func(_ http.ResponseWriter, _ *http.Request) {
			t.Error("no HTTP request expected for a missing local file")
		})

	_, err := vc.UploadReleaseFile(
		context.Background(), "cust", "token", filepath.Join(t.TempDir(), "absent.tar.gz"))
	if err == nil {
		t.Fatal("expected error for missing local file")
	}
}

func TestGetUploadedReleaseMetadata(t *testing.T) {
	var gotPath, gotToken string
	vc, _ := newStubVanillaClient(t,
		func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.Path
			gotToken = r.Header.Get("X-AUTH-YW-API-TOKEN")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"metadata_uuid": "file-uuid-1",
				"version": "2.21.0.0-b1",
				"yb_type": "YBDB",
				"sha256": "abc123",
				"platform": "LINUX",
				"architecture": "aarch64",
				"release_type": "PREVIEW",
				"release_date_msecs": 1722128523000,
				"status": "success"
			}`))
		})

	metadata, err := vc.GetUploadedReleaseMetadata(
		context.Background(), "cust-1", "token", "file-uuid-1")
	if err != nil {
		t.Fatalf("metadata error: %v", err)
	}
	if gotPath != "/api/v1/customers/cust-1/ybdb_release/upload/file-uuid-1" {
		t.Errorf("unexpected path: %s", gotPath)
	}
	if gotToken != "token" {
		t.Errorf("auth token not sent, got %q", gotToken)
	}
	if metadata.Version != "2.21.0.0-b1" || metadata.Sha256 != "abc123" ||
		metadata.Platform != "LINUX" || metadata.Architecture != "aarch64" ||
		metadata.Status != "success" {
		t.Errorf("metadata not parsed: %+v", metadata)
	}
}

func TestGetUploadedReleaseMetadataError(t *testing.T) {
	vc, _ := newStubVanillaClient(t,
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"Cannot find file uuid"}`))
		})

	metadata, err := vc.GetUploadedReleaseMetadata(
		context.Background(), "cust", "token", "absent-uuid")
	if metadata != nil {
		t.Errorf("expected nil metadata on error, got %+v", metadata)
	}
	var httpErr *utils.HTTPResponseError
	if !errors.As(err, &httpErr) {
		t.Fatalf("expected *utils.HTTPResponseError, got %v", err)
	}
	if httpErr.StatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", httpErr.StatusCode)
	}
}
