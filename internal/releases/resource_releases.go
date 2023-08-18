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
	"os"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/customdiff"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/yugabyte/terraform-provider-yba/internal/api"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// ResourceReleases creates and maintains resource for releases
func ResourceReleases() *schema.Resource {
	return &schema.Resource{
		Description: "YBDB Release Version Import Resource.",

		CreateContext: resourceReleasesCreate,
		ReadContext:   resourceReleasesRead,
		DeleteContext: resourceReleasesDelete,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(60 * time.Minute),
			Update: schema.DefaultTimeout(30 * time.Minute),
			Delete: schema.DefaultTimeout(30 * time.Minute),
		},

		CustomizeDiff: resourceReleaseDiff(),

		Schema: map[string]*schema.Schema{
			"state": {
				Type:        schema.TypeString,
				Default:     nil,
				Computed:    true,
				Optional:    true,
				Description: "State of Release.",
			},
			"image_tag": {
				Type:        schema.TypeString,
				Computed:    true,
				Optional:    true,
				Description: "Docker Image Tag for the release.",
			},
			"notes": {
				Type:        schema.TypeList,
				Computed:    true,
				Optional:    true,
				Elem:        &schema.Schema{Type: schema.TypeString},
				Description: "Release Notes.",
			},
			"file_path": {
				Type:        schema.TypeString,
				Computed:    true,
				Optional:    true,
				Description: "File path where the release binary is stored.",
			},
			"chart_path": {
				Type:        schema.TypeString,
				Computed:    true,
				Optional:    true,
				Description: "File path where the release helm chart is stored.",
			},
			"version": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "Version name of the Package.",
			},
			"packages": PackageSchema(),
			"s3": {
				Type:        schema.TypeList,
				MaxItems:    1,
				ForceNew:    true,
				Elem:        S3Schema(),
				Optional:    true,
				Description: "Location of release binary in S3.",
			},
			"gcs": {
				Type:        schema.TypeList,
				MaxItems:    1,
				ForceNew:    true,
				Optional:    true,
				Elem:        GcsSchema(),
				Description: "Location of release binary in GCS.",
			},
			"http": {
				Type:        schema.TypeList,
				MaxItems:    1,
				ForceNew:    true,
				Optional:    true,
				Elem:        HTTPSchema(),
				Description: "Location of release binary in HTTP.",
			},
		},
	}
}

func resourceReleaseDiff() schema.CustomizeDiffFunc {
	return customdiff.All(
		customdiff.ValidateValue("s3", func(ctx context.Context, value,
			meta interface{}) error {
			s3 := value.([]interface{})
			if len(s3) > 0 {
				errorMessage := "Empty env variable: "
				var errorString string
				_, isPresentAccessKeyID := os.LookupEnv(utils.AWSAccessKeyEnv)
				if !isPresentAccessKeyID {
					errorString = fmt.Sprintf("%s%s ", errorString, utils.AWSAccessKeyEnv)
				}
				_, isPresentSecretAccessKey := os.LookupEnv(utils.AWSSecretAccessKeyEnv)
				if !isPresentSecretAccessKey {
					errorString = fmt.Sprintf("%s%s ", errorString, utils.AWSSecretAccessKeyEnv)
				}
				if !(isPresentAccessKeyID && isPresentSecretAccessKey) {
					errorString = fmt.Sprintf("%s%s", errorMessage, errorString)
					return fmt.Errorf(errorString)
				}
			}
			return nil
		}),
		customdiff.ValidateValue("gcs", func(ctx context.Context, value,
			meta interface{}) error {
			errorMessage := "Empty env variable: "
			gcs := value.([]interface{})
			if len(gcs) > 0 {
				_, isPresent := os.LookupEnv(utils.GCPCredentialsEnv)
				if !isPresent {
					return fmt.Errorf("%s%s", errorMessage, utils.GCPCredentialsEnv)
				}
			}
			return nil
		}),
	)
}

func resourceReleasesCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) (
	diag.Diagnostics) {
	var diags diag.Diagnostics

	cUUID := meta.(*api.APIClient).CustomerID
	token := meta.(*api.APIClient).APIKey

	s3 := d.Get("s3").([]interface{})
	gcs := d.Get("gcs").([]interface{})
	http := d.Get("http").([]interface{})
	version := d.Get("version").(string)

	s3Params, err := formatInputS3(ctx, s3)
	if err != nil {
		return diag.FromErr(err)
	}
	gcsParams, err := formatInputGcs(ctx, gcs)
	if err != nil {
		return diag.FromErr(err)
	}
	httpParams := formatInputHTTP(ctx, http)

	vc := meta.(*api.APIClient).VanillaClient
	resp, err := vc.ReleaseImport(ctx, cUUID, version, s3Params, gcsParams, httpParams, token)
	if err != nil {
		return diag.FromErr(err)
	}
	if resp {
		d.SetId(version)
		return resourceReleasesRead(ctx, d, meta)
	}

	return diags

}
func findReleases(ctx context.Context, releases map[string]map[string]interface{},
	version string) (map[string]interface{}, error) {
	for v, r := range releases {
		if v == version {
			return r, nil
		}
	}
	return nil, fmt.Errorf("Could not find release %s", version)
}

func resourceReleasesRead(ctx context.Context, d *schema.ResourceData, meta interface{}) (
	diag.Diagnostics) {
	var diags diag.Diagnostics
	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID

	_, response, err := c.ReleaseManagementApi.Refresh(ctx, cUUID).Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"Release", "Read - Refresh")
		return diag.FromErr(errMessage)
	}
	r, response, err := c.ReleaseManagementApi.GetListOfReleases(ctx, cUUID).IncludeMetadata(
		true).Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"Release", "Read")
		return diag.FromErr(errMessage)
	}

	var p map[string]interface{}
	p, err = findReleases(ctx, r, d.Id())
	if err != nil {
		return diag.FromErr(err)
	}

	if err = d.Set("image_tag", p["imageTag"]); err != nil {
		tflog.Error(ctx, "Image Tag Error")
		return diag.FromErr(err)
	}
	if err = d.Set("state", p["state"]); err != nil {
		tflog.Error(ctx, "State Error")
		return diag.FromErr(err)
	}
	if err = d.Set("notes", p["notes"]); err != nil {
		tflog.Error(ctx, "Notes Error")
		return diag.FromErr(err)
	}
	if err = d.Set("file_path", p["filePath"]); err != nil {
		tflog.Error(ctx, "File Path Error")
		return diag.FromErr(err)
	}
	if err = d.Set("chart_path", p["chartPath"]); err != nil {
		tflog.Error(ctx, "Chart Path Error")
		return diag.FromErr(err)
	}
	if err = d.Set("packages", p["packages"]); err != nil {
		tflog.Error(ctx, "Packages Error")
		return diag.FromErr(err)
	}

	if p["s3"] != nil {
		s3Formatted := formatOutputS3(ctx, p["s3"].(map[string]interface{}))
		if err = d.Set("s3", s3Formatted); err != nil {
			tflog.Error(ctx, "S3 Assignment Error")
			return diag.FromErr(err)
		}
	}

	if p["gcs"] != nil {
		gcsFormatted := formatOutputGcs(ctx, p["gcs"].(map[string]interface{}))
		if err = d.Set("gcs", gcsFormatted); err != nil {
			tflog.Error(ctx, "GCS Assignment Error")
			return diag.FromErr(err)
		}
	}

	if p["http"] != nil {
		httpFormatted := formatOutputHTTP(ctx, p["http"].(map[string]interface{}))
		if err = d.Set("http", httpFormatted); err != nil {
			tflog.Error(ctx, "HTTP Assignment Error")
			return diag.FromErr(err)
		}
	}

	return diags

}

func resourceReleasesDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) (
	diag.Diagnostics) {
	var diags diag.Diagnostics

	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID

	_, response, err := c.ReleaseManagementApi.DeleteRelease(ctx, cUUID, d.Id()).Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.ResourceEntity,
			"Release", "Delete")
		return diag.FromErr(errMessage)
	}
	d.SetId("")
	return diags
}
