package installation

import (
	"bytes"
	"context"
	"fmt"
	"github.com/bramvdbogaerde/go-scp"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"golang.org/x/crypto/ssh"
	"net"
	"os"
)

var (
	installationFiles = map[string]string{
		"replicated_config_file":    "/tmp/replicated.conf",
		"tls_certificate_file":      "/tmp/server.crt",
		"tls_key_file":              "/tmp/server.key",
		"replicated_license_file":   "/tmp/license.rli",
		"application_settings_file": "/tmp/settings.conf",
	}
	installationCommands = []string{
		"sudo mv /tmp/replicated.conf /etc/replicated.conf",
		"curl -sSL https://get.replicated.com/docker | sudo bash",
	}
	deletionCommands = []string{
		// remove yugabyte resources
		"/usr/local/bin/replicated apps | grep \"yugaware\" | awk '{print $1} | xargs /usr/local/bin/replicated app stop",
		"sudo docker images | grep \"yuga\" | awk '{print $3}' | sudo xargs docker rmi -f",
		"sudo rm -rf /opt/yugabyte",
		"sudo rm /etc/replicated.conf /tmp/replicated.conf /tmp/server.crt /tmp/server.key /tmp/license/rli /tmp/settings.conf",
		// remove replicated resources
		"sudo service replicated stop",
		"sudo service replicated-ui stop",
		"sudo service replicated-operator stop",
		"sudo docker stop replicated-premkit",
		"sudo docker stop replicated-statsd",
		"sudo docker rm -f replicated replicated-ui replicated-operator replicated-premkit replicated-statsd retraced-api retraced-processor retraced-cron retraced-nsqd retraced-postgres",
		"sudo docker images | grep \"quay\\.io/replicated\" | awk '{print $3}' | xargs sudo docker rmi -f",
		"sudo docker images | grep \"registry\\.replicated\\.com/library/retraced\" | awk '{print $3}' | sudo xargs docker rmi -f",
		"sudo apt-get remove -y replicated replicated-ui replicated-operator",
		"sudo apt-get purge -y replicated replicated-ui replicated-operator",
		"sudo rm -rf /var/lib/replicated* /etc/replicated* /etc/init/replicated* /etc/init.d/replicated* /etc/default/replicated* /var/log/upstart/replicated* /etc/systemd/system/replicated*",
	}
)

func ResourceInstallation() *schema.Resource {
	return &schema.Resource{
		Description: "Manages the installation of YugabyteDB Anywhere on an existing virtual machine. This resource does not track the remote state and is only provided as a convenience tool. To reinstall, taint this resource and re-apply.",

		CreateContext: resourceInstallationCreate,
		ReadContext:   resourceInstallationRead,
		UpdateContext: resourceInstallationUpdate,
		DeleteContext: resourceInstallationDelete,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Schema: map[string]*schema.Schema{
			"public_ip": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Public ip of the existing virtual machine",
			},
			"ssh_private_key": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Contents of file containing the private key to use for ssh commands",
			},
			"ssh_user": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "User to use for ssh commands",
			},
			"replicated_config_file": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Configuration file to use for automated installation using Replicated",
			},
			"tls_certificate_file": {
				Type:         schema.TypeString,
				Optional:     true,
				RequiredWith: []string{"tls_key_file"},
				Description:  "TLS certificate used to configure HTTPS",
			},
			"tls_key_file": {
				Type:         schema.TypeString,
				Optional:     true,
				RequiredWith: []string{"tls_certificate_file"},
				Description:  "TLS key used to configure HTTPS",
			},
			"replicated_license_file": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "YugabyteDB Anywhere license file used for installation using Replicated",
			},
			"application_settings_file": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Application settings file to configure YugabyteDB Anywhere",
			},
			"cleanup": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "Flag for indicating if resources should be cleaned up during the uninstall. Set this to true if you plan to reuse the virtual machine.",
			},
		},
	}
}

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
	client, err := ssh.Dial("tcp", net.JoinHostPort(ip, "22"), config)
	if err != nil {
		return nil, err
	}
	return client, nil
}

func runCommand(ctx context.Context, client *ssh.Client, cmd string) error {
	tflog.Info(ctx, fmt.Sprintf("running command %s", cmd))
	session, err := client.NewSession()
	if err != nil {
		return err
	}
	var b bytes.Buffer
	session.Stdout = &b
	defer session.Close()
	err = session.Run(cmd)
	tflog.Info(ctx, b.String())
	return err
}

func scpFile(ctx context.Context, sshClient *ssh.Client, localFile string, remoteFile string) error {
	tflog.Info(ctx, fmt.Sprintf("copying %s to %s", localFile, remoteFile))

	client, err := scp.NewClientBySSH(sshClient)
	if err != nil {
		return err
	}
	defer client.Close()

	err = client.Connect()
	if err != nil {
		return err
	}
	defer client.Close()

	f, _ := os.Open(localFile)
	defer f.Close()

	err = client.CopyFromFile(context.Background(), *f, remoteFile, "0666")
	return err
}

func resourceInstallationCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	ip := d.Get("public_ip").(string)
	user := d.Get("ssh_user").(string)
	pk := d.Get("ssh_private_key").(string)

	sshClient, err := newSSHClient(user, ip, pk)
	if err != nil {
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
			return diag.FromErr(err)
		}
	}

	for _, cmd := range installationCommands {
		err = runCommand(ctx, sshClient, cmd)
		if err != nil {
			return diag.FromErr(err)
		}
	}

	d.SetId(uuid.New().String())
	return diags
}

func resourceInstallationRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	// remote state is not read for this resource
	return diag.Diagnostics{}
}

func resourceInstallationUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	// do nothing
	return diag.Diagnostics{}
}

func resourceInstallationDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
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
		err = runCommand(ctx, sshClient, cmd)
		if err != nil {
			return diag.FromErr(err)
		}
	}

	d.SetId("")
	return diags
}
