resource "yb_releases" "s3_ybdb release" {
  version = "<YBDB version>"
  s3 {
    paths {
      x86_64 = "<path-to-ybdb-release-.tar-file-in-s3>"
    }
  }
}

resource "yb_releases" "gcs_ybdb_release" {
  version = "<YBDB version>"
  gcs {
    paths {
      x86_64 = "<path-to-ybdb-release-.tar-file-in-gcs>"
    }
  }
}

resource "yb_releases" "http_ybdb_release" {
  version = "<YBDB version>"
  http {
    paths {
      x86_64          = "<path-to-tar-file>"
      x86_64_checksum = "<checksum>"
    }
  }
}