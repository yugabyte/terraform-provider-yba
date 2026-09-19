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

package acctest

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"math/big"
	"strings"
	"testing"
	"time"
)

// TestCA is a freshly minted RSA root CA that acceptance tests upload to
// YugabyteDB Anywhere as bring-your-own certificate content, and use to issue
// server certificates for yba_custom_server_certificate.
type TestCA struct {
	// CertPEM and KeyPEM are the root certificate and its private key, in the
	// canonical form Go's encoder emits (64-column base64, LF endings).
	CertPEM string
	KeyPEM  string

	cert *x509.Certificate
	key  *rsa.PrivateKey
}

// NewTestCA mints an RSA-2048 root CA valid for two years. YBA signs (or
// verifies) per-node server certificates against it, so the certificate
// carries the full CA key-usage set.
func NewTestCA(t *testing.T, cn string) *TestCA {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          randomSerial(t),
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(2 * 365 * 24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign |
			x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return &TestCA{
		CertPEM: string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})),
		KeyPEM: string(pem.EncodeToMemory(&pem.Block{
			Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})),
		cert: cert,
		key:  key,
	}
}

// IssueServerCert issues a one-year TLS server certificate signed by the CA,
// shaped like the org-issued certificates yba_custom_server_certificate
// carries: serverAuth extended key usage plus DNS SANs for the given name and
// its wildcard domain.
func (ca *TestCA) IssueServerCert(t *testing.T, cn string) (certPEM, keyPEM string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: randomSerial(t),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		DNSNames:     []string{cn, "*." + cn},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca.cert, &key.PublicKey, ca.key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	keyPEM = string(pem.EncodeToMemory(&pem.Block{
		Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
	return certPEM, keyPEM
}

// MangledPEM re-encodes certificate PEM the way real-world tooling often
// emits it — CRLF line endings and 76-column base64 — so it differs textually
// from both the input and YBA's re-encoded read-back while carrying identical
// DER. Acceptance tests feed it into configs to prove the semantic PEM diff
// suppression: without it, every plan after the first apply proposes a
// destroy-and-recreate of the certificate.
func MangledPEM(t *testing.T, content string) string {
	t.Helper()
	block, _ := pem.Decode([]byte(content))
	if block == nil || block.Type != "CERTIFICATE" {
		t.Fatal("MangledPEM: input is not certificate PEM")
	}
	b64 := base64.StdEncoding.EncodeToString(block.Bytes)
	var sb strings.Builder
	sb.WriteString("-----BEGIN CERTIFICATE-----\r\n")
	for i := 0; i < len(b64); i += 76 {
		end := i + 76
		if end > len(b64) {
			end = len(b64)
		}
		sb.WriteString(b64[i:end])
		sb.WriteString("\r\n")
	}
	sb.WriteString("-----END CERTIFICATE-----\r\n")
	return sb.String()
}

func randomSerial(t *testing.T) *big.Int {
	t.Helper()
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		t.Fatal(err)
	}
	return serial
}
