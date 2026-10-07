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

const (
	typeDataDog         = "DATA_DOG"
	typeOTLP            = "OTLP"
	typeAWSCloudWatch   = "AWS_CLOUDWATCH"
	typeGCPCloudMonitor = "GCP_CLOUD_MONITORING"
	typeSplunk          = "SPLUNK"
	typeDynatrace       = "DYNATRACE"
	typeS3              = "S3"
)

type sinkSpec struct {
	resourceType string // e.g. yba_datadog_telemetry_provider
	apiType      string // config["type"], e.g. DATA_DOG
	description  string
	notes        string
	fields       map[string]*schema.Schema
	// Returns YBA's camelCase config; sinkCreate adds "type".
	buildConfig   func(d *schema.ResourceData) map[string]interface{}
	customizeDiff schema.CustomizeDiffFunc
}

func sinkResource(s sinkSpec) *schema.Resource {
	sch := map[string]*schema.Schema{
		"name": {
			Type:        schema.TypeString,
			Required:    true,
			ForceNew:    true,
			Description: "Name of the telemetry provider. YBA requires a unique name.",
		},
		"tags": {
			Type:     schema.TypeMap,
			Optional: true,
			ForceNew: true,
			Description: "Tags that YBA adds as attributes to every record that a " +
				"universe exports to this telemetry provider.",
			Elem:             &schema.Schema{Type: schema.TypeString},
			DiffSuppressFunc: suppressMaskedTag,
		},
	}
	for k, v := range s.fields {
		sch[k] = v
	}

	return &schema.Resource{
		Description: s.description + "\n\n" + previewAdmonition + "\n\n" +
			s.notes + sinkSharedNotes(),

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

const telemetryFlagsNote = "~> **Note:** YBA creates, reads and deletes " +
	"telemetry providers only when the global runtime config " +
	"`yb.universe.audit_logging_enabled`, `yb.universe.query_logging_enabled` " +
	"or `yb.universe.metrics_export_enabled` is `true`. Before YugabyteDB " +
	"Anywhere 2025.2.0.0, YBA checks only `yb.universe.audit_logging_enabled`, " +
	"and its default is `false`. From 2025.2.0.0, its default is `true`. To set " +
	"one, use the `yba_runtime_config` resource."

func sinkSharedNotes() string {
	return telemetryFlagsNote + "\n\n" +
		"~> **Note:** YBA cannot change a telemetry provider in place, so a " +
		"change to any argument replaces the resource. Before Terraform " +
		"deletes a telemetry provider, it removes the telemetry provider " +
		"from every universe that uses it. Each of those universes goes " +
		"through a rolling restart. The universes are not deleted.\n\n" +
		"~> **Note:** Terraform reads back only `name` and `tags`. YBA cannot " +
		"edit a telemetry provider after it creates one, so the other arguments " +
		"change only when Terraform replaces the telemetry provider.\n\n" +
		"~> **Security Note:** Terraform stores the credentials of this " +
		"telemetry provider in the state file, marked sensitive. Use a " +
		"secure backend and restrict access to the state file."
}

// Omits "": YBA reads a missing key as its default, and "" pins the field.
func setIfNonEmpty(out map[string]interface{}, key string, v interface{}) {
	utils.SetIfNonEmpty(out, key, v)
}

// Omits false: a missing key keeps YBA's default, which can change; false pins it.
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
			diag.Diagnostics{previewWarning(s.resourceType)},
			sinkRead(s)(ctx, d, meta)...)
	}
}

// Reads back only name and tags: YBA masks credentials, and every field is
// ForceNew. A type mismatch (import into the wrong sink resource) errors.
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
		tags := unmaskTags(d.Get("tags").(map[string]interface{}), provider.Tags)
		if err := d.Set("tags", tags); err != nil {
			return diag.FromErr(err)
		}
		return nil
	}
}

// YBA masks values of credential-like tag keys (api_owner). Keeps the state
// value, else ForceNew tags replace the provider on every apply.
func unmaskTags(state map[string]interface{}, reported map[string]string) map[string]string {
	tags := make(map[string]string, len(reported))
	for k, v := range reported {
		if s, ok := state[k].(string); ok && isYBAMaskOf(k, v, s) {
			v = s
		}
		tags[k] = v
	}
	return tags
}

// Import (or an older provider) leaves masked values in state. Known gap: a
// later change to a value with the same mask stays hidden.
func suppressMaskedTag(k, oldValue, newValue string, _ *schema.ResourceData) bool {
	if strings.HasSuffix(k, ".%") || newValue == "" || oldValue == newValue {
		return false
	}
	return isYBAMaskOf(strings.TrimPrefix(k, "tags."), oldValue, newValue)
}

// Mirrors YBA's CommonUtils.getMaskedValue.
func isYBAMaskOf(key, masked, value string) bool {
	v := []rune(value)
	if strings.Contains(strings.ToUpper(key), "PASSWORD") || len(v) < 5 {
		return masked == "********"
	}
	m := []rune(masked)
	if len(m) != len(v) {
		return false
	}
	for i := range m {
		if m[i] != v[i] && (m[i] != '*' || i < 2 || i >= len(v)-2) {
			return false
		}
	}
	return true
}

// Detaches first: YBA rejects deleting an in-use provider. A rejected delete
// re-detaches and retries once, instead of matching YBA's "in use" text.
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
	if len(retryDetached) == 0 { // nothing re-attached: not the in-use race
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

// utils.MapFromSingletonList panics on an empty or non-map list.
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
	return utils.StringValue(in)
}
