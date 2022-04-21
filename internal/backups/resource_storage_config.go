package backups

import (
	"context"
	"errors"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	client "github.com/yugabyte/platform-go-client"
	"github.com/yugabyte/terraform-provider-yugabyte-platform/internal/api"
	"time"
)

func ResourceStorageConfig() *schema.Resource {
	return &schema.Resource{
		Description: "Storage Config Resource",

		CreateContext: resourceStorageConfigCreate,
		ReadContext:   resourceStorageConfigRead,
		UpdateContext: resourceStorageConfigUpdate,
		DeleteContext: resourceStorageConfigDelete,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(5 * time.Minute),
			Update: schema.DefaultTimeout(5 * time.Minute),
			Delete: schema.DefaultTimeout(5 * time.Minute),
		},

		Schema: map[string]*schema.Schema{
			"config_name": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "", // TODO: document
			},
			"data": {
				Type:        schema.TypeMap,
				Elem:        &schema.Schema{Type: schema.TypeString},
				Required:    true,
				Description: "", // TODO: document
			},
			"name": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "", // TODO: document
			},
		},
	}
}

func resourceStorageConfigCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	c := meta.(*api.ApiClient).YugawareClient
	cUUID := meta.(*api.ApiClient).CustomerId

	req := client.CustomerConfig{
		ConfigName:   d.Get("config_name").(string),
		CustomerUUID: cUUID,
		Data:         d.Get("data").(map[string]interface{}),
		Name:         d.Get("name").(string),
		Type:         "STORAGE",
	}
	r, _, err := c.CustomerConfigurationApi.CreateCustomerConfig(ctx, cUUID).Config(req).Execute()
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(*r.ConfigUUID)
	return resourceStorageConfigRead(ctx, d, meta)
}

func resourceStorageConfigRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	c := meta.(*api.ApiClient).YugawareClient
	cUUID := meta.(*api.ApiClient).CustomerId

	r, _, err := c.CustomerConfigurationApi.GetListOfCustomerConfig(ctx, cUUID).Execute()
	if err != nil {
		return diag.FromErr(err)
	}
	config, err := findCustomerConfig(r, d.Id())
	if err != nil {
		return diag.FromErr(err)
	}

	if err = d.Set("config_name", config.ConfigName); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("data", config.Data); err != nil {
		return diag.FromErr(err)
	}
	if err = d.Set("name", config.Name); err != nil {
		return diag.FromErr(err)
	}

	d.SetId(*config.ConfigUUID)
	return diags
}

func findCustomerConfig(configs []client.CustomerConfigUI, uuid string) (*client.CustomerConfigUI, error) {
	for _, c := range configs {
		if *c.ConfigUUID == uuid {
			return &c, nil
		}
	}
	return nil, errors.New("could not find config with id " + uuid)
}

func resourceStorageConfigUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	c := meta.(*api.ApiClient).YugawareClient
	cUUID := meta.(*api.ApiClient).CustomerId

	req := client.CustomerConfig{
		ConfigName:   d.Get("config_name").(string),
		CustomerUUID: cUUID,
		Data:         d.Get("data").(map[string]interface{}),
		Name:         d.Get("name").(string),
		Type:         "storage",
	}
	_, _, err := c.CustomerConfigurationApi.EditCustomerConfig(ctx, cUUID, d.Id()).Config(req).Execute()
	if err != nil {
		return diag.FromErr(err)
	}

	return resourceStorageConfigRead(ctx, d, meta)
}

func resourceStorageConfigDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	c := meta.(*api.ApiClient).YugawareClient
	cUUID := meta.(*api.ApiClient).CustomerId

	_, _, err := c.CustomerConfigurationApi.DeleteCustomerConfig(ctx, cUUID, d.Id()).Execute()
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId("")
	return diags
}
