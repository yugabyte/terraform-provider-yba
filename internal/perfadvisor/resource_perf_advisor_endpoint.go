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
		Description: "Manages a Perf Advisor endpoint in YugabyteDB Anywhere: an " +
			"external Perf Advisor that receives the data YBA collects from " +
			"universes registered in `ONLINE` mode. Register a universe against " +
			"an endpoint with `yba_universe_perf_advisor_registration`.\n\n" +
			previewAdmonition +
			"~> **Note:** Perf Advisor online mode must be on. Set the runtime " +
			"config key " + onlineModeKey + " to `true` on the global scope or " +
			"on your customer scope, for example with `yba_runtime_config`. " +
			"The key is `false` by default. While it is `false`, YBA rejects " +
			"every endpoint request, so `terraform plan` fails for this " +
			"resource.\n\n" +
			"~> **Note:** When a Perf Advisor collector exists, YBA uses it to " +
			"test both URLs and their credentials before it saves the endpoint. " +
			"The apply fails when a URL cannot be reached or a credential is " +
			"rejected, so you cannot create or change an endpoint while its " +
			"destination is down.\n\n" +
			"~> **Note:** YBA returns passwords masked, so Terraform cannot " +
			"detect a password that is changed outside Terraform. Terraform " +
			"sends the configured passwords with every update of the " +
			"endpoint. Terraform detects changes to every other field.\n\n" +
			"~> **Security Note:** Terraform stores the endpoint passwords in " +
			"the state file. They are marked sensitive. Use a secure state " +
			"backend and restrict access to your state files.",

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
				Description: "Name of the endpoint. It must be unique for the customer.",
			},
			"type": {
				Type:     schema.TypeString,
				Optional: true,
				Default:  "BYOC",
				ValidateFunc: validation.StringInSlice([]string{
					"BYOC", "PA_ONLINE",
				}, false),
				Description: "Kind of endpoint. Defaults to `BYOC`, the only kind " +
					"YBA accepts. A `BYOC` endpoint is a Perf Advisor that you run, " +
					"or a BYOC ingest gateway in front of one.",
			},
			"collection_endpoint": {
				Type:     schema.TypeString,
				Required: true,
				Description: "URL of the Collection API of the destination. The " +
					"collector sends all data except metrics to this URL.",
			},
			"collection_auth": authSchema(
				"Credentials for `collection_endpoint`."),
			"metrics_endpoint": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "URL that the collector sends metrics to.",
			},
			"metrics_type": {
				Type:     schema.TypeString,
				Optional: true,
				Default:  "otlphttp",
				ValidateFunc: validation.StringInSlice([]string{
					"otlphttp", "remotewrite",
				}, false),
				Description: "Protocol for `metrics_endpoint`. Allowed values: " +
					"`otlphttp`, `remotewrite`. Defaults to `otlphttp`.",
			},
			"metrics_auth": authSchema(
				"Credentials for `metrics_endpoint`."),
			"ybm_account_id": {
				Type:     schema.TypeString,
				Optional: true,
				Description: "YugabyteDB Managed account ID. YBA sends it in the " +
					"`YBM-Account-ID` header to both URLs. A BYOC ingest gateway " +
					"requires it. Leave it unset for a Perf Advisor that you run.",
			},
			"ybm_project_id": {
				Type:     schema.TypeString,
				Optional: true,
				Description: "YugabyteDB Managed project ID. YBA sends it in the " +
					"`YBM-Project-ID` header to both URLs.",
			},
			"universe_uuids": {
				Type:     schema.TypeList,
				Computed: true,
				Elem:     &schema.Schema{Type: schema.TypeString},
				Description: "UUIDs of the universes that are registered in " +
					"`ONLINE` mode against this endpoint.",
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
					Description: "Authentication type. Allowed values: `NONE`, " +
						"`BASIC`. Defaults to `NONE`.",
				},
				"username": {
					Type:        schema.TypeString,
					Optional:    true,
					Description: "Username. Required for `BASIC`.",
				},
				"password": {
					Type:        schema.TypeString,
					Optional:    true,
					Sensitive:   true,
					Description: "Password for `BASIC` authentication.",
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
	return append(
		diag.Diagnostics{previewWarning("yba_perf_advisor_endpoint")},
		resourcePerfAdvisorEndpointRead(ctx, d, meta)...)
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
