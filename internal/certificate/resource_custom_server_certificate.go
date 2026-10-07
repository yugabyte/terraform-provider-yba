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
		Description: "Manages a YugabyteDB Anywhere custom server certificate configuration " +
			"for client-to-node encryption in transit: your organization's root CA " +
			"certificate, and a server certificate and key signed by that CA. YugabyteDB " +
			"Anywhere places the server certificate and key on every node of the universe.\n\n" +
			"~> **Warning:** Use this certificate only as a universe's `client_root_ca`. " +
			"YugabyteDB Anywhere rejects it as `root_ca` (node-to-node encryption).\n\n" +
			"YugabyteDB Anywhere cannot edit a certificate configuration, so a change to any " +
			"argument forces replacement. When a universe uses the certificate, add " +
			"`lifecycle { create_before_destroy = true }`. Terraform then creates the " +
			"replacement and rotates the universe to it before it deletes the old " +
			"configuration. A destroy of a certificate that a universe still uses fails, and " +
			"the error names the universes.\n\n" +
			"To rotate to a server certificate that the same CA re-issued, change `label`, " +
			"`server_certificate` and `server_key`, and keep `root_certificate` as it is.\n\n" +
			"~> **Note:** Labels are unique per customer. With `create_before_destroy`, the " +
			"replacement exists at the same time as the old configuration, so give the " +
			"replacement a new `label`, for example with a date or a version in it.\n\n" +
			"~> **Note:** `server_key` is a write-only argument: Terraform never stores it " +
			"in the plan or the state file, so this resource requires Terraform 1.11 or " +
			"later. Terraform cannot detect a change to `server_key` alone, so change it " +
			"together with `server_certificate`, which forces replacement. YugabyteDB " +
			"Anywhere checks at upload that the server certificate and the key match, " +
			"unless the customer runtime configuration `yb.tls.enable_config_validation` " +
			"is `false`.\n\n" +
			"~> **Note:** YugabyteDB Anywhere never returns `server_certificate`, so after " +
			"`terraform import` the next plan proposes a replacement. The Import section " +
			"below shows how to keep the imported certificate.",

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
				Description: "Name of the certificate configuration in YugabyteDB Anywhere. " +
					"Must be unique per customer. A change forces replacement.",
			},
			"root_certificate": {
				Type:             schema.TypeString,
				Required:         true,
				ForceNew:         true,
				DiffSuppressFunc: suppressPEMContentDiff,
				Description: "Root CA certificate in PEM format, inline or from `file(...)`. " +
					"Clients use it to verify the server certificate. Every certificate in " +
					"it must be a CA certificate. A change forces replacement.",
			},
			"server_certificate": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
				Description: "Server certificate in PEM format, signed by the root CA, inline " +
					"or from `file(...)`. YugabyteDB Anywhere places it on every node for " +
					"client-to-node encryption. YugabyteDB Anywhere never returns it, so an " +
					"imported resource plans a replacement (see Import). A change forces " +
					"replacement.",
			},
			"server_key": {
				Type:      schema.TypeString,
				Required:  true,
				Sensitive: true,
				WriteOnly: true,
				Description: "Private key of the server certificate in PEM format, inline, " +
					"from `file(...)`, or from an ephemeral value. It must be an RSA key of " +
					"2048 bits or more. Write-only: Terraform never stores it in the plan or " +
					"state, and it requires Terraform 1.11 or later.",
			},
			"uuid": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "UUID of the certificate configuration.",
			},
			"start_date": {
				Type:     schema.TypeString,
				Computed: true,
				Description: "Start of the validity period of `root_certificate`, in RFC 3339 " +
					"format.",
			},
			"expiry_date": {
				Type:     schema.TypeString,
				Computed: true,
				Description: "End of the validity period of `root_certificate` (the earliest " +
					"expiry in the chain), in RFC 3339 format. This is not the expiry of " +
					"`server_certificate`.",
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

	// Server certificate and key never come back from the API and stay
	// state-only. The download is YBA's stored bundle (server cert prepended),
	// so it is filtered to the CA chain root_certificate holds.
	return readCertificateResource(ctx, d, meta, "root_certificate", caCertsPEM)
}
