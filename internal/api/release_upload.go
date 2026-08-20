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

// releaseUploadHTTPClient returns a client for the release upload/metadata
// endpoints. Deliberately no overall timeout and no ResponseHeaderTimeout:
// the upload response arrives only after YBA has received and stored the
// whole file, and the metadata GET responds only after YBA has hashed and
// untarred it server-side — minutes for multi-GB tarballs. Cancellation comes
// solely from ctx, which callers derive from the resource operation timeout.
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

// UploadReleaseFile streams a local release tarball to
// POST /api/v1/customers/{cUUID}/ybdb_release/upload (multipart form field
// "file", the name YBA's ReleasesUploadController requires) and returns the
// file UUID YBA assigned. YBA stores the file on its node under
// yb.releases.artifacts.relative_upload_path. The body streams straight from
// disk with a precomputed Content-Length, so tarballs are never buffered in
// memory and the request is not chunk-encoded.
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

	// Only the multipart part header and closing boundary are buffered; the
	// file itself is streamed between them.
	var head bytes.Buffer
	w := multipart.NewWriter(&head)
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
	// io.MultiReader is not a body type net/http can infer a length from —
	// set it explicitly so the request carries Content-Length instead of
	// falling back to chunked transfer encoding.
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

// GetUploadedReleaseMetadata calls
// GET /api/v1/customers/{cUUID}/ybdb_release/upload/{fileUUID}. YBA computes
// the uploaded tarball's sha256 and extracts its version metadata
// synchronously inside this call, so it is hand-rolled on a transport with no
// response-header timeout instead of the generated client (2-minute
// ResponseHeaderTimeout) or vc.Client (30s overall timeout).
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
