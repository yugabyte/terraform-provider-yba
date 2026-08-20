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
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	client "github.com/yugabyte/platform-go-client"

	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// No overall or response-header timeout: YBA answers only after storing
// (upload) or hashing and untarring (metadata) the whole file, minutes for a
// multi-GB tarball. ctx carries the resource timeout.
func releaseUploadHTTPClient(enableHTTPS bool) *http.Client {
	tr := &http.Transport{
		DialContext:         (&net.Dialer{Timeout: 30 * time.Second}).DialContext,
		TLSHandshakeTimeout: 30 * time.Second,
	}
	if enableHTTPS {
		tr.TLSClientConfig = &tls.Config{
			InsecureSkipVerify: true, //nolint:gosec // TLS verification intentionally disabled for YBA self-signed certs
		}
	}
	return &http.Client{Transport: tr}
}

func (vc *VanillaClient) scheme() string {
	if vc.EnableHTTPS {
		return "https"
	}
	return "http"
}

// UploadReleaseFile streams filePath to /ybdb_release/upload and returns the
// file UUID YBA assigned.
func (vc *VanillaClient) UploadReleaseFile(
	ctx context.Context,
	cUUID string,
	apiKey string,
	filePath string,
) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("open release file: %w", err)
	}
	defer func() { _ = f.Close() }()
	fi, err := f.Stat()
	if err != nil {
		return "", fmt.Errorf("stat release file: %w", err)
	}

	var head bytes.Buffer
	w := multipart.NewWriter(&head)
	// YBA reads only the "file" field.
	if _, err = w.CreateFormFile("file", filepath.Base(filePath)); err != nil {
		return "", fmt.Errorf("build multipart header: %w", err)
	}
	trailer := fmt.Sprintf("\r\n--%s--\r\n", w.Boundary())

	url := fmt.Sprintf(
		"%s://%s/api/v1/customers/%s/ybdb_release/upload", vc.scheme(), vc.Host, cUUID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url,
		io.MultiReader(&head, f, strings.NewReader(trailer)))
	if err != nil {
		return "", err
	}
	// net/http cannot size a MultiReader and would fall back to chunked encoding.
	req.ContentLength = int64(head.Len()) + fi.Size() + int64(len(trailer))
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("X-AUTH-YW-API-TOKEN", apiKey)

	r, err := releaseUploadHTTPClient(vc.EnableHTTPS).Do(req)
	if err != nil {
		return "", fmt.Errorf("upload release file: %w", err)
	}
	defer func() { _ = r.Body.Close() }()
	if err := utils.CheckHTTPError(r, "Upload Release"); err != nil {
		return "", err
	}

	var success struct {
		ResourceUUID string `json:"resourceUUID"`
	}
	if err := json.NewDecoder(r.Body).Decode(&success); err != nil {
		return "", fmt.Errorf("parse upload release response: %w", err)
	}
	if success.ResourceUUID == "" {
		return "", errors.New("upload release response missing resourceUUID")
	}
	return success.ResourceUUID, nil
}

// GetUploadedReleaseMetadata returns the metadata YBA extracts from an
// uploaded file. Hand-rolled: YBA hashes and untars inside this call, past the
// generated client's 2-minute header timeout.
func (vc *VanillaClient) GetUploadedReleaseMetadata(
	ctx context.Context,
	cUUID string,
	apiKey string,
	fileUUID string,
) (*client.ResponseExtractMetadata, error) {
	url := fmt.Sprintf(
		"%s://%s/api/v1/customers/%s/ybdb_release/upload/%s",
		vc.scheme(), vc.Host, cUUID, fileUUID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-AUTH-YW-API-TOKEN", apiKey)

	r, err := releaseUploadHTTPClient(vc.EnableHTTPS).Do(req)
	if err != nil {
		return nil, fmt.Errorf("get uploaded release metadata: %w", err)
	}
	defer func() { _ = r.Body.Close() }()
	if err := utils.CheckHTTPError(r, "Get Uploaded Release Metadata"); err != nil {
		return nil, err
	}

	var metadata client.ResponseExtractMetadata
	if err := json.NewDecoder(r.Body).Decode(&metadata); err != nil {
		return nil, fmt.Errorf("parse uploaded release metadata response: %w", err)
	}
	return &metadata, nil
}
