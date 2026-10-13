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

import (
	"fmt"
	"strings"
)

// SetIfNonEmpty writes an optional string setting into a request body only
// when it is set. YBA reads a missing key as "not configured" and applies its
// own default, whereas an empty string is stored as the value.
func SetIfNonEmpty(out map[string]interface{}, key string, v interface{}) {
	if s, ok := v.(string); ok && s != "" {
		out[key] = s
	}
}

// StringValue renders a decoded JSON value as the string YBA sent, with nil
// as "".
func StringValue(in interface{}) string {
	if in == nil {
		return ""
	}
	if s, ok := in.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", in)
}

// IsYBAMaskOf reports whether masked is how YBA reads back value under key.
// Mirrors YBA's CommonUtils.getMaskedValue: a key that contains PASSWORD, or a
// value under 5 characters, reads back as ********; any other value keeps its
// first and last two characters and masks the rest, at the same length. An
// empty value matches nothing: callers keep a state value that YBA masked,
// and after an import there is no state value to keep.
func IsYBAMaskOf(key, masked, value string) bool {
	v := []rune(value)
	if value == "" {
		return false
	}
	if strings.Contains(strings.ToUpper(key), "PASSWORD") || len(v) < 5 {
		return masked == "********"
	}
	m := []rune(masked)
	if len(m) != len(v) {
		return false
	}
	for i := range m {
		if m[i] != v[i] && (m[i] != '*' || i < 2 || i >= len(v)-2) {
			return false
		}
	}
	return true
}

// BoolValue reads a boolean setting that clients may have stored as a JSON
// boolean or as the strings "true"/"false". Anything else is false.
func BoolValue(in interface{}) bool {
	switch v := in.(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(v, "true")
	}
	return false
}
