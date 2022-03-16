package installation

import (
	"bytes"
	"context"
	"github.com/bramvdbogaerde/go-scp"
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
		"docker images | grep \"yuga\" | awk '{print $3}' | xargs docker rmi -f",
		"rm -rf /opt/yugabyte",
		"rm /etc/replicated.conf /tmp/replicated.conf /tmp/server.crt /tmp/server.key /tmp/license/rli /tmp/settings.conf",
		// remove replicated resources
		"service replicated stop",
		"service replicated-ui stop",
		"service replicated-operator stop",
		"docker stop replicated-premkit",
		"docker stop replicated-statsd",
		"docker rm -f replicated replicated-ui replicated-operator replicated-premkit replicated-statsd retraced-api retraced-processor retraced-cron retraced-nsqd retraced-postgres",
		"docker images | grep \"quay\\.io/replicated\" | awk '{print $3}' | xargs sudo docker rmi -f",
		"docker images | grep \"registry\\.replicated\\.com/library/retraced\" | awk '{print $3}' | xargs sudo docker rmi -f",
		"apt-get remove -y replicated replicated-ui replicated-operator",
		"apt-get purge -y replicated replicated-ui replicated-operator",
		"rm -rf /var/lib/replicated* /etc/replicated* /etc/init/replicated* /etc/init.d/replicated* /etc/default/replicated* /var/log/upstart/replicated* /etc/systemd/system/replicated*",
	}
)

func ResourceInstallation() *schema.Resource {
	return &schema.Resource{
		Description: "Manages the installation of YugabyteDB Anywhere on an existing virtual machine",

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
				ForceNew:    true,
				Description: "Configuration file to use for automated installation using Replicated",
			},
			"tls_certificate_file": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "TLS certificate used to configure HTTPS",
			},
			"tls_key_file": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "TLS key used to configure HTTPS",
			},
			"replicated_license_file": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "YugabyteDB Anywhere license file used for installation using Replicated",
			},
			"application_settings_file": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "Application settings file to configure YugabyteDB Anywhere",
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

func runCommand(client *ssh.Client, cmd string) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()
	var b bytes.Buffer
	session.Stdout = &b
	err = session.Run(cmd)
	return b.String(), err
}

func scpFile(client *scp.Client, localFile string, remoteFile string) error {
	err := client.Connect()
	if err != nil {
		return err
	}
	f, _ := os.Open(localFile)
	defer client.Close()
	defer f.Close()

	err = client.CopyFromFile(context.Background(), *f, remoteFile, "0666")
	return err
}

func resourceInstallationCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	ip := d.Get("public_ip").(string)
	user := d.Get("user").(string)
	pk := d.Get("ssh_private_key").(string)

	sshClient, err := newSSHClient(user, ip, pk)
	if err != nil {
		return diag.FromErr(err)
	}
	scpClient, err := scp.NewClientBySSH(sshClient)
	if err != nil {
		return diag.FromErr(err)
	}

	for key, remote := range installationFiles {
		local := d.Get(key).(string)
		if local == "" {
			continue
		}
		err = scpFile(&scpClient, local, remote)
		if err != nil {
			return diag.FromErr(err)
		}
	}

	for _, cmd := range installationCommands {
		_, err = runCommand(sshClient, cmd)
		if err != nil {
			return diag.FromErr(err)
		}
	}
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

	ip := d.Get("public_ip").(string)
	user := d.Get("user").(string)
	pk := d.Get("ssh_private_key").(string)

	sshClient, err := newSSHClient(user, ip, pk)
	if err != nil {
		return diag.FromErr(err)
	}

	for _, cmd := range deletionCommands {
		_, err = runCommand(sshClient, cmd)
		if err != nil {
			return diag.FromErr(err)
		}
	}
	return diags
}
