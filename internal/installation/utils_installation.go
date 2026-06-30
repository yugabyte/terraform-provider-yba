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
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/bramvdbogaerde/go-scp"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	"golang.org/x/crypto/ssh"
)

func newSSHClient(user string, ip string, key string) (*ssh.Client, error) {
	pk, err := ssh.ParsePrivateKey([]byte(key))
	if err != nil {
		return nil, err
	}
	config := &ssh.ClientConfig{
		User:            user,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(pk),
		},
	}
	c, err := ssh.Dial("tcp", net.JoinHostPort(ip, "22"), config)
	if err != nil {
		return nil, err
	}
	return c, nil
}

// Bounds for connectSSHForDelete. These cap only the failure path: a healthy
// host answers on the first attempt and returns immediately. The total wall
// time spent retrying an unreachable host is roughly sshConnectBudget, plus at
// most one in-flight sshDialTimeout of overshoot (ssh.Dial is not
// context-aware, so a dial already in progress runs to its own timeout).
const (
	sshDialTimeout   = 8 * time.Second
	sshRetryInterval = 4 * time.Second
	sshConnectBudget = 30 * time.Second
)

// errSSHHostUnreachable reports that every connection attempt failed at the
// TCP layer within the retry budget, i.e. the host never answered. It is
// distinguished by error *type* (*net.OpError), not by parsing OpenSSH error
// strings, which vary across machines.
var errSSHHostUnreachable = errors.New("ssh host unreachable after retries")

// connectSSHForDelete establishes an SSH connection for resource teardown,
// retrying transient TCP-layer failures within sshConnectBudget. It classifies
// the outcome so the caller can tell "host is gone" apart from "host is alive
// but we can't get in":
//
//   - (client, nil): the host answered; the caller must Close it. Returned on
//     the first successful attempt, so a healthy host is never delayed.
//   - (nil, errSSHHostUnreachable): no TCP connection within the budget. The
//     host is effectively gone (e.g. the cloud VM was destroyed first in a
//     replace cycle) — there is nothing to clean up remotely.
//   - (nil, err): a malformed key, or an SSH handshake/auth failure. The host
//     answered on the port, so it is alive and retrying cannot help; the
//     caller should surface this and fail loudly rather than drop the resource.
func connectSSHForDelete(ctx context.Context, user, ip, key string) (*ssh.Client, error) {
	signer, err := ssh.ParsePrivateKey([]byte(key))
	if err != nil {
		// A bad/unparseable key is a config error, deterministic across
		// retries and unrelated to whether the host is gone.
		return nil, err
	}
	config := &ssh.ClientConfig{
		User:            user,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		Timeout:         sshDialTimeout,
	}
	addr := net.JoinHostPort(ip, "22")

	budgetCtx, cancel := context.WithTimeout(ctx, sshConnectBudget)
	defer cancel()

	var lastErr error
	for attempt := 1; budgetCtx.Err() == nil; attempt++ {
		client, dialErr := ssh.Dial("tcp", addr, config)
		if dialErr == nil {
			return client, nil
		}
		lastErr = dialErr

		// A *net.OpError is a TCP-layer failure (connection refused, dial
		// timeout, no route to host): the host did not answer, so retrying
		// helps if it is mid-teardown or briefly unreachable. Anything else is
		// an SSH handshake/auth failure — the host answered and is alive, so
		// bad credentials won't be fixed by retrying. Type-based, so it does
		// not depend on OpenSSH-specific error text.
		var netErr *net.OpError
		if !errors.As(dialErr, &netErr) {
			return nil, dialErr
		}

		tflog.Info(ctx, fmt.Sprintf(
			"yba_installer: SSH dial to %s failed at TCP layer (attempt %d): %v",
			addr, attempt, dialErr))

		select {
		case <-budgetCtx.Done():
		case <-time.After(sshRetryInterval):
		}
	}

	if ctx.Err() != nil {
		// The caller's context was cancelled (not just our budget); propagate
		// that rather than masquerading it as an unreachable host.
		return nil, ctx.Err()
	}
	return nil, fmt.Errorf("%w: %w", errSSHHostUnreachable, lastErr)
}

func runCommand(ctx context.Context, client *ssh.Client, cmd string) (string, error) {
	tflog.Info(ctx, fmt.Sprintf("Running command: %s", cmd))
	session, err := client.NewSession()
	if err != nil {
		return "", err
	}
	var b bytes.Buffer
	var c bytes.Buffer
	session.Stderr = &c
	session.Stdout = &b
	defer func() { _ = session.Close() }()
	err = session.Run(cmd)
	tflog.Info(ctx, b.String())
	return c.String(), err
}

func waitForIP(ctx context.Context, user string, ip string, pk string, timeout time.Duration) (
	*ssh.Client, error) {
	wait := &retry.StateChangeConf{
		Delay:   1 * time.Second,
		Pending: []string{"Waiting"},
		Target:  []string{"Ready"},
		Timeout: timeout,

		Refresh: func() (result interface{}, state string, err error) {
			tflog.Info(ctx, fmt.Sprintf("Trying SSH connection to host using ip: %s", ip))
			c, cErr := newSSHClient(user, ip, pk)
			if cErr != nil {
				// keep polling — the host may still be coming up
				return nil, "Waiting", nil //nolint:nilerr // intentional: error means "not ready yet"
			}

			return c, "Ready", nil
		},
	}

	c, err := wait.WaitForStateContext(ctx)
	if err != nil {
		return nil, err
	}
	return c.(*ssh.Client), nil
}

// scpContent copies the given in-memory content to the remote host
// under the supplied path. It does not create any temporary file on
// the local filesystem.
func scpContent(ctx context.Context,
	sshClient *ssh.Client,
	content string,
	remoteFile string) error {
	tflog.Info(ctx, fmt.Sprintf("Copying inline content (%d bytes) to remote host under "+
		"filename %s", len(content), remoteFile))

	// NewClientBySSH reuses the already-established SSH connection. Do NOT call
	// c.Connect() afterwards: per go-scp's API it would dial a fresh session with
	// an empty Host and nil ClientConfig, panicking on a nil-pointer deref.
	c, err := scp.NewClientBySSH(sshClient)
	if err != nil {
		return err
	}
	defer c.Close()

	reader := strings.NewReader(content)
	return c.Copy(context.Background(), reader, remoteFile, "0666", int64(len(content)))
}
