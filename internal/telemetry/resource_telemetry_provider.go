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
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/yugabyte/terraform-provider-yba/internal/api"
)

// supported telemetry provider config types
const (
	typeDataDog         = "DATA_DOG"
	typeOTLP            = "OTLP"
	typeAWSCloudWatch   = "AWS_CLOUDWATCH"
	typeGCPCloudMonitor = "GCP_CLOUD_MONITORING"
	typeSplunk          = "SPLUNK"
	typeLoki            = "LOKI"
	typeDynatrace       = "DYNATRACE"
)

// telemetryConfigBlocks lists every nested block that maps to a YBA
// TelemetryProviderConfig type. Exactly one of them must be set on a
// `yba_telemetry_provider` resource.
var telemetryConfigBlocks = []string{
	"data_dog",
	"otlp",
	"aws_cloud_watch",
	"gcp_cloud_monitoring",
	"splunk",
	"loki",
	"dynatrace",
}

// ResourceTelemetryProvider exposes the YBA telemetry provider as a Terraform
// resource. Telemetry providers are reusable destinations (DataDog,
// OpenTelemetry-compatible endpoints, AWS CloudWatch, etc.) that any
// universe can attach via `yba_universe_telemetry_config`.
func ResourceTelemetryProvider() *schema.Resource {
	return &schema.Resource{
		Description: "Telemetry Provider Resource. Defines a reusable export destination " +
			"(Datadog, OTLP, AWS CloudWatch, GCP Cloud Monitoring, Splunk, Loki, " +
			"Dynatrace) that universes can use to export audit logs, query logs, " +
			"and metrics.\n\n" +
			"~> **Note:** YBA does not allow editing a telemetry provider in place. " +
			"Any change to a config field forces a recreate.\n\n" +
			"~> **Security Note:** Credentials such as API keys, tokens, and secret " +
			"access keys are stored in the Terraform state file (marked sensitive). " +
			"Use a secure backend and restrict access to your state files.",

		CreateContext: resourceTelemetryProviderCreate,
		ReadContext:   resourceTelemetryProviderRead,
		DeleteContext: resourceTelemetryProviderDelete,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(5 * time.Minute),
			Read:   schema.DefaultTimeout(5 * time.Minute),
			Delete: schema.DefaultTimeout(5 * time.Minute),
		},

		Schema: map[string]*schema.Schema{
			"name": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "Name of the telemetry provider configuration.",
			},
			"tags": {
				Type:        schema.TypeMap,
				Optional:    true,
				ForceNew:    true,
				Description: "Optional string tags associated with the configuration.",
				Elem:        &schema.Schema{Type: schema.TypeString},
			},
			// Computed fields
			"type": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "Telemetry provider type, derived from the configured block.",
			},
			"customer_uuid": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "Customer UUID this telemetry provider belongs to.",
			},
			// Polymorphic config blocks. Exactly one must be set.
			"data_dog": {
				Type:         schema.TypeList,
				Optional:     true,
				ForceNew:     true,
				MaxItems:     1,
				ExactlyOneOf: telemetryConfigBlocks,
				Description:  "Datadog destination configuration.",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"site": {
							Type:        schema.TypeString,
							Required:    true,
							ForceNew:    true,
							Description: "Datadog site (e.g. datadoghq.com, datadoghq.eu).",
						},
						"api_key": {
							Type:        schema.TypeString,
							Required:    true,
							ForceNew:    true,
							Sensitive:   true,
							Description: "Datadog API key.",
						},
					},
				},
			},
			"otlp": {
				Type:         schema.TypeList,
				Optional:     true,
				ForceNew:     true,
				MaxItems:     1,
				ExactlyOneOf: telemetryConfigBlocks,
				Description:  "Generic OTLP (OpenTelemetry Protocol) destination configuration.",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"endpoint": {
							Type:        schema.TypeString,
							Required:    true,
							ForceNew:    true,
							Description: "OTLP endpoint URL.",
						},
						"auth_type": {
							Type:     schema.TypeString,
							Optional: true,
							ForceNew: true,
							Default:  "NoAuth",
							ValidateFunc: validation.StringInSlice([]string{
								"NoAuth", "BasicAuth", "BearerAuth",
							}, false),
							Description: "Authentication type. One of NoAuth, BasicAuth, BearerAuth.",
						},
						"protocol": {
							Type:         schema.TypeString,
							Optional:     true,
							ForceNew:     true,
							Default:      "gRPC",
							ValidateFunc: validation.StringInSlice([]string{"gRPC", "HTTP"}, false),
							Description:  "Transport protocol. One of gRPC, HTTP.",
						},
						"compression": {
							Type:        schema.TypeString,
							Optional:    true,
							ForceNew:    true,
							Default:     "gzip",
							Description: "Compression for OTLP exporter (e.g. gzip, none).",
						},
						"timeout_seconds": {
							Type:        schema.TypeInt,
							Optional:    true,
							ForceNew:    true,
							Default:     5,
							Description: "Timeout in seconds for the OTLP exporter.",
						},
						"basic_auth_username": {
							Type:        schema.TypeString,
							Optional:    true,
							ForceNew:    true,
							Sensitive:   true,
							Description: "BasicAuth username (only used when auth_type=BasicAuth).",
						},
						"basic_auth_password": {
							Type:        schema.TypeString,
							Optional:    true,
							ForceNew:    true,
							Sensitive:   true,
							Description: "BasicAuth password (only used when auth_type=BasicAuth).",
						},
						"bearer_token": {
							Type:        schema.TypeString,
							Optional:    true,
							ForceNew:    true,
							Sensitive:   true,
							Description: "Bearer token (only used when auth_type=BearerAuth).",
						},
						"headers": {
							Type:        schema.TypeMap,
							Optional:    true,
							ForceNew:    true,
							Description: "Additional headers to send on every OTLP request.",
							Elem:        &schema.Schema{Type: schema.TypeString},
						},
						"logs_endpoint": {
							Type:     schema.TypeString,
							Optional: true,
							ForceNew: true,
							Description: "Override endpoint for log export (HTTP protocol only). " +
								"When set, the value of `endpoint` is ignored for logs.",
						},
						"metrics_endpoint": {
							Type:     schema.TypeString,
							Optional: true,
							ForceNew: true,
							Description: "Override endpoint for metric export (HTTP protocol " +
								"only). When set, the value of `endpoint` is ignored for metrics.",
						},
					},
				},
			},
			"aws_cloud_watch": {
				Type:         schema.TypeList,
				Optional:     true,
				ForceNew:     true,
				MaxItems:     1,
				ExactlyOneOf: telemetryConfigBlocks,
				Description:  "AWS CloudWatch destination configuration.",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"log_group": {
							Type:        schema.TypeString,
							Required:    true,
							ForceNew:    true,
							Description: "CloudWatch log group.",
						},
						"log_stream": {
							Type:        schema.TypeString,
							Required:    true,
							ForceNew:    true,
							Description: "CloudWatch log stream.",
						},
						"region": {
							Type:        schema.TypeString,
							Required:    true,
							ForceNew:    true,
							Description: "AWS region.",
						},
						"access_key": {
							Type:        schema.TypeString,
							Required:    true,
							ForceNew:    true,
							Sensitive:   true,
							Description: "AWS access key with CloudWatch permissions.",
						},
						"secret_key": {
							Type:        schema.TypeString,
							Required:    true,
							ForceNew:    true,
							Sensitive:   true,
							Description: "AWS secret key for the access key.",
						},
						"role_arn": {
							Type:        schema.TypeString,
							Optional:    true,
							ForceNew:    true,
							Description: "Optional IAM role ARN to assume.",
						},
						"endpoint": {
							Type:        schema.TypeString,
							Optional:    true,
							ForceNew:    true,
							Description: "Optional override endpoint URL (e.g. for VPC endpoints).",
						},
					},
				},
			},
			"gcp_cloud_monitoring": {
				Type:         schema.TypeList,
				Optional:     true,
				ForceNew:     true,
				MaxItems:     1,
				ExactlyOneOf: telemetryConfigBlocks,
				Description:  "Google Cloud Monitoring/Logging destination configuration.",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"project": {
							Type:     schema.TypeString,
							Optional: true,
							ForceNew: true,
							Description: "GCP project ID. If empty, the project_id from the " +
								"service-account credentials is used.",
						},
						"credentials_json": {
							Type:        schema.TypeString,
							Required:    true,
							ForceNew:    true,
							Sensitive:   true,
							Description: "GCP service account credentials as a JSON string.",
						},
					},
				},
			},
			"splunk": {
				Type:         schema.TypeList,
				Optional:     true,
				ForceNew:     true,
				MaxItems:     1,
				ExactlyOneOf: telemetryConfigBlocks,
				Description:  "Splunk HTTP Event Collector destination configuration.",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"endpoint": {
							Type:        schema.TypeString,
							Required:    true,
							ForceNew:    true,
							Description: "Splunk HEC endpoint URL.",
						},
						"token": {
							Type:        schema.TypeString,
							Required:    true,
							ForceNew:    true,
							Sensitive:   true,
							Description: "Splunk HEC access token.",
						},
						"source": {
							Type:        schema.TypeString,
							Optional:    true,
							ForceNew:    true,
							Description: "Optional Splunk source field.",
						},
						"source_type": {
							Type:        schema.TypeString,
							Optional:    true,
							ForceNew:    true,
							Description: "Optional Splunk source type field.",
						},
						"index": {
							Type:        schema.TypeString,
							Optional:    true,
							ForceNew:    true,
							Description: "Optional Splunk index name.",
						},
					},
				},
			},
			"loki": {
				Type:         schema.TypeList,
				Optional:     true,
				ForceNew:     true,
				MaxItems:     1,
				ExactlyOneOf: telemetryConfigBlocks,
				Description:  "Grafana Loki destination configuration.",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"endpoint": {
							Type:        schema.TypeString,
							Required:    true,
							ForceNew:    true,
							Description: "Loki push endpoint URL.",
						},
						"auth_type": {
							Type:     schema.TypeString,
							Optional: true,
							ForceNew: true,
							Default:  "NoAuth",
							ValidateFunc: validation.StringInSlice([]string{
								"NoAuth", "BasicAuth",
							}, false),
							Description: "Authentication type. One of NoAuth, BasicAuth.",
						},
						"organization_id": {
							Type:        schema.TypeString,
							Optional:    true,
							ForceNew:    true,
							Description: "Optional Loki organization (tenant) ID header.",
						},
						"basic_auth_username": {
							Type:        schema.TypeString,
							Optional:    true,
							ForceNew:    true,
							Sensitive:   true,
							Description: "BasicAuth username (only used when auth_type=BasicAuth).",
						},
						"basic_auth_password": {
							Type:        schema.TypeString,
							Optional:    true,
							ForceNew:    true,
							Sensitive:   true,
							Description: "BasicAuth password (only used when auth_type=BasicAuth).",
						},
					},
				},
			},
			"dynatrace": {
				Type:         schema.TypeList,
				Optional:     true,
				ForceNew:     true,
				MaxItems:     1,
				ExactlyOneOf: telemetryConfigBlocks,
				Description:  "Dynatrace OTLP destination configuration.",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"endpoint": {
							Type:     schema.TypeString,
							Required: true,
							ForceNew: true,
							Description: "Dynatrace OTLP ingest base URL " +
								"(e.g. https://<env>.live.dynatrace.com/api/v2/otlp).",
						},
						"api_token": {
							Type:        schema.TypeString,
							Required:    true,
							ForceNew:    true,
							Sensitive:   true,
							Description: "Dynatrace ingest access token.",
						},
					},
				},
			},
		},
	}
}

// telemetryProviderType returns the YBA ProviderType enum value for the
// nested config block that is set in the resource data, or an error if no
// supported block is configured.
func telemetryProviderType(d *schema.ResourceData) (string, error) {
	for _, block := range telemetryConfigBlocks {
		if v, ok := d.GetOk(block); ok {
			if list, ok := v.([]interface{}); ok && len(list) > 0 {
				switch block {
				case "data_dog":
					return typeDataDog, nil
				case "otlp":
					return typeOTLP, nil
				case "aws_cloud_watch":
					return typeAWSCloudWatch, nil
				case "gcp_cloud_monitoring":
					return typeGCPCloudMonitor, nil
				case "splunk":
					return typeSplunk, nil
				case "loki":
					return typeLoki, nil
				case "dynatrace":
					return typeDynatrace, nil
				}
			}
		}
	}
	return "", fmt.Errorf("no telemetry provider config block set")
}

// buildTelemetryProviderConfig converts the configured nested block into the
// JSON shape that YBA expects. The map is round-trippable so the read flow
// can populate the resource state from the API response.
func buildTelemetryProviderConfig(d *schema.ResourceData) (map[string]interface{}, error) {
	pType, err := telemetryProviderType(d)
	if err != nil {
		return nil, err
	}
	out := map[string]interface{}{"type": pType}
	switch pType {
	case typeDataDog:
		c := firstMap(d.Get("data_dog"))
		out["site"] = c["site"]
		out["apiKey"] = c["api_key"]
	case typeOTLP:
		c := firstMap(d.Get("otlp"))
		out["endpoint"] = c["endpoint"]
		out["authType"] = c["auth_type"]
		out["protocol"] = c["protocol"]
		out["compression"] = c["compression"]
		out["timeoutSeconds"] = c["timeout_seconds"]
		if v, ok := c["logs_endpoint"].(string); ok && v != "" {
			out["logsEndpoint"] = v
		}
		if v, ok := c["metrics_endpoint"].(string); ok && v != "" {
			out["metricsEndpoint"] = v
		}
		if h, ok := c["headers"].(map[string]interface{}); ok && len(h) > 0 {
			out["headers"] = h
		}
		switch c["auth_type"] {
		case "BasicAuth":
			out["basicAuth"] = map[string]string{
				"username": stringValue(c["basic_auth_username"]),
				"password": stringValue(c["basic_auth_password"]),
			}
		case "BearerAuth":
			out["bearerToken"] = map[string]string{
				"token": stringValue(c["bearer_token"]),
			}
		}
	case typeAWSCloudWatch:
		c := firstMap(d.Get("aws_cloud_watch"))
		out["logGroup"] = c["log_group"]
		out["logStream"] = c["log_stream"]
		out["region"] = c["region"]
		out["accessKey"] = c["access_key"]
		out["secretKey"] = c["secret_key"]
		if v, ok := c["role_arn"].(string); ok && v != "" {
			out["roleARN"] = v
		}
		if v, ok := c["endpoint"].(string); ok && v != "" {
			out["endpoint"] = v
		}
	case typeGCPCloudMonitor:
		c := firstMap(d.Get("gcp_cloud_monitoring"))
		if v, ok := c["project"].(string); ok && v != "" {
			out["project"] = v
		}
		out["credentialsString"] = c["credentials_json"]
	case typeSplunk:
		c := firstMap(d.Get("splunk"))
		out["endpoint"] = c["endpoint"]
		out["token"] = c["token"]
		if v, ok := c["source"].(string); ok && v != "" {
			out["source"] = v
		}
		if v, ok := c["source_type"].(string); ok && v != "" {
			out["sourceType"] = v
		}
		if v, ok := c["index"].(string); ok && v != "" {
			out["index"] = v
		}
	case typeLoki:
		c := firstMap(d.Get("loki"))
		out["endpoint"] = c["endpoint"]
		out["authType"] = c["auth_type"]
		if v, ok := c["organization_id"].(string); ok && v != "" {
			out["organizationID"] = v
		}
		if c["auth_type"] == "BasicAuth" {
			out["basicAuth"] = map[string]string{
				"username": stringValue(c["basic_auth_username"]),
				"password": stringValue(c["basic_auth_password"]),
			}
		}
	case typeDynatrace:
		c := firstMap(d.Get("dynatrace"))
		out["endpoint"] = c["endpoint"]
		out["apiToken"] = c["api_token"]
	}
	return out, nil
}

func resourceTelemetryProviderCreate(
	ctx context.Context, d *schema.ResourceData, meta interface{},
) diag.Diagnostics {
	apiClient := meta.(*api.APIClient)
	cfg, err := buildTelemetryProviderConfig(d)
	if err != nil {
		return diag.FromErr(err)
	}

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
		req.Name, cfg["type"]))

	resp, err := apiClient.VanillaClient.CreateTelemetryProvider(
		ctx, apiClient.CustomerID, apiClient.APIKey, req)
	if err != nil {
		return diag.FromErr(err)
	}
	if resp.UUID == "" {
		return diag.Errorf("create telemetry provider returned an empty UUID")
	}
	d.SetId(resp.UUID)
	return resourceTelemetryProviderRead(ctx, d, meta)
}

func resourceTelemetryProviderRead(
	ctx context.Context, d *schema.ResourceData, meta interface{},
) diag.Diagnostics {
	apiClient := meta.(*api.APIClient)
	provider, resp, err := apiClient.VanillaClient.GetTelemetryProvider(
		ctx, apiClient.CustomerID, d.Id(), apiClient.APIKey)
	if err != nil {
		return diag.FromErr(err)
	}
	if provider == nil {
		// 404 - resource was deleted out-of-band; signal Terraform to
		// recreate it on the next apply.
		tflog.Warn(ctx, fmt.Sprintf(
			"telemetry provider %q not found (HTTP %d), removing from state",
			d.Id(), resp.StatusCode))
		d.SetId("")
		return nil
	}
	if err := d.Set("name", provider.Name); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("customer_uuid", provider.CustomerUUID); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("tags", provider.Tags); err != nil {
		return diag.FromErr(err)
	}
	if t, ok := provider.Config["type"].(string); ok {
		if err := d.Set("type", t); err != nil {
			return diag.FromErr(err)
		}
	}
	return nil
}

func resourceTelemetryProviderDelete(
	ctx context.Context, d *schema.ResourceData, meta interface{},
) diag.Diagnostics {
	apiClient := meta.(*api.APIClient)
	if err := apiClient.VanillaClient.DeleteTelemetryProvider(
		ctx, apiClient.CustomerID, d.Id(), apiClient.APIKey); err != nil {
		return diag.FromErr(err)
	}
	d.SetId("")
	return nil
}

// firstMap returns the first map element from a TypeList of MaxItems=1 nested
// blocks, or an empty map when the block is unset.
func firstMap(in interface{}) map[string]interface{} {
	list, ok := in.([]interface{})
	if !ok || len(list) == 0 || list[0] == nil {
		return map[string]interface{}{}
	}
	m, _ := list[0].(map[string]interface{})
	if m == nil {
		return map[string]interface{}{}
	}
	return m
}

// stringValue safely extracts a string from an interface{} value.
func stringValue(in interface{}) string {
	if in == nil {
		return ""
	}
	if s, ok := in.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", in)
}
