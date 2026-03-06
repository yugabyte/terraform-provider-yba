// Licensed to YugabyteDB, Inc. under one or more contributor license
// agreements. See the NOTICE file distributed with this work for
// additional information regarding copyright ownership. Yugabyte
// licenses this file to you under the Mozilla License, Version 2.0
// (the "License"); you may not use this file except in compliance
// with the License.  You may obtain a copy of the License at
// http://mozilla.org/MPL/2.0/.
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package universe

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/yugabyte/terraform-provider-yba/internal/api"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// DataSourceUniverseSchema returns the schema for listing namespaces and tables
func DataSourceUniverseSchema() *schema.Resource {
	return &schema.Resource{
		Description: "Discover the schema (namespaces and tables) of a universe. " +
			"Use this data source to find database/keyspace names and table UUIDs for backups.",

		ReadContext: dataSourceUniverseSchemaRead,

		Schema: map[string]*schema.Schema{
			"universe_uuid": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "The UUID of the universe to inspect.",
			},
			"include_system_namespaces": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     false,
				Description: "Include system namespaces in the results. Default: false.",
			},
			"include_tables": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
				Description: "Include tables in the results. Default: false. " +
					"Enable this only when you need table UUIDs for table-level backups.",
			},
			"table_type": {
				Type:     schema.TypeString,
				Optional: true,
				ValidateDiagFunc: validation.ToDiagFunc(validation.StringInSlice(
					[]string{"YQL_TABLE_TYPE", "PGSQL_TABLE_TYPE"}, false)),
				Description: "Filter by table type. " +
					"Allowed values: YQL_TABLE_TYPE (YCQL), PGSQL_TABLE_TYPE (YSQL). " +
					"If not specified, returns all types.",
			},

			// Computed outputs
			"namespaces": {
				Type:        schema.TypeList,
				Computed:    true,
				Description: "List of namespaces (databases/keyspaces) in the universe.",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"name": {
							Type:        schema.TypeString,
							Computed:    true,
							Description: "Name of the namespace (database/keyspace).",
						},
						"namespace_uuid": {
							Type:        schema.TypeString,
							Computed:    true,
							Description: "UUID of the namespace.",
						},
						"table_type": {
							Type:        schema.TypeString,
							Computed:    true,
							Description: "Type of tables in this namespace (YQL_TABLE_TYPE or PGSQL_TABLE_TYPE).",
						},
					},
				},
			},
			"tables": {
				Type:     schema.TypeList,
				Computed: true,
				Description: "List of tables in the universe. " +
					"Only populated when include_tables is true.",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"table_uuid": {
							Type:        schema.TypeString,
							Computed:    true,
							Description: "UUID of the table. Use this for table-level backups.",
						},
						"table_name": {
							Type:        schema.TypeString,
							Computed:    true,
							Description: "Name of the table.",
						},
						"keyspace": {
							Type:        schema.TypeString,
							Computed:    true,
							Description: "Keyspace/database the table belongs to.",
						},
						"table_type": {
							Type:        schema.TypeString,
							Computed:    true,
							Description: "Type of the table.",
						},
						"size_bytes": {
							Type:        schema.TypeFloat,
							Computed:    true,
							Description: "Size of the table in bytes.",
						},
						"pg_schema_name": {
							Type:        schema.TypeString,
							Computed:    true,
							Description: "PostgreSQL schema name (for YSQL tables).",
						},
						"colocated": {
							Type:        schema.TypeBool,
							Computed:    true,
							Description: "Whether the table is colocated.",
						},
					},
				},
			},

			// Convenience outputs for direct use
			"namespace_names": {
				Type:        schema.TypeList,
				Computed:    true,
				Elem:        &schema.Schema{Type: schema.TypeString},
				Description: "List of namespace names. Convenient for use with backup keyspaces argument.",
			},
			"ysql_database_names": {
				Type:        schema.TypeList,
				Computed:    true,
				Elem:        &schema.Schema{Type: schema.TypeString},
				Description: "List of YSQL database names only.",
			},
			"ycql_keyspace_names": {
				Type:        schema.TypeList,
				Computed:    true,
				Elem:        &schema.Schema{Type: schema.TypeString},
				Description: "List of YCQL keyspace names only.",
			},
		},
	}
}

func dataSourceUniverseSchemaRead(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics
	c := meta.(*api.APIClient).YugawareClient
	cUUID := meta.(*api.APIClient).CustomerID
	uUUID := d.Get("universe_uuid").(string)
	includeSystem := d.Get("include_system_namespaces").(bool)
	includeTables := d.Get("include_tables").(bool)
	tableTypeFilter := d.Get("table_type").(string)

	// Fetch namespaces
	namespaces, response, err := c.TableManagementAPI.GetAllNamespaces(ctx, cUUID, uUUID).
		IncludeSystemNamespaces(includeSystem).Execute()
	if err != nil {
		errMessage := utils.ErrorFromHTTPResponse(response, err, utils.DataSourceEntity,
			"Universe Schema", "Read")
		return diag.FromErr(errMessage)
	}

	// Build the namespace lists
	namespaceList := make([]map[string]interface{}, 0)
	namespaceNames := make([]string, 0)
	ysqlDatabases := make([]string, 0)
	ycqlKeyspaces := make([]string, 0)

	for _, ns := range namespaces {
		// Apply table type filter if specified
		if tableTypeFilter != "" && ns.GetTableType() != tableTypeFilter {
			continue
		}

		namespaceMap := map[string]interface{}{
			"name":           ns.GetName(),
			"namespace_uuid": ns.GetNamespaceUUID(),
			"table_type":     ns.GetTableType(),
		}
		namespaceList = append(namespaceList, namespaceMap)
		namespaceNames = append(namespaceNames, ns.GetName())

		// Categorize by type
		switch ns.GetTableType() {
		case "PGSQL_TABLE_TYPE":
			ysqlDatabases = append(ysqlDatabases, ns.GetName())
		case "YQL_TABLE_TYPE":
			ycqlKeyspaces = append(ycqlKeyspaces, ns.GetName())
		}
	}

	if err := d.Set("namespaces", namespaceList); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("namespace_names", namespaceNames); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("ysql_database_names", ysqlDatabases); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("ycql_keyspace_names", ycqlKeyspaces); err != nil {
		return diag.FromErr(err)
	}

	// Fetch tables only if requested
	tableList := make([]map[string]interface{}, 0)
	if includeTables {
		tables, response, err := c.TableManagementAPI.GetAllTables(ctx, cUUID, uUUID).Execute()
		if err != nil {
			errMessage := utils.ErrorFromHTTPResponse(response, err, utils.DataSourceEntity,
				"Universe Schema", "Read")
			return diag.FromErr(errMessage)
		}

		for _, t := range tables {
			// Apply table type filter if specified
			if tableTypeFilter != "" && t.GetTableType() != tableTypeFilter {
				continue
			}

			tableMap := map[string]interface{}{
				"table_uuid":     t.GetTableUUID(),
				"table_name":     t.GetTableName(),
				"keyspace":       t.GetKeySpace(),
				"table_type":     t.GetTableType(),
				"size_bytes":     t.GetSizeBytes(),
				"pg_schema_name": t.GetPgSchemaName(),
				"colocated":      t.GetColocated(),
			}
			tableList = append(tableList, tableMap)
		}
	}

	if err := d.Set("tables", tableList); err != nil {
		return diag.FromErr(err)
	}

	d.SetId(uUUID)
	return diags
}
