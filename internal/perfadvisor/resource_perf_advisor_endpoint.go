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

package perfadvisor

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	clientv2 "github.com/yugabyte/platform-go-client/v2"

	"github.com/yugabyte/terraform-provider-yba/internal/api"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// maskedPassword is what YBA returns in place of a stored password, and what it
// accepts back to mean "keep the one you already have".
const maskedPassword = "********"

// ResourcePerfAdvisorEndpoint manages an external Perf Advisor destination that
// universes registered in online mode forward their collected data to.
//
// One resource covers every endpoint kind: the kinds differ in which URLs and
// auth types are valid, not in which fields exist, so `type` is a field rather
// than a separate resource per kind.
func ResourcePerfAdvisorEndpoint() *schema.Resource {
	return &schema.Resource{
		Description: previewAdmonition +
			"Perf Advisor Endpoint resource. Defines an external Perf Advisor " +
			"that universes registered in online mode send their collected " +
			"data to, attached to a universe via " +
			"`yba_universe_perf_advisor_registration`.\n\n" +
			"~> **Validation Note:** YBA probes both endpoints from a Perf " +
			"Advisor collector before storing anything, so an unreachable URL " +
			"or a rejected credential fails the apply rather than surfacing " +
			"later as silently dropped data. A destination that is temporarily " +
			"down therefore cannot be created or edited.\n\n" +
			"~> **Drift Note:** Passwords are read back masked, so they are " +
			"not reconciled against the server. Everything else is. A " +
			"password changed out-of-band in the YBA UI is not detected as " +
			"drift; re-apply from Terraform to restore the intended value.\n\n" +
			"~> **Security Note:** Endpoint passwords are stored in the " +
			"Terraform state file (marked sensitive). Use a secure backend " +
			"and restrict access to your state files.",

		CreateContext: resourcePerfAdvisorEndpointCreate,
		ReadContext:   resourcePerfAdvisorEndpointRead,
		UpdateContext: resourcePerfAdvisorEndpointUpdate,
		DeleteContext: resourcePerfAdvisorEndpointDelete,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(5 * time.Minute),
			Read:   schema.DefaultTimeout(5 * time.Minute),
			Update: schema.DefaultTimeout(5 * time.Minute),
			Delete: schema.DefaultTimeout(5 * time.Minute),
		},

		Schema: map[string]*schema.Schema{
			"name": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Name of the endpoint. Unique per customer.",
			},
			"type": {
				Type:     schema.TypeString,
				Optional: true,
				Default:  "BYOC",
				ValidateFunc: validation.StringInSlice([]string{
					"BYOC", "PA_ONLINE",
				}, false),
				Description: "Endpoint kind. One of BYOC, PA_ONLINE. " +
					"PA_ONLINE is reserved for the Yugabyte-hosted service and " +
					"is rejected by YBA until that exists.",
			},
			"collection_endpoint": {
				Type:     schema.TypeString,
				Required: true,
				Description: "URL of the destination's Collection API, where " +
					"everything other than metrics goes.",
			},
			"collection_auth": authSchema(
				"Credentials for the collection endpoint."),
			"metrics_endpoint": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "URL the collector sends metrics to.",
			},
			"metrics_type": {
				Type:     schema.TypeString,
				Optional: true,
				Default:  "otlphttp",
				ValidateFunc: validation.StringInSlice([]string{
					"otlphttp", "remotewrite",
				}, false),
				Description: "Metrics protocol. One of otlphttp, remotewrite.",
			},
			"metrics_auth": authSchema(
				"Credentials for the metrics endpoint."),
			"ybm_account_id": {
				Type:     schema.TypeString,
				Optional: true,
				Description: "YugabyteDB Managed account ID, sent as the " +
					"YBM-Account-ID header on both endpoints. Required by a " +
					"BYOC ingest gateway, and left unset for a plain Perf Advisor.",
			},
			"ybm_project_id": {
				Type:     schema.TypeString,
				Optional: true,
				Description: "YugabyteDB Managed project ID, sent as the " +
					"YBM-Project-ID header on both endpoints.",
			},
			"universe_uuids": {
				Type:     schema.TypeList,
				Computed: true,
				Elem:     &schema.Schema{Type: schema.TypeString},
				Description: "Universes currently registered in online mode " +
					"against this endpoint.",
			},
		},
	}
}

func authSchema(description string) *schema.Schema {
	return &schema.Schema{
		Type:        schema.TypeList,
		Optional:    true,
		MaxItems:    1,
		Description: description,
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				"type": {
					Type:         schema.TypeString,
					Optional:     true,
					Default:      "NONE",
					ValidateFunc: validation.StringInSlice([]string{"NONE", "BASIC"}, false),
					Description:  "Authentication type. One of NONE, BASIC.",
				},
				"username": {
					Type:        schema.TypeString,
					Optional:    true,
					Description: "Username. Required for BASIC.",
				},
				"password": {
					Type:        schema.TypeString,
					Optional:    true,
					Sensitive:   true,
					Description: "Password. Required for BASIC.",
				},
			},
		},
	}
}

func resourcePerfAdvisorEndpointCreate(
	ctx context.Context, d *schema.ResourceData, meta interface{},
) diag.Diagnostics {
	c := meta.(*api.APIClient)
	spec := buildEndpointSpec(d)

	tflog.Info(ctx, "Creating Perf Advisor endpoint "+spec.Name)
	endpoint, response, err := c.YugawareClientV2.PerfAdvisorEndpointAPI.
		CreatePerfAdvisorEndpoint(ctx, c.CustomerID).
		PerfAdvisorEndpointSpec(spec).
		Execute()
	if err != nil {
		return diag.FromErr(utils.ErrorFromHTTPResponse(
			response, err, "Perf Advisor Endpoint", "Create", "Create"))
	}
	if endpoint.Info == nil || endpoint.Info.Uuid == "" {
		return diag.Errorf("create Perf Advisor endpoint returned an empty UUID")
	}
	d.SetId(endpoint.Info.Uuid)

	return append(
		diag.Diagnostics{previewWarning("yba_perf_advisor_endpoint")},
		resourcePerfAdvisorEndpointRead(ctx, d, meta)...)
}

func resourcePerfAdvisorEndpointRead(
	ctx context.Context, d *schema.ResourceData, meta interface{},
) diag.Diagnostics {
	c := meta.(*api.APIClient)

	endpoint, response, err := c.YugawareClientV2.PerfAdvisorEndpointAPI.
		GetPerfAdvisorEndpoint(ctx, c.CustomerID, d.Id()).Execute()
	if err != nil {
		if response != nil && response.StatusCode == 404 {
			// Removed out-of-band: drop it from state rather than failing every plan.
			d.SetId("")
			return nil
		}
		return diag.FromErr(utils.ErrorFromHTTPResponse(
			response, err, "Perf Advisor Endpoint", "Read", "Get"))
	}

	spec := endpoint.Spec
	if spec == nil {
		d.SetId("")
		return nil
	}
	if err := d.Set("name", spec.Name); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("type", string(spec.Type)); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("collection_endpoint", spec.CollectionEndpoint); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("metrics_endpoint", spec.MetricsEndpoint); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("metrics_type", string(spec.MetricsType)); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("collection_auth",
		flattenAuth(spec.CollectionAuth, d.Get("collection_auth"))); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("metrics_auth",
		flattenAuth(spec.MetricsAuth, d.Get("metrics_auth"))); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("ybm_account_id", spec.GetYbmAccountId()); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("ybm_project_id", spec.GetYbmProjectId()); err != nil {
		return diag.FromErr(err)
	}
	if endpoint.Info != nil {
		if err := d.Set("universe_uuids", endpoint.Info.UniverseUuids); err != nil {
			return diag.FromErr(err)
		}
	}
	return nil
}

func resourcePerfAdvisorEndpointUpdate(
	ctx context.Context, d *schema.ResourceData, meta interface{},
) diag.Diagnostics {
	c := meta.(*api.APIClient)
	spec := buildEndpointSpec(d)

	tflog.Info(ctx, "Updating Perf Advisor endpoint "+d.Id())
	_, response, err := c.YugawareClientV2.PerfAdvisorEndpointAPI.
		EditPerfAdvisorEndpoint(ctx, c.CustomerID, d.Id()).
		PerfAdvisorEndpointSpec(spec).
		Execute()
	if err != nil {
		return diag.FromErr(utils.ErrorFromHTTPResponse(
			response, err, "Perf Advisor Endpoint", "Update", "Edit"))
	}
	return resourcePerfAdvisorEndpointRead(ctx, d, meta)
}

func resourcePerfAdvisorEndpointDelete(
	ctx context.Context, d *schema.ResourceData, meta interface{},
) diag.Diagnostics {
	c := meta.(*api.APIClient)

	tflog.Info(ctx, "Deleting Perf Advisor endpoint "+d.Id())
	response, err := c.YugawareClientV2.PerfAdvisorEndpointAPI.
		DeletePerfAdvisorEndpoint(ctx, c.CustomerID, d.Id()).Execute()
	if err != nil {
		// YBA refuses while a universe is still registered against it, and
		// names those universes - pass that through rather than retrying.
		return diag.FromErr(utils.ErrorFromHTTPResponse(
			response, err, "Perf Advisor Endpoint", "Delete", "Delete"))
	}
	d.SetId("")
	return nil
}

func buildEndpointSpec(d *schema.ResourceData) clientv2.PerfAdvisorEndpointSpec {
	spec := clientv2.PerfAdvisorEndpointSpec{
		Name:               d.Get("name").(string),
		Type:               clientv2.PerfAdvisorEndpointType(d.Get("type").(string)),
		CollectionEndpoint: d.Get("collection_endpoint").(string),
		MetricsEndpoint:    d.Get("metrics_endpoint").(string),
		MetricsType: clientv2.PerfAdvisorEndpointMetricsType(
			d.Get("metrics_type").(string)),
		CollectionAuth: expandAuth(d.Get("collection_auth")),
		MetricsAuth:    expandAuth(d.Get("metrics_auth")),
	}
	if v, ok := d.GetOk("ybm_account_id"); ok {
		spec.YbmAccountId = utils.GetStringPointer(v.(string))
	}
	if v, ok := d.GetOk("ybm_project_id"); ok {
		spec.YbmProjectId = utils.GetStringPointer(v.(string))
	}
	return spec
}

func expandAuth(raw interface{}) *clientv2.PerfAdvisorEndpointAuth {
	list, ok := raw.([]interface{})
	if !ok || len(list) == 0 || list[0] == nil {
		return nil
	}
	block := list[0].(map[string]interface{})
	auth := clientv2.PerfAdvisorEndpointAuth{
		Type: block["type"].(string),
	}
	if s, ok := block["username"].(string); ok && s != "" {
		auth.Username = utils.GetStringPointer(s)
	}
	if s, ok := block["password"].(string); ok && s != "" {
		auth.Password = utils.GetStringPointer(s)
	}
	return &auth
}

// flattenAuth reconciles the auth block without clobbering the password:
// YBA returns it masked, so the configured value is carried over and only the
// fields the server actually reports are refreshed. Without this every plan
// would show a password diff from the real value to "********".
func flattenAuth(
	auth *clientv2.PerfAdvisorEndpointAuth, configured interface{},
) []interface{} {
	if auth == nil {
		return nil
	}
	password := ""
	if list, ok := configured.([]interface{}); ok && len(list) > 0 && list[0] != nil {
		if block, ok := list[0].(map[string]interface{}); ok {
			if s, ok := block["password"].(string); ok {
				password = s
			}
		}
	}
	if reported := auth.GetPassword(); reported != "" && reported != maskedPassword {
		// Not masked, so it is a real value and worth reconciling.
		password = reported
	}
	return []interface{}{
		map[string]interface{}{
			"type":     auth.Type,
			"username": auth.GetUsername(),
			"password": password,
		},
	}
}
