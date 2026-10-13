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

package utils

import "testing"

// A missing key and an empty string mean different things to YBA, so the
// helper must leave the key out for "" and write every other string.
func TestSetIfNonEmpty(t *testing.T) {
	out := map[string]interface{}{}
	SetIfNonEmpty(out, "empty", "")
	SetIfNonEmpty(out, "nil", nil)
	SetIfNonEmpty(out, "number", 7)
	SetIfNonEmpty(out, "set", "value")
	if len(out) != 1 || out["set"] != "value" {
		t.Errorf("out = %v: want only the non-empty string", out)
	}
}

func TestBoolValue(t *testing.T) {
	for in, want := range map[interface{}]bool{
		true: true, false: false, "true": true, "TRUE": true, "false": false, "yes": false,
		"": false, nil: false, 1: false,
	} {
		if got := BoolValue(in); got != want {
			t.Errorf("BoolValue(%#v) = %t want %t", in, got, want)
		}
	}
}

// Cases follow YBA's CommonUtils.getMaskedValue: a PASSWORD key or a value
// under 5 characters reads back as ********; any other value keeps its first
// and last two characters, at the same length.
func TestIsYBAMaskOf(t *testing.T) {
	cases := []struct {
		key, masked, value string
		want               bool
	}{
		{"api_owner", "pl****rm", "platform", true},
		{"api_owner", "px****rm", "platform", false},
		{"api_owner", "pl***rm", "platform", false},
		{"api_owner", "********", "platform", false},
		{"team_key", "********", "abcd", true},
		{"team_key", "********", "", false},
		{"team_key", "ab*de", "abcde", true},
		{"team_key", "********", "abcde", false},
		{"yb.security.ldap.ldap_service_account_password", "********", "long-secret", true},
		{"basicAuth.Password", "lo*******et", "long-secret", false},
	}
	for _, tc := range cases {
		if got := IsYBAMaskOf(tc.key, tc.masked, tc.value); got != tc.want {
			t.Errorf("IsYBAMaskOf(%q, %q, %q) = %t want %t",
				tc.key, tc.masked, tc.value, got, tc.want)
		}
	}
}

func TestStringValue(t *testing.T) {
	for in, want := range map[interface{}]string{
		nil: "", "s": "s", 3: "3", true: "true", 1.5: "1.5",
	} {
		if got := StringValue(in); got != want {
			t.Errorf("StringValue(%#v) = %q want %q", in, got, want)
		}
	}
}
