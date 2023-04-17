resource "yb_releases" "new_s3" {
    version = "<YBDB version>"
    s3 {
        paths {
            x86_64 = "<s3-bucket-path>"
        }
    }
}