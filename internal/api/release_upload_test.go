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
	"testing"
)

// Guards the hand-built multipart framing: a wrong Content-Length truncates or
// overruns the body, and YBA reads only the "file" form field.
func TestUploadReleaseFileStreamsOneFilePart(t *testing.T) {
	content := bytes.Repeat([]byte("yugabyte-release-payload-0123456789"), 100000)
	path := filepath.Join(t.TempDir(), "yugabyte-2.21.0.0-b1-linux-x86_64.tar.gz")
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatalf("write release file: %v", err)
	}
	var (
		gotChunked     bool
		gotFormName    string
		gotFileContent []byte
		gotExtraPart   bool
	)
	vc, _ := newStubVanillaClient(t,
		func(w http.ResponseWriter, r *http.Request) {
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
			gotFileContent, _ = io.ReadAll(part)
			_, err = mr.NextPart()
			gotExtraPart = !errors.Is(err, io.EOF)
			_, _ = w.Write([]byte(`{"resourceUUID":"file-uuid-1"}`))
		})

	fileUUID, err := vc.UploadReleaseFile(context.Background(), "cust-1", "token", path)

	if err != nil {
		t.Fatalf("upload error: %v", err)
	}
	if fileUUID != "file-uuid-1" {
		t.Errorf("file UUID = %q, want file-uuid-1", fileUUID)
	}
	if gotChunked {
		t.Error("request is chunk-encoded; want a fixed Content-Length")
	}
	if gotFormName != "file" {
		t.Errorf("form field = %q, want file", gotFormName)
	}
	if !bytes.Equal(gotFileContent, content) {
		t.Errorf("uploaded %d bytes, want %d", len(gotFileContent), len(content))
	}
	if gotExtraPart {
		t.Error("want exactly one multipart part")
	}
}
