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

package releases

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	client "github.com/yugabyte/platform-go-client"

	"github.com/yugabyte/terraform-provider-yba/internal/api"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// ResourceRelease manages a YBDB software release in YugabyteDB Anywhere via
// the new release management (/ybdb_release) APIs.
func ResourceRelease() *schema.Resource {
	return &schema.Resource{
		Description: "YugabyteDB Release Resource. Manages a YBDB software release in " +
			"YugabyteDB Anywhere, including uploading release tarballs from the machine " +
			"running Terraform and registering per-architecture artifacts (x86_64, aarch64, " +
			"Kubernetes). The resource owns the release's complete artifact set: removing an " +
			"`artifact` block deletes that artifact from the release. Requires YugabyteDB " +
			"Anywhere version 2024.2.0.0-b1 (stable) or 2.23.1.0-b27 (preview) and above." +
			"\n\n~> **Note:** `local_file` tarballs are streamed to the YugabyteDB Anywhere " +
			"node over HTTP(S) and stored there. If an apply fails after a file was uploaded " +
			"but before the release was registered, the uploaded file stays on the node until " +
			"the next YugabyteDB Anywhere restart removes it. Deleting the release deletes its " +
			"uploaded files from the node.",

		CreateContext: resourceReleaseCreate,
		ReadContext:   resourceReleaseRead,
		UpdateContext: resourceReleaseUpdate,
		DeleteContext: resourceReleaseDelete,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(releaseCreateTimeout),
			Update: schema.DefaultTimeout(releaseUpdateTimeout),
			Delete: schema.DefaultTimeout(releaseDeleteTimeout),
		},

		CustomizeDiff: resourceReleaseDiff,

		Schema: map[string]*schema.Schema{
			"version": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
				Description: "Version of the release (e.g. 2024.2.3.0-b1). YugabyteDB Anywhere " +
					"allows a single release per version and rejects versions newer than the " +
					"YugabyteDB Anywhere installation itself. The API cannot change a " +
					"release's version, so changing this field forces recreation of the " +
					"release.",
			},
			"release_type": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
				ForceNew: true,
				ValidateDiagFunc: validation.ToDiagFunc(
					validation.StringInSlice(releaseTypes, false)),
				Description: "Type of the release. Allowed values: LTS, STS, PREVIEW. When " +
					"unset, it is inferred from the metadata of the first `local_file` " +
					"artifact; it must be set explicitly when every artifact uses " +
					"`package_url`. The update API cannot change a release's type, so " +
					"changing this field forces recreation of the release.",
			},
			"release_tag": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Tag of the release.",
			},
			"release_notes": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Release notes.",
			},
			"release_date_msecs": {
				Type:             schema.TypeInt,
				Optional:         true,
				Computed:         true,
				DiffSuppressFunc: suppressSubSecondDateDiff,
				Description: "Release date in milliseconds since epoch. When unset, it is " +
					"inferred from the metadata of the first `local_file` artifact. " +
					"YugabyteDB Anywhere stores updates to this field with second precision.",
			},
			"state": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
				ValidateDiagFunc: validation.ToDiagFunc(
					validation.StringInSlice(releaseStates, false)),
				Description: "State of the release. Allowed values: ACTIVE, DISABLED. A new " +
					"release becomes ACTIVE once it has a LINUX artifact; YugabyteDB Anywhere " +
					"keeps a release without one INCOMPLETE and rejects state changes until a " +
					"LINUX artifact is added, so this field cannot be set on a " +
					"Kubernetes-only release. May also read as INCOMPLETE or DELETED.",
			},
			"artifact": {
				Type:     schema.TypeList,
				Required: true,
				MinItems: 1,
				Description: "Artifacts of the release, at most one per (platform, " +
					"architecture) pair. The resource manages the complete set: removing a " +
					"block deletes that artifact from the release. Blocks are matched to " +
					"YugabyteDB Anywhere artifacts by platform and architecture, so " +
					"reordering blocks only produces a cosmetic diff.",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"platform": {
							Type:     schema.TypeString,
							Required: true,
							ValidateDiagFunc: validation.ToDiagFunc(
								validation.StringInSlice(releasePlatforms, false)),
							Description: "Platform of the artifact. Allowed values: LINUX, " +
								"KUBERNETES.",
						},
						"architecture": {
							Type:     schema.TypeString,
							Optional: true,
							ValidateDiagFunc: validation.ToDiagFunc(
								validation.StringInSlice(releaseArchitectures, false)),
							Description: "CPU architecture of the artifact. Allowed values: " +
								"x86_64, aarch64. Required when platform is LINUX; must not " +
								"be set when platform is KUBERNETES.",
						},
						"local_file": {
							Type:     schema.TypeString,
							Optional: true,
							Description: "Path to a release tarball (.tar.gz) on the machine " +
								"running Terraform, uploaded to the YugabyteDB Anywhere node " +
								"over HTTP(S). Exactly one of local_file or package_url must " +
								"be set. Changes to the file's content are not tracked - " +
								"change the path (e.g. the file name) to trigger a re-upload.",
						},
						"package_url": {
							Type:     schema.TypeString,
							Optional: true,
							Description: "HTTP(S) URL that YugabyteDB Anywhere downloads the " +
								"release package from on demand. Exactly one of local_file " +
								"or package_url must be set.",
						},
						"package_file_id": {
							Type:     schema.TypeString,
							Computed: true,
							Description: "UUID of the uploaded file in YugabyteDB Anywhere. " +
								"Empty for package_url artifacts.",
						},
						"sha256": {
							Type:     schema.TypeString,
							Computed: true,
							Description: "SHA256 checksum of the uploaded tarball, computed " +
								"by YugabyteDB Anywhere at upload time. Empty for " +
								"package_url artifacts and after import.",
						},
					},
				},
			},
		},
	}
}

// suppressSubSecondDateDiff hides sub-second drift on release_date_msecs: the
// update API takes seconds and truncates, so a configured millisecond value
// comes back rounded down after the first update.
func suppressSubSecondDateDiff(_, oldValue, newValue string, _ *schema.ResourceData) bool {
	o, errOld := strconv.ParseInt(oldValue, 10, 64)
	n, errNew := strconv.ParseInt(newValue, 10, 64)
	if errOld != nil || errNew != nil {
		return false
	}
	return o/1000 == n/1000
}

// resourceReleaseDiff runs the artifact invariants at plan time when every
// artifact value is known. Interpolated values unknown until apply skip the
// plan-time pass; Create and Update re-validate with the final values.
func resourceReleaseDiff(_ context.Context, diff *schema.ResourceDiff, _ interface{}) error {
	if !diff.NewValueKnown("artifact") {
		return nil
	}
	raw, ok := diff.Get("artifact").([]interface{})
	if !ok {
		return nil
	}
	for i := range raw {
		for _, field := range []string{"platform", "architecture", "local_file", "package_url"} {
			if !diff.NewValueKnown(fmt.Sprintf("artifact.%d.%s", i, field)) {
				return nil
			}
		}
	}
	return validateArtifactSpecs(expandArtifactSpecs(raw))
}

func resourceReleaseCreate(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{}) diag.Diagnostics {
	c := meta.(*api.APIClient).YugawareClient
	vc := meta.(*api.APIClient).VanillaClient
	apiKey := meta.(*api.APIClient).APIKey
	cUUID := meta.(*api.APIClient).CustomerID

	if err := newReleaseAPIVersionCheck(ctx, c); err != nil {
		return diag.FromErr(err)
	}

	version := d.Get("version").(string)
	specs := expandArtifactSpecs(d.Get("artifact").([]interface{}))
	if err := validateArtifactSpecs(specs); err != nil {
		return diag.FromErr(err)
	}
	configuredState := d.Get("state").(string)
	if configuredState != "" && !hasLinuxArtifact(specs) {
		return diag.Errorf(
			"state cannot be set on a release with no LINUX artifact: YugabyteDB Anywhere " +
				"keeps Kubernetes-only releases INCOMPLETE")
	}

	// Upload local files first; their extracted metadata backs the
	// release_type / release date inference below.
	inferredType := ""
	inferredDateMsecs := int64(0)
	for i := range specs {
		if specs[i].LocalFile == "" {
			continue
		}
		metadata, err := uploadArtifactFile(ctx, vc, cUUID, apiKey, version, i, &specs[i])
		if err != nil {
			return diag.FromErr(err)
		}
		if inferredType == "" {
			inferredType = metadata.ReleaseType
			inferredDateMsecs = metadata.ReleaseDateMsecs
		}
	}

	releaseType := d.Get("release_type").(string)
	if releaseType == "" {
		releaseType = strings.ToUpper(inferredType)
	}
	if releaseType == "" {
		return diag.Errorf(
			"release_type must be set when no artifact uses local_file")
	}
	releaseDateMsecs := int64(d.Get("release_date_msecs").(int))
	if releaseDateMsecs == 0 {
		releaseDateMsecs = inferredDateMsecs
	}

	req := client.CreateRelease{
		Artifacts:        toClientArtifacts(specs),
		ReleaseDateMsecs: releaseDateMsecs,
		ReleaseNotes:     d.Get("release_notes").(string),
		ReleaseTag:       d.Get("release_tag").(string),
		ReleaseType:      releaseType,
		Version:          version,
		YbType:           "YBDB",
	}
	r, response, err := c.NewReleaseManagementAPI.CreateNewRelease(
		ctx, cUUID).Release(req).Execute()
	if err != nil {
		return diag.FromErr(utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			version, "Create"))
	}
	if r.ResourceUUID == nil || *r.ResourceUUID == "" {
		return diag.Errorf("create release %s: response is missing the release UUID", version)
	}
	d.SetId(*r.ResourceUUID)
	// Persist the uploaded file IDs and checksums now: Read cannot recover
	// them (the GET response omits sha256), and a failure below must not
	// lose them.
	if err := d.Set("artifact", flattenArtifactSpecs(specs)); err != nil {
		return diag.FromErr(err)
	}

	// A new release starts INCOMPLETE and flips to ACTIVE with its first
	// LINUX artifact; only a different requested state needs a follow-up
	// update. If it fails, the release exists with its ID set, and the next
	// apply converges the state.
	if configuredState != "" && configuredState != "ACTIVE" {
		updateReq := api.ReleaseUpdateRequest{
			Artifacts:    toReleaseUpdateArtifacts(specs),
			ReleaseDate:  releaseDateMsecs / 1000,
			ReleaseNotes: d.Get("release_notes").(string),
			ReleaseTag:   d.Get("release_tag").(string),
			State:        configuredState,
		}
		if err := vc.UpdateRelease(ctx, cUUID, apiKey, d.Id(), updateReq); err != nil {
			return diag.FromErr(fmt.Errorf("release %s created; setting state failed: %w",
				version, err))
		}
	}

	return resourceReleaseRead(ctx, d, meta)
}

func resourceReleaseRead(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID

	r, response, err := c.NewReleaseManagementAPI.GetNewRelease(ctx, cUUID, d.Id()).Execute()
	if err != nil {
		if utils.IsReleaseNotFound(response, err) {
			tflog.Warn(ctx, fmt.Sprintf("Release %s not found, removing from state", d.Id()))
			d.SetId("")
			return diags
		}
		return diag.FromErr(utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			d.Id(), "Read"))
	}

	if err := d.Set("version", r.Version); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("release_type", r.ReleaseType); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("release_tag", r.ReleaseTag); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("release_notes", r.ReleaseNotes); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("release_date_msecs", int(r.ReleaseDateMsecs)); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("state", r.State); err != nil {
		return diag.FromErr(err)
	}

	// Match state blocks to YBA artifacts by (platform, architecture). The
	// GET response omits sha256 and never knew local_file, so both are
	// carried over from prior state; package_file_id and package_url are
	// refreshed from YBA.
	prior := expandArtifactSpecs(d.Get("artifact").([]interface{}))
	remote := map[string]client.Artifact{}
	for _, artifact := range r.Artifacts {
		remote[artifactSpec{
			Platform:     artifact.Platform,
			Architecture: artifact.Architecture,
		}.key()] = artifact
	}
	specs := make([]artifactSpec, 0, len(r.Artifacts))
	matched := map[string]bool{}
	for _, p := range prior {
		artifact, ok := remote[p.key()]
		if !ok {
			// Deleted out-of-band; dropping it from state surfaces the
			// removal as a re-add diff.
			continue
		}
		matched[p.key()] = true
		specs = append(specs, artifactSpec{
			Platform:      p.Platform,
			Architecture:  p.Architecture,
			LocalFile:     p.LocalFile,
			Sha256:        p.Sha256,
			PackageFileID: artifact.PackageFileId,
			PackageURL:    artifact.PackageUrl,
		})
	}
	for _, artifact := range r.Artifacts {
		key := artifactSpec{
			Platform:     artifact.Platform,
			Architecture: artifact.Architecture,
		}.key()
		if matched[key] {
			continue
		}
		// Added out-of-band or first Read after import.
		specs = append(specs, artifactSpec{
			Platform:      artifact.Platform,
			Architecture:  artifact.Architecture,
			PackageFileID: artifact.PackageFileId,
			PackageURL:    artifact.PackageUrl,
		})
	}
	if err := d.Set("artifact", flattenArtifactSpecs(specs)); err != nil {
		return diag.FromErr(err)
	}

	return diags
}

func resourceReleaseUpdate(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{}) diag.Diagnostics {
	c := meta.(*api.APIClient).YugawareClient
	vc := meta.(*api.APIClient).VanillaClient
	apiKey := meta.(*api.APIClient).APIKey
	cUUID := meta.(*api.APIClient).CustomerID

	if err := newReleaseAPIVersionCheck(ctx, c); err != nil {
		return diag.FromErr(err)
	}

	version := d.Get("version").(string)
	planSpecs := expandArtifactSpecs(d.Get("artifact").([]interface{}))
	if err := validateArtifactSpecs(planSpecs); err != nil {
		return diag.FromErr(err)
	}
	if d.HasChange("state") && d.Get("state").(string) != "" && !hasLinuxArtifact(planSpecs) {
		return diag.Errorf(
			"state cannot be changed on a release with no LINUX artifact: YugabyteDB " +
				"Anywhere keeps Kubernetes-only releases INCOMPLETE")
	}

	oldRaw, _ := d.GetChange("artifact")
	oldSpecs := expandArtifactSpecs(oldRaw.([]interface{}))
	oldByKey := map[string]artifactSpec{}
	for _, old := range oldSpecs {
		oldByKey[old.key()] = old
	}
	planKeys := map[string]bool{}
	for _, spec := range planSpecs {
		planKeys[spec.key()] = true
	}

	removedKeys, flipped := classifyArtifactChanges(oldSpecs, planSpecs)

	// Artifact deletions ahead - YBA rejects them on a release any universe
	// uses, so fail early with the universe names.
	if removedKeys || len(flipped) > 0 {
		r, response, err := c.NewReleaseManagementAPI.GetNewRelease(
			ctx, cUUID, d.Id()).Execute()
		if err != nil {
			return diag.FromErr(utils.ErrorFromHTTPResponse(response, err,
				utils.ResourceEntity, d.Id(), "Update - Get"))
		}
		if len(r.Universes) > 0 {
			return diag.FromErr(inUseReleaseError(version, "remove artifacts from",
				r.Universes))
		}
	}

	inferredDateMsecs := int64(0)
	for i := range planSpecs {
		spec := &planSpecs[i]
		old, existed := oldByKey[spec.key()]
		if flipped[spec.key()] {
			existed = false
		}
		switch {
		case spec.LocalFile != "":
			if existed && old.LocalFile == spec.LocalFile && old.PackageFileID != "" {
				spec.PackageFileID = old.PackageFileID
				spec.Sha256 = old.Sha256
				continue
			}
			metadata, err := uploadArtifactFile(ctx, vc, cUUID, apiKey, version, i, spec)
			if err != nil {
				return diag.FromErr(err)
			}
			if inferredDateMsecs == 0 {
				inferredDateMsecs = metadata.ReleaseDateMsecs
			}
		case spec.PackageURL != "":
			spec.PackageFileID = ""
			if existed && old.PackageURL == spec.PackageURL {
				spec.Sha256 = old.Sha256
			} else {
				spec.Sha256 = ""
			}
		}
	}

	releaseDateMsecs := int64(d.Get("release_date_msecs").(int))
	if releaseDateMsecs == 0 {
		releaseDateMsecs = inferredDateMsecs
	}
	baseReq := api.ReleaseUpdateRequest{
		ReleaseDate:  releaseDateMsecs / 1000,
		ReleaseNotes: d.Get("release_notes").(string),
		ReleaseTag:   d.Get("release_tag").(string),
		State:        d.Get("state").(string),
	}

	if len(flipped) > 0 {
		// Phase 1: update without the flipped artifacts so YBA deletes them;
		// the final update then recreates them with only the new source set.
		phase1 := make([]artifactSpec, 0, len(oldSpecs))
		for _, old := range oldSpecs {
			if planKeys[old.key()] && !flipped[old.key()] {
				phase1 = append(phase1, old)
			}
		}
		phase1Req := baseReq
		phase1Req.Artifacts = toReleaseUpdateArtifacts(phase1)
		if err := vc.UpdateRelease(ctx, cUUID, apiKey, d.Id(), phase1Req); err != nil {
			return diag.FromErr(err)
		}
	}

	finalReq := baseReq
	finalReq.Artifacts = toReleaseUpdateArtifacts(planSpecs)
	if err := vc.UpdateRelease(ctx, cUUID, apiKey, d.Id(), finalReq); err != nil {
		return diag.FromErr(err)
	}

	if err := d.Set("artifact", flattenArtifactSpecs(planSpecs)); err != nil {
		return diag.FromErr(err)
	}

	return resourceReleaseRead(ctx, d, meta)
}

func resourceReleaseDelete(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID

	// YBA rejects deleting a release any universe uses; fail early with the
	// universe names instead of the bare server error.
	r, response, err := c.NewReleaseManagementAPI.GetNewRelease(ctx, cUUID, d.Id()).Execute()
	if err != nil {
		if utils.IsReleaseNotFound(response, err) {
			d.SetId("")
			return diags
		}
		return diag.FromErr(utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			d.Id(), "Delete - Get"))
	}
	if len(r.Universes) > 0 {
		return diag.FromErr(inUseReleaseError(r.Version, "delete", r.Universes))
	}

	_, response, err = c.NewReleaseManagementAPI.DeleteNewRelease(ctx, cUUID, d.Id()).Execute()
	if err != nil {
		if utils.IsReleaseNotFound(response, err) {
			d.SetId("")
			return diags
		}
		return diag.FromErr(utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			d.Id(), "Delete"))
	}

	d.SetId("")
	return diags
}

// inUseReleaseError names the universes that block an operation on a release.
func inUseReleaseError(version, operation string, universes []client.Universe) error {
	names := make([]string, 0, len(universes))
	for _, universe := range universes {
		names = append(names, fmt.Sprintf("%s (%s)", universe.Name, universe.Uuid))
	}
	return fmt.Errorf("cannot %s release %s: it is in use by universe(s) %s",
		operation, version, strings.Join(names, ", "))
}
