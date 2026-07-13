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

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

// ResourceOTLPTelemetryProvider exposes a generic OTLP (OpenTelemetry
// Protocol) export destination that universes attach via
// yba_universe_telemetry_config.
func ResourceOTLPTelemetryProvider() *schema.Resource {
	return sinkResource(sinkSpec{
		resourceType: "yba_otlp_telemetry_provider",
		displayName:  "OTLP",
		apiType:      typeOTLP,
		description: "OTLP Telemetry Provider resource. Defines a reusable " +
			"OpenTelemetry Protocol destination that universes can use to " +
			"export audit logs, query logs, and metrics.",
		fields: map[string]*schema.Schema{
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
					"NoAuth", "BasicAuth", "BearerToken",
				}, false),
				Description: "Authentication type. One of NoAuth, BasicAuth, BearerToken.",
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
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
				Default:  "gzip",
				ValidateFunc: validation.StringInSlice([]string{
					"gzip", "none", "snappy", "zstd",
				}, false),
				Description: "Compression for the OTLP exporter. One of " +
					"gzip, none, snappy, zstd.",
			},
			"timeout_seconds": {
				Type:         schema.TypeInt,
				Optional:     true,
				ForceNew:     true,
				Default:      5,
				ValidateFunc: validation.IntAtLeast(1),
				Description:  "Timeout in seconds for the OTLP exporter. Must be positive.",
			},
			"basic_auth_username": {
				Type:        schema.TypeString,
				Optional:    true,
				ForceNew:    true,
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
				Description: "Bearer token (only used when auth_type=BearerToken).",
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
		buildConfig:   buildOTLPConfig,
		customizeDiff: validateOTLPConfig,
	})
}

func buildOTLPConfig(d *schema.ResourceData) map[string]interface{} {
	out := map[string]interface{}{
		"endpoint":       d.Get("endpoint"),
		"authType":       d.Get("auth_type"),
		"protocol":       d.Get("protocol"),
		"compression":    d.Get("compression"),
		"timeoutSeconds": d.Get("timeout_seconds"),
	}
	setIfNonEmpty(out, "logsEndpoint", d.Get("logs_endpoint"))
	setIfNonEmpty(out, "metricsEndpoint", d.Get("metrics_endpoint"))
	if h, ok := d.Get("headers").(map[string]interface{}); ok && len(h) > 0 {
		out["headers"] = h
	}
	switch d.Get("auth_type") {
	case "BasicAuth":
		out["basicAuth"] = map[string]string{
			"username": stringValue(d.Get("basic_auth_username")),
			"password": stringValue(d.Get("basic_auth_password")),
		}
	case "BearerToken":
		out["bearerToken"] = map[string]string{
			"token": stringValue(d.Get("bearer_token")),
		}
	}
	return out
}

// validateOTLPConfig rejects OTLP misconfigs at plan time, before YBA rejects
// them once credentials are already in state.
func validateOTLPConfig(
	_ context.Context, d *schema.ResourceDiff, _ interface{},
) error {
	switch stringValue(d.Get("auth_type")) {
	case "BasicAuth":
		if stringValue(d.Get("basic_auth_username")) == "" ||
			stringValue(d.Get("basic_auth_password")) == "" {
			return fmt.Errorf("basic_auth_username and " +
				"basic_auth_password are required when auth_type = \"BasicAuth\"")
		}
	case "BearerToken":
		if stringValue(d.Get("bearer_token")) == "" {
			return fmt.Errorf("bearer_token is required when " +
				"auth_type = \"BearerToken\"")
		}
	}
	if protocol := stringValue(d.Get("protocol")); protocol != "HTTP" {
		for _, field := range []string{"logs_endpoint", "metrics_endpoint"} {
			if stringValue(d.Get(field)) != "" {
				return fmt.Errorf(
					"%s is only honoured when protocol = \"HTTP\" "+
						"(current protocol is %q); remove %s or set protocol = \"HTTP\"",
					field, protocol, field)
			}
		}
	}
	return nil
}
