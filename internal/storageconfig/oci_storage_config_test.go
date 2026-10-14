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
	"strings"
	"testing"
)

func TestOCIStorageSecretsAreWriteOnly(t *testing.T) {
	s := ResourceOCIStorageConfig().Schema
	for _, name := range []string{"access_key_id", "secret_access_key"} {
		attr := s[name]
		if attr == nil || !attr.Sensitive || !attr.WriteOnly {
			t.Errorf("%s must be Sensitive and WriteOnly", name)
		}
		if attr != nil && attr.ForceNew {
			t.Errorf("%s cannot be ForceNew (the SDK forbids it with WriteOnly)", name)
		}
	}
	if s["host_base"].WriteOnly {
		t.Error("host_base is returned by YBA and must be read back")
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
	if data["OCI_S3_ACCESS_KEY_ID"] != "abcd" {
		t.Fatalf("short access key was rewritten: %#v", data["OCI_S3_ACCESS_KEY_ID"])
	}
	if data["OCI_S3_SECRET_ACCESS_KEY"] != "secret-value-1" {
		t.Fatalf("secret was rewritten: %#v", data["OCI_S3_SECRET_ACCESS_KEY"])
	}
	if _, ok := data["OCI_NAMESPACE"]; ok {
		t.Fatal("omitted namespace was sent")
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

func TestFlattenOCIDataIgnoresMaskedSecrets(t *testing.T) {
	flat, err := flattenOCIData(map[string]interface{}{
		"BACKUP_LOCATION":          "s3://backup-sjc",
		"OCI_REGION":               "us-sanjose-1",
		"OCI_NAMESPACE":            "mytenancy",
		"USE_OCI_IAM":              false,
		"OCI_S3_HOST_BASE":         "ns.compat.objectstorage.us-sanjose-1.oraclecloud.com",
		"OCI_S3_ACCESS_KEY_ID":     "********",
		"OCI_S3_SECRET_ACCESS_KEY": "se********1",
		"REGION_LOCATIONS": []interface{}{
			map[string]interface{}{
				"REGION":           "us-ashburn-1",
				"LOCATION":         "s3://backup-iad",
				"OCI_S3_HOST_BASE": "ns.compat.objectstorage.us-ashburn-1.oraclecloud.com",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if flat.useIAM {
		t.Fatal("useIAM")
	}
	if flat.hostBase == "" || flat.namespace != "mytenancy" || len(flat.locations) != 1 {
		t.Fatalf("flat = %+v", flat)
	}
	if flat.locations[0].hostBase == "" {
		t.Fatal("region host was dropped")
	}

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
