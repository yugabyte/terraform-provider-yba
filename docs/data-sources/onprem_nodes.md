---
page_title: "yba_onprem_nodes Data Source - YugabyteDB Anywhere"
description: |-
  Filter list of nodes handled in the onprem provider.
---

# yba_onprem_nodes (Data Source)

Filter list of nodes handled in the onprem provider.

<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `provider_id` (String) UUID of the onprem provider.

### Optional

- `in_use` (Boolean) Nodes of the on premises provider used in a universe.
- `instance_name` (String) Nodes with instance names containing given filter string.
- `instance_type` (String) Nodes with instance types containing given filter string.
- `ip` (String) Nodes with IP addresses containing given filter string. For example, setting ip = "10.1.2" may return nodes with IPs "10.1.20.3" and "10.10.1.23".
- `region` (String) Nodes in a particular region.
- `zone` (String) Nodes in a particular zone.

### Read-Only

- `id` (String) The ID of this resource.
- `nodes` (List of Object) Node instances associated with the provider and given filters. (see [below for nested schema](#nestedatt--nodes))

<a id="nestedatt--nodes"></a>
### Nested Schema for `nodes`

Read-Only:

- `details_json` (String)
- `in_use` (Boolean)
- `instance_name` (String)
- `instance_type` (String)
- `instance_type_code` (String)
- `ip` (String)
- `node_configs` (List of Object) (see [below for nested schema](#nestedobjatt--nodes--node_configs))
- `node_name` (String)
- `node_uuid` (String)
- `region` (String)
- `ssh_user` (String)
- `zone` (String)
- `zone_uuid` (String)

<a id="nestedobjatt--nodes--node_configs"></a>
### Nested Schema for `nodes.node_configs`

Read-Only:

- `type` (String)
- `value` (String)