data "yba_release_version" "release_version" {
  // To fetch default YBDB version
}

data "yba_release_version" "release_version_x" {
  // To fetch particular version
  version = "<YBDB-version-x>"
}
