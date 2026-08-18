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

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	client "github.com/yugabyte/platform-go-client"

	"github.com/yugabyte/terraform-provider-yba/internal/api"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// ResourceSelfSignedCertificate defines the self-signed certificate config resource.
func ResourceSelfSignedCertificate() *schema.Resource {
	return &schema.Resource{
		Description: "Self-signed certificate configuration for universe encryption in transit. " +
			"YugabyteDB Anywhere holds the root certificate's private key and signs the " +
			"per-node server certificates itself. Two modes are supported: omit `certificate` " +
			"and `private_key` to have YugabyteDB Anywhere mint a new root certificate " +
			"(4-year root, 1-year server certificates by platform default), or provide both " +
			"to bring your own root CA. Certificate configurations are immutable in " +
			"YugabyteDB Anywhere: changing any argument forces replacement, so add " +
			"`lifecycle { create_before_destroy = true }` when the certificate is referenced " +
			"by a universe — Terraform then rotates the universe to the replacement before " +
			"deleting the old configuration.\n\n" +
			"~> **Note:** Labels are unique per customer, and with `create_before_destroy` " +
			"the replacement is created while the old configuration still exists. Give the " +
			"replacement a new `label` (include a date or version, for example), or the " +
			"create fails with a duplicate-label error.\n\n" +
			"~> **Note:** `private_key` is a write-only argument: Terraform never stores it " +
			"in the plan or the state file. Setting it requires Terraform 1.11 or later " +
			"(the mint mode works with any Terraform version). Because nothing is stored, " +
			"a change to only `private_key` is not detected — the key belongs to its " +
			"`certificate` and they change together, which forces the replacement. " +
			"YugabyteDB Anywhere verifies at upload that the certificate and key match.\n\n" +
			"~> **Note:** Same-CA server-certificate refresh (fresh 1-year node certificates " +
			"from the unchanged root) is triggered from the universe resource via " +
			"`cert_rotation.server_cert_trigger` / `cert_rotation.client_cert_trigger`, not " +
			"from this resource.",

		CreateContext: resourceSelfSignedCertificateCreate,
		ReadContext:   resourceSelfSignedCertificateRead,
		DeleteContext: resourceCertificateDelete,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(certOperationTimeout),
			Delete: schema.DefaultTimeout(certOperationTimeout),
		},

		Schema: map[string]*schema.Schema{
			"label": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
				Description: "Name YugabyteDB Anywhere uses to identify the certificate. " +
					"Must be unique per customer. Certificate configurations cannot be " +
					"edited (the API has no update for this type), so changing the label " +
					"forces recreation of the resource.",
			},
			"certificate": {
				Type:             schema.TypeString,
				Optional:         true,
				Computed:         true,
				ForceNew:         true,
				RequiredWith:     []string{"private_key"},
				DiffSuppressFunc: suppressPEMContentDiff,
				Description: "Root certificate in PEM format, provided inline or via " +
					"`file(...)`. Omit (together with `private_key`) to have YugabyteDB " +
					"Anywhere mint a new self-signed root certificate; the minted " +
					"certificate is then exported through this attribute for distribution " +
					"to clients. Certificate configurations cannot be edited, so changing " +
					"the content forces recreation of the resource.",
			},
			"private_key": {
				Type:         schema.TypeString,
				Optional:     true,
				Sensitive:    true,
				WriteOnly:    true,
				RequiredWith: []string{"certificate"},
				Description: "Private key of the root certificate in PEM format, provided " +
					"inline or via `file(...)` or an ephemeral value. Required when " +
					"`certificate` is set. YugabyteDB Anywhere uses this key to sign " +
					"per-node server certificates. Write-only: never stored in the " +
					"Terraform plan or state, never returned by the API (for minted " +
					"certificates the key stays on the YugabyteDB Anywhere host only, and " +
					"imported resources cannot recover it). Requires Terraform 1.11+ to " +
					"set. The key rotates together with `certificate`, whose change " +
					"forces recreation of the resource.",
			},
			"uuid": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "UUID of the certificate configuration.",
			},
			"start_date": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "Creation date of the root certificate (RFC 3339).",
			},
			"expiry_date": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "Expiry date of the root certificate (RFC 3339).",
			},
			"in_use": {
				Type:     schema.TypeBool,
				Computed: true,
				Description: "True while at least one universe references this certificate. " +
					"Certificates in use cannot be deleted.",
			},
		},
	}
}

func resourceSelfSignedCertificateCreate(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{}) diag.Diagnostics {

	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID
	label := d.Get("label").(string)

	certContent := d.Get("certificate").(string)
	keyContent, err := writeOnlyStringAttr(d, "private_key")
	if err != nil {
		return diag.FromErr(err)
	}
	if certContent != "" && keyContent == "" {
		return diag.Errorf(
			"private_key must be provided together with certificate: " +
				"YugabyteDB Anywhere needs the root certificate's key to sign per-node " +
				"server certificates")
	}

	var certUUID string
	if certContent == "" {
		// Mint mode: YBA generates the root certificate. Routed through the
		// vanilla client because the generated CreateSelfSignedCert marshals
		// the request body incorrectly (bare string instead of {"label": ...}).
		vc := meta.(*api.APIClient).VanillaClient
		token := meta.(*api.APIClient).APIKey
		certUUID, err = vc.CreateSelfSignedCertificate(ctx, cUUID, token, label)
		if err != nil {
			return diag.FromErr(err)
		}
	} else {
		params := client.CertificateParams{
			Label:       label,
			CertType:    certTypeSelfSigned,
			CertContent: normalizePEM(certContent),
			KeyContent:  utils.GetStringPointer(normalizePEM(keyContent)),
		}
		certUUID, err = uploadCertificate(ctx, c, cUUID, params)
		if err != nil {
			return diag.FromErr(err)
		}
	}

	d.SetId(certUUID)
	return resourceSelfSignedCertificateRead(ctx, d, meta)
}

func resourceSelfSignedCertificateRead(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{}) diag.Diagnostics {

	// The exported PEM is how minted configurations hand the user the CA for
	// client distribution; for bring-your-own it keeps state aligned with
	// YBA's canonical stored form.
	return readCertificateResource(ctx, d, meta, "certificate")
}
