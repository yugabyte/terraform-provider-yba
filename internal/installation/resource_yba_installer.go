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
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/customdiff"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
	"golang.org/x/crypto/ssh"
)

const (
	// GADownloadURL is the base URL for GA release versions (e.g. 2024.x, 2025.x).
	GADownloadURL = "https://downloads.yugabyte.com/releases"
	// PreReleaseDownloadURL is the base URL for pre-release/CI builds (e.g. 2.25.x).
	PreReleaseDownloadURL = "https://releases.yugabyte.com"
)

// gaVersionRegex matches GA release version strings, which begin with a 4-digit
// year followed by a dot (e.g. "2024.1.0.0", "2025.2.3.0-b50").
var gaVersionRegex = regexp.MustCompile(`^20\d{2}\.`)

// installerFileSpec describes one logical input that the YBA installer
// resource needs to upload to the remote host. Each spec exposes both a
// file-path attribute and a content attribute. Either may be set; the
// content attribute takes precedence when both are populated (the schema
// also marks them as conflicting so this is normally rejected by
// Terraform up-front).
type installerFileSpec struct {
	// fileAttr is the attribute that accepts a path to a local file.
	fileAttr string
	// contentAttr is the attribute that accepts the file contents
	// directly as a string.
	contentAttr string
	// remotePath is where the contents are written on the remote host.
	remotePath string
}

var (
	tlsCertificateSpec = installerFileSpec{
		fileAttr:    "tls_certificate_file",
		contentAttr: "tls_certificate",
		remotePath:  "/tmp/server.crt",
	}
	tlsKeySpec = installerFileSpec{
		fileAttr:    "tls_key_file",
		contentAttr: "tls_key",
		remotePath:  "/tmp/server.key",
	}
	applicationSettingsSpec = installerFileSpec{
		fileAttr:    "application_settings_file",
		contentAttr: "application_settings",
		remotePath:  "/tmp/settings.yml",
	}
	licenseSpec = installerFileSpec{
		fileAttr:    "yba_license_file",
		contentAttr: "yba_license",
		remotePath:  "/tmp/license.lic",
	}
	sshPrivateKeySpec = installerFileSpec{
		fileAttr:    "ssh_private_key_file_path",
		contentAttr: "ssh_private_key",
		// ssh_private_key is consumed locally (to authenticate) and is
		// not transferred to the remote host. remotePath is unused.
	}
)

// reconfigurationYBAInstallerSpecs lists inputs that participate in a
// reconfigure cycle (their values are written to /tmp/* on the remote
// host before yba-ctl reconfigure runs).
func reconfigurationYBAInstallerSpecs() []installerFileSpec {
	return []installerFileSpec{
		tlsCertificateSpec,
		tlsKeySpec,
		applicationSettingsSpec,
	}
}

// licenseYBAInstallerSpecs lists inputs that are uploaded as part of a
// license-update flow.
func licenseYBAInstallerSpecs() []installerFileSpec {
	return []installerFileSpec{
		licenseSpec,
	}
}

// installationYBAInstallerSpecs lists every spec that may need to be
// uploaded during a fresh install.
func installationYBAInstallerSpecs() []installerFileSpec {
	specs := make([]installerFileSpec, 0,
		len(reconfigurationYBAInstallerSpecs())+len(licenseYBAInstallerSpecs()))
	specs = append(specs, reconfigurationYBAInstallerSpecs()...)
	specs = append(specs, licenseYBAInstallerSpecs()...)
	return specs
}

// resolveInstallerInput returns the content for a given installer input,
// preferring the inline content attribute when set and falling back to
// reading the file pointed to by the file-path attribute. An empty
// string with a nil error is returned when neither attribute is set
// (callers should treat that as "input not provided").
func resolveInstallerInput(d *schema.ResourceData, spec installerFileSpec) (string, error) {
	if spec.contentAttr != "" {
		if v, ok := d.GetOk(spec.contentAttr); ok {
			content := v.(string)
			if content != "" {
				return content, nil
			}
		}
	}
	if spec.fileAttr != "" {
		if v, ok := d.GetOk(spec.fileAttr); ok {
			path := v.(string)
			if path != "" {
				data, err := utils.ReadFileContents(path)
				if err != nil {
					return "", err
				}
				return data, nil
			}
		}
	}
	return "", nil
}

// changeDetector is the minimal surface area needed by
// installerInputHasChange. Both *schema.ResourceData and
// *schema.ResourceDiff satisfy it.
type changeDetector interface {
	HasChange(key string) bool
}

// inputReader is the minimal surface area needed by
// installerInputProvided. Both *schema.ResourceData and
// *schema.ResourceDiff satisfy it.
type inputReader interface {
	GetOk(key string) (interface{}, bool)
}

// installerInputHasChange returns true if either of the attributes for
// the given spec has changed.
func installerInputHasChange(d changeDetector, spec installerFileSpec) bool {
	if spec.contentAttr != "" && d.HasChange(spec.contentAttr) {
		return true
	}
	if spec.fileAttr != "" && d.HasChange(spec.fileAttr) {
		return true
	}
	return false
}

// installerInputProvided returns true if either attribute for the spec
// is set to a non-empty value.
func installerInputProvided(d inputReader, spec installerFileSpec) bool {
	if spec.contentAttr != "" {
		if v, ok := d.GetOk(spec.contentAttr); ok && v.(string) != "" {
			return true
		}
	}
	if spec.fileAttr != "" {
		if v, ok := d.GetOk(spec.fileAttr); ok && v.(string) != "" {
			return true
		}
	}
	return false
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
				Optional: true,
				ExactlyOneOf: []string{
					"ssh_private_key_file_path",
					"ssh_private_key",
				},
				ConflictsWith: []string{"ssh_private_key"},
				Description: "Path to file containing the private key to use for ssh " +
					"commands. Conflicts with `ssh_private_key`.",
			},
			"ssh_private_key": {
				Type:      schema.TypeString,
				Optional:  true,
				Sensitive: true,
				ConflictsWith: []string{
					"ssh_private_key_file_path",
				},
				Description: "Contents of the private key to use for ssh commands. " +
					"Use this instead of `ssh_private_key_file_path` to pass the key " +
					"directly without writing it to a local file.",
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
				ConflictsWith: []string{"tls_certificate"},
				Description: "Path to a TLS certificate file used to configure HTTPS. " +
					"Ensure the application settings have *server_cert_path* set to " +
					"/tmp/server.crt. Conflicts with `tls_certificate`.",
			},
			"tls_certificate": {
				Type:      schema.TypeString,
				Optional:  true,
				Sensitive: true,
				// change should trigger yba-ctl reconfigure
				ConflictsWith: []string{"tls_certificate_file"},
				Description: "Inline TLS certificate contents used to configure HTTPS. " +
					"Ensure the application settings have *server_cert_path* set to " +
					"/tmp/server.crt.",
			},
			"tls_key_file": {
				Type:     schema.TypeString,
				Optional: true,
				// change should trigger yba-ctl reconfigure
				ConflictsWith: []string{"tls_key"},
				Description: "Path to a TLS key file used to configure HTTPS. Ensure " +
					"the application settings have *server_key_path* set to " +
					"/tmp/server.key. Conflicts with `tls_key`.",
			},
			"tls_key": {
				Type:      schema.TypeString,
				Optional:  true,
				Sensitive: true,
				// change should trigger yba-ctl reconfigure
				ConflictsWith: []string{"tls_key_file"},
				Description: "Inline TLS key contents used to configure HTTPS. Ensure " +
					"the application settings have *server_key_path* set to " +
					"/tmp/server.key.",
			},
			"yba_license_file": {
				Type:     schema.TypeString,
				Optional: true,
				ExactlyOneOf: []string{
					"yba_license_file",
					"yba_license",
				},
				ConflictsWith: []string{"yba_license"},
				Description: "Path to a YugabyteDB Anywhere license file used for " +
					"installation. Conflicts with `yba_license`.",
			},
			"yba_license": {
				Type:      schema.TypeString,
				Optional:  true,
				Sensitive: true,
				ConflictsWith: []string{
					"yba_license_file",
				},
				Description: "Inline YugabyteDB Anywhere license contents used for " +
					"installation. Use this instead of `yba_license_file` to pass " +
					"the license without writing it to a local file.",
			},
			"application_settings_file": {
				Type:     schema.TypeString,
				Optional: true,
				// Change in this should trigger yba-ctl reconfigure
				ConflictsWith: []string{"application_settings"},
				Description: "Path to an application settings file to configure " +
					"YugabyteDB Anywhere. If left empty, the [default configuration]" +
					"(https://github.com/yugabyte/terraform-provider-yba/tree/main" +
					"/modules/resources/yba-ctl.yml)" +
					" would be used. Conflicts with `application_settings`.",
			},
			"application_settings": {
				Type:      schema.TypeString,
				Optional:  true,
				Sensitive: true,
				// Change in this should trigger yba-ctl reconfigure
				ConflictsWith: []string{"application_settings_file"},
				Description: "Inline application settings contents used to configure " +
					"YugabyteDB Anywhere. If left empty, the [default configuration]" +
					"(https://github.com/yugabyte/terraform-provider-yba/tree/main" +
					"/modules/resources/yba-ctl.yml)" +
					" would be used.",
			},
			"reconfigure": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
				// True should trigger yba-ctl reconfigure
				// if the contents of application_settings_file have been modified
				Description: "Force a reconfiguration on the next apply, even when " +
					"no other tracked attribute has changed. Content changes to " +
					"`application_settings`, `tls_certificate`, or `tls_key` " +
					"already trigger reconfiguration automatically.",
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

// validateInstallerFileAttr returns a CustomizeDiff function that
// confirms a file-path attribute points at a real file (only when the
// attribute is non-empty - empty values are valid because the user may
// be supplying contents via the corresponding content attribute).
func validateInstallerFileAttr(attr string) schema.CustomizeDiffFunc {
	return customdiff.ValidateValue(attr, func(ctx context.Context, value,
		meta interface{}) error {
		name, _ := value.(string)
		if name == "" {
			return nil
		}
		return utils.FileExist(name)
	})
}

func resourceYBAInstallerDiff() schema.CustomizeDiffFunc {
	return customdiff.All(
		validateInstallerFileAttr("tls_certificate_file"),
		validateInstallerFileAttr("tls_key_file"),
		validateInstallerFileAttr("yba_license_file"),
		validateInstallerFileAttr("application_settings_file"),
		validateInstallerFileAttr("ssh_private_key_file_path"),
		// TLS cert and key must be supplied together. Either side may
		// be provided through the file-path or the inline content
		// attribute.
		func(ctx context.Context, d *schema.ResourceDiff, meta interface{}) error {
			certProvided := installerInputProvided(d, tlsCertificateSpec)
			keyProvided := installerInputProvided(d, tlsKeySpec)
			if certProvided != keyProvided {
				return errors.New(
					"tls_certificate / tls_certificate_file and tls_key / " +
						"tls_key_file must be set together",
				)
			}
			return nil
		},
		customdiff.IfValue("reconfigure",
			func(ctx context.Context, value, meta interface{}) bool {
				return value.(bool)
			},
			func(ctx context.Context, d *schema.ResourceDiff, meta interface{}) error {
				if !installerInputProvided(d, applicationSettingsSpec) {
					return errEmptyApplicationSettings
				}
				return nil
			}),
	)
}

// errEmptyApplicationSettings is returned when a reconfigure is
// requested but neither application_settings nor
// application_settings_file is set.
var errEmptyApplicationSettings = errors.New(
	"Cannot reconfigure YBA Installer with empty application_settings " +
		"(or application_settings_file)",
)

// resolveSSHPrivateKey returns the SSH private key contents either from
// the inline `ssh_private_key` attribute or by reading the file pointed
// to by `ssh_private_key_file_path`.
func resolveSSHPrivateKey(d *schema.ResourceData) (string, error) {
	content, err := resolveInstallerInput(d, sshPrivateKeySpec)
	if err != nil {
		return "", err
	}
	if content == "" {
		return "", errors.New("ssh_private_key or ssh_private_key_file_path must be set")
	}
	return content, nil
}

// uploadInstallerInputs resolves and uploads each given input to the
// remote host. Inputs that are not set (neither file path nor content)
// are skipped.
func uploadInstallerInputs(
	ctx context.Context,
	sshClient *ssh.Client,
	d *schema.ResourceData,
	specs []installerFileSpec,
) error {
	for _, spec := range specs {
		if spec.remotePath == "" {
			continue
		}
		content, err := resolveInstallerInput(d, spec)
		if err != nil {
			return err
		}
		if content == "" {
			continue
		}
		if err := scpContent(ctx, sshClient, content, spec.remotePath); err != nil {
			return err
		}
	}
	return nil
}

func resourceYBAInstallerCreate(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	hostIPForSSH := d.Get("ssh_host_ip").(string)
	user := d.Get("ssh_user").(string)
	pk, err := resolveSSHPrivateKey(d)
	if err != nil {
		return diag.FromErr(err)
	}

	sshClient, err := waitForIP(ctx, user, hostIPForSSH, pk, d.Timeout(schema.TimeoutCreate))
	if err != nil {
		tflog.Error(ctx, "Timeout: Couldn't connect to YugabyteDB Anywhere host")
		return diag.FromErr(err)
	}
	defer sshClient.Close()

	if err := uploadInstallerInputs(ctx, sshClient, d, installationYBAInstallerSpecs()); err != nil {
		tflog.Error(ctx, "Error occurred while transferring files required for installation")
		return diag.FromErr(err)
	}

	ybaVersion := d.Get("yba_version").(string)
	hostOS := d.Get("host_os").(string)
	hostArch := d.Get("host_architecture").(string)
	skipPreflight := d.Get("skip_preflight_checks")
	var skipPreflightChecksList *[]string
	if skipPreflight != nil {
		skipPreflightChecksList = utils.StringSlice(d.Get("skip_preflight_checks").([]interface{}))
	}
	configExists := installerInputProvided(d, applicationSettingsSpec)

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
	pk, err := resolveSSHPrivateKey(d)
	if err != nil {
		return diag.FromErr(err)
	}
	skipPreflightChecksList := utils.StringSlice(d.Get("skip_preflight_checks").([]interface{}))
	sshClient, err := waitForIP(ctx, user, hostIPForSSH, pk, d.Timeout(schema.TimeoutCreate))
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

	if installerInputHasChange(d, licenseSpec) {
		if err := uploadInstallerInputs(ctx, sshClient, d, licenseYBAInstallerSpecs()); err != nil {
			tflog.Error(ctx, "Error occurred while transferring files required for "+
				"updating license")
			return diag.FromErr(err)
		}
		folder, _, _ := getYBAInstallerPackageString(oldVersion, hostOS, hostArch)
		commands = append(commands, getAddLicenseCommand(oldVersion, folder))
	}

	reconfigureRequested := d.Get("reconfigure").(bool)
	contentChanged := installerInputHasChange(d, applicationSettingsSpec) ||
		installerInputHasChange(d, tlsCertificateSpec) ||
		installerInputHasChange(d, tlsKeySpec)
	if reconfigureRequested || contentChanged {
		if !installerInputProvided(d, applicationSettingsSpec) {
			return diag.FromErr(errEmptyApplicationSettings)
		}
		if err := uploadInstallerInputs(
			ctx, sshClient, d, reconfigurationYBAInstallerSpecs(),
		); err != nil {
			tflog.Error(ctx, "Error occurred while transferring files required for "+
				"reconfiguration")
			return diag.FromErr(err)
		}
		commands = append(commands, getReconfigureCommands(oldVersion)...)
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
	pk, err := resolveSSHPrivateKey(d)
	if err != nil {
		return diag.FromErr(err)
	}

	sshClient, err := newSSHClient(user, hostIPForSSH, pk)
	if err != nil {
		return diag.FromErr(err)
	}
	defer sshClient.Close()

	ybaVersion := d.Get("yba_version").(string)
	for _, cmd := range getDeleteCommands(ybaVersion) {
		m, err := runCommand(ctx, sshClient, cmd)
		if err != nil {
			tflog.Error(ctx, m)
		}
	}

	d.SetId("")
	return diags
}

// IsGAVersion returns true for GA release versions (e.g. 2024.x, 2025.x).
// Pre-release / CI builds use the legacy 2.x scheme (e.g. 2.25.0.0).
func IsGAVersion(version string) bool {
	return gaVersionRegex.MatchString(version)
}

func getYBAInstallerPackageString(version, os, arch string) (folder, bundle, v string) {
	folder = fmt.Sprintf("yba_installer_full-%s", version)
	// Pre-release versions use "centos" instead of "linux" in the bundle name
	if !IsGAVersion(version) && os == "linux" {
		os = "centos"
	}
	bundle = fmt.Sprintf("%s-%s-%s", folder, os, arch)
	vParts := strings.Split(version, "-")
	// Get the version without build number to access remote folder for releases
	v = vParts[0]
	return folder, bundle, v
}

func getYBAInstallerBundle(version, os, arch string) (string, []string) {
	getBundle := make([]string, 0)
	folder, bundle, v := getYBAInstallerPackageString(version, os, arch)
	// GA versions use downloads.yugabyte.com, pre-release use releases.yugabyte.com
	var s string
	if IsGAVersion(version) {
		s = fmt.Sprintf("curl -O %s/%s/%s.tar.gz", GADownloadURL, v, bundle)
	} else {
		s = fmt.Sprintf("curl -O %s/%s/%s.tar.gz", PreReleaseDownloadURL, version, bundle)
	}
	getBundle = append(getBundle, s)
	s = fmt.Sprintf("tar -xf %s.tar.gz", bundle)
	getBundle = append(getBundle, s)
	return folder, getBundle
}

// getAddLicenseCommand returns the yba-ctl invocation that registers the license
// file. Pre-release builds run with YBA_MODE=dev to skip strict version comparison
// checks that would otherwise reject non-GA version strings.
func getAddLicenseCommand(version, folder string) string {
	if IsGAVersion(version) {
		return fmt.Sprintf("sudo ./%s/yba-ctl license add -l /tmp/license.lic", folder)
	}
	return fmt.Sprintf("sudo YBA_MODE=dev ./%s/yba-ctl license add -l /tmp/license.lic", folder)
}

func getInstallCommands(
	version, os, arch string,
	config bool, skipPreflightCheckList *[]string) []string {
	var s string
	folder, installationCommands := getYBAInstallerBundle(version, os, arch)
	s = getAddLicenseCommand(version, folder)
	installationCommands = append(installationCommands, s)
	if config {
		installationCommands = append(installationCommands,
			"sudo mv /tmp/settings.yml /opt/yba-ctl/yba-ctl.yml")
	}
	if IsGAVersion(version) {
		s = fmt.Sprintf("sudo ./%s/yba-ctl install -f", folder)
	} else {
		s = fmt.Sprintf("sudo YBA_MODE=dev ./%s/yba-ctl install -f", folder)
	}
	if skipPreflightCheckList != nil && len(*skipPreflightCheckList) != 0 {
		s = fmt.Sprintf("%s -s %s", s, strings.Join(*skipPreflightCheckList, ","))
	}
	installationCommands = append(installationCommands, s)
	return installationCommands
}

func getReconfigureCommands(version string) []string {
	var reconfigureCommands = []string{"sudo mv /tmp/settings.yml /opt/yba-ctl/yba-ctl.yml"}
	if IsGAVersion(version) {
		reconfigureCommands = append(reconfigureCommands, "sudo /opt/yba-ctl/yba-ctl reconfigure -f")
	} else {
		reconfigureCommands = append(reconfigureCommands, "sudo YBA_MODE=dev /opt/yba-ctl/yba-ctl reconfigure -f")
	}
	return reconfigureCommands
}

func getUpgradeCommands(version, os, arch string, skipPreflightCheckList *[]string) []string {
	var s string
	folder, updateCommands := getYBAInstallerBundle(version, os, arch)
	if IsGAVersion(version) {
		s = fmt.Sprintf("sudo ./%s/yba-ctl upgrade -f", folder)
	} else {
		s = fmt.Sprintf("sudo YBA_MODE=dev ./%s/yba-ctl upgrade -f", folder)
	}
	if skipPreflightCheckList != nil && len(*skipPreflightCheckList) != 0 {
		s = fmt.Sprintf("%s -s %s", s, strings.Join(*skipPreflightCheckList, ","))
	}
	updateCommands = append(updateCommands, s)
	return updateCommands
}

func getDeleteCommands(version string) []string {
	var s string
	if IsGAVersion(version) {
		s = "sudo /opt/yba-ctl/yba-ctl clean"
	} else {
		s = "sudo YBA_MODE=dev /opt/yba-ctl/yba-ctl clean"
	}
	var deleteCommands = []string{s}
	deleteCommands = append(deleteCommands, "sudo rm -rf /opt/yugabyte")
	deleteCommands = append(deleteCommands,
		"sudo rm /tmp/server.crt /tmp/server.key /tmp/license.lic /tmp/settings.yml")
	return deleteCommands
}
