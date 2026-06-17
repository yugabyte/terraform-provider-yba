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

package acctest

import (
	"strings"
	"testing"
)

// TestRandomNameRespectsMaxLen guards the varchar(100) safeguard: even an
// absurdly long branch prefix must not produce a name over maxNameLen, and the
// kind and a random suffix must survive (so names stay unique and readable).
func TestRandomNameRespectsMaxLen(t *testing.T) {
	t.Setenv("TF_ACCTEST_PREFIX", "acc-"+strings.Repeat("verylongbranchname-", 8))

	for _, kind := range []string{"azure-provider", "gcp-universe", "nfs"} {
		name := RandomName(kind)
		if len(name) > maxNameLen {
			t.Errorf("RandomName(%q) = %q (len %d), want <= %d",
				kind, name, len(name), maxNameLen)
		}
		if !strings.Contains(name, kind) {
			t.Errorf("RandomName(%q) = %q, want it to still contain the kind", kind, name)
		}
		if strings.Contains(name, "--") || strings.HasSuffix(name, "-") {
			t.Errorf("RandomName(%q) = %q, malformed separators", kind, name)
		}
	}
}

// TestRandomNameKeepsShortPrefix confirms short prefixes (the common case, e.g.
// "acc-main" on a push build) are preserved verbatim — the cap only trims when
// a prefix would otherwise overflow.
func TestRandomNameKeepsShortPrefix(t *testing.T) {
	t.Setenv("TF_ACCTEST_PREFIX", "acc-main")

	name := RandomName("azure-provider")
	const wantPrefix = "acc-main-azure-provider-"
	if !strings.HasPrefix(name, wantPrefix) {
		t.Errorf("RandomName = %q, want prefix %q", name, wantPrefix)
	}
	if len(name) > maxNameLen {
		t.Errorf("RandomName = %q (len %d), want <= %d", name, len(name), maxNameLen)
	}
}
