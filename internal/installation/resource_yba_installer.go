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

// Package installation provides the yba_installer resource for bootstrapping
// YugabyteDB Anywhere over SSH using yba-ctl.
package installation

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/customdiff"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"golang.org/x/crypto/ssh"

	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

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

const (
	// GADownloadURL is the base URL for GA release versions (e.g. 2024.x, 2025.x).
	GADownloadURL = "https://downloads.yugabyte.com/releases"
	// PreReleaseDownloadURL is the base URL for pre-release/CI builds (e.g. 2.25.x).
	PreReleaseDownloadURL = "https://releases.yugabyte.com"
)

// gaVersionRegex matches GA release version strings, which begin with a 4-digit
// year followed by a dot (e.g. "2024.1.0.0", "2025.2.3.0-b50").
var gaVersionRegex = regexp.MustCompile(`^20\d{2}\.`)

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
				data, err := os.ReadFile(path)
				if err != nil {
					return "", fmt.Errorf("failed reading data from %s: %w", path, err)
				}
				return string(data), nil
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

const defaultSSHPort = 22

// ResourceYBAInstaller handles installation of YugabyteDB Anywhere using YBA installer
func ResourceYBAInstaller() *schema.Resource {
	r := &schema.Resource{
		Description: "Manages the installation of YugabyteDB Anywhere on an existing virtual" +
			" machine using YBA Installer.\n\n" +
			"~> **Note:** Destroy runs `yba-ctl clean` on the host. This removes the " +
			"YugabyteDB Anywhere software and keeps the data directory (" +
			"`/opt/yugabyte/data` with the default `installRoot`). To delete the data, " +
			"remove that directory on the host yourself.\n\n" +
			"~> **Note:** When `/opt/yugabyte/data` already holds YugabyteDB Anywhere data " +
			"(for example, on a persistent disk that outlives the VM), create installs the " +
			"software without data and starts YBA on the existing data. So when you destroy " +
			"and recreate this resource on the same host, YBA keeps its data. For a fresh " +
			"installation, delete `/opt/yugabyte/data` on the host before the next apply. " +
			"The provider checks only this path, also when `installRoot` in the " +
			"application settings names another directory.\n\n" +
			"~> **Warning:** If nothing accepts an SSH connection at `ssh_host_ip` and " +
			"`ssh_port` for about 30 seconds, destroy treats the host as gone. It removes " +
			"the resource from state and does not clean up the host. Make sure that the host " +
			"(and any SSH tunnel to it) is reachable before you destroy this resource.\n\n" +
			"~> **Security Note:** The values of `ssh_private_key`, `yba_license`, " +
			"`application_settings`, `tls_certificate` and `tls_key` are stored in the " +
			"Terraform state file (marked as sensitive). Use an encrypted backend and " +
			"restrict access to your state files.",

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

		SchemaVersion: 1,

		Schema: map[string]*schema.Schema{
			"yba_version": {
				Type:     schema.TypeString,
				Required: true,
				// Change in this triggers ./yba-ctl upgrade
				Description: "Version of YugabyteDB Anywhere to install, with its build " +
					"number, for example `2025.2.7.0-b107`. A change to this field on an " +
					"existing installation runs `yba-ctl upgrade` to the new version. " +
					"YBA Installer does not support downgrades, so the plan fails when the " +
					"new version is lower.",
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
				Description: "IP address of the host for SSH and SCP connections. With a " +
					"local SSH tunnel to the host, use `127.0.0.1`.",
			},
			"ssh_port": {
				Type:         schema.TypeInt,
				Optional:     true,
				Default:      defaultSSHPort,
				ValidateFunc: validation.IntBetween(1, 65535),
				Description: "TCP port for SSH and SCP connections to the host. Default is " +
					"22. Set this field when sshd listens on a different port at " +
					"`ssh_host_ip`: for example, a custom sshd port, a NAT or firewall " +
					"port mapping, or the local end of an SSH tunnel.",
			},
			"ssh_private_key_file_path": {
				Type:     schema.TypeString,
				Optional: true,
				ExactlyOneOf: []string{
					"ssh_private_key_file_path",
					"ssh_private_key",
				},
				ConflictsWith: []string{"ssh_private_key"},
				Description: "Path to a local file that contains the private key for SSH " +
					"connections. Set exactly one of `ssh_private_key_file_path` or " +
					"`ssh_private_key`.",
			},
			"ssh_private_key": {
				Type:      schema.TypeString,
				Optional:  true,
				Sensitive: true,
				ConflictsWith: []string{
					"ssh_private_key_file_path",
				},
				Description: "Contents of the private key for SSH connections. Use this " +
					"field instead of `ssh_private_key_file_path` to pass the key without " +
					"a local file. Set exactly one of `ssh_private_key_file_path` or " +
					"`ssh_private_key`.",
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
				Description: "Path to a local TLS certificate file for HTTPS. The provider " +
					"copies it to `/tmp/server.crt` on the host, so set `server_cert_path` " +
					"to that path in the application settings. Conflicts with " +
					"`tls_certificate`.",
			},
			"tls_certificate": {
				Type:      schema.TypeString,
				Optional:  true,
				Sensitive: true,
				// change should trigger yba-ctl reconfigure
				ConflictsWith: []string{"tls_certificate_file"},
				Description: "Contents of the TLS certificate for HTTPS. The provider " +
					"copies it to `/tmp/server.crt` on the host, so set `server_cert_path` " +
					"to that path in the application settings. Conflicts with " +
					"`tls_certificate_file`.",
			},
			"tls_key_file": {
				Type:     schema.TypeString,
				Optional: true,
				// change should trigger yba-ctl reconfigure
				ConflictsWith: []string{"tls_key"},
				Description: "Path to a local TLS key file for HTTPS. The provider copies " +
					"it to `/tmp/server.key` on the host, so set `server_key_path` to that " +
					"path in the application settings. Conflicts with `tls_key`.",
			},
			"tls_key": {
				Type:      schema.TypeString,
				Optional:  true,
				Sensitive: true,
				// change should trigger yba-ctl reconfigure
				ConflictsWith: []string{"tls_key_file"},
				Description: "Contents of the TLS key for HTTPS. The provider copies it to " +
					"`/tmp/server.key` on the host, so set `server_key_path` to that path in " +
					"the application settings. Conflicts with `tls_key_file`.",
			},
			"yba_license_file": {
				Type:     schema.TypeString,
				Optional: true,
				ExactlyOneOf: []string{
					"yba_license_file",
					"yba_license",
				},
				ConflictsWith: []string{"yba_license"},
				Description: "Path to a local YugabyteDB Anywhere license file. Set exactly " +
					"one of `yba_license_file` or `yba_license`.",
			},
			"yba_license": {
				Type:      schema.TypeString,
				Optional:  true,
				Sensitive: true,
				ConflictsWith: []string{
					"yba_license_file",
				},
				Description: "Contents of the YugabyteDB Anywhere license. Use this field " +
					"instead of `yba_license_file` to pass the license without a local " +
					"file. Set exactly one of `yba_license_file` or `yba_license`.",
			},
			"application_settings_file": {
				Type:     schema.TypeString,
				Optional: true,
				// Change in this should trigger yba-ctl reconfigure
				ConflictsWith: []string{"application_settings"},
				Description: "Path to a local YBA Installer settings file (`yba-ctl.yml`) " +
					"that configures YugabyteDB Anywhere. If you set neither this field nor " +
					"`application_settings`, YBA Installer uses its default settings. " +
					"Conflicts with `application_settings`.",
			},
			"application_settings": {
				Type:      schema.TypeString,
				Optional:  true,
				Sensitive: true,
				// Change in this should trigger yba-ctl reconfigure
				ConflictsWith: []string{"application_settings_file"},
				Description: "Contents of the YBA Installer settings file (`yba-ctl.yml`) " +
					"that configures YugabyteDB Anywhere. If you set neither this field nor " +
					"`application_settings_file`, YBA Installer uses its default settings. " +
					"Conflicts with `application_settings_file`.",
			},
			"reconfigure": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
				// True should trigger yba-ctl reconfigure
				// if the contents of application_settings_file have been modified
				Description: "Change this field to `true` to run `yba-ctl reconfigure` on the " +
					"next apply, also when no other input changed. While it stays `true`, every " +
					"update of this resource also runs a reconfiguration. Requires " +
					"`application_settings` or `application_settings_file`. A change to " +
					"`application_settings`, `tls_certificate` or `tls_key`, or to the path " +
					"in their `_file` fields, starts a reconfiguration without this field.",
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
	// Version 0 state comes from v1.0.0 (no ssh_port) or from a pre-release
	// that already has it. The current schema covers both shapes.
	r.StateUpgraders = []schema.StateUpgrader{{
		Version: 0,
		Type:    r.CoreConfigSchema().ImpliedType(),
		Upgrade: upgradeYBAInstallerStateV0,
	}}
	return r
}

// upgradeYBAInstallerStateV0 sets ssh_port in state that does not have it.
// Without the value, every plan shows ssh_port changing from null to 22, and
// the apply connects to the host (and runs yba-ctl reconfigure when
// reconfigure = true).
func upgradeYBAInstallerStateV0(
	_ context.Context, rawState map[string]interface{}, _ interface{},
) (map[string]interface{}, error) {
	if rawState == nil {
		return rawState, nil
	}
	if v, ok := rawState["ssh_port"]; !ok || v == nil {
		rawState["ssh_port"] = defaultSSHPort
	}
	return rawState, nil
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
		// yba-ctl upgrade refuses a target version lower than the
		// installed one, but only at runtime on the host - by then the
		// failed apply has already burned time staging the bundle.
		// Reject the downgrade at plan time instead.
		func(ctx context.Context, d *schema.ResourceDiff, meta interface{}) error {
			if d.Id() == "" || !d.HasChange("yba_version") {
				return nil
			}
			old, new := d.GetChange("yba_version")
			return validateNoYBAVersionDowngrade(old.(string), new.(string))
		},
	)
}

// validateNoYBAVersionDowngrade rejects a yba_version change to a lower
// version. Version strings CompareYbVersions cannot parse (e.g. local
// builds) are left for yba-ctl's own version check at upgrade time.
func validateNoYBAVersionDowngrade(oldVersion, newVersion string) error {
	compare, err := utils.CompareYbVersions(newVersion, oldVersion)
	if err == nil && compare < 0 {
		return fmt.Errorf(
			"yba_version %s is lower than the installed version %s: yba-ctl "+
				"does not support downgrades", newVersion, oldVersion)
	}
	return nil
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
	sshPort := d.Get("ssh_port").(int)
	pk, err := resolveSSHPrivateKey(d)
	if err != nil {
		return diag.FromErr(err)
	}

	sshClient, err := waitForIP(
		ctx,
		user,
		hostIPForSSH,
		sshPort,
		pk,
		d.Timeout(schema.TimeoutCreate),
	)
	if err != nil {
		tflog.Error(ctx, "Timeout: Couldn't connect to YugabyteDB Anywhere host")
		return diag.FromErr(err)
	}
	defer func() { _ = sshClient.Close() }()

	if err := uploadInstallerInputs(
		ctx,
		sshClient,
		d,
		installationYBAInstallerSpecs(),
	); err != nil {
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

// installerUpdateRevertAttrs are the attributes resourceYBAInstallerUpdate
// applies to the host. On a failed update the SDK persists the planned
// values to state anyway, so they must be reverted to their pre-update
// values - otherwise the next plan shows no diff and the unapplied change
// is silently recorded as done.
var installerUpdateRevertAttrs = []string{
	"yba_version",
	"reconfigure",
	"yba_license", "yba_license_file",
	"application_settings", "application_settings_file",
	"tls_certificate", "tls_certificate_file",
	"tls_key", "tls_key_file",
}

func resourceYBAInstallerUpdate(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{}) (diags diag.Diagnostics) {
	// same steps as installation
	// run ./yba-ctl with upgrade instead of install

	defer func() {
		if diags.HasError() {
			utils.RevertFields(d, installerUpdateRevertAttrs...)
		}
	}()

	// Terraform plans an update with no attribute change when only the
	// sensitivity of an attribute changed: once after an upgrade from v1.0.0,
	// whose state does not mark the sensitive inputs added since. Such an
	// update must not touch the host. With reconfigure = true, it would
	// restart YBA.
	if !d.HasChangesExcept() {
		return diag.Diagnostics{}
	}

	hostIPForSSH := d.Get("ssh_host_ip").(string)
	user := d.Get("ssh_user").(string)
	sshPort := d.Get("ssh_port").(int)
	pk, err := resolveSSHPrivateKey(d)
	if err != nil {
		return diag.FromErr(err)
	}
	skipPreflightChecksList := utils.StringSlice(d.Get("skip_preflight_checks").([]interface{}))
	sshClient, err := waitForIP(
		ctx,
		user,
		hostIPForSSH,
		sshPort,
		pk,
		d.Timeout(schema.TimeoutCreate),
	)
	if err != nil {
		tflog.Error(ctx, "Timeout: Couldn't connect to YugabyteDB Anywhere host")
		return diag.FromErr(err)
	}
	defer func() { _ = sshClient.Close() }()

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
		folder, _, _ := getYBAInstallerPackageNames(oldVersion, hostOS, hostArch)
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
		commands = append(commands, getReconfigureCommands(oldVersion, hostOS, hostArch)...)
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
	sshPort := d.Get("ssh_port").(int)
	pk, err := resolveSSHPrivateKey(d)
	if err != nil {
		return diag.FromErr(err)
	}

	sshClient, err := connectSSHForDelete(ctx, user, hostIPForSSH, sshPort, pk)
	if err != nil {
		if errors.Is(err, errSSHHostUnreachable) {
			// Host gone (replace_triggered_by destroyed the VM first) or a tunnel
			// to nowhere: nothing to clean up.
			tflog.Warn(ctx, fmt.Sprintf(
				"yba_installer: host %s unreachable after retries, treating as "+
					"already destroyed: %v", hostIPForSSH, err))
			d.SetId("")
			return diags
		}
		return diag.FromErr(err)
	}
	defer func() { _ = sshClient.Close() }()

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

// getYBAInstallerPackageNames derives the package naming pieces for a version:
// the extracted folder name, the tarball base name, and the build-stripped
// version used in GA download paths.
func getYBAInstallerPackageNames(version, os, arch string) (folder, bundle, v string) {
	folder = fmt.Sprintf("yba_installer_full-%s", version)
	// Pre-release bundles are named "centos" rather than "linux".
	if !IsGAVersion(version) && os == "linux" {
		os = "centos"
	}
	bundle = fmt.Sprintf("%s-%s-%s", folder, os, arch)
	// Strip the build number ("-bNNN") to get the remote folder for GA releases.
	v = strings.Split(version, "-")[0]
	return folder, bundle, v
}

// getYBAInstallerDownloadURL returns the tarball URL for the given version, OS and
// arch. GA versions come from downloads.yugabyte.com under the build-stripped
// version path. Pre-release builds come from releases.yugabyte.com under the full
// version including the build number.
func getYBAInstallerDownloadURL(version, os, arch string) string {
	_, bundle, v := getYBAInstallerPackageNames(version, os, arch)
	if IsGAVersion(version) {
		return fmt.Sprintf("%s/%s/%s.tar.gz", GADownloadURL, v, bundle)
	}
	return fmt.Sprintf("%s/%s/%s.tar.gz", PreReleaseDownloadURL, version, bundle)
}

// getBundleDownloadCommands returns the extracted folder name and the commands
// that download and unpack the YBA Installer tarball.
func getBundleDownloadCommands(version, os, arch string) (string, []string) {
	folder, bundle, _ := getYBAInstallerPackageNames(version, os, arch)
	commands := []string{
		fmt.Sprintf("curl -O %s", getYBAInstallerDownloadURL(version, os, arch)),
		fmt.Sprintf("tar -xf %s.tar.gz", bundle),
	}
	return folder, commands
}

// ybaCtlSudo returns the shell prefix for invoking yba-ctl. Pre-release builds add
// YBA_MODE=dev to bypass the strict version checks that reject non-GA versions.
func ybaCtlSudo(version string) string {
	if IsGAVersion(version) {
		return "sudo"
	}
	return "sudo YBA_MODE=dev"
}

// getAddLicenseCommand returns the yba-ctl invocation that registers the license file.
func getAddLicenseCommand(version, folder string) string {
	return fmt.Sprintf("%s ./%s/yba-ctl license add -l /tmp/license.lic",
		ybaCtlSudo(version), folder)
}

// getInstallCommands stages and runs a fresh YBA install. The final command
// branches on /opt/yugabyte/data/yb-platform — not mere dir non-emptiness,
// since a fresh mkfs leaves lost+found — so a surviving data disk gets
// `install --without-data` + `start` instead of reinitialising storage.
func getInstallCommands(
	version, os, arch string,
	config bool, skipPreflightCheckList *[]string) []string {
	folder, cmds := getBundleDownloadCommands(version, os, arch)
	cmds = append(cmds, getAddLicenseCommand(version, folder))
	if config {
		// /opt/yba-ctl doesn't exist until `install` runs; mkdir so the mv lands.
		cmds = append(cmds,
			"sudo mkdir -p /opt/yba-ctl",
			"sudo mv /tmp/settings.yml /opt/yba-ctl/yba-ctl.yml")
	}
	installPrefix := fmt.Sprintf("%s ./%s/yba-ctl install -f", ybaCtlSudo(version), folder)
	ybactl := fmt.Sprintf("%s /opt/yba-ctl/yba-ctl", ybaCtlSudo(version))

	skipSuffix := ""
	if skipPreflightCheckList != nil && len(*skipPreflightCheckList) != 0 {
		skipSuffix = fmt.Sprintf(" -s %s", strings.Join(*skipPreflightCheckList, ","))
	}

	cmds = append(cmds, fmt.Sprintf(
		`if sudo test -d /opt/yugabyte/data/yb-platform; then `+
			`%s --without-data%s && %s start; `+
			`else %s%s; `+
			`fi`,
		installPrefix, skipSuffix, ybactl,
		installPrefix, skipSuffix,
	))
	return cmds
}

func getReconfigureCommands(version, os, arch string) []string {
	folder, downloadCmds := getBundleDownloadCommands(version, os, arch)
	// yba-ctl reconfigure globs perf_advisor-*.tar.gz relative to CWD, so it must
	// run from the extracted bundle dir. A reconfigure-only apply may not have it
	// (import, cleaned home, different SSH user), so fetch on demand.
	return []string{
		"sudo mv /tmp/settings.yml /opt/yba-ctl/yba-ctl.yml",
		fmt.Sprintf("[ -d ~/%s ] || ( %s )", folder, strings.Join(downloadCmds, " && ")),
		fmt.Sprintf(
			"cd ~/%s && %s /opt/yba-ctl/yba-ctl reconfigure -f",
			folder,
			ybaCtlSudo(version),
		),
	}
}

func getUpgradeCommands(version, os, arch string, skipPreflightCheckList *[]string) []string {
	folder, updateCommands := getBundleDownloadCommands(version, os, arch)
	s := fmt.Sprintf("%s ./%s/yba-ctl upgrade -f", ybaCtlSudo(version), folder)
	if skipPreflightCheckList != nil && len(*skipPreflightCheckList) != 0 {
		s = fmt.Sprintf("%s -s %s", s, strings.Join(*skipPreflightCheckList, ","))
	}
	updateCommands = append(updateCommands, s)
	return updateCommands
}

// getDeleteCommands runs `clean` without --all: /opt/yugabyte/data must outlive
// the host when it lives on a separate data disk; wiping it is the operator's job.
func getDeleteCommands(version string) []string {
	return []string{
		fmt.Sprintf("%s /opt/yba-ctl/yba-ctl clean", ybaCtlSudo(version)),
		"sudo rm -f /tmp/server.crt /tmp/server.key /tmp/license.lic /tmp/settings.yml",
	}
}
