resource "yb_cloud_provider" "cloud_provider" {
    code        = "<code>"
    dest_vpc_id = "<vpc network>"
    name        = "<cloud-provider-name>"
    regions {
        code = "<region-code>"
        name = "<region-name>"
    }
    ssh_port        = 22
    air_gap_install = false
}