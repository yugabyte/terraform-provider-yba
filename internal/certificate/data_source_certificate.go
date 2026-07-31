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
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

	"github.com/yugabyte/terraform-provider-yba/internal/api"
)

// DataSourceCertificate defines the certificate lookup data source.
func DataSourceCertificate() *schema.Resource {
	return &schema.Resource{
		Description: "Looks up an encryption-in-transit certificate configuration by label. " +
			"Useful for referencing certificates created outside Terraform — for example " +
			"the self-signed root certificate YugabyteDB Anywhere generates automatically " +
			"when a universe is created with encryption enabled and no certificate set.",

		ReadContext: dataSourceCertificateRead,

		Schema: map[string]*schema.Schema{
			"label": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Label of the certificate configuration. Unique per customer.",
			},
			"uuid": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "UUID of the certificate configuration.",
			},
			"cert_type": {
				Type:     schema.TypeString,
				Computed: true,
				Description: "Type of the certificate configuration: SelfSigned, " +
					"CustomServerCert, CustomCertHostPath, HashicorpVault or K8SCertManager.",
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
				Type:        schema.TypeBool,
				Computed:    true,
				Description: "True while at least one universe references this certificate.",
			},
		},
	}
}

func dataSourceCertificateRead(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{}) diag.Diagnostics {

	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID
	label := d.Get("label").(string)

	cert, err := getCertificateByLabel(ctx, c, cUUID, label)
	if err != nil {
		return diag.FromErr(err)
	}
	if cert == nil {
		return diag.FromErr(fmt.Errorf(
			"no certificate with label %q found", label))
	}

	d.SetId(cert.GetUuid())
	if err = d.Set("uuid", cert.GetUuid()); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("cert_type", cert.GetCertType()); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("in_use", cert.GetInUse()); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("start_date", formatCertDate(cert.StartDateIso)); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("expiry_date", formatCertDate(cert.ExpiryDateIso)); err != nil {
		return diag.FromErr(err)
	}

	return nil
}
