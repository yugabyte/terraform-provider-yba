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

package backups

// Shared CustomizeDiff validators used by both yba_backup and yba_backup_schedule.
//
// Descriptions for fields shared across both resources are also defined here as
// package-level constants so that any wording change is applied consistently.

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/yugabyte/terraform-provider-yba/internal/utils"
)

// Shared field descriptions for fields that appear in both resources.
const (
	descKeyspaces = "List of keyspaces (YCQL) or databases (YSQL) to back up. " +
		"If empty or not specified, a full universe backup of all databases/keyspaces is taken. " +
		"For YSQL each entry is a database name; for YCQL each entry is a keyspace name."

	descTableUUIDList = "List of specific table UUIDs to back up. " +
		"Allowed for YQL_TABLE_TYPE (YCQL) and REDIS_TABLE_TYPE; " +
		"requires exactly one keyspace in 'keyspaces'."

	descBackupType = "Type of tables to back up. " +
		"Allowed values: YQL_TABLE_TYPE (YCQL), REDIS_TABLE_TYPE, PGSQL_TABLE_TYPE (YSQL)."

	descStorageConfigUUID = "UUID of the storage configuration to use. " +
		"Can be retrieved from the yba_storage_configs data source."

	descKMSConfigUUID = "KMS configuration UUID for encrypted backups."

	descSSE = "Enable server-side encryption for backups. Defaults to false."

	descParallelism = "Number of concurrent commands to run on nodes over SSH."

	descTableByTableBackup = "Take table-by-table backups."

	descUseTablespaces = "Include tablespace information in the backup. " +
		"Allowed for PGSQL_TABLE_TYPE (YSQL) backups only."

	descUseRoles = "Backup global YSQL roles and grants. " +
		"Allowed for PGSQL_TABLE_TYPE (YSQL) backups only."

	descTimeBeforeDelete = "Time before the backup is deleted from storage. " +
		"Accepts string duration in the standard format <https://pkg.go.dev/time#Duration>. " +
		"Backups are kept indefinitely if not set."
)

// validateTableUUIDListDiff returns a CustomizeDiffFunc that enforces:
//   - table_uuid_list requires exactly one keyspace
//   - table_uuid_list is not supported for PGSQL_TABLE_TYPE
func validateTableUUIDListDiff() schema.CustomizeDiffFunc {
	return func(ctx context.Context, d *schema.ResourceDiff, meta interface{}) error {
		tableUUIDs := d.Get("table_uuid_list").([]interface{})
		if len(tableUUIDs) == 0 {
			return nil
		}
		keyspaces := d.Get("keyspaces").([]interface{})
		if len(keyspaces) != 1 {
			return errors.New(
				"table_uuid_list is only valid when exactly one keyspace is specified",
			)
		}
		if d.Get("backup_type").(string) == "PGSQL_TABLE_TYPE" {
			return errors.New(
				"table_uuid_list is not supported for backup_type PGSQL_TABLE_TYPE",
			)
		}
		return nil
	}
}

// validateIncrementalFrequencyDiff returns a CustomizeDiffFunc that enforces:
//   - incremental_backup_frequency must not exceed frequency (full backup interval)
//
// The individual upper-bound checks (freq >= 1h, incr_freq <= 1d) are handled by
// separate customdiff.ValidateValue calls; this function only covers the cross-field
// relationship so that terraform plan can surface the error before apply.
func validateIncrementalFrequencyDiff() schema.CustomizeDiffFunc {
	return func(ctx context.Context, d *schema.ResourceDiff, meta interface{}) error {
		freqStr := d.Get("frequency").(string)
		incrFreqStr := d.Get("incremental_backup_frequency").(string)

		if freqStr == "" || incrFreqStr == "" {
			return nil
		}

		fullMs, _, _, err := utils.GetMsFromDurationString(freqStr)
		if err != nil {
			// The individual frequency validator will report the format error.
			return nil
		}
		incrMs, _, _, err := utils.GetMsFromDurationString(incrFreqStr)
		if err != nil {
			// The individual incremental frequency validator will report this.
			return nil
		}

		if incrMs > fullMs {
			return errors.New(
				"incremental_backup_frequency cannot be greater than frequency " +
					"(the full backup interval)",
			)
		}
		return nil
	}
}

// validateYSQLOnlyFieldsDiff returns a CustomizeDiffFunc that enforces:
//   - use_tablespaces must not be true for non-PGSQL_TABLE_TYPE backups
//   - use_roles must not be true for non-PGSQL_TABLE_TYPE backups
func validateYSQLOnlyFieldsDiff() schema.CustomizeDiffFunc {
	return func(ctx context.Context, d *schema.ResourceDiff, meta interface{}) error {
		backupType := d.Get("backup_type").(string)
		if backupType == "PGSQL_TABLE_TYPE" || backupType == "" {
			return nil
		}
		if d.Get("use_tablespaces").(bool) {
			return fmt.Errorf(
				"use_tablespaces is only supported for PGSQL_TABLE_TYPE, got %s",
				backupType,
			)
		}
		if d.Get("use_roles").(bool) {
			return fmt.Errorf(
				"use_roles is only supported for PGSQL_TABLE_TYPE, got %s",
				backupType,
			)
		}
		return nil
	}
}
