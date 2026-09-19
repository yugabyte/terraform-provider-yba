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

package installation

import (
	"errors"
	"testing"
)

// TestSSHAddr guards the dial-address join every SSH/SCP connection uses:
// non-default ports honored, IPv6 literals bracketed.
func TestSSHAddr(t *testing.T) {
	tests := []struct {
		name string
		ip   string
		port int
		want string
	}{
		{"default ssh port", "192.168.1.10", 22, "192.168.1.10:22"},
		{"port-forwarded local address", "127.0.0.1", 2222, "127.0.0.1:2222"},
		{"hostname", "yba.example.com", 22, "yba.example.com:22"},
		{"ipv6 literal is bracketed", "::1", 2222, "[::1]:2222"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sshAddr(tt.ip, tt.port); got != tt.want {
				t.Errorf("sshAddr(%q, %d) = %q, want %q", tt.ip, tt.port, got, tt.want)
			}
		})
	}
}

// The real-world error is a plain string: x/crypto/ssh formats the cause with %v,
// so errors.Is(err, io.EOF) alone would miss it.
func TestIsDroppedConnection(t *testing.T) {
	if !isDroppedConnection(errors.New("ssh: handshake failed: EOF")) {
		t.Error("handshake EOF must count as a dropped connection")
	}
	if isDroppedConnection(errors.New(
		"ssh: handshake failed: ssh: unable to authenticate, attempted methods [none publickey]",
	)) {
		t.Error("auth failure must not count as a dropped connection")
	}
}
