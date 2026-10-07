# Generated mode: YugabyteDB Anywhere generates the root certificate and keeps
# its private key. By default, the root certificate is valid for 4 years and the
# server certificates for 1 year. The `certificate` attribute holds the
# generated root certificate, for you to distribute to clients.
resource "yba_self_signed_certificate" "generated" {
  label = "prod-n2n-ca"

  # Create the replacement and rotate the universe to it before the old
  # configuration is deleted: YugabyteDB Anywhere does not delete a certificate
  # that a universe uses.
  lifecycle {
    create_before_destroy = true
  }
}

# Bring-your-own mode: set the root certificate and its private key, from files
# or inline. YugabyteDB Anywhere signs the server certificate of each node with
# this key.
resource "yba_self_signed_certificate" "byo" {
  label       = "prod-byo-ca"
  certificate = file("${path.module}/ca.crt")
  private_key = file("${path.module}/ca.key")

  lifecycle {
    create_before_destroy = true
  }
}
