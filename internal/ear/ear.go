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

// Package ear manages YugabyteDB Anywhere encryption-at-rest (EAR)
// configurations: the per-provider key management service (KMS) settings
// whose master key wraps a universe's universe keys. Each KMS provider is its
// own resource (yba_gcp_ear_config, ...) built on the earSpec factory, and
// yba_ear_config looks a configuration up by name.
package ear

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	client "github.com/yugabyte/platform-go-client"

	"github.com/yugabyte/terraform-provider-yba/internal/api"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// YBA KeyProvider names: the create path segment and the list metadata value.
const (
	providerAWS         = "AWS"
	providerAzure       = "AZU"
	providerCipherTrust = "CIPHERTRUST"
	providerGCP         = "GCP"
	providerHashicorp   = "HASHICORP"
)

// earEntityName labels errors from the shared helpers.
const earEntityName = "Encryption At Rest Config"

// earTaskTimeout bounds the create, edit and delete tasks. Each is short: YBA
// validates the settings against the KMS, and on create may also create the
// master key. The default leaves room for slow KMS endpoints.
const earTaskTimeout = 30 * time.Minute

// earConfig is one element of GET /kms_configs, decoded.
type earConfig struct {
	UUID      string
	Name      string
	Provider  string
	InUse     bool
	Universes []universeRef
	// Settings is the stored authConfig as YBA lists it: non-secret keys in
	// clear, credential values masked.
	Settings map[string]interface{}
}

type universeRef struct {
	UUID string
	Name string
}

// parseEARConfig decodes a list element ({"metadata": {...}, "credentials":
// {...}}). ok is false for an element without a UUID.
func parseEARConfig(raw map[string]interface{}) (earConfig, bool) {
	meta, ok := raw["metadata"].(map[string]interface{})
	if !ok {
		return earConfig{}, false
	}
	cfg := earConfig{
		UUID:     utils.StringValue(meta["configUUID"]),
		Name:     utils.StringValue(meta["name"]),
		Provider: utils.StringValue(meta["provider"]),
		Settings: map[string]interface{}{},
	}
	cfg.InUse, _ = meta["in_use"].(bool)
	if settings, ok := raw["credentials"].(map[string]interface{}); ok {
		cfg.Settings = settings
	}
	if universes, ok := meta["universeDetails"].([]interface{}); ok {
		for _, u := range universes {
			um, ok := u.(map[string]interface{})
			if !ok {
				continue
			}
			cfg.Universes = append(cfg.Universes, universeRef{
				UUID: utils.StringValue(um["uuid"]),
				Name: utils.StringValue(um["name"]),
			})
		}
	}
	return cfg, cfg.UUID != ""
}

// listEARConfigs returns every configuration of the customer. This is the only
// endpoint that carries a configuration's name and provider; the by-UUID GET
// returns the raw settings with credentials in clear, and only editSaved uses
// it.
func listEARConfigs(
	ctx context.Context, c *client.APIClient, cUUID string,
) ([]earConfig, error) {
	r, response, err := c.EncryptionAtRestAPI.ListKMSConfigs(ctx, cUUID).Execute()
	if err != nil {
		return nil, utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			earEntityName, "List")
	}
	out := make([]earConfig, 0, len(r))
	for _, raw := range r {
		if cfg, ok := parseEARConfig(raw); ok {
			out = append(out, cfg)
		}
	}
	return out, nil
}

func findEARConfig(
	ctx context.Context, c *client.APIClient, cUUID string, match func(earConfig) bool,
) (*earConfig, error) {
	configs, err := listEARConfigs(ctx, c, cUUID)
	if err != nil {
		return nil, err
	}
	for i := range configs {
		if match(configs[i]) {
			return &configs[i], nil
		}
	}
	return nil, nil
}

// getEARConfig returns the configuration with the UUID, or nil when YBA no
// longer lists it.
func getEARConfig(
	ctx context.Context, c *client.APIClient, cUUID string, configUUID string,
) (*earConfig, error) {
	return findEARConfig(ctx, c, cUUID, func(cfg earConfig) bool {
		return cfg.UUID == configUUID
	})
}

// getEARConfigByName returns the configuration with the name, or nil. Names
// are unique per customer in YBA.
func getEARConfigByName(
	ctx context.Context, c *client.APIClient, cUUID string, name string,
) (*earConfig, error) {
	return findEARConfig(ctx, c, cUUID, func(cfg earConfig) bool {
		return cfg.Name == name
	})
}

// createEARConfig submits the create task, waits for it, and returns the new
// configuration's UUID. YBA builds that mint the UUID before running the task
// return it as the task's resourceUUID; older builds return only the task
// UUID, and the configuration is then recovered by listing and matching on
// its name.
func createEARConfig(
	ctx context.Context, c *client.APIClient, cUUID string,
	provider string, name string, settings map[string]interface{}, timeout time.Duration,
) (string, error) {
	body := make(map[string]interface{}, len(settings)+1)
	for k, v := range settings {
		body[k] = v
	}
	body["name"] = name

	task, response, err := c.EncryptionAtRestAPI.CreateKMSConfig(ctx, cUUID, provider).
		KMSConfig(body).Execute()
	if err != nil {
		return "", utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			earEntityName, "Create")
	}
	if err = utils.WaitForTask(ctx, task.GetTaskUUID(), cUUID, c, timeout); err != nil {
		return "", err
	}
	if uuid := task.GetResourceUUID(); uuid != "" {
		return uuid, nil
	}
	cfg, err := getEARConfigByName(ctx, c, cUUID, name)
	if err != nil {
		return "", err
	}
	if cfg == nil {
		return "", fmt.Errorf(
			"encryption at rest config %q: create task %s succeeded but YugabyteDB Anywhere "+
				"does not list the configuration", name, task.GetTaskUUID())
	}
	return cfg.UUID, nil
}

// editEARConfig submits the edit task with the changed settings and waits for
// it. YBA checks the merged configuration against every universe that uses
// it, stores it, and then checks it with the KMS (EditKMSConfig,
// EncryptionAtRestService.updateAuthConfig). A failure in the KMS check fails
// the task but leaves the merged configuration stored.
func editEARConfig(
	ctx context.Context, c *client.APIClient, cUUID string,
	configUUID string, settings map[string]interface{}, timeout time.Duration,
) error {
	task, response, err := c.EncryptionAtRestAPI.EditKMSConfig(ctx, cUUID, configUUID).
		KMSConfig(settings).Execute()
	if err != nil {
		return utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			earEntityName, "Update")
	}
	return utils.WaitForTask(ctx, task.GetTaskUUID(), cUUID, c, timeout)
}

// editSaved reports whether YBA holds every setting that a failed edit sent,
// that is, whether YBA saved the edit before the step that failed. YBA saves
// the merged edit body unchanged, so the JSON forms match. The by-UUID GET
// returns credentials in clear; they are only compared, never logged or
// stored.
func editSaved(
	ctx context.Context, c *api.APIClient, configUUID string, sent map[string]interface{},
) (bool, error) {
	stored, err := c.VanillaClient.GetKMSConfig(ctx, c.CustomerID, configUUID, c.APIKey)
	if err != nil {
		return false, err
	}
	for k, v := range sent {
		want, err := json.Marshal(v)
		if err != nil {
			return false, err
		}
		got, err := json.Marshal(stored[k])
		if err != nil {
			return false, err
		}
		if !bytes.Equal(want, got) {
			return false, nil
		}
	}
	return true, nil
}

// earDelete is the DeleteContext of every provider's resource. A
// configuration that is already gone succeeds. A configuration with key
// history fails before the API call: YBA accepts the delete request and only
// fails the task, with a message that names neither the universes nor the
// reason the history persists.
func earDelete(s earSpec) schema.DeleteContextFunc {
	return func(
		ctx context.Context, d *schema.ResourceData, meta interface{},
	) diag.Diagnostics {
		apiClient := meta.(*api.APIClient)
		c := apiClient.YugawareClient
		cUUID := apiClient.CustomerID

		cfg, err := getEARConfig(ctx, c, cUUID, d.Id())
		if err != nil {
			return diag.FromErr(err)
		}
		if cfg == nil {
			tflog.Warn(ctx, fmt.Sprintf(
				"Encryption at rest config %s already removed from YugabyteDB Anywhere", d.Id()))
			d.SetId("")
			return nil
		}
		if cfg.InUse {
			return diag.Errorf(
				"encryption at rest config %q (%s) holds key history for universe(s) %s and "+
					"cannot be deleted", cfg.Name, d.Id(), formatUniverseRefs(cfg.Universes))
		}
		if err = s.checkRuntimeFlag(ctx, apiClient); err != nil {
			return diag.FromErr(err)
		}

		task, response, err := c.EncryptionAtRestAPI.DeleteKMSConfig(ctx, cUUID, d.Id()).
			Execute()
		if err != nil {
			return diag.FromErr(utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
				earEntityName, "Delete"))
		}
		if err = utils.WaitForTask(ctx, task.GetTaskUUID(), cUUID, c,
			d.Timeout(schema.TimeoutDelete)); err != nil {
			return diag.FromErr(err)
		}
		d.SetId("")
		return nil
	}
}

// ignoreAfterCreate is the DiffSuppressFunc of an argument that YBA uses only
// when it creates the master key, such as its size. For an existing key, YBA
// replaces the configured value with the key's own, and Read writes that into
// state. A key keeps these properties, and a replacement reuses the same key,
// so a difference after creation can never be applied. The argument keeps
// ForceNew, which applies on create and when another argument replaces the
// configuration: the SDK then diffs against an empty state, with no ID.
func ignoreAfterCreate(_, _, _ string, d *schema.ResourceData) bool {
	return d.Id() != ""
}

// setIntSetting writes an integer setting into attr, and leaves attr alone
// when YBA does not list the key. YBA lists a JSON number, or a string when a
// client created the configuration with one.
func setIntSetting(
	d *schema.ResourceData, attr string, settings map[string]interface{}, key string,
) error {
	v := utils.StringValue(settings[key])
	if v == "" {
		return nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fmt.Errorf("encryption at rest config %s: %s %q is not an integer",
			d.Id(), key, v)
	}
	return d.Set(attr, n)
}

func formatUniverseRefs(refs []universeRef) string {
	if len(refs) == 0 {
		return "(not listed)"
	}
	parts := make([]string, 0, len(refs))
	for _, r := range refs {
		parts = append(parts, fmt.Sprintf("%s (%s)", r.Name, r.UUID))
	}
	return strings.Join(parts, ", ")
}

// earSpec is what varies between the per-provider resources; earResource
// supplies the shared lifecycle (create with UUID recovery, provider-guarded
// read, credential-only update, history-aware delete, import, timeouts).
type earSpec struct {
	displayName string // human name for logs, e.g. "GCP KMS"
	apiProvider string // YBA KeyProvider, e.g. GCP
	description string // provider-specific lead of the resource docs
	fields      map[string]*schema.Schema
	// credentialFields are the arguments that carry or select the
	// credential. Read cannot restore them from the server (the list masks
	// secrets), so earUpdate reverts them when an edit fails and YBA did not
	// save it.
	credentialFields []string
	// buildCreate maps the resource arguments onto YBA's authConfig keys for
	// the create body. The factory adds the name.
	buildCreate func(d *schema.ResourceData) (map[string]interface{}, error)
	// buildEdit maps only the editable arguments onto the edit body. YBA's
	// EncryptionAtRestController merges the body with the stored
	// configuration by a rule that differs per provider; each builder states
	// its provider's rule.
	buildEdit func(d *schema.ResourceData) (map[string]interface{}, error)
	// flatten writes the non-secret settings YBA lists into state.
	flatten       func(d *schema.ResourceData, settings map[string]interface{}) error
	customizeDiff schema.CustomizeDiffFunc
	// warnings, when set, returns the diagnostics that a successful Create or
	// Update adds, such as a preview warning for the arguments in use.
	warnings func(d *schema.ResourceData) diag.Diagnostics
	// runtimeFlag, when set, is the global runtime config key that YBA
	// requires to be true before it creates, edits or deletes a configuration
	// of this provider. The resource reads it before each of these writes.
	runtimeFlag string
}

// checkRuntimeFlag fails when the spec's runtime flag is false. YBA rejects
// the write too, but the failed edit then also fails editSaved, which reads
// the configuration through a route that the flag also blocks.
func (s earSpec) checkRuntimeFlag(ctx context.Context, apiClient *api.APIClient) error {
	if s.runtimeFlag == "" {
		return nil
	}
	enabled, err := utils.GetGlobalRuntimeConfigBool(
		ctx, apiClient.YugawareClient, apiClient.CustomerID, s.runtimeFlag)
	if err != nil {
		return err
	}
	if !enabled {
		return fmt.Errorf("%s encryption at rest config requires global runtime config "+
			"%s=true (%s=false)", s.displayName, s.runtimeFlag, s.runtimeFlag)
	}
	return nil
}

func earResource(s earSpec) *schema.Resource {
	sch := map[string]*schema.Schema{
		"name": {
			Type:     schema.TypeString,
			Required: true,
			ForceNew: true,
			Description: "Name of the configuration. Must be unique per customer. " +
				"YugabyteDB Anywhere cannot rename a configuration, so a change forces " +
				"replacement.",
		},
		"uuid": {
			Type:        schema.TypeString,
			Computed:    true,
			Description: "UUID of the configuration.",
		},
		"in_use": {
			Type:     schema.TypeBool,
			Computed: true,
			Description: "Whether a universe holds key history for this configuration. " +
				"A configuration in use cannot be deleted.",
		},
	}
	for k, v := range s.fields {
		sch[k] = v
	}

	return &schema.Resource{
		Description: s.description + "\n\n" + earSharedNotes,

		CreateContext: earCreate(s),
		ReadContext:   earRead(s),
		UpdateContext: earUpdate(s),
		DeleteContext: earDelete(s),

		CustomizeDiff: s.customizeDiff,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(earTaskTimeout),
			Update: schema.DefaultTimeout(earTaskTimeout),
			Delete: schema.DefaultTimeout(earTaskTimeout),
		},

		Schema: sch,
	}
}

// earSharedNotes holds the lifecycle callouts every provider's resource
// shares; these strings ship verbatim into the user-facing docs. Import
// behaviour goes in each resource's template Import section.
const earSharedNotes = "~> **Note:** Only the arguments whose description says so can change in " +
	"place, and Terraform ignores a change after creation to the arguments whose description " +
	"says that. A change to any other argument forces replacement. YugabyteDB Anywhere does not " +
	"delete a configuration that a universe has used: it keeps the universe's key history, also " +
	"after encryption is disabled or the universe moves to another configuration, until the " +
	"universe is deleted. A destroy of such a configuration fails, and the error names the " +
	"universes. To move universes to a new configuration, create it, change " +
	"`encryption_at_rest.kms_config_uuid` on each universe, and keep the old configuration (or " +
	"remove it from state) until its universes are deleted. Give the new configuration " +
	"`depends_on` on the old one, so that `terraform destroy` deletes the universe before the " +
	"old configuration.\n\n" +
	"~> **Note:** Terraform refreshes `in_use` and the non-secret settings. YugabyteDB " +
	"Anywhere masks credentials, so a credential changed outside Terraform, for example in " +
	"the YugabyteDB Anywhere UI, does not show as drift. An apply sends a credential again " +
	"only when the credential or the authentication mode changes."

func earCreate(s earSpec) schema.CreateContextFunc {
	return func(
		ctx context.Context, d *schema.ResourceData, meta interface{},
	) diag.Diagnostics {
		apiClient := meta.(*api.APIClient)
		name := d.Get("name").(string)

		settings, err := s.buildCreate(d)
		if err != nil {
			return diag.FromErr(err)
		}
		if err = s.checkRuntimeFlag(ctx, apiClient); err != nil {
			return diag.FromErr(err)
		}
		tflog.Info(ctx, fmt.Sprintf("Creating %s encryption at rest config %q",
			s.displayName, name))
		configUUID, err := createEARConfig(ctx, apiClient.YugawareClient, apiClient.CustomerID,
			s.apiProvider, name, settings, d.Timeout(schema.TimeoutCreate))
		if err != nil {
			return diag.FromErr(err)
		}
		d.SetId(configUUID)
		return append(s.successWarnings(d), earRead(s)(ctx, d, meta)...)
	}
}

func (s earSpec) successWarnings(d *schema.ResourceData) diag.Diagnostics {
	if s.warnings == nil {
		return nil
	}
	return s.warnings(d)
}

// earRead refreshes the shared fields and the provider's non-secret settings,
// and guards the provider: a configuration whose YBA provider differs from
// this resource's (an import into the wrong resource) is an error, not silent
// drift. A configuration YBA no longer lists clears the ID.
func earRead(s earSpec) schema.ReadContextFunc {
	return func(
		ctx context.Context, d *schema.ResourceData, meta interface{},
	) diag.Diagnostics {
		apiClient := meta.(*api.APIClient)
		cfg, err := getEARConfig(ctx, apiClient.YugawareClient, apiClient.CustomerID, d.Id())
		if err != nil {
			return diag.FromErr(err)
		}
		if cfg == nil {
			tflog.Warn(ctx, fmt.Sprintf(
				"Encryption at rest config %s not found, removing from state", d.Id()))
			d.SetId("")
			return nil
		}
		if cfg.Provider != s.apiProvider {
			return diag.Errorf(
				"encryption at rest config %s (%q) uses key provider %s, not %s",
				d.Id(), cfg.Name, cfg.Provider, s.apiProvider)
		}
		if err := d.Set("name", cfg.Name); err != nil {
			return diag.FromErr(err)
		}
		if err := d.Set("uuid", cfg.UUID); err != nil {
			return diag.FromErr(err)
		}
		if err := d.Set("in_use", cfg.InUse); err != nil {
			return diag.FromErr(err)
		}
		if err := s.flatten(d, cfg.Settings); err != nil {
			return diag.FromErr(err)
		}
		return nil
	}
}

// earUpdate is reached only for the arguments YBA lets an edit change: every
// other argument is ForceNew. After a failed edit, the credential fields hold
// what YBA holds: the new values when YBA saved the edit before the step that
// failed (see editEARConfig), and the prior values when it did not, or when
// editSaved cannot tell. The next plan then shows what an apply changes. The
// deferred Read refreshes the rest.
func earUpdate(s earSpec) schema.UpdateContextFunc {
	return func(
		ctx context.Context, d *schema.ResourceData, meta interface{},
	) (diags diag.Diagnostics) {
		apiClient := meta.(*api.APIClient)

		// Always refresh state before returning. On success, Read errors are
		// propagated. On failure, they are swallowed so the original error is
		// preserved.
		defer func() {
			readDiags := earRead(s)(ctx, d, meta)
			if !diags.HasError() {
				diags = append(diags, readDiags...)
			}
		}()

		settings, err := s.buildEdit(d)
		if err == nil {
			err = s.checkRuntimeFlag(ctx, apiClient)
		}
		if err != nil {
			utils.RevertFields(d, s.credentialFields...)
			return diag.FromErr(err)
		}
		tflog.Info(ctx, fmt.Sprintf("Updating %s encryption at rest config %s credentials",
			s.displayName, d.Id()))
		if err := editEARConfig(ctx, apiClient.YugawareClient, apiClient.CustomerID, d.Id(),
			settings, d.Timeout(schema.TimeoutUpdate)); err != nil {
			diags = diag.FromErr(err)
			saved, getErr := editSaved(ctx, apiClient, d.Id(), settings)
			if getErr != nil {
				diags = append(diags, diag.Errorf(
					"encryption at rest config %s: read the stored settings after the failed "+
						"edit: %v", d.Id(), getErr)...)
			}
			if !saved {
				utils.RevertFields(d, s.credentialFields...)
			}
			return diags
		}
		return s.successWarnings(d)
	}
}
