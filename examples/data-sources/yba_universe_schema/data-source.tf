// List all user-created namespaces (databases and keyspaces) in a universe.
data "yba_universe_schema" "namespaces" {
  universe_uuid = "<universe-uuid>"
}

// Include tables and filter to YSQL only, for table-level backup configuration.
data "yba_universe_schema" "ysql_tables" {
  universe_uuid             = "<universe-uuid>"
  table_type                = "PGSQL_TABLE_TYPE"
  include_tables            = true
  include_system_namespaces = false
}
