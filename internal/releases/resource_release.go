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
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	client "github.com/yugabyte/platform-go-client"

	"github.com/yugabyte/terraform-provider-yba/internal/api"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// 3h: Create/Update stream multi-GB tarballs, then wait while YBA hashes and
// untars each one synchronously.
const releaseOperationTimeout = 3 * time.Hour

// ResourceRelease manages a YBDB release through the /ybdb_release API.
func ResourceRelease() *schema.Resource {
	return &schema.Resource{
		Description: "YugabyteDB Release Resource. Manages a release of the YugabyteDB (YBDB) " +
			"database software that YugabyteDB Anywhere stores and uses to deploy universes." +
			"\n\n" + previewAdmonition +
			"A release is a YBDB version, such as 2024.2.3.0-b116, not a version of YugabyteDB " +
			"Anywhere. " +
			"Use the `yba_release_version` data source to look up these YBDB versions. Each " +
			"`artifact` block adds one package to the release: a LINUX package for x86_64 or " +
			"aarch64, or a KUBERNETES Helm chart. The provider uploads the package from the " +
			"machine that runs Terraform, or gives YBA a URL to download it from. The resource " +
			"manages all the artifacts of the release: when you remove an `artifact` block, " +
			"the provider deletes that artifact from the release. Requires the global runtime " +
			"config `yb.releases.use_redesign` to be `true`, which is the default." +
			"\n\n~> **Note:** Some YugabyteDB Anywhere versions cannot change the source of an " +
			"artifact in place. So when `local_file` or `package_url` changes, the provider " +
			"deletes the artifact and adds it again. While a universe uses the release, " +
			"YugabyteDB Anywhere does not let you delete or replace an artifact or change " +
			"`state`. The apply then fails with an error that names the universes. You can " +
			"add an artifact and change `release_tag`, `release_notes` or " +
			"`release_date_msecs` while a universe uses the release." +
			"\n\n~> **Note:** The provider uploads each `local_file` tarball to the " +
			"YugabyteDB Anywhere host over HTTP(S), and YBA stores the file there. When you " +
			"delete the release, YBA deletes the files of its current artifacts. Two kinds " +
			"of file stay on the host: a file that an apply uploads before it fails to " +
			"create the release, and the file of an artifact that an update replaces or " +
			"removes.",

		CreateContext: resourceReleaseCreate,
		ReadContext:   resourceReleaseRead,
		UpdateContext: resourceReleaseUpdate,
		DeleteContext: resourceReleaseDelete,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(releaseOperationTimeout),
			Update: schema.DefaultTimeout(releaseOperationTimeout),
			Delete: schema.DefaultTimeout(releaseOperationTimeout),
		},

		CustomizeDiff: resourceReleaseDiff,

		Schema: map[string]*schema.Schema{
			"version": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
				Description: "YBDB version of the release, for example 2024.2.3.0-b116. This " +
					"is not a YugabyteDB Anywhere version. YugabyteDB Anywhere allows one " +
					"release for each version. It rejects a version newer than its own " +
					"version unless the global runtime config " +
					"`yb.allow_db_version_more_than_yba_version` or `yb.skip_version_checks` " +
					"is `true`. YBA cannot change the version of a release, so a change to " +
					"this field replaces the release.",
			},
			"release_type": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
				ForceNew: true,
				ValidateDiagFunc: validation.ToDiagFunc(
					validation.StringInSlice(releaseTypes, false)),
				Description: "Type of the release. Allowed values: LTS, STS, PREVIEW. When " +
					"unset, the provider reads the type from the metadata of the first " +
					"`local_file` artifact. Set this field when every artifact uses " +
					"`package_url`. YBA cannot change the type of a release, so a change to " +
					"this field replaces the release.",
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
				Description: "Release date in milliseconds since the Unix epoch. When unset, " +
					"the provider reads the date from the metadata of the first `local_file` " +
					"artifact. When you change this field, YugabyteDB Anywhere stores the new " +
					"date with second precision.",
			},
			"state": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
				ValidateDiagFunc: validation.ToDiagFunc(
					validation.StringInSlice(releaseStates, false)),
				Description: "State of the release. Allowed values: ACTIVE, DISABLED. You " +
					"cannot select a DISABLED release when you create a universe. A release " +
					"is INCOMPLETE until it has a LINUX artifact, and then it becomes ACTIVE. " +
					"YugabyteDB Anywhere does not change the state of an INCOMPLETE release or " +
					"of a release that a universe uses. To set this field on an existing " +
					"Kubernetes-only release, add the LINUX artifact in one apply and set this " +
					"field in the next apply. The value can also read as INCOMPLETE or " +
					"DELETED.",
			},
			"artifact": {
				Type:     schema.TypeList,
				Required: true,
				MinItems: 1,
				Description: "Artifacts of the release, at most one for each platform and " +
					"architecture. The resource manages all the artifacts: when you remove a " +
					"block, the provider deletes that artifact from the release. When the " +
					"`local_file` or `package_url` of a block changes, the provider deletes " +
					"the artifact and adds it again. The provider matches blocks to artifacts " +
					"by platform and architecture. If you only change the order of the " +
					"blocks, the plan shows a change, but the apply does not change the " +
					"release.",
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
								"x86_64, aarch64. Required when `platform` is LINUX. Do not " +
								"set it when `platform` is KUBERNETES.",
						},
						"local_file": {
							Type:     schema.TypeString,
							Optional: true,
							Description: "Path to a release tarball (.tar.gz) on the machine " +
								"that runs Terraform. The provider uploads it to the " +
								"YugabyteDB Anywhere host over HTTP(S). The version, platform " +
								"and architecture in the tarball must match the release and " +
								"this block. Set exactly one of `local_file` or " +
								"`package_url`. The provider does not track the content of " +
								"the file, so it does not detect a new file at the same " +
								"path. To upload a file again, change the path (for example, " +
								"the file name).",
						},
						"package_url": {
							Type:     schema.TypeString,
							Optional: true,
							Description: "HTTP(S) URL of the release package. YugabyteDB " +
								"Anywhere downloads the package from this URL when it needs " +
								"it, so the URL must stay available. Set exactly one of " +
								"`local_file` or `package_url`.",
						},
						"package_file_id": {
							Type:     schema.TypeString,
							Computed: true,
							Description: "UUID of the file of the artifact in YugabyteDB " +
								"Anywhere: the uploaded `local_file`, or the Kubernetes Helm " +
								"chart that YBA downloaded from `package_url`.",
						},
						"sha256": {
							Type:     schema.TypeString,
							Computed: true,
							Description: "SHA-256 checksum of the uploaded tarball. " +
								"YugabyteDB Anywhere computes it when the provider uploads " +
								"the file. Empty for `package_url` artifacts, Kubernetes " +
								"charts and imported artifacts.",
						},
					},
				},
			},
		},
	}
}

// The update API takes seconds, so a configured millisecond value reads back
// truncated.
func suppressSubSecondDateDiff(_, oldValue, newValue string, _ *schema.ResourceData) bool {
	o, errOld := strconv.ParseInt(oldValue, 10, 64)
	n, errNew := strconv.ParseInt(newValue, 10, 64)
	if errOld != nil || errNew != nil {
		return false
	}
	return o/1000 == n/1000
}

// Unknown artifact values skip plan-time validation; Create and Update
// re-validate.
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

	if err := releaseAPICheck(ctx, meta.(*api.APIClient)); err != nil {
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

	// Upload first: tarball metadata fills an unset release_type and date.
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
	// GET omits sha256, so record it before anything below can fail.
	if err := d.Set("artifact", flattenArtifactSpecs(specs)); err != nil {
		return diag.FromErr(err)
	}

	// The LINUX artifact already made the release ACTIVE; only DISABLED
	// needs an update.
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

	return append(diag.Diagnostics{previewWarning("yba_ybdb_release")},
		resourceReleaseRead(ctx, d, meta)...)
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

	// GET omits sha256 and local_file, so both carry over from state.
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
			// Deleted out-of-band: dropping it plans a re-add.
			continue
		}
		matched[p.key()] = true
		packageURL := artifact.PackageUrl
		if packageURL == "" {
			// GET hides the URL once YBA caches a downloaded Kubernetes chart.
			packageURL = p.PackageURL
		}
		specs = append(specs, artifactSpec{
			Platform:      p.Platform,
			Architecture:  p.Architecture,
			LocalFile:     p.LocalFile,
			Sha256:        p.Sha256,
			PackageFileID: artifact.PackageFileId,
			PackageURL:    packageURL,
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
	meta interface{}) (diags diag.Diagnostics) {
	c := meta.(*api.APIClient).YugawareClient
	vc := meta.(*api.APIClient).VanillaClient
	apiKey := meta.(*api.APIClient).APIKey
	cUUID := meta.(*api.APIClient).CustomerID

	// Read carries local_file and sha256 over from state, so an update that
	// fails before YBA has the planned artifacts must not save them.
	applied := false
	defer func() {
		if diags.HasError() && !applied {
			utils.RevertFields(d, "artifact")
		}
	}()

	if err := releaseAPICheck(ctx, meta.(*api.APIClient)); err != nil {
		return diag.FromErr(err)
	}

	version := d.Get("version").(string)
	planSpecs := expandArtifactSpecs(d.Get("artifact").([]interface{}))
	if err := validateArtifactSpecs(planSpecs); err != nil {
		return diag.FromErr(err)
	}
	// YBA applies state before artifacts, so adding the first LINUX artifact
	// does not unlock it within the same update.
	if oldState, _ := d.GetChange("state"); d.HasChange("state") && oldState == "INCOMPLETE" {
		return diag.Errorf("state cannot change while the release is INCOMPLETE")
	}

	oldRaw, _ := d.GetChange("artifact")
	oldSpecs := expandArtifactSpecs(oldRaw.([]interface{}))
	oldByKey := map[string]artifactSpec{}
	for _, old := range oldSpecs {
		oldByKey[old.key()] = old
	}

	removed, replaced := classifyArtifactChanges(oldSpecs, planSpecs)

	// YBA blocks these on an in-use release; fail before uploading, and name
	// the universes, which YBA's own error does not.
	if removed || len(replaced) > 0 || d.HasChange("state") {
		r, response, err := c.NewReleaseManagementAPI.GetNewRelease(
			ctx, cUUID, d.Id()).Execute()
		if err != nil {
			return diag.FromErr(utils.ErrorFromHTTPResponse(response, err,
				utils.ResourceEntity, d.Id(), "Update - Get"))
		}
		if len(r.Universes) > 0 {
			operation := "change the state of"
			if removed || len(replaced) > 0 {
				operation = "delete or replace artifacts of"
			}
			return diag.FromErr(inUseReleaseError(version, operation, r.Universes))
		}
	}

	inferredDateMsecs := int64(0)
	for i := range planSpecs {
		spec := &planSpecs[i]
		if old, existed := oldByKey[spec.key()]; existed && !replaced[spec.key()] {
			// A URL artifact omits the chart file ID that YBA may have cached.
			spec.PackageFileID = ""
			if spec.LocalFile != "" {
				spec.PackageFileID = old.PackageFileID
			}
			spec.Sha256 = old.Sha256
			continue
		}
		if spec.LocalFile == "" {
			spec.PackageFileID = ""
			spec.Sha256 = ""
			continue
		}
		metadata, err := uploadArtifactFile(ctx, vc, cUUID, apiKey, version, i, spec)
		if err != nil {
			return diag.FromErr(err)
		}
		if inferredDateMsecs == 0 {
			inferredDateMsecs = metadata.ReleaseDateMsecs
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

	if len(replaced) > 0 {
		// Delete replaced artifacts first; see classifyArtifactChanges.
		kept := make([]artifactSpec, 0, len(planSpecs))
		for _, spec := range planSpecs {
			if _, existed := oldByKey[spec.key()]; existed && !replaced[spec.key()] {
				kept = append(kept, spec)
			}
		}
		keptReq := baseReq
		keptReq.Artifacts = toReleaseUpdateArtifacts(kept)
		if err := vc.UpdateRelease(ctx, cUUID, apiKey, d.Id(), keptReq); err != nil {
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
	applied = true

	return append(diag.Diagnostics{previewWarning("yba_ybdb_release")},
		resourceReleaseRead(ctx, d, meta)...)
}

func resourceReleaseDelete(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID

	// YBA's own in-use error does not name the universes.
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
		return diag.FromErr(utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			d.Id(), "Delete"))
	}

	d.SetId("")
	return diags
}

func inUseReleaseError(version, operation string, universes []client.Universe) error {
	names := make([]string, 0, len(universes))
	for _, universe := range universes {
		names = append(names, fmt.Sprintf("%s (%s)", universe.Name, universe.Uuid))
	}
	return fmt.Errorf("cannot %s release %s: it is in use by universe(s) %s",
		operation, version, strings.Join(names, ", "))
}
