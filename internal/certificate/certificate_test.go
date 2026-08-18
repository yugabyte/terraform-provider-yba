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

package certificate

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/go-cty/cty"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	client "github.com/yugabyte/platform-go-client"

	"github.com/yugabyte/terraform-provider-yba/internal/api"
)

// testResourceDataWithRawConfig builds a ResourceData whose raw config is
// populated, unlike schema.TestResourceDataRaw. Write-only arguments exist
// only in the raw config (never in plan or state), so create tests must go
// through this helper for writeOnlyStringAttr to see them — exactly as the
// real protocol delivers them during apply.
func testResourceDataWithRawConfig(
	t *testing.T, s map[string]*schema.Schema, raw map[string]interface{},
) *schema.ResourceData {
	t.Helper()
	sm := schema.InternalMap(s)
	diff, err := sm.Diff(context.Background(), nil,
		terraform.NewResourceConfigRaw(raw), nil, nil, true)
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	if diff == nil {
		diff = new(terraform.InstanceDiff)
	}
	impliedType := sm.CoreConfigSchema().ImpliedType()
	vals := make(map[string]cty.Value, len(impliedType.AttributeTypes()))
	for name, ty := range impliedType.AttributeTypes() {
		rv, ok := raw[name]
		if !ok {
			vals[name] = cty.NullVal(ty)
			continue
		}
		switch {
		case ty.Equals(cty.String):
			vals[name] = cty.StringVal(rv.(string))
		case ty.Equals(cty.Bool):
			vals[name] = cty.BoolVal(rv.(bool))
		default:
			t.Fatalf("unsupported raw config attribute %s of type %v", name, ty)
		}
	}
	diff.RawConfig = cty.ObjectVal(vals)
	d, err := sm.Data(nil, diff)
	if err != nil {
		t.Fatalf("data: %v", err)
	}
	return d
}

const (
	testCertUUID = "6db4c6f2-2c1b-4a3e-8a9f-3d3f2a1b0c9d"
	testPEM      = "-----BEGIN CERTIFICATE-----\nabc\n-----END CERTIFICATE-----"
	testKeyPEM   = "-----BEGIN RSA PRIVATE KEY-----\nxyz\n-----END RSA PRIVATE KEY-----"
)

// fakeYBA is a minimal certificate-API stub. Handlers that were not hit stay
// zero-valued so tests can assert which endpoints were exercised.
type fakeYBA struct {
	listBody      string
	uploadPayload map[string]interface{}
	mintBody      map[string]string
	deleteCalled  bool
	deleteStatus  int
	deleteBody    string
}

func (f *fakeYBA) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/certificates"):
			_, _ = w.Write([]byte(f.listBody))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/download"):
			_ = json.NewEncoder(w).Encode(map[string]string{"root.crt": testPEM + "\n"})
		case r.Method == http.MethodPost &&
			strings.HasSuffix(r.URL.Path, "/create_self_signed_cert"):
			body, _ := io.ReadAll(r.Body)
			if err := json.Unmarshal(body, &f.mintBody); err != nil {
				t.Errorf("mint body is not a JSON object: %s", body)
			}
			_, _ = fmt.Fprintf(w, "%q", testCertUUID)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/certificates"):
			body, _ := io.ReadAll(r.Body)
			if err := json.Unmarshal(body, &f.uploadPayload); err != nil {
				t.Errorf("upload body is not a JSON object: %s", body)
			}
			_, _ = fmt.Fprintf(w, "%q", testCertUUID)
		case r.Method == http.MethodDelete:
			f.deleteCalled = true
			status := f.deleteStatus
			if status == 0 {
				status = http.StatusOK
			}
			w.WriteHeader(status)
			body := f.deleteBody
			if body == "" {
				body = `{"success":true}`
			}
			_, _ = w.Write([]byte(body))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}
}

func (f *fakeYBA) apiClient(t *testing.T) *api.APIClient {
	t.Helper()
	srv := httptest.NewServer(f.handler(t))
	t.Cleanup(srv.Close)
	addr := strings.TrimPrefix(srv.URL, "http://")

	cfg := client.NewConfiguration()
	cfg.Scheme = "http"
	cfg.Host = addr
	return &api.APIClient{
		VanillaClient: &api.VanillaClient{
			Client: srv.Client(), Host: addr, EnableHTTPS: false,
		},
		YugawareClient: client.NewAPIClient(cfg),
		CustomerID:     "cust",
		APIKey:         "tok",
	}
}

func listWith(uuid, label, certType string, inUse bool) string {
	return fmt.Sprintf(`[{
		"uuid": %q, "label": %q, "certType": %q, "inUse": %t,
		"startDateIso": "2026-07-29T00:00:00Z",
		"expiryDateIso": "2030-07-29T00:00:00Z"
	}]`, uuid, label, certType, inUse)
}

// --- schema sanity ---------------------------------------------------------

func TestSelfSignedCertificateSchemaSanity(t *testing.T) {
	s := ResourceSelfSignedCertificate().Schema
	for _, field := range []string{"label", "certificate"} {
		if !s[field].ForceNew {
			t.Errorf("%s must be ForceNew: cert configs are immutable in YBA", field)
		}
	}
	if !s["private_key"].WriteOnly {
		t.Error("private_key must be WriteOnly: secrets must never land in state")
	}
	if s["private_key"].ForceNew {
		t.Error("private_key cannot be ForceNew (SDK forbids it with WriteOnly); " +
			"replacement is driven by the paired certificate field")
	}
	if !s["private_key"].Sensitive {
		t.Error("private_key must be Sensitive")
	}
	if len(s["certificate"].RequiredWith) == 0 || len(s["private_key"].RequiredWith) == 0 {
		t.Error("certificate and private_key must require each other (BYO mode)")
	}
	if s["certificate"].Required || s["private_key"].Required {
		t.Error("certificate/private_key must stay Optional so mint mode works")
	}
}

func TestCustomServerCertificateSchemaSanity(t *testing.T) {
	s := ResourceCustomServerCertificate().Schema
	for _, field := range []string{"label", "root_certificate", "server_certificate"} {
		if !s[field].ForceNew {
			t.Errorf("%s must be ForceNew: cert configs are immutable in YBA", field)
		}
	}
	for _, field := range []string{"label", "root_certificate", "server_certificate",
		"server_key"} {
		if !s[field].Required {
			t.Errorf("%s must be Required", field)
		}
	}
	if !s["server_key"].WriteOnly {
		t.Error("server_key must be WriteOnly: secrets must never land in state")
	}
	if s["server_key"].ForceNew {
		t.Error("server_key cannot be ForceNew (SDK forbids it with WriteOnly); " +
			"replacement is driven by the paired server_certificate field")
	}
	if !s["server_key"].Sensitive {
		t.Error("server_key must be Sensitive")
	}
}

// --- create ---------------------------------------------------------------

func TestSelfSignedCreateUploadsNormalizedPEM(t *testing.T) {
	f := &fakeYBA{listBody: listWith(testCertUUID, "byo-ca", "SelfSigned", false)}
	meta := f.apiClient(t)

	d := testResourceDataWithRawConfig(t, ResourceSelfSignedCertificate().Schema,
		map[string]interface{}{
			"label":       "byo-ca",
			"certificate": testPEM, // deliberately no trailing newline
			"private_key": testKeyPEM,
		})

	if diags := resourceSelfSignedCertificateCreate(
		context.Background(),
		d,
		meta,
	); diags.HasError() {
		t.Fatalf("create diags: %v", diags)
	}
	if d.Id() != testCertUUID {
		t.Errorf("id = %q, want %q", d.Id(), testCertUUID)
	}
	if got := f.uploadPayload["certType"]; got != "SelfSigned" {
		t.Errorf("certType = %v, want SelfSigned", got)
	}
	cert := f.uploadPayload["certContent"].(string)
	if !strings.HasSuffix(cert, "-----END CERTIFICATE-----\n") ||
		strings.HasSuffix(cert, "\n\n") {
		t.Errorf("certContent must end with exactly one newline, got %q", cert)
	}
	if key, ok := f.uploadPayload["keyContent"].(string); !ok ||
		!strings.HasSuffix(key, "\n") {
		t.Errorf("keyContent must be set and newline-terminated, got %v",
			f.uploadPayload["keyContent"])
	}
}

func TestSelfSignedCreateMintMode(t *testing.T) {
	f := &fakeYBA{listBody: listWith(testCertUUID, "minted-ca", "SelfSigned", false)}
	meta := f.apiClient(t)

	d := testResourceDataWithRawConfig(t, ResourceSelfSignedCertificate().Schema,
		map[string]interface{}{"label": "minted-ca"})

	if diags := resourceSelfSignedCertificateCreate(
		context.Background(),
		d,
		meta,
	); diags.HasError() {
		t.Fatalf("create diags: %v", diags)
	}
	if d.Id() != testCertUUID {
		t.Errorf("id = %q, want %q", d.Id(), testCertUUID)
	}
	if f.mintBody["label"] != "minted-ca" {
		t.Errorf(`mint body = %v, want {"label": "minted-ca"}`, f.mintBody)
	}
	if f.uploadPayload != nil {
		t.Error("mint mode must not call the upload endpoint")
	}
	// Read exports the minted CA for client distribution.
	if got := d.Get("certificate").(string); !strings.Contains(got, "BEGIN CERTIFICATE") {
		t.Errorf("certificate not populated from download after mint, got %q", got)
	}
}

func TestCustomServerCreateUploadsServerCertData(t *testing.T) {
	f := &fakeYBA{listBody: listWith(testCertUUID, "c2n", "CustomServerCert", false)}
	meta := f.apiClient(t)

	d := testResourceDataWithRawConfig(t, ResourceCustomServerCertificate().Schema,
		map[string]interface{}{
			"label":              "c2n",
			"root_certificate":   testPEM,
			"server_certificate": testPEM,
			"server_key":         testKeyPEM,
		})

	if diags := resourceCustomServerCertificateCreate(
		context.Background(),
		d,
		meta,
	); diags.HasError() {
		t.Fatalf("create diags: %v", diags)
	}
	if got := f.uploadPayload["certType"]; got != "CustomServerCert" {
		t.Errorf("certType = %v, want CustomServerCert", got)
	}
	data, ok := f.uploadPayload["customServerCertData"].(map[string]interface{})
	if !ok {
		t.Fatalf("customServerCertData missing from payload: %v", f.uploadPayload)
	}
	for _, k := range []string{"serverCertContent", "serverKeyContent"} {
		if v, ok := data[k].(string); !ok || !strings.HasSuffix(v, "\n") {
			t.Errorf("%s must be set and newline-terminated, got %v", k, data[k])
		}
	}
	if _, present := f.uploadPayload["keyContent"]; present {
		t.Error("keyContent must be absent for CustomServerCert uploads")
	}
}

// --- read / delete ---------------------------------------------------------

func TestReadRemovesMissingCertificateFromState(t *testing.T) {
	f := &fakeYBA{listBody: `[]`}
	meta := f.apiClient(t)

	d := schema.TestResourceDataRaw(t, ResourceSelfSignedCertificate().Schema,
		map[string]interface{}{"label": "gone"})
	d.SetId(testCertUUID)

	if diags := resourceSelfSignedCertificateRead(context.Background(), d, meta); diags.HasError() {
		t.Fatalf("read diags: %v", diags)
	}
	if d.Id() != "" {
		t.Error("out-of-band deleted certificate must be dropped from state")
	}
}

func TestDeleteIsIdempotentForMissingCertificate(t *testing.T) {
	f := &fakeYBA{listBody: `[]`}
	meta := f.apiClient(t)

	d := schema.TestResourceDataRaw(t, ResourceSelfSignedCertificate().Schema,
		map[string]interface{}{"label": "gone"})
	d.SetId(testCertUUID)

	if diags := resourceCertificateDelete(context.Background(), d, meta); diags.HasError() {
		t.Fatalf("delete diags: %v", diags)
	}
	if f.deleteCalled {
		t.Error("DELETE must not be issued for an already-gone certificate")
	}
}

func TestDeleteInUseFailsFastWithActionableError(t *testing.T) {
	f := &fakeYBA{
		listBody: fmt.Sprintf(`[{
			"uuid": %q, "label": "in-use", "certType": "SelfSigned", "inUse": true,
			"startDateIso": "2026-07-29T00:00:00Z",
			"expiryDateIso": "2030-07-29T00:00:00Z",
			"universeDetails": [
				{"uuid": "u-1", "name": "prod-universe", "creationDate": 0,
				 "universePaused": false, "updateInProgress": false,
				 "updateSucceeded": true}
			]
		}]`, testCertUUID),
	}
	meta := f.apiClient(t)

	d := schema.TestResourceDataRaw(t, ResourceSelfSignedCertificate().Schema,
		map[string]interface{}{"label": "in-use"})
	d.SetId(testCertUUID)

	diags := resourceCertificateDelete(context.Background(), d, meta)
	if !diags.HasError() {
		t.Fatal("in-use delete must surface an error, not be swallowed")
	}
	msg := diags[0].Summary
	for _, want := range []string{"prod-universe", "create_before_destroy", "new label"} {
		if !strings.Contains(msg, want) {
			t.Errorf("in-use error must mention %q, got: %s", want, msg)
		}
	}
	if f.deleteCalled {
		t.Error("in-use delete must fail fast without dispatching DELETE to YBA")
	}
	if d.Id() == "" {
		t.Error("failed delete must keep the certificate in state")
	}
}

// --- data source -----------------------------------------------------------

func TestDataSourceCertificateByLabel(t *testing.T) {
	f := &fakeYBA{listBody: listWith(testCertUUID, "prod-ca", "SelfSigned", true)}
	meta := f.apiClient(t)

	d := schema.TestResourceDataRaw(t, DataSourceCertificate().Schema,
		map[string]interface{}{"label": "prod-ca"})

	if diags := dataSourceCertificateRead(context.Background(), d, meta); diags.HasError() {
		t.Fatalf("read diags: %v", diags)
	}
	if d.Id() != testCertUUID {
		t.Errorf("id = %q, want %q", d.Id(), testCertUUID)
	}
	if got := d.Get("cert_type").(string); got != "SelfSigned" {
		t.Errorf("cert_type = %q, want SelfSigned", got)
	}
	if !d.Get("in_use").(bool) {
		t.Error("in_use = false, want true")
	}
	if got := d.Get("expiry_date").(string); got != "2030-07-29T00:00:00Z" {
		t.Errorf("expiry_date = %q, want RFC3339 value", got)
	}
}

func TestDataSourceCertificateNotFound(t *testing.T) {
	f := &fakeYBA{listBody: `[]`}
	meta := f.apiClient(t)

	d := schema.TestResourceDataRaw(t, DataSourceCertificate().Schema,
		map[string]interface{}{"label": "absent"})

	if diags := dataSourceCertificateRead(context.Background(), d, meta); !diags.HasError() {
		t.Fatal("lookup of a missing label must error")
	}
}

// --- helpers ----------------------------------------------------------------

func TestNormalizePEM(t *testing.T) {
	cases := map[string]string{
		"x":         "x\n",
		"x\n":       "x\n",
		"x\r\n":     "x\n",
		"x\n\n\n":   "x\n",
		"x\r\n\r\n": "x\n",
	}
	for in, want := range cases {
		if got := normalizePEM(in); got != want {
			t.Errorf("normalizePEM(%q) = %q, want %q", in, got, want)
		}
	}
}

// testCertPEM mints a self-signed certificate and returns it PEM-encoded with
// 64-column base64 (the encoding/pem and BouncyCastle canonical form).
func testCertPEM(t *testing.T, cn string) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

// rewrapPEM re-encodes every certificate block's base64 at the given column
// width, emulating a PEM authored by a tool other than YBA's writer.
func rewrapPEM(t *testing.T, content string, width int) string {
	t.Helper()
	block, _ := pem.Decode([]byte(content))
	if block == nil {
		t.Fatal("rewrapPEM: no PEM block")
	}
	b64 := base64.StdEncoding.EncodeToString(block.Bytes)
	var sb strings.Builder
	sb.WriteString("-----BEGIN CERTIFICATE-----\n")
	for i := 0; i < len(b64); i += width {
		end := i + width
		if end > len(b64) {
			end = len(b64)
		}
		sb.WriteString(b64[i:end])
		sb.WriteString("\n")
	}
	sb.WriteString("-----END CERTIFICATE-----\n")
	return sb.String()
}

// The PEM DiffSuppressFunc must compare the certificates, not the text: YBA
// does not store the uploaded bytes — it parses the PEM and re-emits it
// through its own writer — so a user's CRLF-terminated, 76-column, or
// header-annotated file differs textually from the read-back value forever.
// On a ForceNew attribute that textual diff is a destroy-and-recreate plan on
// every run, which the in-use delete guard then blocks.
func TestSuppressPEMDiffSemanticEquality(t *testing.T) {
	canonical := testCertPEM(t, "same-cert")

	equal := map[string]string{
		"crlf line endings": strings.ReplaceAll(canonical, "\n", "\r\n"),
		"76-column base64":  rewrapPEM(t, canonical, 76),
		"bag attributes header": "Bag Attributes\n    friendlyName: my-cert\n" +
			"subject=/CN=same-cert\n" + canonical,
		"surrounding whitespace": "\n" + canonical + "\n\n",
	}
	for name, variant := range equal {
		if !suppressPEMContentDiff("certificate", canonical, variant, nil) {
			t.Errorf("%s: same certificate must suppress the diff", name)
		}
	}
}

func TestSuppressPEMDiffChain(t *testing.T) {
	leaf := testCertPEM(t, "server")
	root := testCertPEM(t, "root")

	chain := leaf + root
	spaced := leaf + "\n" + root // blank line between members
	if !suppressPEMContentDiff("certificate", chain, spaced, nil) {
		t.Error("same chain with blank line between members must suppress the diff")
	}
	reordered := root + leaf
	if suppressPEMContentDiff("certificate", chain, reordered, nil) {
		t.Error("reordered chain must not suppress the diff")
	}
	if suppressPEMContentDiff("certificate", chain, leaf, nil) {
		t.Error("dropped chain member must not suppress the diff")
	}
}

func TestSuppressPEMDiffDifferentCertificates(t *testing.T) {
	a := testCertPEM(t, "cert-a")
	b := testCertPEM(t, "cert-b")
	if suppressPEMContentDiff("certificate", a, b, nil) {
		t.Error("different certificates must not suppress the diff")
	}
	// Unparseable content falls back to whitespace-insensitive comparison.
	if !suppressPEMContentDiff("certificate", "not pem\n", " not pem ", nil) {
		t.Error("equal non-PEM content must still suppress")
	}
	if suppressPEMContentDiff("certificate", "not pem", "other", nil) {
		t.Error("different non-PEM content must not suppress")
	}
}
