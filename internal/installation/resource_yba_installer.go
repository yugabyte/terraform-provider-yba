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
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/customdiff"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
	"golang.org/x/exp/maps"
)

var (
	reconfigurationYBAInstallerFiles = map[string]string{
		"tls_certificate_file":      "/tmp/server.crt",
		"tls_key_file":              "/tmp/server.key",
		"application_settings_file": "/tmp/settings.yml",
	}
	licenseYBAInstallerFiles = map[string]string{
		"yba_license_file": "/tmp/license.lic",
	}
)

func getInstallationFiles() map[string]string {
	installationYBAInstallerFiles := reconfigurationYBAInstallerFiles
	maps.Copy(installationYBAInstallerFiles, licenseYBAInstallerFiles)
	return installationYBAInstallerFiles
}

// ResourceYBAInstaller handles installation of YugabyteDB Anywhere using YBA installer
func ResourceYBAInstaller() *schema.Resource {
	return &schema.Resource{
		Description: "Manages the installation of YugabyteDB Anywhere on an existing virtual" +
			" machine using YBA Installer. ",

		CreateContext: resourceYBAInstallerCreate,
		ReadContext:   resourceYBAInstallerRead,
		UpdateContext: resourceYBAInstallerUpdate,
		DeleteContext: resourceYBAInstallerDelete,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(10 * time.Minute),
			Update: schema.DefaultTimeout(5 * time.Minute),
			Delete: schema.DefaultTimeout(10 * time.Minute),
		},

		CustomizeDiff: resourceYBAInstallerDiff(),

		Schema: map[string]*schema.Schema{
			"yba_version": {
				Type:     schema.TypeString,
				Required: true,
				// Change in this triggers ./yba-ctl upgrade
				Description: "Version of YugabyteDB Anywhere to be installed.",
			},
			"host_os": {
				Type:        schema.TypeString,
				Optional:    true,
				ForceNew:    true,
				Default:     "linux",
				Description: "Operating System of the host Virtual Machine. Default is linux.",
			},
			"host_architecture": {
				Type:        schema.TypeString,
				Optional:    true,
				ForceNew:    true,
				Default:     "x86_64",
				Description: "Architecture of the host Virtual Machine. Default is x86_64.",
			},
			"ssh_host_ip": {
				Type:     schema.TypeString,
				Required: true,
				Description: "IP address of VM for SSH. Typically same as public_ip or " +
					"private_ip.",
			},
			"ssh_private_key_file_path": {
				Type:     schema.TypeString,
				Required: true,
				Description: "Path to file containing the private key to use for ssh " +
					"commands.",
			},
			"ssh_user": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "User with sudo access to use for ssh commands.",
			},
			"tls_certificate_file": {
				Type:     schema.TypeString,
				Optional: true,
				// change should trigger yba-ctl reconfigure
				RequiredWith: []string{"tls_key_file"},
				Description: "TLS certificate used to configure HTTPS. Ensure " +
					"yba_application_settings file has *server_cert_path* set to /tmp/server.crt",
			},
			"tls_key_file": {
				Type:     schema.TypeString,
				Optional: true,
				// change should trigger yba-ctl reconfigure
				RequiredWith: []string{"tls_certificate_file"},
				Description: "TLS key used to configure HTTPS. Ensure " +
					"yba_application_settings file has *server_key_path* set to /tmp/server.key",
			},
			"yba_license_file": {
				Type:     schema.TypeString,
				Required: true,
				Description: "YugabyteDB Anywhere license file used for installation using " +
					"YBA installer.",
			},
			"application_settings_file": {
				Type:     schema.TypeString,
				Optional: true,
				// Change in this should trigger yba-ctl reconfigure
				Description: "Application settings file to configure YugabyteDB Anywhere. " +
					"If left empty, the [default configuration]" +
					"(https://github.com/yugabyte/terraform-provider-yba/tree/main" +
					"/modules/resources/yba-ctl.yml)" +
					" would be used for the application.",
			},
			"reconfigure": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
				// True should trigger yba-ctl reconfigure
				// if the contents of application_settings_file have been modified
				Description: "Set to true for reconfiguration (If the contents of " +
					"application_settings_file have been modified).",
			},
			"skip_preflight_checks": {
				Type:     schema.TypeList,
				Optional: true,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
				Description: "Check names to be skipped during preflight check.",
			},
		},
	}
}

func resourceYBAInstallerDiff() schema.CustomizeDiffFunc {
	return customdiff.All(
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
		customdiff.ValidateValue("yba_license_file", func(ctx context.Context, value,
			meta interface{}) error {
			name := value.(string)
			if err := utils.FileExist(name); err != nil {
				return err
			}
			return nil
		}),
		customdiff.ValidateValue("application_settings_file", func(ctx context.Context, value,
			meta interface{}) error {
			if value.(string) != "" {
				name := value.(string)
				if err := utils.FileExist(name); err != nil {
					return err
				}
			}
			return nil
		}),
		customdiff.ValidateValue("ssh_private_key_file_path", func(ctx context.Context, value,
			meta interface{}) error {
			name := value.(string)
			if err := utils.FileExist(name); err != nil {
				return err
			}
			return nil
		}),
		customdiff.IfValue("reconfigure",
			func(ctx context.Context, value, meta interface{}) bool {
				return value.(bool)
			},
			func(ctx context.Context, d *schema.ResourceDiff, meta interface{}) error {
				applicationSettingsFile := d.Get("application_settings_file").(string)
				if applicationSettingsFile == "" {
					return errors.New("Cannot reconfigure YBA Installer with empty " +
						"application_settings_file file")
				}
				return nil
			}),
	)
}

func resourceYBAInstallerCreate(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	hostIPForSSH := d.Get("ssh_host_ip").(string)
	user := d.Get("ssh_user").(string)
	pk, err := utils.ReadSSHPrivateKey(d.Get("ssh_private_key_file_path").(string))
	if err != nil {
		return diag.FromErr(err)
	}

	sshClient, err := waitForIP(ctx, user, hostIPForSSH, *pk, d.Timeout(schema.TimeoutCreate))
	if err != nil {
		tflog.Error(ctx, "Timeout: Couldn't connect to YugabyteDB Anywhere host")
		return diag.FromErr(err)
	}
	defer sshClient.Close()

	for key, remote := range getInstallationFiles() {
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

	ybaVersion := d.Get("yba_version").(string)
	hostOS := d.Get("host_os").(string)
	hostArch := d.Get("host_architecture").(string)
	configExists := false
	skipPreflight := d.Get("skip_preflight_checks")
	var skipPreflightChecksList *[]string
	if skipPreflight != nil {
		skipPreflightChecksList = utils.StringSlice(d.Get("skip_preflight_checks").([]interface{}))
	}
	if d.Get("application_settings_file").(string) != "" {
		configExists = true
	}

	for _, cmd := range getInstallCommands(ybaVersion, hostOS, hostArch, configExists,
		skipPreflightChecksList) {
		m, err := runCommand(ctx, sshClient, cmd)
		if err != nil {
			tflog.Error(ctx, m)
			if m != "" {
				return diag.FromErr(errors.New(m))
			}
			return diag.Errorf("Please run with TF_LOG=INFO for error logs")
		}
	}

	d.SetId(uuid.New().String())
	return diags
}

func resourceYBAInstallerRead(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{}) diag.Diagnostics {
	// remote state is not read for this resource
	return diag.Diagnostics{}
}

func resourceYBAInstallerUpdate(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{}) diag.Diagnostics {
	// same steps as installation
	// run ./yba-ctl with upgrade instead of install

	hostIPForSSH := d.Get("ssh_host_ip").(string)
	user := d.Get("ssh_user").(string)
	pk, err := utils.ReadSSHPrivateKey(d.Get("ssh_private_key_file_path").(string))
	skipPreflightChecksList := utils.StringSlice(d.Get("skip_preflight_checks").([]interface{}))
	if err != nil {
		return diag.FromErr(err)
	}
	sshClient, err := waitForIP(ctx, user, hostIPForSSH, *pk, d.Timeout(schema.TimeoutCreate))
	if err != nil {
		tflog.Error(ctx, "Timeout: Couldn't connect to YugabyteDB Anywhere host")
		return diag.FromErr(err)
	}
	defer sshClient.Close()

	hostOS := d.Get("host_os").(string)
	hostArch := d.Get("host_architecture").(string)
	var oldVersion, newVersion string
	if d.HasChange("yba_version") {
		old, new := d.GetChange("yba_version")
		oldVersion = old.(string)
		newVersion = new.(string)
	} else {
		oldVersion = d.Get("yba_version").(string)
	}

	commands := make([]string, 0)

	if d.HasChange("yba_license_file") {
		for key, remote := range licenseYBAInstallerFiles {
			local := d.Get(key).(string)
			if local == "" {
				continue
			}
			err = scpFile(ctx, sshClient, local, remote)
			if err != nil {
				tflog.Error(ctx, "Error occurred while transferring files required for "+
					"updating license")
				return diag.FromErr(err)
			}
		}

		folder, _, _ := getYBAInstallerPackageString(oldVersion, hostOS, hostArch)
		commands = append(commands, getAddLicenseCommands(folder))
	}

	if d.Get("reconfigure").(bool) || d.HasChange("application_settings_file") ||
		d.HasChange("tls_certificate_file") || d.HasChange("tls_key_file") {
		applicationSettingsFile := d.Get("application_settings_file").(string)
		if applicationSettingsFile == "" {
			err := errors.New("Cannot reconfigure YBA Installer with empty application_settings_file " +
				"file")
			return diag.FromErr(err)
		}
		for key, remote := range reconfigurationYBAInstallerFiles {
			local := d.Get(key).(string)
			if local == "" {
				continue
			}
			err = scpFile(ctx, sshClient, local, remote)
			if err != nil {
				tflog.Error(ctx, "Error occurred while transferring files required for "+
					"reconfiguration")
				return diag.FromErr(err)
			}
		}
		commands = append(commands, getReconfigureCommands()...)
	}

	if d.HasChange("yba_version") {
		commands = append(commands, getUpgradeCommands(newVersion, hostOS, hostArch,
			skipPreflightChecksList)...)
	}

	for _, cmd := range commands {
		m, err := runCommand(ctx, sshClient, cmd)
		if err != nil {
			tflog.Error(ctx, m)
			if m != "" {
				return diag.FromErr(errors.New(m))
			}
			return diag.Errorf("Please run with TF_LOG=INFO for error logs")
		}
	}

	return diag.Diagnostics{}
}

func resourceYBAInstallerDelete(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	hostIPForSSH := d.Get("ssh_host_ip").(string)
	user := d.Get("ssh_user").(string)
	pk, err := utils.ReadSSHPrivateKey(d.Get("ssh_private_key_file_path").(string))
	if err != nil {
		return diag.FromErr(err)
	}

	sshClient, err := newSSHClient(user, hostIPForSSH, *pk)
	if err != nil {
		return diag.FromErr(err)
	}
	defer sshClient.Close()

	for _, cmd := range getDeleteCommands() {
		m, err := runCommand(ctx, sshClient, cmd)
		if err != nil {
			tflog.Error(ctx, m)
		}
	}

	d.SetId("")
	return diags
}

func getYBAInstallerPackageString(version, os, arch string) (folder, bundle, v string) {
	folder = fmt.Sprintf("yba_installer_full-%s", version)
	bundle = fmt.Sprintf("%s-%s-%s", folder, os, arch)
	vParts := strings.Split(version, "-")
	// Get the version without build number to access remote folder for releases
	v = vParts[0]
	return folder, bundle, v
}

func getYBAInstallerBundle(version, os, arch string) (string, []string) {
	getBundle := make([]string, 0)
	folder, bundle, v := getYBAInstallerPackageString(version, os, arch)
	s := fmt.Sprintf("curl -O https://downloads.yugabyte.com/releases/%s/%s.tar.gz", v, bundle)
	getBundle = append(getBundle, s)
	s = fmt.Sprintf("tar -xf %s.tar.gz", bundle)
	getBundle = append(getBundle, s)
	return folder, getBundle
}

func getInstallCommands(
	version, os, arch string,
	config bool, skipPreflightCheckList *[]string) []string {
	var s string
	folder, installationCommands := getYBAInstallerBundle(version, os, arch)
	s = getAddLicenseCommands(folder)
	installationCommands = append(installationCommands, s)
	if config {
		k := fmt.Sprintf("sudo mv /tmp/settings.yml /opt/yba-ctl/yba-ctl.yml")
		installationCommands = append(installationCommands, k)
	}
	s = fmt.Sprintf("sudo ./%s/yba-ctl install -f", folder)
	if skipPreflightCheckList != nil && len(*skipPreflightCheckList) != 0 {
		skipPreflight := getSkipPreflightChecksSubCommand(*skipPreflightCheckList)
		s = fmt.Sprintf("%s -s %s", s, skipPreflight)
	}
	installationCommands = append(installationCommands, s)
	return installationCommands
}

func getReconfigureCommands() []string {
	var reconfigureCommands = []string{"sudo mv /tmp/settings.yml /opt/yba-ctl/yba-ctl.yml"}
	s := fmt.Sprintf("sudo /opt/yba-ctl/yba-ctl reconfigure -f")
	reconfigureCommands = append(reconfigureCommands, s)
	return reconfigureCommands
}

func getUpgradeCommands(version, os, arch string, skipPreflightCheckList *[]string) []string {
	var s string
	folder, updateCommands := getYBAInstallerBundle(version, os, arch)
	s = fmt.Sprintf("sudo ./%s/yba-ctl upgrade -f", folder)
	if skipPreflightCheckList != nil && len(*skipPreflightCheckList) != 0 {
		skipPreflight := getSkipPreflightChecksSubCommand(*skipPreflightCheckList)
		s = fmt.Sprintf("%s -s %s", s, skipPreflight)
	}
	updateCommands = append(updateCommands, s)
	return updateCommands
}

func getAddLicenseCommands(folder string) string {
	return fmt.Sprintf("sudo ./%s/yba-ctl license add -l /tmp/license.lic", folder)
}

func getDeleteCommands() []string {
	var deleteCommands = []string{"sudo /opt/yba-ctl/yba-ctl clean"}
	s := fmt.Sprintf("sudo rm -rf /opt/yugabyte")
	deleteCommands = append(deleteCommands, s)
	s = fmt.Sprintf("sudo rm /tmp/server.crt /tmp/server.key /tmp/license.lic /tmp/settings.yml")
	deleteCommands = append(deleteCommands, s)
	return deleteCommands
}

func getSkipPreflightChecksSubCommand(commands []string) string {
	var s string
	for i, v := range commands {
		if i == 0 {
			s = v
		} else {
			s = fmt.Sprintf("%s,%s", s, v)
		}
	}
	return s
}
