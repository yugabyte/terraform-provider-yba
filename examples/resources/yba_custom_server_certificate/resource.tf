# Custom server certificate for client-to-node TLS: your organization's CA
# certificate plus a server certificate/key it signed. Reference it from the
# universe's `client_root_ca` — YugabyteDB Anywhere rejects this type for
# node-to-node (`root_ca`) use.
resource "yba_custom_server_certificate" "c2n" {
  label              = "prod-c2n-2026"
  root_certificate   = file("${path.module}/org-ca.crt")
  server_certificate = file("${path.module}/server.crt")
  server_key         = file("${path.module}/server.key")

  # Rotation pattern: when the org re-issues the server certificate from the
  # same CA, change label/server_certificate/server_key (or create a new
  # resource) and repoint the universe's client_root_ca. With
  # create_before_destroy, Terraform uploads the replacement, rotates the
  # universe, then deletes this configuration.
  lifecycle {
    create_before_destroy = true
  }
}
