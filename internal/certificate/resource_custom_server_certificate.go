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
)

// ResourceCustomServerCertificate defines the custom server certificate config resource.
func ResourceCustomServerCertificate() *schema.Resource {
	return &schema.Resource{
		Description: "Custom server certificate configuration for universe client-to-node " +
			"encryption in transit: a root CA certificate for verification plus an " +
			"externally-signed server certificate and key that YugabyteDB Anywhere places " +
			"on every DB node. Use this when your organization issues the client-to-node " +
			"certificates from its own CA.\n\n" +
			"~> **Warning:** YugabyteDB Anywhere accepts this certificate type only for " +
			"client-to-node TLS — reference it from the universe's `client_root_ca`. Using " +
			"it as `root_ca` (node-to-node) is rejected by the API.\n\n" +
			"~> **Note:** `server_key` is a write-only argument: Terraform never stores it " +
			"in the plan or the state file, so this resource requires Terraform 1.11 or " +
			"later. Because nothing is stored, a change to only `server_key` is not " +
			"detected — the key belongs to its `server_certificate` and they change " +
			"together, which forces the replacement. YugabyteDB Anywhere verifies at " +
			"upload that the certificate and key match.\n\n" +
			"Certificate configurations are immutable in YugabyteDB Anywhere: changing any " +
			"argument forces replacement, so add `lifecycle { create_before_destroy = true }` " +
			"when the certificate is referenced by a universe.\n\n" +
			"~> **Note:** After `terraform import`, `server_certificate` is empty in state " +
			"(the API never returns it), so the next plan proposes a replacement. Add " +
			"`lifecycle { ignore_changes = [server_certificate] }` to adopt the imported " +
			"certificate as-is, or recreate the resource from the original files instead " +
			"of importing.\n\n" +
			"~> **Note:** Labels are unique per customer, and with `create_before_destroy` " +
			"the replacement is created while the old configuration still exists. Give the " +
			"replacement a new `label` (include a date or version, for example), or the " +
			"create fails with a duplicate-label error.\n\n" +
			"To rotate a server " +
			"certificate re-issued from the same CA, create the replacement with identical " +
			"`root_certificate` content and the new `server_certificate`/`server_key`, then " +
			"point the universe's `client_root_ca` at it — YugabyteDB Anywhere detects the " +
			"unchanged root and performs the lightweight server-certificate rotation " +
			"instead of a full root swap.",

		CreateContext: resourceCustomServerCertificateCreate,
		ReadContext:   resourceCustomServerCertificateRead,
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
			"root_certificate": {
				Type:             schema.TypeString,
				Required:         true,
				ForceNew:         true,
				DiffSuppressFunc: suppressPEMContentDiff,
				Description: "Root CA certificate in PEM format, provided inline or via " +
					"`file(...)`. Clients use it to verify the server certificate. " +
					"Changing the content forces recreation of the resource.",
			},
			"server_certificate": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
				Description: "Server certificate in PEM format signed by the root CA, " +
					"provided inline or via `file(...)`. Placed on every DB node for " +
					"client-to-node TLS. Never returned by the API, so imported resources " +
					"cannot recover it and plan a replacement — see the import note above. " +
					"Changing the value forces recreation of the resource.",
			},
			"server_key": {
				Type:      schema.TypeString,
				Required:  true,
				Sensitive: true,
				WriteOnly: true,
				Description: "Private key of the server certificate in PEM format, " +
					"provided inline or via `file(...)` or an ephemeral value. " +
					"Write-only: never stored in the Terraform plan or state, never " +
					"returned by the API (imported resources cannot recover it). " +
					"Requires Terraform 1.11+. The key rotates together with " +
					"`server_certificate`, whose change forces recreation of the " +
					"resource.",
			},
			"uuid": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "UUID of the certificate configuration.",
			},
			"start_date": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "Creation date of the certificate (RFC 3339).",
			},
			"expiry_date": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "Expiry date of the certificate (RFC 3339).",
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

func resourceCustomServerCertificateCreate(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{}) diag.Diagnostics {

	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID

	serverKey, err := writeOnlyStringAttr(d, "server_key")
	if err != nil {
		return diag.FromErr(err)
	}
	if serverKey == "" {
		return diag.Errorf("server_key must be provided: YugabyteDB Anywhere places it " +
			"on every DB node together with the server certificate")
	}

	params := client.CertificateParams{
		Label:       d.Get("label").(string),
		CertType:    certTypeCustomServerCert,
		CertContent: normalizePEM(d.Get("root_certificate").(string)),
		CustomServerCertData: &client.CustomServerCertData{
			ServerCertContent: normalizePEM(d.Get("server_certificate").(string)),
			ServerKeyContent:  normalizePEM(serverKey),
		},
	}

	certUUID, err := uploadCertificate(ctx, c, cUUID, params)
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(certUUID)
	return resourceCustomServerCertificateRead(ctx, d, meta)
}

func resourceCustomServerCertificateRead(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{}) diag.Diagnostics {

	// The server certificate and key are never returned by the API and remain
	// state-only; only the root CA is read back. The download is the stored
	// bundle with the server certificate(s) prepended to the CA chain, so it
	// is filtered back down to the CA chain root_certificate actually holds.
	return readCertificateResource(ctx, d, meta, "root_certificate", caCertsPEM)
}
