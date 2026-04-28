// List all image bundles for a provider.
data "yba_provider_image_bundles" "all_bundles" {
  provider_id = yba_aws_provider.aws.id
}

// Retrieve only the default x86_64 image bundle.
data "yba_provider_image_bundles" "default_x86" {
  provider_id  = yba_aws_provider.aws.id
  arch         = "x86_64"
  default_only = true
}

// Find a bundle by name (case-insensitive substring match).
data "yba_provider_image_bundles" "named_bundle" {
  provider_id = yba_aws_provider.aws.id
  name        = "<bundle-name>"
}
