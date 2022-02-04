package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func resourceUniverse() *schema.Resource {
	return &schema.Resource{
		Description: "Cloud Provider Resource",

		CreateContext: resourceUniverseCreate,
		ReadContext:   resourceUniverseRead,
		UpdateContext: resourceUniverseUpdate,
		DeleteContext: resourceUniverseDelete,

		Schema: map[string]*schema.Schema{
			"customer_id": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"clusters": {
				Type:     schema.TypeList,
				Required: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"cluster_type": {
							Type:     schema.TypeString,
							Required: true,
						},
						"user_intent": {
							Type:     schema.TypeList,
							MaxItems: 1,
							Required: true,
							Elem:     userIntentSchema(),
						},
					},
				},
			},
		},
	}
}

func userIntentSchema() *schema.Resource {
	return &schema.Resource{
		Schema: map[string]*schema.Schema{
			"universe_name": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"provider_type": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"provider": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"region_list": {
				Type:     schema.TypeList,
				Elem:     schema.TypeString,
				Optional: true,
			},
			"num_nodes": {
				Type:     schema.TypeInt,
				Optional: true,
			},
			"replication_factor": {
				Type:     schema.TypeInt,
				Optional: true,
			},
			"instance_type": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"device_info": {
				Type:     schema.TypeList,
				MaxItems: 1,
				Required: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"num_volumes": {
							Type:     schema.TypeInt,
							Optional: true,
						},
						"volume_size": {
							Type:     schema.TypeInt,
							Optional: true,
						},
						"storage_type": {
							Type:     schema.TypeString,
							Optional: true,
						},
					},
				},
			},
			"assign_public_ip": {
				Type:     schema.TypeBool,
				Optional: true,
			},
			"use_time_sync": {
				Type:     schema.TypeBool,
				Optional: true,
			},
			"enable_ysql": {
				Type:     schema.TypeBool,
				Optional: true,
			},
			"enable_yedis": {
				Type:     schema.TypeBool,
				Optional: true,
			},
			"enable_node_to_node_encrypt": {
				Type:     schema.TypeBool,
				Optional: true,
			},
			"enable_client_to_node_encrypt": {
				Type:     schema.TypeBool,
				Optional: true,
			},
			"enable_volume_encryption": {
				Type:     schema.TypeBool,
				Optional: true,
			},
			"yb_software_version": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"access_key_code": {
				Type:     schema.TypeBool,
				Optional: true,
			},
			"tserver_gflags": {
				Type:     schema.TypeMap,
				Elem:     schema.TypeString,
				Optional: true,
			},
			"master_gflags": {
				Type:     schema.TypeMap,
				Elem:     schema.TypeString,
				Optional: true,
			},
		},
	}
}

func resourceUniverseCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	p := buildUniverse(d)
	body, err := json.Marshal(p)
	if err != nil {
		return diag.FromErr(err)
	}

	c := meta.(*ApiClient)
	cUUID := d.Get("customer_id").(string)
	r, err := c.MakeRequest(http.MethodPost, fmt.Sprintf("api/v1/customers/%s/universes/clusters", cUUID), bytes.NewBuffer(body))
	if err != nil {
		return diag.FromErr(err)
	}
	defer r.Body.Close()

	res := make(map[string]interface{})
	if err = json.NewDecoder(r.Body).Decode(&res); err != nil {
		return diag.FromErr(err)
	}

	//err = waitForUniverseToBeActive(ctx, cUUID, d.Id(), c)
	//if err != nil {
	//	return diag.FromErr(err)
	//}

	d.SetId(res["resourceUUID"].(string))
	return resourceUniverseRead(ctx, d, meta)
}

func buildUniverse(d *schema.ResourceData) map[string]interface{} {
	// TODO: implement
	return nil
}

func resourceUniverseRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	// use the meta value to retrieve your client from the provider configure method
	// client := meta.(*apiClient)

	return diag.Errorf("not implemented")
}

func resourceUniverseUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	// use the meta value to retrieve your client from the provider configure method
	// client := meta.(*apiClient)

	return diag.Errorf("not implemented")
}

func resourceUniverseDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	c := meta.(*ApiClient)
	cUUID := d.Get("customer_id").(string)
	uUUID := d.Id()
	r, err := c.MakeRequest(http.MethodDelete, fmt.Sprintf("api/v1/customers/%s/universes/%s", cUUID, uUUID), nil)
	if err != nil {
		return diag.FromErr(err)
	}
	defer r.Body.Close()

	d.SetId("")
	return diags
}
