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
	typeS3              = "S3"
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
	"s3",
}

// ResourceTelemetryProvider exposes the YBA telemetry provider as a Terraform
// resource. Telemetry providers are reusable destinations (DataDog,
// OpenTelemetry-compatible endpoints, AWS CloudWatch, etc.) that any
// universe can attach via `yba_universe_telemetry_config`.
func ResourceTelemetryProvider() *schema.Resource {
	return &schema.Resource{
		Description: "Telemetry Provider Resource. Defines a reusable export destination " +
			"(Datadog, OTLP, AWS CloudWatch, GCP Cloud Monitoring, Splunk, Loki, " +
			"Dynatrace, S3) that universes can use to export audit logs, query " +
			"logs, and metrics.\n\n" +
			"~> **Note:** YBA does not allow editing a telemetry provider in place. " +
			"Any change to a config field forces Terraform to destroy and recreate " +
			"the resource. YBA also refuses to delete a provider that is still " +
			"referenced by a universe's telemetry config, so the destroy step first " +
			"enumerates every universe whose audit / query / metrics exporter list " +
			"references this provider and rewrites that list with the provider " +
			"removed (via a rolling-upgrade task on each universe). Once every " +
			"detach task reaches a terminal state, the provider itself is deleted. " +
			"The universes themselves are never destroyed — only their " +
			"OpenTelemetry collector configuration is updated.\n\n" +
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
			// Delete may have to wait for one rolling-upgrade task per
			// universe currently referencing this provider. See
			// telemetryUpgradeTimeout for the rationale.
			Delete: schema.DefaultTimeout(telemetryUpgradeTimeout),
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
			"s3": {
				Type:         schema.TypeList,
				Optional:     true,
				ForceNew:     true,
				MaxItems:     1,
				ExactlyOneOf: telemetryConfigBlocks,
				Description: "Amazon S3 destination configuration. Useful for " +
					"long-term archival of audit and query logs.",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"bucket": {
							Type:        schema.TypeString,
							Required:    true,
							ForceNew:    true,
							Description: "S3 bucket name.",
						},
						"region": {
							Type:        schema.TypeString,
							Required:    true,
							ForceNew:    true,
							Description: "AWS region of the bucket.",
						},
						"access_key": {
							Type:        schema.TypeString,
							Required:    true,
							ForceNew:    true,
							Sensitive:   true,
							Description: "AWS access key with bucket write permissions.",
						},
						"secret_key": {
							Type:        schema.TypeString,
							Required:    true,
							ForceNew:    true,
							Sensitive:   true,
							Description: "AWS secret key for the access key.",
						},
						"directory_prefix": {
							Type:     schema.TypeString,
							Optional: true,
							ForceNew: true,
							Description: "S3 prefix (root directory inside the bucket) " +
								"to write objects under.",
						},
						"file_prefix": {
							Type:        schema.TypeString,
							Optional:    true,
							ForceNew:    true,
							Description: "Optional file-name prefix prepended to every object.",
						},
						"endpoint": {
							Type:     schema.TypeString,
							Optional: true,
							ForceNew: true,
							Description: "Optional override endpoint URL " +
								"(e.g. for VPC endpoints or S3-compatible stores).",
						},
						"role_arn": {
							Type:        schema.TypeString,
							Optional:    true,
							ForceNew:    true,
							Description: "Optional IAM role ARN to assume.",
						},
						"partition": {
							Type:     schema.TypeString,
							Optional: true,
							ForceNew: true,
							Description: "Optional AWS partition " +
								"(e.g. aws, aws-us-gov, aws-cn).",
						},
						"marshaler": {
							Type:     schema.TypeString,
							Optional: true,
							ForceNew: true,
							Description: "Optional marshaler used to serialize " +
								"records (defaults to YBA's choice).",
						},
						"disable_ssl": {
							Type:        schema.TypeBool,
							Optional:    true,
							ForceNew:    true,
							Default:     false,
							Description: "Disable SSL when talking to the S3 endpoint.",
						},
						"force_path_style": {
							Type:     schema.TypeBool,
							Optional: true,
							ForceNew: true,
							Default:  false,
							Description: "Force path-style addressing instead of the " +
								"default virtual-hosted style.",
						},
						"include_universe_and_node_in_prefix": {
							Type:     schema.TypeBool,
							Optional: true,
							ForceNew: true,
							Default:  false,
							Description: "Append `<universe-uuid>/<node-name>` to the " +
								"directory prefix when writing objects.",
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
				case "s3":
					return typeS3, nil
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
	case typeS3:
		c := firstMap(d.Get("s3"))
		out["bucket"] = c["bucket"]
		out["region"] = c["region"]
		out["accessKey"] = c["access_key"]
		out["secretKey"] = c["secret_key"]
		if v, ok := c["directory_prefix"].(string); ok && v != "" {
			out["directoryPrefix"] = v
		}
		if v, ok := c["file_prefix"].(string); ok && v != "" {
			out["filePrefix"] = v
		}
		if v, ok := c["endpoint"].(string); ok && v != "" {
			out["endpoint"] = v
		}
		if v, ok := c["role_arn"].(string); ok && v != "" {
			out["roleArn"] = v
		}
		if v, ok := c["partition"].(string); ok && v != "" {
			out["partition"] = v
		}
		if v, ok := c["marshaler"].(string); ok && v != "" {
			out["marshaler"] = v
		}
		if v, ok := c["disable_ssl"].(bool); ok && v {
			out["disableSSL"] = v
		}
		if v, ok := c["force_path_style"].(bool); ok && v {
			out["forcePathStyle"] = v
		}
		if v, ok := c["include_universe_and_node_in_prefix"].(bool); ok && v {
			out["includeUniverseAndNodeInPrefix"] = v
		}
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
	provider, _, err := apiClient.VanillaClient.GetTelemetryProvider(
		ctx, apiClient.CustomerID, d.Id(), apiClient.APIKey)
	if err != nil {
		// Resource was deleted out-of-band (or YBA reports it as missing
		// via one of its non-404 shapes); drop from state so Terraform
		// re-plans a recreate on the next apply.
		if errors.Is(err, api.ErrTelemetryProviderMissing) {
			tflog.Warn(ctx, fmt.Sprintf(
				"telemetry provider %q not found, removing from state", d.Id()))
			d.SetId("")
			return nil
		}
		return diag.FromErr(err)
	}
	if err := d.Set("name", provider.Name); err != nil {
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

// resourceTelemetryProviderDelete tears down a telemetry provider.
//
// YBA refuses to delete a provider that is still wired into any universe's
// audit / query / metrics exporter list ("Cannot delete Telemetry Provider
// 'X', as it is in use."). Because YBA does not support editing a provider
// in place, ANY change to a `yba_telemetry_provider` config field becomes a
// destroy-and-recreate plan; if that provider is still attached to a
// universe Terraform's destroy step would fail before the create + universe
// re-attach steps even get a chance to run.
//
// To make destroy-and-recreate (and plain destroys) reliable we proactively
// detach the provider from every referencing universe BEFORE issuing the
// YBA delete. The detach goes through the unified
// `/api/v2/.../export-telemetry-configs` endpoint and triggers one rolling
// upgrade per affected universe; the function blocks until each task
// completes. The universes themselves are never destroyed — only their
// OpenTelemetry collector config is updated.
//
// If YBA still rejects the delete after a successful detach (an external
// actor must have re-attached the provider in the brief gap between our
// detach and our delete) we re-list universes once more and, if any
// references remain, repeat the detach + delete cycle exactly one more
// time. This avoids substring-matching YBA's "as it is in use" error
// message and instead trusts our own preemptive view of universe state.
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

	// YBA still rejected the delete. Verify by re-listing whether any
	// universe references this provider before assuming it is the
	// "in use" race we know how to recover from. If no universe still
	// references it, surface the original error untouched — it is
	// something else entirely (permission revoked, YBA outage, …).
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
	if len(retryDetached) == 0 {
		// Nothing to detach this time round — the original delete
		// failure is unrelated to the in-use race. Surface verbatim.
		return diag.FromErr(deleteErr)
	}
	tflog.Warn(ctx, fmt.Sprintf(
		"telemetry provider %s was re-attached between detach and delete "+
			"(detached %d universe(s) on second pass: %s); retrying delete",
		providerUUID, len(retryDetached), formatUniverseRefs(retryDetached)))
	if err := apiClient.VanillaClient.DeleteTelemetryProvider(
		ctx, apiClient.CustomerID, providerUUID, apiClient.APIKey); err != nil {
		return diag.FromErr(fmt.Errorf(
			"telemetry provider %s still could not be deleted after "+
				"detaching from %d universe(s) total (%s): %w",
			providerUUID, len(detached)+len(retryDetached),
			formatUniverseRefs(append(detached, retryDetached...)), err))
	}
	d.SetId("")
	return nil
}

// formatUniverseRefs renders a slice of universeRef as a human-readable
// "name (uuid), name (uuid)" string for inclusion in error messages and
// log lines.
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
