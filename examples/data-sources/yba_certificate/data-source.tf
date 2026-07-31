# Look up a certificate configuration by label — for example the self-signed
# root certificate YugabyteDB Anywhere generated automatically for a universe
# created with encryption enabled and no certificate set (labeled after the
# universe's node prefix).
data "yba_certificate" "auto_generated" {
  label = "yb-prod-universe"
}

output "auto_generated_cert_uuid" {
  value = data.yba_certificate.auto_generated.uuid
}

output "auto_generated_cert_expiry" {
  value = data.yba_certificate.auto_generated.expiry_date
}
