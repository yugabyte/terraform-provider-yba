data "yb_release_version" "release_version" {
    // To fetch default YBDB version
}

data "yb_release_version" "release_version_x" {
    // To fetch particular version
    version ="<YBDB version x>"
}