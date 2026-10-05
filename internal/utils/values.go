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
