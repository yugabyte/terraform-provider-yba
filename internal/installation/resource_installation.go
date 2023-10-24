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
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/customdiff"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/yugabyte/terraform-provider-yba/internal/api"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

var (
	installationFiles = map[string]string{
		"replicated_config_file":    "/tmp/replicated.conf",
		"tls_certificate_file":      "/tmp/server.crt",
		"tls_key_file":              "/tmp/server.key",
		"replicated_license_file":   "/tmp/license.rli",
		"application_settings_file": "/tmp/settings.conf",
	}
	deletionCommands = []string{
		// remove yugabyte resources
		"/usr/local/bin/replicated apps | grep \"yugaware\" | awk '{print $1}' | " +
			"xargs -I {} /usr/local/bin/replicated app {} stop",
		"sudo docker images | grep \"yuga\" | awk '{print $3}' | sudo xargs docker rmi -f",
		"sudo rm -rf /opt/yugabyte",
		"sudo rm /etc/replicated.conf /tmp/replicated.conf /tmp/server.crt /tmp/server.key " +
			"/tmp/license/rli /tmp/settings.conf",
		// remove replicated resources
		"sudo service replicated stop",
		"sudo service replicated-ui stop",
		"sudo service replicated-operator stop",
		"sudo docker stop replicated-premkit",
		"sudo docker stop replicated-statsd",
		"sudo docker rm -f replicated replicated-ui replicated-operator replicated-premkit " +
			"replicated-statsd retraced-api retraced-processor retraced-cron retraced-nsqd " +
			"retraced-postgres",
		"sudo docker images | grep \"quay\\.io/replicated\" | awk '{print $3}' | xargs sudo " +
			"docker rmi -f",
		"sudo docker images | grep \"registry\\.replicated\\.com/library/retraced\" | awk " +
			"'{print $3}' | sudo xargs docker rmi -f",
		"sudo apt-get remove -y replicated replicated-ui replicated-operator",
		"sudo apt-get purge -y replicated replicated-ui replicated-operator",
		"sudo rm -rf /var/lib/replicated* /etc/replicated* /etc/init/replicated* " +
			"/etc/init.d/replicated* /etc/default/replicated* /var/log/upstart/replicated* " +
			" /etc/systemd/system/replicated*",
	}
)

func getInstallationCommands(publicIP string, privateIP string) []string {
	var installationCommands = []string{"sudo mv /tmp/replicated.conf /etc/replicated.conf"}
	s := fmt.Sprintf("curl -sSL https://get.replicated.com/docker | sudo bash -s "+
		"public-address=%s private-address=%s fast-timeouts", publicIP, privateIP)
	return append(installationCommands, s)
}

// ResourceInstallation handles installation of YugabyteDB Anywhere
func ResourceInstallation() *schema.Resource {
	return &schema.Resource{
		Description: "Manages the installation of YugabyteDB Anywhere on an existing virtual" +
			" machine. This resource does not track the remote state and is only provided as a " +
			"convenience tool. To reinstall, taint this resource and re-apply. To see remote output," +
			" run with TF_LOG=INFO.",

		CreateContext: resourceInstallationCreate,
		ReadContext:   resourceInstallationRead,
		UpdateContext: resourceInstallationUpdate,
		DeleteContext: resourceInstallationDelete,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(10 * time.Minute),
			Delete: schema.DefaultTimeout(10 * time.Minute),
		},

		DeprecationMessage: "Use yba_installer resource instead.",

		CustomizeDiff: resourceInstallationDiff(),

		Schema: map[string]*schema.Schema{
			"public_ip": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Public ip of the existing virtual machine.",
			},
			"private_ip": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Private ip of the existing virtual machine.",
			},
			"ssh_host_ip": {
				Type:      schema.TypeString,
				Required:  true,
				Sensitive: true,
				Description: "IP address of VM for SSH. Typically same as public_ip or " +
					"private_ip.",
			},
			"ssh_private_key": {
				Type:      schema.TypeString,
				Required:  true,
				Sensitive: true,
				Description: "Contents of file containing the private key to use for ssh " +
					"commands.",
			},
			"ssh_user": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "User to use for ssh commands.",
			},
			"replicated_config_file": {
				Type:     schema.TypeString,
				Required: true,
				Description: "Configuration file to use for automated installation using " +
					"Replicated.",
			},
			"tls_certificate_file": {
				Type:         schema.TypeString,
				Optional:     true,
				RequiredWith: []string{"tls_key_file"},
				Description:  "TLS certificate used to configure HTTPS.",
			},
			"tls_key_file": {
				Type:         schema.TypeString,
				Optional:     true,
				RequiredWith: []string{"tls_certificate_file"},
				Description:  "TLS key used to configure HTTPS.",
			},
			"replicated_license_file": {
				Type:     schema.TypeString,
				Required: true,
				Description: "YugabyteDB Anywhere license file used for installation using " +
					"Replicated.",
			},
			"application_settings_file": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Application settings file to configure YugabyteDB Anywhere.",
			},
			"cleanup": {
				Type:     schema.TypeBool,
				Optional: true,
				Description: "Flag for indicating if resources should be cleaned up during " +
					"the uninstall. Set this to true if you plan to reuse the virtual machine.",
			},
		},
	}
}

func resourceInstallationDiff() schema.CustomizeDiffFunc {
	return customdiff.All(
		customdiff.ValidateValue("replicated_config_file", func(ctx context.Context, value,
			meta interface{}) error {
			name := value.(string)
			if err := utils.FileExist(name); err != nil {
				return err
			}
			return nil
		}),
		customdiff.ValidateValue("tls_certificate_file", func(ctx context.Context, value,
			meta interface{}) error {
			if value.(string) != "" {
				name := value.(string)
				if err := utils.FileExist(name); err != nil {
					return err
				}
			}
			return nil
		}),
		customdiff.ValidateValue("tls_key_file", func(ctx context.Context, value,
			meta interface{}) error {
			if value.(string) != "" {
				name := value.(string)
				if err := utils.FileExist(name); err != nil {
					return err
				}
			}
			return nil
		}),
		customdiff.ValidateValue("replicated_license_file", func(ctx context.Context, value,
			meta interface{}) error {
			name := value.(string)
			if err := utils.FileExist(name); err != nil {
				return err
			}
			return nil
		}),
		customdiff.ValidateValue("application_settings_file", func(ctx context.Context, value,
			meta interface{}) error {
			name := value.(string)
			if err := utils.FileExist(name); err != nil {
				return err
			}
			return nil
		}),
	)
}

func resourceInstallationCreate(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	publicIP := d.Get("public_ip").(string)
	privateIP := d.Get("private_ip").(string)
	hostIPForSSH := d.Get("ssh_host_ip").(string)
	user := d.Get("ssh_user").(string)
	pk := d.Get("ssh_private_key").(string)

	sshClient, err := waitForIP(ctx, user, hostIPForSSH, pk, d.Timeout(schema.TimeoutCreate))
	if err != nil {
		tflog.Error(ctx, "Timeout: Couldn't connect to YugabyteDB Anywhere host")
		return diag.FromErr(err)
	}
	defer sshClient.Close()

	for key, remote := range installationFiles {
		local := d.Get(key).(string)
		if local == "" {
			continue
		}
		err = scpFile(ctx, sshClient, local, remote)
		if err != nil {
			tflog.Error(ctx, "Error occurred while transferring files required for installation")
			return diag.FromErr(err)
		}
	}

	for _, cmd := range getInstallationCommands(publicIP, privateIP) {
		m, err := runCommand(ctx, sshClient, cmd)
		if err != nil {
			tflog.Error(ctx, m)
			return diag.FromErr(errors.New(m))
		}
	}

	c := meta.(*api.APIClient).YugawareClient
	// Giving 20 mins for YBA application to start
	err = waitForStart(ctx, c, 2*d.Timeout(schema.TimeoutCreate))
	if err != nil {
		tflog.Error(ctx, "Timeout: YugabyteDB Anywhere Application is not running")
		return diag.FromErr(err)
	}

	d.SetId(uuid.New().String())
	return diags
}

func resourceInstallationRead(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{}) diag.Diagnostics {
	// remote state is not read for this resource
	return diag.Diagnostics{}
}

func resourceInstallationUpdate(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{}) diag.Diagnostics {
	// do nothing
	return diag.Diagnostics{}
}

func resourceInstallationDelete(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	cleanup := d.Get("cleanup").(bool)
	if !cleanup {
		d.SetId("")
		return diags
	}

	ip := d.Get("public_ip").(string)
	user := d.Get("ssh_user").(string)
	pk := d.Get("ssh_private_key").(string)

	sshClient, err := newSSHClient(user, ip, pk)
	if err != nil {
		return diag.FromErr(err)
	}
	defer sshClient.Close()

	for _, cmd := range deletionCommands {
		m, err := runCommand(ctx, sshClient, cmd)
		if err != nil {
			tflog.Error(ctx, m)
		}
	}

	d.SetId("")
	return diags
}
