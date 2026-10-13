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
	// Maps each field to its dot-separated path in YBA's config. Create sends
	// the fields that the configuration sets, and Read reads every field back.
	configPaths   map[string]string
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
			DiffSuppressFunc: suppressMaskedMapValue,
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
		"~> **Note:** When YBA reads a telemetry provider, it masks the " +
		"credentials, and the values of tags whose keys look like credentials, " +
		"such as `api_owner`. When YBA returns the mask of the value in the " +
		"state, Terraform keeps the value in the state.\n\n" +
		"~> **Security Note:** Terraform stores the credentials of this " +
		"telemetry provider in the state file, marked sensitive. Use a " +
		"secure backend and restrict access to the state file."
}

// buildSinkConfig returns YBA's config with each field that the configuration
// sets at its path in paths. It omits "", false, 0 and an empty map: YBA reads
// a missing key as its own default, which can change, and a sent value pins it.
func buildSinkConfig(d *schema.ResourceData, paths map[string]string) map[string]interface{} {
	out := map[string]interface{}{}
	for field, path := range paths {
		v, ok := d.GetOk(field)
		if !ok {
			continue
		}
		keys := strings.Split(path, ".")
		m := out
		for _, k := range keys[:len(keys)-1] {
			next, ok := m[k].(map[string]interface{})
			if !ok {
				next = map[string]interface{}{}
				m[k] = next
			}
			m = next
		}
		m[keys[len(keys)-1]] = v
	}
	return out
}

func sinkCreate(s sinkSpec) schema.CreateContextFunc {
	return func(
		ctx context.Context, d *schema.ResourceData, meta interface{},
	) diag.Diagnostics {
		apiClient := meta.(*api.APIClient)
		cfg := buildSinkConfig(d, s.configPaths)
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

// Reads back every field that YBA returns. A type mismatch (import into the
// wrong sink resource) errors.
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
		tags := make(map[string]interface{}, len(provider.Tags))
		for k, v := range provider.Tags {
			tags[k] = v
		}
		if err := d.Set("tags", unmask("tags", tags, d.Get("tags"))); err != nil {
			return diag.FromErr(err)
		}
		for field, path := range s.configPaths {
			v, ok := configValue(provider.Config, path)
			if !ok {
				continue // YBA omits an unset field
			}
			key := path[strings.LastIndex(path, ".")+1:]
			if err := d.Set(field, unmask(key, v, d.Get(field))); err != nil {
				return diag.FromErr(err)
			}
		}
		return nil
	}
}

func configValue(cfg map[string]interface{}, path string) (interface{}, bool) {
	var v interface{} = cfg
	for _, k := range strings.Split(path, ".") {
		m, _ := v.(map[string]interface{})
		v = m[k]
	}
	return v, v != nil
}

// unmask returns YBA's value with every string that YBA masked (credentials,
// and keys such as api_owner) replaced by the state value it masks: else the
// ForceNew field replaces the provider on every apply. JSON numbers become
// ints for TypeInt fields.
func unmask(key string, v, state interface{}) interface{} {
	switch v := v.(type) {
	case string:
		if s, ok := state.(string); ok && utils.IsYBAMaskOf(key, v, s) {
			return s
		}
	case map[string]interface{}:
		stateMap, _ := state.(map[string]interface{})
		out := make(map[string]interface{}, len(v))
		for k, e := range v {
			out[k] = unmask(k, e, stateMap[k])
		}
		return out
	case float64:
		return int(v)
	}
	return v
}

// suppressMaskedMapValue hides the diff of a map value (tags, headers) that YBA
// masks because its key looks like a credential: import (or an older
// provider) leaves the mask in state. Known gap: a later change to a value with
// the same mask stays hidden.
func suppressMaskedMapValue(k, oldValue, newValue string, _ *schema.ResourceData) bool {
	if strings.HasSuffix(k, ".%") || newValue == "" || oldValue == newValue {
		return false
	}
	// k is "<attribute>.<map key>"; the map key can contain dots.
	_, key, _ := strings.Cut(k, ".")
	return utils.IsYBAMaskOf(key, oldValue, newValue)
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
