# Mint mode: YugabyteDB Anywhere generates the root certificate and holds its
# private key (4-year root, 1-year server certificates by platform default).
# The generated CA is exported through the `certificate` attribute for
# distribution to clients.
resource "yba_self_signed_certificate" "minted" {
  label = "prod-n2n-ca"

  # The universe rotates to a replacement before the old configuration is
  # deleted; YBA refuses to delete certificates that are still in use.
  lifecycle {
    create_before_destroy = true
  }
}

# Bring-your-own mode: provide the root certificate and its private key, from
# files or inline. YugabyteDB Anywhere signs the per-node server certificates
# with this key.
resource "yba_self_signed_certificate" "byo" {
  label       = "prod-byo-ca"
  certificate = file("${path.module}/ca.crt")
  private_key = file("${path.module}/ca.key")

  lifecycle {
    create_before_destroy = true
  }
}
