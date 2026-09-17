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
	"context"
	"fmt"
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
	providerGCP = "GCP"
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
		UUID:     stringValue(meta["configUUID"]),
		Name:     stringValue(meta["name"]),
		Provider: stringValue(meta["provider"]),
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
				UUID: stringValue(um["uuid"]),
				Name: stringValue(um["name"]),
			})
		}
	}
	return cfg, cfg.UUID != ""
}

// listEARConfigs returns every configuration of the customer. This is the only
// endpoint that carries a configuration's name and provider; the by-UUID GET
// returns the raw settings with credentials in clear and is never used here.
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
// it. YBA re-validates the merged configuration against every universe that
// uses it before storing anything.
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

// resourceEARConfigDelete is the DeleteContext shared by every provider's
// resource. A configuration that is already gone succeeds. A configuration
// with key history fails before the API call: YBA accepts the delete request
// and only fails the task, with a message that names neither the universes
// nor the reason the history persists.
func resourceEARConfigDelete(
	ctx context.Context, d *schema.ResourceData, meta interface{},
) diag.Diagnostics {
	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID

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
			"encryption at rest config %q still holds key history for universe(s) %s. "+
				"YugabyteDB Anywhere keeps that history after encryption is disabled and after "+
				"a universe moves to another configuration, and refuses to delete the "+
				"configuration until the universes themselves are deleted. Remove the resource "+
				"from state instead if you want to stop managing it",
			cfg.Name, formatUniverseRefs(cfg.Universes))
	}

	task, response, err := c.EncryptionAtRestAPI.DeleteKMSConfig(ctx, cUUID, d.Id()).Execute()
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
	resourceType string // Terraform type, e.g. "yba_gcp_ear_config"
	displayName  string // human name for docs and logs, e.g. "GCP KMS"
	apiProvider  string // YBA KeyProvider, e.g. GCP
	description  string // provider-specific lead of the resource docs
	fields       map[string]*schema.Schema
	// credentialFields are the arguments YBA lets an edit change. They are
	// never read back, so a failed update reverts them to their prior state.
	credentialFields []string
	// buildCreate maps the resource arguments onto YBA's authConfig keys for
	// the create body. The factory adds the name.
	buildCreate func(d *schema.ResourceData) (map[string]interface{}, error)
	// buildEdit maps only the editable arguments onto the edit body; YBA
	// merges every other key from the stored configuration.
	buildEdit func(d *schema.ResourceData) (map[string]interface{}, error)
	// flatten writes the non-secret settings YBA lists into state.
	flatten func(d *schema.ResourceData, settings map[string]interface{}) error
	// createHint, when set, adds context to a failed create (for example a
	// YBA-version requirement of the arguments in use). It never inspects the
	// error itself.
	createHint    func(d *schema.ResourceData) string
	customizeDiff schema.CustomizeDiffFunc
}

func earResource(s earSpec) *schema.Resource {
	sch := map[string]*schema.Schema{
		"name": {
			Type:     schema.TypeString,
			Required: true,
			ForceNew: true,
			Description: "Name of the configuration, unique per customer. YugabyteDB " +
				"Anywhere does not allow renaming, so a change forces replacement.",
		},
		"uuid": {
			Type:        schema.TypeString,
			Computed:    true,
			Description: "UUID of the configuration.",
		},
		"in_use": {
			Type:     schema.TypeBool,
			Computed: true,
			Description: "True while any universe holds key history for this configuration. " +
				"Such a configuration cannot be deleted.",
		},
	}
	for k, v := range s.fields {
		sch[k] = v
	}

	return &schema.Resource{
		Description: s.description + "\n\n" + earSharedNotes(s),

		CreateContext: earCreate(s),
		ReadContext:   earRead(s),
		UpdateContext: earUpdate(s),
		DeleteContext: resourceEARConfigDelete,

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

// earSharedNotes renders the lifecycle callouts every provider's resource
// shares; these strings ship verbatim into the user-facing docs.
func earSharedNotes(s earSpec) string {
	return fmt.Sprintf(
		"~> **Note:** Only the credential arguments can change in place. Every other "+
			"argument is fixed by YugabyteDB Anywhere and forces replacement, and a "+
			"configuration that any universe has used cannot be deleted: YugabyteDB "+
			"Anywhere keeps the universe's key history after encryption is disabled and "+
			"after the universe moves to another configuration, and only deleting the "+
			"universe clears it. To move universes off a configuration, create the new "+
			"one, change each universe's `encryption_at_rest.kms_config_uuid`, and keep the "+
			"old configuration (or remove it from state) until its universes are gone.\n\n"+
			"~> **Drift Note:** Read refreshes `in_use` and the non-secret settings. "+
			"Credentials are never read back, because YugabyteDB Anywhere masks them: a "+
			"credential changed in the YugabyteDB Anywhere UI is not detected as drift. "+
			"Re-apply from Terraform to restore the intended value.\n\n"+
			"~> **Import Note:** Import verifies the provider: importing a configuration "+
			"that is not a %s configuration fails with the actual provider, so it can be "+
			"imported with the matching `yba_*_ear_config` resource instead. Credentials "+
			"cannot be recovered through the API and stay empty after import; the first "+
			"apply submits them again.",
		s.displayName)
}

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
		tflog.Info(ctx, fmt.Sprintf("Creating %s encryption at rest config %q",
			s.displayName, name))
		configUUID, err := createEARConfig(ctx, apiClient.YugawareClient, apiClient.CustomerID,
			s.apiProvider, name, settings, d.Timeout(schema.TimeoutCreate))
		if err != nil {
			if s.createHint != nil {
				if hint := s.createHint(d); hint != "" {
					return diag.FromErr(fmt.Errorf("%s: %w", hint, err))
				}
			}
			return diag.FromErr(err)
		}
		d.SetId(configUUID)
		return earRead(s)(ctx, d, meta)
	}
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
				"encryption at rest config %s (%q) uses key provider %s, not %s: import it "+
					"with the yba_*_ear_config resource matching its provider",
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

// earUpdate is reached only for credential changes: every other argument is
// ForceNew. A failed edit reverts the credential fields, which Read cannot
// restore from the server.
func earUpdate(s earSpec) schema.UpdateContextFunc {
	return func(
		ctx context.Context, d *schema.ResourceData, meta interface{},
	) diag.Diagnostics {
		apiClient := meta.(*api.APIClient)

		settings, err := s.buildEdit(d)
		if err != nil {
			utils.RevertFields(d, s.credentialFields...)
			return diag.FromErr(err)
		}
		tflog.Info(ctx, fmt.Sprintf("Updating %s encryption at rest config %s credentials",
			s.displayName, d.Id()))
		if err := editEARConfig(ctx, apiClient.YugawareClient, apiClient.CustomerID, d.Id(),
			settings, d.Timeout(schema.TimeoutUpdate)); err != nil {
			utils.RevertFields(d, s.credentialFields...)
			return diag.FromErr(err)
		}
		return earRead(s)(ctx, d, meta)
	}
}

// setIfNonEmpty writes an optional string setting only when it is set: YBA
// reads a missing key as "not configured", whereas an empty string is stored.
func setIfNonEmpty(out map[string]interface{}, key string, v interface{}) {
	if s, ok := v.(string); ok && s != "" {
		out[key] = s
	}
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

// boolValue reads a boolean setting that clients may have stored as a JSON
// boolean or as the strings "true"/"false".
func boolValue(in interface{}) bool {
	switch v := in.(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(v, "true")
	}
	return false
}
