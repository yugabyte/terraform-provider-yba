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

package api

import (
	"net/http"
	"testing"
)

// TestNewAPIClientTransportTimeouts: the generated clients' transport must
// bound dial, TLS handshake, and time-to-first-byte. GetSessionInfo runs on
// context.Background() during provider configure, so a connection that dies
// without an RST (black-holed load balancer or tunnel) would otherwise hang
// every terraform command forever.
func TestNewAPIClientTransportTimeouts(t *testing.T) {
	// Empty API key skips the GetSessionInfo call, so no server is needed.
	c, err := NewAPIClient(true, "127.0.0.1:1", "")
	if err != nil {
		t.Fatalf("NewAPIClient: %v", err)
	}
	for name, hc := range map[string]*http.Client{
		"v1": c.YugawareClient.GetConfig().HTTPClient,
		"v2": c.YugawareClientV2.GetConfig().HTTPClient,
	} {
		tr, ok := hc.Transport.(*http.Transport)
		if !ok {
			t.Fatalf("%s client: transport is %T, want *http.Transport", name, hc.Transport)
		}
		if tr.DialContext == nil {
			t.Errorf("%s client: no dial timeout (DialContext is nil)", name)
		}
		if tr.TLSHandshakeTimeout <= 0 {
			t.Errorf("%s client: TLSHandshakeTimeout not set", name)
		}
		if tr.ResponseHeaderTimeout <= 0 {
			t.Errorf("%s client: ResponseHeaderTimeout not set", name)
		}
	}
}
