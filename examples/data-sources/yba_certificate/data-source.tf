# Look up a certificate configuration by label. This example finds the root
# certificate that YugabyteDB Anywhere generated for a universe created with
# encryption in transit and no certificate set. YugabyteDB Anywhere labels that
# certificate with the universe's node prefix, yb-<customer code>-<universe name>,
# and adds a ~1, ~2, ... suffix when another self-signed certificate label starts
# with that prefix. A separate client-to-node root certificate gets the label
# <node prefix>-client.
data "yba_certificate" "auto_generated" {
  label = "yb-admin-orders"
}

output "auto_generated_cert_uuid" {
  value = data.yba_certificate.auto_generated.uuid
}

output "auto_generated_cert_expiry" {
  value = data.yba_certificate.auto_generated.expiry_date
}
