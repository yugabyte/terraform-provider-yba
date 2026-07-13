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

package provider

import (
	"testing"
)

var telemetrySinkResourceNames = []string{
	"yba_datadog_telemetry_provider",
	"yba_otlp_telemetry_provider",
	"yba_aws_cloudwatch_telemetry_provider",
	"yba_gcp_cloud_monitoring_telemetry_provider",
	"yba_splunk_telemetry_provider",
	"yba_dynatrace_telemetry_provider",
	"yba_s3_telemetry_provider",
}

// Every telemetry sink ships as its own resource; the generic polymorphic
// yba_telemetry_provider resource must not come back (it never shipped in a
// release, and the per-sink split is the public contract). The name-keyed
// lookup data source stays.
func TestTelemetrySinkResourcesRegistered(t *testing.T) {
	p := New()
	for _, name := range telemetrySinkResourceNames {
		if _, ok := p.ResourcesMap[name]; !ok {
			t.Errorf("resource %q must be registered", name)
		}
	}
	if _, ok := p.ResourcesMap["yba_telemetry_provider"]; ok {
		t.Error("generic yba_telemetry_provider resource must not be registered " +
			"(per-sink resources replaced it before it ever shipped)")
	}
	if _, ok := p.DataSourcesMap["yba_telemetry_provider"]; !ok {
		t.Error("the yba_telemetry_provider data source (name → uuid/type/tags " +
			"lookup) must remain registered")
	}
	if _, ok := p.ResourcesMap["yba_universe_telemetry_config"]; !ok {
		t.Error("yba_universe_telemetry_config must remain registered")
	}
}

// InternalValidate catches malformed schemas (bad ExactlyOneOf paths, missing
// Elem, Required+Computed clashes) across every registered resource.
func TestProviderInternalValidate(t *testing.T) {
	if err := New().InternalValidate(); err != nil {
		t.Fatalf("provider schema is invalid: %v", err)
	}
}
