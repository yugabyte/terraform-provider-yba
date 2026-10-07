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

	"github.com/yugabyte/terraform-provider-yba/internal/api"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

const (
	registrationTimeout     = 30 * time.Minute
	registrationReadTimeout = 5 * time.Minute
)

// ResourceUniversePerfAdvisorRegistration registers one universe with a Perf
// Advisor collector, in one of the three collection modes.
//
// A resource of its own rather than a field on yba_universe: registration is a
// task-based operation, and the universes being registered are frequently not
// the ones Terraform created (a BYOC setup adopts an existing fleet).
func ResourceUniversePerfAdvisorRegistration() *schema.Resource {
	return &schema.Resource{
		Description: "Registers a YugabyteDB Anywhere universe with a Perf " +
			"Advisor collector, which collects the universe's data for Perf " +
			"Advisor.\n\n" +
			previewAdmonition +
			"The `mode` sets where the data goes:\n\n" +
			"- `BASIC` (the default): YBA stores the data locally.\n" +
			"- `ADVANCED`: YBA stores the data locally and also writes the " +
			"metrics into its own Prometheus.\n" +
			"- `ONLINE`: YBA sends all the data to the Perf Advisor endpoint " +
			"in `perf_advisor_endpoint_uuid` and keeps no copy.\n\n" +
			"~> **Note:** A universe has one registration, and the resource ID " +
			"is the universe UUID. Use one resource per universe. Two " +
			"resources for the same `universe_uuid` overwrite each other on " +
			"every apply.\n\n" +
			"~> **Note:** `ONLINE` mode requires the runtime config key " +
			onlineModeKey + " set to `true` on the global scope or on your " +
			"customer scope. The key is `false` by default. YBA sends the " +
			"endpoint to the collector before it registers the universe, so " +
			"the apply fails when the endpoint cannot be reached or rejects " +
			"its credentials.\n\n" +
			"~> **Note:** YBA checks that it has enough free memory before it " +
			"registers the universe or moves it to `ADVANCED` mode. The memory " +
			"it needs grows with the number of TServers in the universe. When " +
			"YBA does not have enough free memory, the apply fails.\n\n" +
			"Registration and unregistration run as YBA universe tasks, and " +
			"this resource waits for them. They do not restart the universe. " +
			"YBA registers Kubernetes universes in the same way as VM " +
			"universes. Destroying the resource unregisters the universe from " +
			"the collector and leaves the universe running.",

		CreateContext: resourceRegistrationCreate,
		ReadContext:   resourceRegistrationRead,
		UpdateContext: resourceRegistrationCreate,
		DeleteContext: resourceRegistrationDelete,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(registrationTimeout),
			Read:   schema.DefaultTimeout(registrationReadTimeout),
			Update: schema.DefaultTimeout(registrationTimeout),
			Delete: schema.DefaultTimeout(registrationTimeout),
		},

		Schema: map[string]*schema.Schema{
			"universe_uuid": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
				Description: "UUID of the universe to register. Changing it " +
					"replaces the resource.",
			},
			"pa_collector_uuid": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
				Description: "UUID of the Perf Advisor collector, for example from " +
					"the `yba_pa_collector` data source. Changing it replaces the " +
					"resource.",
			},
			"mode": {
				Type:     schema.TypeString,
				Optional: true,
				Default:  "BASIC",
				ValidateFunc: validation.StringInSlice([]string{
					"BASIC", "ADVANCED", "ONLINE",
				}, false),
				Description: "Collection mode. Allowed values: `BASIC`, " +
					"`ADVANCED`, `ONLINE`. Defaults to `BASIC`. A change registers " +
					"the universe again in the new mode.",
			},
			"perf_advisor_endpoint_uuid": {
				Type:     schema.TypeString,
				Optional: true,
				Description: "UUID of the `yba_perf_advisor_endpoint` that " +
					"receives the data in `ONLINE` mode. Required for `ONLINE`. " +
					"Leave it unset for the other modes.",
			},
		},
	}
}

func resourceRegistrationCreate(
	ctx context.Context, d *schema.ResourceData, meta interface{},
) diag.Diagnostics {
	c := meta.(*api.APIClient)
	universeUUID := d.Get("universe_uuid").(string)
	collectorUUID := d.Get("pa_collector_uuid").(string)
	mode := d.Get("mode").(string)
	endpointUUID := d.Get("perf_advisor_endpoint_uuid").(string)

	// YBA rejects these combinations too, but failing here keeps the user out
	// of a task that can only fail.
	if mode == "ONLINE" && endpointUUID == "" {
		return diag.Errorf(
			"perf_advisor_endpoint_uuid is required when mode is ONLINE")
	}
	if mode != "ONLINE" && endpointUUID != "" {
		return diag.Errorf(
			"perf_advisor_endpoint_uuid only applies to ONLINE mode, not %s", mode)
	}

	req := c.YugawareClient.PACollectorAPI.
		RegisterUniverse(ctx, c.CustomerID, universeUUID, collectorUUID).
		Mode(mode)
	if endpointUUID != "" {
		req = req.PaEndpointUUID(endpointUUID)
	}

	tflog.Info(ctx, "Registering universe "+universeUUID+" in mode "+mode)
	task, response, err := req.Execute()
	if err != nil {
		return diag.FromErr(utils.ErrorFromHTTPResponse(
			response, err, "Universe Perf Advisor Registration", "Create", "Register"))
	}
	if err := utils.WaitForTask(
		ctx, task.GetTaskUUID(), c.CustomerID, c.YugawareClient,
		d.Timeout(schema.TimeoutCreate)); err != nil {
		return diag.FromErr(err)
	}

	d.SetId(universeUUID)
	return append(
		diag.Diagnostics{previewWarning("yba_universe_perf_advisor_registration")},
		resourceRegistrationRead(ctx, d, meta)...)
}

func resourceRegistrationRead(
	ctx context.Context, d *schema.ResourceData, meta interface{},
) diag.Diagnostics {
	c := meta.(*api.APIClient)

	// The v1 universe GET exists on every supported YBA, so a 404 here cannot
	// be an unknown route.
	uni, response, err := c.YugawareClient.UniverseManagementAPI.
		GetUniverse(ctx, c.CustomerID, d.Id()).Execute()
	if err != nil {
		if utils.IsUniverseMissing(response, err) {
			d.SetId("")
			return nil
		}
		return diag.FromErr(utils.ErrorFromHTTPResponse(
			response, err, "Universe Perf Advisor Registration", "Read", "Get Universe"))
	}
	// YBA keeps the collector UUID on the universe while it is registered and
	// clears it on unregister. This is the "not registered" signal; a
	// CheckRegistered 404 below surfaces as an error, because on a preview
	// route it can also mean that this YBA build lacks the route.
	collectorUUID := uni.UniverseDetails.GetPaCollectorUuid()
	if collectorUUID == "" {
		d.SetId("")
		return nil
	}

	status, response, err := c.YugawareClient.PACollectorAPI.
		CheckRegistered(ctx, c.CustomerID, d.Id()).Execute()
	if err != nil {
		return diag.FromErr(utils.ErrorFromHTTPResponse(
			response, err, "Universe Perf Advisor Registration", "Read", "Check"))
	}

	if err := d.Set("universe_uuid", d.Id()); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("pa_collector_uuid", collectorUUID); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("mode", status.GetMode()); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set(
		"perf_advisor_endpoint_uuid", status.GetPaEndpointUuid()); err != nil {
		return diag.FromErr(err)
	}
	return nil
}

func resourceRegistrationDelete(
	ctx context.Context, d *schema.ResourceData, meta interface{},
) diag.Diagnostics {
	c := meta.(*api.APIClient)

	tflog.Info(ctx, "Unregistering universe "+d.Id()+" from Perf Advisor")
	task, response, err := c.YugawareClient.PACollectorAPI.
		UnregisterUniverse(ctx, c.CustomerID, d.Id()).Execute()
	if err != nil {
		// A gone universe answers 400 "Cannot find universe". A 404 can mean
		// that this YBA build lacks the route, so it is not read as gone.
		if !utils.IsHTTPNotFound(response) && utils.IsUniverseMissing(response, err) {
			d.SetId("")
			return nil
		}
		return diag.FromErr(utils.ErrorFromHTTPResponse(
			response, err, "Universe Perf Advisor Registration", "Delete", "Unregister"))
	}
	// A universe that was already unregistered comes back with no task.
	if task.GetTaskUUID() != "" {
		if err := utils.WaitForTask(
			ctx, task.GetTaskUUID(), c.CustomerID, c.YugawareClient,
			d.Timeout(schema.TimeoutDelete)); err != nil {
			return diag.FromErr(err)
		}
	}
	d.SetId("")
	return nil
}
