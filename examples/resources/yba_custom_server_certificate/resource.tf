# Custom server certificate for client-to-node encryption: your organization's
# CA certificate, and a server certificate and key signed by that CA. Use it as
# a universe's `client_root_ca`. YugabyteDB Anywhere rejects this type as
# `root_ca` (node-to-node encryption).
resource "yba_custom_server_certificate" "c2n" {
  label              = "prod-c2n-2026"
  root_certificate   = file("${path.module}/org-ca.crt")
  server_certificate = file("${path.module}/server.crt")
  server_key         = file("${path.module}/server.key")

  # To rotate to a server certificate that the same CA re-issued, change
  # `label`, `server_certificate` and `server_key`. With create_before_destroy,
  # Terraform uploads the replacement, rotates the universe to it, and then
  # deletes this configuration.
  lifecycle {
    create_before_destroy = true
  }
}
