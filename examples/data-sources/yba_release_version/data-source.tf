data "yba_release_version" "release_version" {
  // To fetch default YBDB version
}

data "yba_release_version" "release_version_x" {
  // Retrieve a list of YBDB versions that start with this prefix.
  version = "2024.2"
}

data "yba_release_version" "release_version_aarch64" {
  // Retrieve the latest stable version that ships an aarch64 artifact.
  track           = "stable"
  deployment_type = "aarch64"
}
