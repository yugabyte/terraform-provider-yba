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

package storageconfig

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/go-cty/cty"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func TestValidateOCIStorageSkipsUnknownSecrets(t *testing.T) {
	// An unknown Customer Secret Key is a value computed in the same apply.
	// Plan must not treat it as missing; apply rechecks once it is known.
	unknown := ociStorageSpec{
		name:           "oci",
		backupLocation: "s3://backup-sjc",
		region:         "us-sanjose-1",
		accessKnown:    false,
		secretKnown:    false,
		hostBase:       "ns.compat.objectstorage.us-sanjose-1.oraclecloud.com",
	}
	if err := validateOCIStorage(unknown); err != nil {
		t.Fatalf("unknown keys: %v", err)
	}

	knownEmpty := unknown
	knownEmpty.accessKnown = true
	knownEmpty.secretKnown = true
	err := validateOCIStorage(knownEmpty)
	if err == nil || !strings.Contains(err.Error(), "access_key_id is required") {
		t.Fatalf("known empty access key: %v", err)
	}

	unknownHost := ociStorageSpec{
		name: "n", backupLocation: "s3://b", region: "us-sanjose-1",
		accessKeyID: "k", secretAccessKey: "secret", accessKnown: true, secretKnown: true,
		hostUnknown: true,
	}
	if err := validateOCIStorage(unknownHost); err != nil {
		t.Fatalf("unknown host_base: %v", err)
	}

	unknownNamespace := ociStorageSpec{
		name: "n", backupLocation: "s3://b", region: "us-sanjose-1",
		useIAM: true, namespaceUnknown: true, accessKnown: true, secretKnown: true,
	}
	if err := validateOCIStorage(unknownNamespace); err != nil {
		t.Fatalf("unknown namespace: %v", err)
	}

	unknownRegionHost := ociStorageSpec{
		name: "n", backupLocation: "s3://b", region: "us-sanjose-1",
		accessKeyID: "k", secretAccessKey: "secret", accessKnown: true, secretKnown: true,
		hostBase: "host.example",
		locations: []ociLocationSpec{{
			region: "us-ashburn-1", location: "s3://other", hostUnknown: true,
		}},
	}
	if err := validateOCIStorage(unknownRegionHost); err != nil {
		t.Fatalf("unknown region host_base: %v", err)
	}

	if err := validateOCIStorage(ociStorageSpec{
		nameUnknown: true, backupLocation: "s3://b", region: "us-sanjose-1",
		accessKnown: false, secretKnown: false, hostUnknown: true, useIAMUnknown: true,
	}); err != nil {
		t.Fatalf("unknown name and auth mode: %v", err)
	}
}

// staticConfig is a config source whose raw values can be unknown.
type staticConfig struct {
	vals map[string]interface{}
	raw  map[string]cty.Value
}

func (s staticConfig) Get(key string) interface{} {
	return s.vals[key]
}

func (s staticConfig) GetRawConfigAt(path cty.Path) (cty.Value, diag.Diagnostics) {
	if v, ok := s.raw[configPathKey(path)]; ok {
		return v, nil
	}
	return cty.NullVal(cty.DynamicPseudoType), nil
}

func configPathKey(path cty.Path) string {
	var b strings.Builder
	for _, step := range path {
		switch s := step.(type) {
		case cty.GetAttrStep:
			b.WriteByte('.')
			b.WriteString(s.Name)
		case cty.IndexStep:
			fmt.Fprintf(&b, "[%s]", s.Key.GoString())
		}
	}
	return b.String()
}

func TestOCISpecFromSkipsUnknownPlanValues(t *testing.T) {
	locHost := cty.GetAttrPath("region_locations").Index(cty.NumberIntVal(0)).GetAttr("host_base")
	cfg := staticConfig{
		vals: map[string]interface{}{
			"backup_location": "s3://backup-sjc",
			"region":          "us-sanjose-1",
			"region_locations": []interface{}{
				map[string]interface{}{
					"region": "us-ashburn-1", "location": "s3://backup-iad", "host_base": "",
				},
			},
		},
		raw: map[string]cty.Value{
			configPathKey(cty.GetAttrPath("name")):              cty.StringVal("oci"),
			configPathKey(cty.GetAttrPath("access_key_id")):     cty.UnknownVal(cty.String),
			configPathKey(cty.GetAttrPath("secret_access_key")): cty.UnknownVal(cty.String),
			configPathKey(cty.GetAttrPath("host_base")):         cty.UnknownVal(cty.String),
			configPathKey(cty.GetAttrPath("namespace")):         cty.UnknownVal(cty.String),
			configPathKey(cty.GetAttrPath("use_oci_iam")):       cty.UnknownVal(cty.Bool),
			configPathKey(locHost):                              cty.UnknownVal(cty.String),
		},
	}
	spec, err := ociSpecFrom(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateOCIStorage(spec); err != nil {
		t.Fatalf("unknown plan values: %v", err)
	}
	if spec.accessKnown || !spec.hostUnknown || !spec.namespaceUnknown || !spec.useIAMUnknown {
		t.Fatalf("known flags = access %t hostUnknown %t namespaceUnknown %t useIAMUnknown %t",
			spec.accessKnown, spec.hostUnknown, spec.namespaceUnknown, spec.useIAMUnknown)
	}
	if len(spec.locations) != 1 || !spec.locations[0].hostUnknown {
		t.Fatalf("region host unknown = %+v", spec.locations)
	}
}

func TestOCIStorageConfigGuardrails(t *testing.T) {
	res := ResourceOCIStorageConfig()
	if err := res.InternalValidate(nil, true); err != nil {
		t.Fatalf("schema failed InternalValidate: %v", err)
	}
	if res.Description == "" {
		t.Error("resource description is required")
	}
	if res.Importer == nil {
		t.Error("the resource must be importable")
	}
	if _, ok := res.Schema["customer_uuid"]; ok {
		t.Error("customer_uuid is a noise field and must not be exposed")
	}
	for op, timeout := range map[string]*time.Duration{
		"create": res.Timeouts.Create,
		"update": res.Timeouts.Update,
		"delete": res.Timeouts.Delete,
	} {
		if timeout == nil || *timeout != 5*time.Minute {
			t.Errorf("%s timeout must default to 5 minutes", op)
		}
	}
	assertDescriptions(t, res.Schema)
	for _, name := range []string{"use_oci_iam", "access_key_id", "secret_access_key", "host_base"} {
		if len(res.Schema[name].ConflictsWith) != 0 {
			t.Errorf("%s ConflictsWith = %v; mode is checked on the value, so "+
				"use_oci_iam = false stays valid next to the keys",
				name, res.Schema[name].ConflictsWith)
		}
	}
	for _, name := range []string{"access_key_id", "secret_access_key"} {
		attr := res.Schema[name]
		if !attr.Sensitive || !attr.WriteOnly {
			t.Errorf("%s must be Sensitive and WriteOnly", name)
		}
		if attr.ForceNew {
			t.Errorf("%s cannot be ForceNew (the SDK forbids it with WriteOnly)", name)
		}
	}
	if res.Schema["host_base"].WriteOnly {
		t.Error("host_base is returned by YBA and must be read back")
	}
}

func assertDescriptions(t *testing.T, attrs map[string]*schema.Schema) {
	t.Helper()
	for name, attr := range attrs {
		if attr.Description == "" {
			t.Errorf("%s needs a Description: it renders into the docs", name)
		}
		nested, _ := attr.Elem.(*schema.Resource)
		if nested != nil {
			assertDescriptions(t, nested.Schema)
		}
	}
}

func TestBuildOCIDataAuthModes(t *testing.T) {
	static := ociStorageSpec{
		name:            "oci",
		backupLocation:  "s3://backup-sjc",
		region:          "us-sanjose-1",
		accessKeyID:     "abcd",
		secretAccessKey: "secret-value-1",
		accessKnown:     true,
		secretKnown:     true,
		hostBase:        "ns.compat.objectstorage.us-sanjose-1.oraclecloud.com",
		locations: []ociLocationSpec{{
			region:   "us-ashburn-1",
			location: "s3://backup-iad",
			hostBase: "ns.compat.objectstorage.us-ashburn-1.oraclecloud.com",
		}},
	}
	data, err := buildOCIData(static)
	if err != nil {
		t.Fatalf("static build: %v", err)
	}
	if data["USE_OCI_IAM"] != false {
		t.Fatalf("USE_OCI_IAM = %#v, want boolean false", data["USE_OCI_IAM"])
	}

	iam := ociStorageSpec{
		name:           "oci-iam",
		backupLocation: "https://objectstorage.us-sanjose-1.oraclecloud.com/n/mytenancy/b/backup",
		region:         "us-sanjose-1",
		namespace:      "mytenancy",
		useIAM:         true,
		accessKnown:    true,
		secretKnown:    true,
	}
	data, err = buildOCIData(iam)
	if err != nil {
		t.Fatalf("iam build: %v", err)
	}
	if data["USE_OCI_IAM"] != true {
		t.Fatalf("USE_OCI_IAM = %#v, want boolean true", data["USE_OCI_IAM"])
	}
	for _, key := range []string{
		"OCI_S3_ACCESS_KEY_ID", "OCI_S3_SECRET_ACCESS_KEY", "OCI_S3_HOST_BASE",
	} {
		if _, ok := data[key]; ok {
			t.Errorf("IAM payload contains %s", key)
		}
	}

	cases := []struct {
		name    string
		spec    ociStorageSpec
		wantErr string
	}{
		{
			name: "static without host",
			spec: ociStorageSpec{
				name: "n", backupLocation: "s3://b", region: "us-sanjose-1",
				accessKeyID: "k", secretAccessKey: "secret", accessKnown: true, secretKnown: true,
			},
			wantErr: "host_base is required",
		},
		{
			name: "iam without namespace",
			spec: ociStorageSpec{
				name: "n", backupLocation: "s3://b", region: "us-sanjose-1",
				useIAM: true, accessKnown: true, secretKnown: true,
			},
			wantErr: "namespace is required",
		},
		{
			name: "iam with host",
			spec: ociStorageSpec{
				name: "n", backupLocation: "s3://b", region: "us-sanjose-1",
				namespace: "ns", useIAM: true, hostBase: "host.example",
				accessKnown: true, secretKnown: true,
			},
			wantErr: "conflict with use_oci_iam",
		},
		{
			name: "namespace does not match native url",
			spec: ociStorageSpec{
				name:           "n",
				region:         "us-sanjose-1",
				namespace:      "other",
				backupLocation: "https://objectstorage.us-sanjose-1.oraclecloud.com/n/mytenancy/b/backup",
				useIAM:         true,
				accessKnown:    true,
				secretKnown:    true,
			},
			wantErr: `namespace "other" must match namespace "mytenancy" in backup_location`,
		},
		{
			name: "static region missing host",
			spec: ociStorageSpec{
				name: "n", backupLocation: "s3://b", region: "us-sanjose-1",
				accessKeyID: "k", secretAccessKey: "secret", accessKnown: true, secretKnown: true,
				hostBase: "host.example",
				locations: []ociLocationSpec{{
					region: "us-ashburn-1", location: "s3://other",
				}},
			},
			wantErr: `region_locations.host_base is required for region "us-ashburn-1"`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := buildOCIData(c.spec)
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("error %v, want substring %q", err, c.wantErr)
			}
		})
	}
}

func TestFlattenOCIDataIAMString(t *testing.T) {
	fromString, err := flattenOCIData(map[string]interface{}{"USE_OCI_IAM": "true"})
	if err != nil {
		t.Fatal(err)
	}
	if !fromString.useIAM {
		t.Fatal(`string "true" was not read as IAM`)
	}
}

func TestOCIStorageVersionGate(t *testing.T) {
	cases := []struct {
		version string
		wantErr string
	}{
		{"2026.1.2.0-b136", "yba_oci_storage_config requires YugabyteDB Anywhere 2026.1.2.0-b137"},
		{"2026.1.2.0-b137", ""},
		{"2.31.0.0-b385", "yba_oci_storage_config requires YugabyteDB Anywhere 2.31.0.0-b386"},
		{"2.31.0.0-b386", ""},
		{"dev-build", ""},
		{"", ""},
	}
	for _, c := range cases {
		t.Run(c.version, func(t *testing.T) {
			err := ociStorageVersionError(context.Background(), c.version)
			switch {
			case c.wantErr == "" && err != nil:
				t.Fatal(err)
			case c.wantErr != "" && (err == nil || !strings.Contains(err.Error(), c.wantErr)):
				t.Fatalf("error %v, want substring %q", err, c.wantErr)
			}
		})
	}
}
