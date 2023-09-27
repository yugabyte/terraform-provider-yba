resource "yba_onprem_node_instance" "onprem" {
    provider_uuid = "<onprem-provider-uuid>"
    instance_type = "<instance-type-name>"
    ip            = "<node-ip-instance>"
    region        = "<region-name>"
    zone          = "<zone-name>" 
}

resource "yba_onprem_node_instance" "onprem" {
    provider_name = "<onprem-provider-name>"
    instance_type = "<instance-type-name>"
    ip            = "<node-ip-instance>"
    region        = "<region-name>"
    zone          = "<zone-name>" 
}
