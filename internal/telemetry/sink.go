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

package telemetry

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

	"github.com/yugabyte/terraform-provider-yba/internal/api"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// YBA telemetry provider config type discriminators.
const (
	typeDataDog         = "DATA_DOG"
	typeOTLP            = "OTLP"
	typeAWSCloudWatch   = "AWS_CLOUDWATCH"
	typeGCPCloudMonitor = "GCP_CLOUD_MONITORING"
	typeSplunk          = "SPLUNK"
	typeDynatrace       = "DYNATRACE"
	typeS3              = "S3"
)

// sinkSpec is what varies between the per-sink telemetry provider resources;
// sinkResource supplies the shared lifecycle (create, type-guarded read,
// detach-before-delete, import, timeouts).
type sinkSpec struct {
	resourceType string // Terraform type, e.g. "yba_datadog_telemetry_provider"
	displayName  string // human name for docs, e.g. "Datadog"
	apiType      string // YBA config discriminator, e.g. DATA_DOG
	description  string // sink-specific lead sentence of the resource docs
	fields       map[string]*schema.Schema
	// buildConfig maps the flat resource fields onto YBA's camelCase config
	// keys. The factory adds the "type" discriminator itself.
	buildConfig   func(d *schema.ResourceData) map[string]interface{}
	customizeDiff schema.CustomizeDiffFunc
}

func sinkResource(s sinkSpec) *schema.Resource {
	sch := map[string]*schema.Schema{
		"name": {
			Type:        schema.TypeString,
			Required:    true,
			ForceNew:    true,
			Description: "Name of the telemetry provider configuration.",
		},
		"tags": {
			Type:        schema.TypeMap,
			Optional:    true,
			ForceNew:    true,
			Description: "Optional string tags associated with the configuration.",
			Elem:        &schema.Schema{Type: schema.TypeString},
		},
	}
	for k, v := range s.fields {
		sch[k] = v
	}

	return &schema.Resource{
		Description: experimentalAdmonition + s.description + "\n\n" +
			sinkSharedNotes(s),

		CreateContext: sinkCreate(s),
		ReadContext:   sinkRead(s),
		DeleteContext: resourceTelemetryProviderDelete,

		CustomizeDiff: s.customizeDiff,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(5 * time.Minute),
			Read:   schema.DefaultTimeout(5 * time.Minute),
			Delete: schema.DefaultTimeout(telemetryUpgradeTimeout),
		},

		Schema: sch,
	}
}

// sinkSharedNotes renders the lifecycle/drift/security callouts every sink
// resource shares; these strings ship verbatim into the user-facing docs.
func sinkSharedNotes(s sinkSpec) string {
	return fmt.Sprintf(
		"~> **Note:** YBA does not allow editing a telemetry provider in "+
			"place. Any change to a field forces Terraform to destroy and "+
			"recreate the resource. YBA also refuses to delete a provider that "+
			"is still referenced by a universe's telemetry config, so the "+
			"destroy step first enumerates every universe whose audit / query "+
			"/ metrics exporter list references this provider and rewrites "+
			"that list with the provider removed (via a rolling-upgrade task "+
			"on each universe). Once every detach task reaches a terminal "+
			"state, the provider itself is deleted. The universes themselves "+
			"are never destroyed — only their OpenTelemetry collector "+
			"configuration is updated.\n\n"+
			"~> **Drift Note:** Read refreshes only `name` and `tags`. The "+
			"%s connection fields are **not** reconciled against the server, "+
			"because YBA masks credentials in its responses and every field "+
			"is `ForceNew` anyway. A field edited out-of-band in the YBA UI "+
			"is therefore not detected as drift — re-apply from Terraform to "+
			"restore the intended configuration.\n\n"+
			"~> **Import Note:** Import verifies the provider's type: "+
			"importing a provider that is not a %s destination fails with "+
			"the actual type, so it can be imported with the matching "+
			"`yba_*_telemetry_provider` resource instead.\n\n"+
			"~> **Security Note:** Credentials such as API keys, tokens, and "+
			"secret access keys are stored in the Terraform state file "+
			"(marked sensitive). Use a secure backend and restrict access to "+
			"your state files.",
		s.displayName, s.displayName)
}

// setIfNonEmpty writes an optional string field into the config payload only
// when it is set: YBA reads a missing key as "use my default", whereas an
// empty string pins the field.
func setIfNonEmpty(out map[string]interface{}, key string, v interface{}) {
	if s, ok := v.(string); ok && s != "" {
		out[key] = s
	}
}

// setIfTrue writes an optional bool field only when true: a missing key lets
// YBA apply (and later change) its own default, an explicit false pins it.
func setIfTrue(out map[string]interface{}, key string, v interface{}) {
	if b, ok := v.(bool); ok && b {
		out[key] = b
	}
}

func sinkCreate(s sinkSpec) schema.CreateContextFunc {
	return func(
		ctx context.Context, d *schema.ResourceData, meta interface{},
	) diag.Diagnostics {
		apiClient := meta.(*api.APIClient)
		cfg := s.buildConfig(d)
		cfg["type"] = s.apiType

		tags := map[string]string{}
		if raw, ok := d.GetOk("tags"); ok {
			for k, v := range raw.(map[string]interface{}) {
				tags[k] = stringValue(v)
			}
		}

		req := api.TelemetryProvider{
			Name:   d.Get("name").(string),
			Config: cfg,
			Tags:   tags,
		}
		tflog.Info(ctx, fmt.Sprintf("Creating telemetry provider %q (type=%s)",
			req.Name, s.apiType))

		resp, err := apiClient.VanillaClient.CreateTelemetryProvider(
			ctx, apiClient.CustomerID, apiClient.APIKey, req)
		if err != nil {
			return diag.FromErr(err)
		}
		if resp.UUID == "" {
			return diag.Errorf("create telemetry provider returned an empty UUID")
		}
		d.SetId(resp.UUID)
		return append(
			diag.Diagnostics{experimentalWarning(s.resourceType)},
			sinkRead(s)(ctx, d, meta)...)
	}
}

// sinkRead refreshes name/tags and guards the sink type: a provider whose YBA
// type differs from this resource's sink (an import into the wrong resource)
// is an error, not silent drift. Config fields are not reconciled — YBA masks
// credentials and every field is ForceNew.
func sinkRead(s sinkSpec) schema.ReadContextFunc {
	return func(
		ctx context.Context, d *schema.ResourceData, meta interface{},
	) diag.Diagnostics {
		apiClient := meta.(*api.APIClient)
		//nolint:bodyclose // response body is closed inside GetTelemetryProvider
		provider, _, err := apiClient.VanillaClient.GetTelemetryProvider(
			ctx, apiClient.CustomerID, d.Id(), apiClient.APIKey)
		if err != nil {
			if errors.Is(err, api.ErrTelemetryProviderMissing) {
				tflog.Warn(ctx, fmt.Sprintf(
					"telemetry provider %q not found, removing from state", d.Id()))
				d.SetId("")
				return nil
			}
			return diag.FromErr(err)
		}
		if got, ok := provider.Config["type"].(string); ok && got != s.apiType {
			return diag.Errorf(
				"telemetry provider %s (%q) has type %s, not %s: import it "+
					"with the yba_*_telemetry_provider resource matching its type",
				d.Id(), provider.Name, got, s.apiType)
		}
		if err := d.Set("name", provider.Name); err != nil {
			return diag.FromErr(err)
		}
		if err := d.Set("tags", provider.Tags); err != nil {
			return diag.FromErr(err)
		}
		return nil
	}
}

// resourceTelemetryProviderDelete detaches the provider from every referencing
// universe before deleting it, since YBA rejects deleting an in-use provider. On
// a re-attach race (delete still rejected) it re-detaches and retries once,
// instead of substring-matching YBA's "in use" error. Shared by every sink: the
// delete flow is type-agnostic.
func resourceTelemetryProviderDelete(
	ctx context.Context, d *schema.ResourceData, meta interface{},
) diag.Diagnostics {
	apiClient := meta.(*api.APIClient)
	providerUUID := d.Id()
	timeout := d.Timeout(schema.TimeoutDelete)

	detached, err := detachTelemetryProviderFromUniverses(
		ctx, apiClient, providerUUID, timeout)
	if err != nil {
		return diag.FromErr(fmt.Errorf(
			"detach of telemetry provider %s failed after detaching "+
				"from %d universe(s) (%s): %w",
			providerUUID, len(detached), formatUniverseRefs(detached), err))
	}
	if len(detached) > 0 {
		tflog.Info(ctx, fmt.Sprintf(
			"Detached telemetry provider %s from %d universe(s) before "+
				"delete: %s",
			providerUUID, len(detached), formatUniverseRefs(detached)))
	}

	deleteErr := apiClient.VanillaClient.DeleteTelemetryProvider(
		ctx, apiClient.CustomerID, providerUUID, apiClient.APIKey)
	if deleteErr == nil {
		d.SetId("")
		return nil
	}

	// Delete rejected. Re-list: if nothing references the provider, this isn't
	// the in-use race — surface the original error verbatim.
	retryDetached, retryErr := detachTelemetryProviderFromUniverses(
		ctx, apiClient, providerUUID, timeout)
	if retryErr != nil {
		return diag.FromErr(fmt.Errorf(
			"telemetry provider %s could not be deleted (%v); subsequent "+
				"detach attempt also failed after detaching from %d "+
				"universe(s) (%s): %w",
			providerUUID, deleteErr, len(retryDetached),
			formatUniverseRefs(retryDetached), retryErr))
	}
	if len(retryDetached) == 0 {
		return diag.FromErr(deleteErr)
	}
	tflog.Warn(ctx, fmt.Sprintf(
		"telemetry provider %s was re-attached between detach and delete "+
			"(detached %d universe(s) on second pass: %s); retrying delete",
		providerUUID, len(retryDetached), formatUniverseRefs(retryDetached)))
	if err := apiClient.VanillaClient.DeleteTelemetryProvider(
		ctx, apiClient.CustomerID, providerUUID, apiClient.APIKey); err != nil {
		return diag.FromErr(fmt.Errorf(
			"telemetry provider %s keeps getting re-attached during deletion — "+
				"another writer (a separate Terraform state, a YBA UI user, or "+
				"an automation) is racing this destroy. It was detached from %d "+
				"universe(s) total (%s) but YBA still reports it in use. Stop the "+
				"other writer and retry the destroy: %w",
			providerUUID, len(detached)+len(retryDetached),
			formatUniverseRefs(append(detached, retryDetached...)), err))
	}
	d.SetId("")
	return nil
}

func formatUniverseRefs(refs []universeRef) string {
	if len(refs) == 0 {
		return "(none)"
	}
	parts := make([]string, 0, len(refs))
	for _, r := range refs {
		parts = append(parts, fmt.Sprintf("%s (%s)", r.Name, r.UUID))
	}
	return strings.Join(parts, ", ")
}

// Nil-tolerant guard over utils.MapFromSingletonList, which panics on bad input.
func firstMap(in interface{}) map[string]interface{} {
	list, ok := in.([]interface{})
	if !ok || len(list) == 0 {
		return map[string]interface{}{}
	}
	if _, isMap := list[0].(map[string]interface{}); !isMap {
		return map[string]interface{}{}
	}
	return utils.MapFromSingletonList(list)
}

func stringValue(in interface{}) string {
	if in == nil {
		return ""
	}
	if s, ok := in.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", in)
}
