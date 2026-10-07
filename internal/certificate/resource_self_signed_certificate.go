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
		Description: "Manages a YugabyteDB Anywhere self-signed certificate configuration for " +
			"universe encryption in transit. YugabyteDB Anywhere keeps the root certificate's " +
			"private key and uses it to sign the server certificate of each node.\n\n" +
			"The resource has two modes. In the generated mode, omit `certificate` and " +
			"`private_key`, and YugabyteDB Anywhere generates a new root certificate. By " +
			"default, the root certificate is valid for 4 years and the server certificates " +
			"for 1 year. In the bring-your-own mode, set both arguments to use your own root " +
			"certificate.\n\n" +
			"YugabyteDB Anywhere cannot edit a certificate configuration, so a change to any " +
			"argument forces replacement. When a universe uses the certificate, add " +
			"`lifecycle { create_before_destroy = true }`. Terraform then creates the " +
			"replacement and rotates the universe to it before it deletes the old " +
			"configuration. A destroy of a certificate that a universe still uses fails, and " +
			"the error names the universes.\n\n" +
			"~> **Note:** Labels are unique per customer. With `create_before_destroy`, the " +
			"replacement exists at the same time as the old configuration, so give the " +
			"replacement a new `label`, for example with a date or a version in it.\n\n" +
			"~> **Note:** `private_key` is a write-only argument: Terraform never stores it " +
			"in the plan or the state file. Setting it requires Terraform 1.11 or later; the " +
			"generated mode works with any Terraform version. Terraform cannot detect a " +
			"change to `private_key` alone, so change it together with `certificate`, which " +
			"forces replacement. YugabyteDB Anywhere checks at upload that the certificate " +
			"and the key match, unless the customer runtime configuration " +
			"`yb.tls.enable_config_validation` is `false`.\n\n" +
			"~> **Note:** To re-issue the server certificates from the same root certificate, " +
			"change a `cert_rotation` trigger on the `yba_universe` resource. This resource " +
			"does not re-issue them.",

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
				Description: "Name of the certificate configuration in YugabyteDB Anywhere. " +
					"Must be unique per customer. A change forces replacement.",
			},
			"certificate": {
				Type:             schema.TypeString,
				Optional:         true,
				Computed:         true,
				ForceNew:         true,
				RequiredWith:     []string{"private_key"},
				DiffSuppressFunc: suppressPEMContentDiff,
				Description: "Root certificate in PEM format, inline or from `file(...)`. " +
					"Every certificate in it must be a CA certificate. Omit it, together " +
					"with `private_key`, to have YugabyteDB Anywhere generate a new root " +
					"certificate; this attribute then holds the generated certificate, for " +
					"you to distribute to clients. A change forces replacement.",
			},
			"private_key": {
				Type:         schema.TypeString,
				Optional:     true,
				Sensitive:    true,
				WriteOnly:    true,
				RequiredWith: []string{"certificate"},
				Description: "Private key of the root certificate in PEM format, inline, from " +
					"`file(...)`, or from an ephemeral value. Required when `certificate` is " +
					"set. It must be an RSA key of 2048 bits or more. Write-only: Terraform " +
					"never stores it in the plan or state, and setting it requires Terraform " +
					"1.11 or later.",
			},
			"uuid": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "UUID of the certificate configuration.",
			},
			"start_date": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "Start of the root certificate's validity period, in RFC 3339 format.",
			},
			"expiry_date": {
				Type:     schema.TypeString,
				Computed: true,
				Description: "End of the root certificate's validity period, in RFC 3339 " +
					"format. For a certificate chain, the earliest expiry in the chain.",
			},
			"in_use": {
				Type:     schema.TypeBool,
				Computed: true,
				Description: "Whether a universe uses this certificate. A certificate in use " +
					"cannot be deleted.",
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
	return readCertificateResource(ctx, d, meta, "certificate", nil)
}
