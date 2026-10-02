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
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"encoding/pem"
	"errors"
	"net"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"golang.org/x/crypto/ssh"
)

// TestInstallStateFromProbe pins the verdict for each thing the host can say.
// The gone/kept/error split is what decides whether terraform reinstalls, so a
// status landing in the wrong bucket is the regression to guard against.
func TestInstallStateFromProbe(t *testing.T) {
	cases := []struct {
		name string
		out  string
		want string // "kept", "gone", or "error"
	}{
		{"no state file", ybactlStateAbsent + "\n", "gone"},
		{"installed", `{"version":"2.31.0.0-b417","current_status":"Installed"}`, "kept"},
		{"state predates statuses", `{"version":"2.20.0.0-b1"}`, "kept"},
		{"soft cleaned", `{"current_status":"Soft Cleaned"}`, "gone"},
		{"uninstalled", `{"current_status":"Uninstalled"}`, "gone"},
		{"upgrade in progress", `{"current_status":"Upgrading"}`, "error"},
		{"install in progress", `{"current_status":"Installing"}`, "error"},
		{"unknown status", `{"current_status":"Teleported"}`, "error"},
		{"garbage", "sudo: a password is required", "error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := installStateFromProbe(tc.out)
			got := "kept"
			switch {
			case errors.Is(err, errYBANotInstalled):
				got = "gone"
			case err != nil:
				got = "error"
			}
			if got != tc.want {
				t.Fatalf("want %s, got %s (err=%v)", tc.want, got, err)
			}
		})
	}
}

// TestReadFollowsTheHost drives Read against an in-process SSH server. The
// decision under test is what happens to state: the install is dropped only
// when the host answers and says it is gone, and an SSH failure is surfaced
// rather than read as a missing install.
func TestReadFollowsTheHost(t *testing.T) {
	clientKey := newTestClientKey(t)

	t.Run("install gone drops the resource", func(t *testing.T) {
		port := serveFakeSSH(t, ybactlStateAbsent+"\n")
		d := newReadData(t, port, clientKey)
		if diags := resourceYBAInstallerRead(context.Background(), d, nil); diags.HasError() {
			t.Fatalf("unexpected error: %v", diags)
		}
		if d.Id() != "" {
			t.Fatalf("expected the resource to be dropped from state, id=%q", d.Id())
		}
	})

	t.Run("install present keeps the resource", func(t *testing.T) {
		port := serveFakeSSH(t, `{"current_status":"Installed"}`)
		d := newReadData(t, port, clientKey)
		if diags := resourceYBAInstallerRead(context.Background(), d, nil); diags.HasError() {
			t.Fatalf("unexpected error: %v", diags)
		}
		if d.Id() == "" {
			t.Fatal("expected the resource to stay in state")
		}
	})

	t.Run("ssh failure is an error, not a missing install", func(t *testing.T) {
		// Accept the TCP connection and hang up: a handshake failure, which the
		// dialer does not retry. The resource must survive it untouched.
		ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = ln.Close() })
		go func() {
			for {
				c, err := ln.Accept()
				if err != nil {
					return
				}
				_ = c.Close()
			}
		}()
		d := newReadData(t, ln.Addr().(*net.TCPAddr).Port, clientKey)
		diags := resourceYBAInstallerRead(context.Background(), d, nil)
		if !diags.HasError() {
			t.Fatal("expected an error when the host cannot be probed")
		}
		if d.Id() == "" {
			t.Fatal("an unreachable host must not drop the resource from state")
		}
	})
}

// newReadData builds the state Read sees: an installed resource pointing at
// the fake server.
func newReadData(t *testing.T, port int, clientKey string) *schema.ResourceData {
	t.Helper()
	d := newInstallerData(t, map[string]interface{}{
		"ssh_host_ip":       "127.0.0.1",
		"ssh_port":          port,
		"ssh_user":          "yugabyte",
		"ssh_private_key":   clientKey,
		"yba_version":       "2.31.0.0-b417",
		"host_os":           "linux",
		"host_architecture": "x86_64",
	})
	d.SetId("installed")
	return d
}

// newTestClientKey returns an OpenSSH-encoded private key for the client side.
func newTestClientKey(t *testing.T) string {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	block, err := ssh.MarshalPrivateKey(priv, "")
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(block))
}

// serveFakeSSH listens on a free port and answers every exec request with
// output, exit status 0. Any client key is accepted: the test is about what
// the host says, not who may ask.
func serveFakeSSH(t *testing.T, output string) int {
	t.Helper()
	_, hostPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(hostPriv)
	if err != nil {
		t.Fatal(err)
	}
	config := &ssh.ServerConfig{
		PublicKeyCallback: func(ssh.ConnMetadata, ssh.PublicKey) (*ssh.Permissions, error) {
			return &ssh.Permissions{}, nil
		},
	}
	config.AddHostKey(signer)

	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go serveFakeSSHConn(conn, config, output)
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port
}

func serveFakeSSHConn(conn net.Conn, config *ssh.ServerConfig, output string) {
	defer func() { _ = conn.Close() }()
	sconn, chans, reqs, err := ssh.NewServerConn(conn, config)
	if err != nil {
		return
	}
	defer func() { _ = sconn.Close() }()
	go ssh.DiscardRequests(reqs)
	for newChan := range chans {
		if newChan.ChannelType() != "session" {
			_ = newChan.Reject(ssh.UnknownChannelType, "session only")
			continue
		}
		ch, chReqs, err := newChan.Accept()
		if err != nil {
			return
		}
		go func() {
			defer func() { _ = ch.Close() }()
			for req := range chReqs {
				if req.Type != "exec" {
					if req.WantReply {
						_ = req.Reply(false, nil)
					}
					continue
				}
				_ = req.Reply(true, nil)
				_, _ = ch.Write([]byte(output))
				status := make([]byte, 4)
				binary.BigEndian.PutUint32(status, 0)
				_, _ = ch.SendRequest("exit-status", false, status)
				return
			}
		}()
	}
}
