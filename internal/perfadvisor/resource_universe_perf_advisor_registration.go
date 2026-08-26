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

const registrationTimeout = 30 * time.Minute

// ResourceUniversePerfAdvisorRegistration registers one universe with a Perf
// Advisor collector, in one of the three collection modes.
//
// A resource of its own rather than a field on yba_universe: registration is a
// task-based operation, and the universes being registered are frequently not
// the ones Terraform created (a BYOC setup adopts an existing fleet).
func ResourceUniversePerfAdvisorRegistration() *schema.Resource {
	return &schema.Resource{
		Description: previewAdmonition +
			"Registers a universe with a Perf Advisor collector.\n\n" +
			"Modes: `BASIC` collects and stores locally; `ADVANCED` also " +
			"remote-writes metrics into YBA's Prometheus; `ONLINE` forwards " +
			"everything to the `perf_advisor_endpoint_uuid` destination and " +
			"keeps nothing locally.\n\n" +
			"~> **Note:** Registration runs as a YBA task and this resource " +
			"waits for it. In `ONLINE` mode the endpoint is pushed to the " +
			"collector first, so a destination YBA cannot reach fails the " +
			"apply. Destroying the resource unregisters the universe; the " +
			"universe itself is never touched.",

		CreateContext: resourceRegistrationCreate,
		ReadContext:   resourceRegistrationRead,
		UpdateContext: resourceRegistrationCreate,
		DeleteContext: resourceRegistrationDelete,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(registrationTimeout),
			Read:   schema.DefaultTimeout(5 * time.Minute),
			Update: schema.DefaultTimeout(registrationTimeout),
			Delete: schema.DefaultTimeout(registrationTimeout),
		},

		Schema: map[string]*schema.Schema{
			"universe_uuid": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "UUID of the universe to register.",
			},
			"pa_collector_uuid": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "UUID of the Perf Advisor collector to register with.",
			},
			"mode": {
				Type:     schema.TypeString,
				Optional: true,
				Default:  "BASIC",
				ValidateFunc: validation.StringInSlice([]string{
					"BASIC", "ADVANCED", "ONLINE",
				}, false),
				Description: "Collection mode. One of BASIC, ADVANCED, ONLINE.",
			},
			"perf_advisor_endpoint_uuid": {
				Type:     schema.TypeString,
				Optional: true,
				Description: "Destination for ONLINE mode. Required for " +
					"ONLINE and rejected for any other mode.",
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

	status, response, err := c.YugawareClient.PACollectorAPI.
		CheckRegistered(ctx, c.CustomerID, d.Id()).Execute()
	if err != nil {
		if response != nil && response.StatusCode == 404 {
			// YBA answers 404 when the universe is not registered at all, which
			// is the shape "someone unregistered it out-of-band" takes.
			d.SetId("")
			return nil
		}
		return diag.FromErr(utils.ErrorFromHTTPResponse(
			response, err, "Universe Perf Advisor Registration", "Read", "Check"))
	}

	if err := d.Set("universe_uuid", d.Id()); err != nil {
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
