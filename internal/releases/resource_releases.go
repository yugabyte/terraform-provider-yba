package releases

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/api"
)

func ResourceReleases() *schema.Resource {
	return &schema.Resource{
		Description: "YBDB Release Version Import Resource",

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

		Schema: map[string]*schema.Schema{
			"state": {
				Type:        schema.TypeString,
				Default:     nil,
				Computed:    true,
				Optional:    true,
				Description: "State of Release",
			},
			"image_tag": {
				Type:        schema.TypeString,
				Computed:    true,
				Optional:    true,
				Description: "Docker Image Tag for the release",
			},
			"notes": {
				Type:        schema.TypeList,
				Computed:    true,
				Optional:    true,
				Elem:        &schema.Schema{Type: schema.TypeString},
				Description: "Release Notes",
			},
			"file_path": {
				Type:        schema.TypeString,
				Computed:    true,
				Optional:    true,
				Description: "File path where the release binary is stored",
			},
			"chart_path": {
				Type:        schema.TypeString,
				Computed:    true,
				Optional:    true,
				Description: "File path where the release helm chart is stored",
			},
			"version": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "Version name of the Package",
			},
			"packages": PackageSchema(),
			"s3": {
				Type:        schema.TypeList,
				MaxItems:    1,
				ForceNew:    true,
				Elem:        S3Schema(),
				Optional:    true,
				Description: "Location of release binary in S3",
			},
			"gcs": {
				Type:        schema.TypeList,
				MaxItems:    1,
				ForceNew:    true,
				Optional:    true,
				Elem:        GcsSchema(),
				Description: "Location of release binary in GCS",
			},
			"http": {
				Type:        schema.TypeList,
				MaxItems:    1,
				ForceNew:    true,
				Optional:    true,
				Elem:        HttpSchema(),
				Description: "Location of release binary in S3",
			},
		},
	}
}

func resourceReleasesCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	cUUID := meta.(*api.ApiClient).CustomerId
	token := meta.(*api.ApiClient).ApiKey

	s3 := d.Get("s3").([]interface{})
	gcs := d.Get("gcs").([]interface{})
	http := d.Get("http").([]interface{})
	version := d.Get("version").(string)

	s3_params := formatInputS3(ctx, s3)
	gcs_params := formatInputGcs(ctx, gcs)
	http_params := formatInputHttp(ctx, http)

	vc := meta.(*api.ApiClient).VanillaClient
	err, resp := vc.ReleaseImport(ctx, cUUID, version, s3_params, gcs_params, http_params, token)
	if err != nil {
		return diag.FromErr(err)
	}
	if resp {
		d.SetId(version)
		return resourceReleasesRead(ctx, d, meta)
	}

	return diags

}
func findReleases(ctx context.Context, releases map[string]map[string]interface{}, version string) (map[string]interface{}, error) {
	for v, r := range releases {
		if v == version {
			return r, nil
		}
	}
	return nil, errors.New(fmt.Sprintf("could not find release %s", version))
}

func resourceReleasesRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics
	c := meta.(*api.ApiClient).YugawareClient
	cUUID := meta.(*api.ApiClient).CustomerId

	r, _, err := c.ReleaseManagementApi.GetListOfReleases(ctx, cUUID).IncludeMetadata(true).Execute()
	if err != nil {
		return diag.FromErr(err)
	}

	var p map[string]interface{}
	p, err = findReleases(ctx, r, d.Id())
	if err != nil {
		return diag.FromErr(err)
	}

	if err = d.Set("image_tag", p["imageTag"]); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("state", p["state"]); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("notes", p["notes"]); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("file_path", p["filePath"]); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("chart_path", p["chartPath"]); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("packages", p["packages"]); err != nil {
		return diag.FromErr(err)
	}

	if p["s3"] != nil {
		s3_formatted := formatOutputS3(ctx, p["s3"].(map[string]interface{}))
		if err = d.Set("s3", s3_formatted); err != nil {
			return diag.FromErr(err)
		}
	}

	if p["gcs"] != nil {
		gcs_formatted := formatOutputGcs(ctx, p["gcs"].(map[string]interface{}))
		if err = d.Set("gcs", gcs_formatted); err != nil {
			return diag.FromErr(err)
		}
	}

	if p["http"] != nil {
		http_formatted := formatOutputHttp(ctx, p["http"].(map[string]interface{}))
		if err = d.Set("http", http_formatted); err != nil {
			return diag.FromErr(err)
		}
	}

	return diags

}

func resourceReleasesDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	c := meta.(*api.ApiClient).YugawareClient
	cUUID := meta.(*api.ApiClient).CustomerId

	_, _, err := c.ReleaseManagementApi.DeleteRelease(ctx, cUUID, d.Id()).Execute()
	if err != nil {
		return diag.FromErr(err)
	}
	d.SetId("")
	return diags
}
