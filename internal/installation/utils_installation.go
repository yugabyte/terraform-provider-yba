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
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/bramvdbogaerde/go-scp"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
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
	wait := &resource.StateChangeConf{
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

	c, err := scp.NewClientBySSH(sshClient)
	if err != nil {
		return err
	}
	defer c.Close()

	err = c.Connect()
	if err != nil {
		return err
	}
	defer c.Close()

	reader := strings.NewReader(content)
	return c.Copy(context.Background(), reader, remoteFile, "0666", int64(len(content)))
}
