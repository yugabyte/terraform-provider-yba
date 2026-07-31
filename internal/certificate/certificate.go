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

// Package certificate manages YugabyteDB Anywhere encryption-in-transit
// certificate configurations: self-signed root certificates (YBA-minted or
// bring-your-own) and custom server certificates for client-to-node TLS.
package certificate

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/go-cty/cty"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	client "github.com/yugabyte/platform-go-client"

	"github.com/yugabyte/terraform-provider-yba/internal/api"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// Certificate type strings as defined by YBA's CertConfigType enum.
const (
	certTypeSelfSigned       = "SelfSigned"
	certTypeCustomServerCert = "CustomServerCert"
)

// certOperationTimeout bounds the certificate CRUD calls, which are all
// API-synchronous (no YBA task to wait on).
const certOperationTimeout = 10 * time.Minute

// normalizePEM guarantees the content ends with exactly one trailing newline.
// YBA rejects uploads whose certificate content does not end with a newline
// ("Certificate must end with a newline"), which is easy to hit with
// Terraform heredocs or trimmed file reads.
func normalizePEM(content string) string {
	return strings.TrimRight(content, "\r\n") + "\n"
}

// suppressPEMWhitespaceDiff treats PEM values that differ only in surrounding
// whitespace as equal, so YBA's canonical stored form (trailing newline) never
// produces a perpetual diff against the user's config value.
func suppressPEMWhitespaceDiff(k, old, new string, d *schema.ResourceData) bool {
	return strings.TrimSpace(old) == strings.TrimSpace(new)
}

// writeOnlyStringAttr reads a write-only string argument from the raw config.
// Write-only values never reach plan or state, so d.Get returns the zero
// value for them — the raw config, delivered on every apply, is the only
// place they exist. Returns "" when the argument is not set.
func writeOnlyStringAttr(d *schema.ResourceData, name string) (string, error) {
	v, diags := d.GetRawConfigAt(cty.GetAttrPath(name))
	if diags.HasError() {
		return "", fmt.Errorf("read write-only argument %s: %s", name, diags[0].Summary)
	}
	if v.IsNull() || !v.IsKnown() || v.Type() != cty.String {
		return "", nil
	}
	return v.AsString(), nil
}

// findCertificate returns the first certificate matching the predicate, or
// nil when none does. YBA has no public by-UUID GET, so reads filter the list.
func findCertificate(
	ctx context.Context, c *client.APIClient, cUUID string,
	match func(*client.CertificateInfoExt) bool,
) (*client.CertificateInfoExt, error) {
	r, response, err := c.CertificateInfoAPI.GetListOfCertificate(ctx, cUUID).Execute()
	if err != nil {
		return nil, utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"Certificate", "Read")
	}
	for i := range r {
		if match(&r[i]) {
			return &r[i], nil
		}
	}
	return nil, nil
}

// getCertificate fetches the certificate with the given UUID, or nil when YBA
// no longer has it.
func getCertificate(
	ctx context.Context, c *client.APIClient, cUUID string, certUUID string,
) (*client.CertificateInfoExt, error) {
	return findCertificate(ctx, c, cUUID, func(cert *client.CertificateInfoExt) bool {
		return cert.GetUuid() == certUUID
	})
}

// getCertificateByLabel fetches the certificate with the given label, or nil
// when no certificate carries it. Labels are unique per customer in YBA.
func getCertificateByLabel(
	ctx context.Context, c *client.APIClient, cUUID string, label string,
) (*client.CertificateInfoExt, error) {
	return findCertificate(ctx, c, cUUID, func(cert *client.CertificateInfoExt) bool {
		return cert.GetLabel() == label
	})
}

// downloadRootCertPEM returns the certificate config's root certificate PEM
// via GET .../certificates/{rUUID}/download, which responds {"root.crt": "<PEM>"}.
func downloadRootCertPEM(
	ctx context.Context, c *client.APIClient, cUUID string, certUUID string,
) (string, error) {
	r, response, err := c.CertificateInfoAPI.GetRootCert(ctx, cUUID, certUUID).Execute()
	if err != nil {
		return "", utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"Certificate", "Download root certificate")
	}
	pem, ok := r["root.crt"].(string)
	if !ok {
		return "", fmt.Errorf("unexpected root certificate download response shape")
	}
	return pem, nil
}

// setCommonCertificateFields writes the fields shared by both certificate
// resources into state.
func setCommonCertificateFields(d *schema.ResourceData, cert *client.CertificateInfoExt) error {
	if err := d.Set("label", cert.GetLabel()); err != nil {
		return err
	}
	if err := d.Set("uuid", cert.GetUuid()); err != nil {
		return err
	}
	if err := d.Set("in_use", cert.GetInUse()); err != nil {
		return err
	}
	if err := d.Set("start_date", formatCertDate(cert.StartDateIso)); err != nil {
		return err
	}
	return d.Set("expiry_date", formatCertDate(cert.ExpiryDateIso))
}

func formatCertDate(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// readCertificateResource loads the certificate into state, exporting the
// root CA PEM (via download) into pemAttr — the resources differ only in
// which attribute carries it. Missing certificates clear the ID so Terraform
// plans a recreate (out-of-band delete idempotency).
func readCertificateResource(
	ctx context.Context, d *schema.ResourceData, meta interface{}, pemAttr string,
) diag.Diagnostics {
	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID

	cert, err := getCertificate(ctx, c, cUUID, d.Id())
	if err != nil {
		return diag.FromErr(err)
	}
	if cert == nil {
		tflog.Warn(ctx, fmt.Sprintf(
			"Certificate %s not found, removing from state", d.Id()))
		d.SetId("")
		return nil
	}

	if err = setCommonCertificateFields(d, cert); err != nil {
		return diag.FromErr(err)
	}

	pem, err := downloadRootCertPEM(ctx, c, cUUID, d.Id())
	if err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set(pemAttr, pem); err != nil {
		return diag.FromErr(err)
	}

	return nil
}

// resourceCertificateDelete deletes a certificate config. Certificates that
// are already gone succeed (out-of-band delete idempotency, checked via the
// list rather than by matching YBA's error body). Certificates still
// referenced by a universe fail fast with a targeted error built from the
// typed inUse/universeDetails fields — YBA would reject the delete anyway,
// with a message that names neither the universes nor the fix.
func resourceCertificateDelete(
	ctx context.Context, d *schema.ResourceData, meta interface{},
) diag.Diagnostics {
	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID

	cert, err := getCertificate(ctx, c, cUUID, d.Id())
	if err != nil {
		return diag.FromErr(err)
	}
	if cert == nil {
		tflog.Warn(ctx, fmt.Sprintf(
			"Certificate %s already removed from YugabyteDB Anywhere", d.Id()))
		d.SetId("")
		return nil
	}

	if cert.GetInUse() {
		names := make([]string, 0, len(cert.GetUniverseDetails()))
		for _, u := range cert.GetUniverseDetails() {
			names = append(names, u.Name)
		}
		return diag.Errorf(
			"certificate %q is still referenced by universe(s) %s: rotate them to "+
				"another certificate first, or — if this delete is part of a "+
				"replacement — set lifecycle { create_before_destroy = true } and a "+
				"new label on the certificate resource so the replacement is created "+
				"and the universes rotated before this configuration is deleted",
			cert.GetLabel(), strings.Join(names, ", "))
	}

	_, response, err := c.CertificateInfoAPI.DeleteCertificate(ctx, cUUID, d.Id()).Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"Certificate", "Delete")
		return diag.FromErr(errMessage)
	}

	d.SetId("")
	return nil
}

// uploadCertificate uploads a certificate config and returns the new UUID.
// certStart/certExpiry are required by the API schema but ignored by YBA,
// which derives the real dates from the certificate content; 0 is sent.
func uploadCertificate(
	ctx context.Context, c *client.APIClient, cUUID string, params client.CertificateParams,
) (string, error) {
	r, response, err := c.CertificateInfoAPI.Upload(ctx, cUUID).Certificate(params).Execute()
	if err != nil {
		return "", utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"Certificate", "Create")
	}
	// YBA returns the UUID as a bare JSON string, and the generated client
	// copies the raw response body verbatim for string return values — the
	// surrounding JSON quotes included. Strip them to get the UUID.
	return strings.Trim(strings.TrimSpace(r), `"`), nil
}
