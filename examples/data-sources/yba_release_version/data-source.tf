data "yba_release_version" "release_version" {
  // To fetch default YBDB version
}

data "yba_release_version" "release_version_x" {
  // Retrieve a list of YBDB versions corresponding to the pattern string.
  version = "<YBDB-version-string-to-be-matched>"
}
