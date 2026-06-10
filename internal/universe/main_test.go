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

package universe_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/yugabyte/terraform-provider-yba/internal/acctest"
)

// TestMain materializes the GCP SA key to a GOOGLE_APPLICATION_CREDENTIALS file
// for the whole package run and removes it afterward: the GCP universe test's
// yba_cloud_provider reads the GCP key from that file (not inline), and doing it
// here keeps the file alive across the package's parallel tests, then deletes it
// once they finish. Mirrors internal/cloud_provider/main_test.go.
func TestMain(m *testing.M) {
	cleanup, err := acctest.SetupGCPCredentialsFile()
	if err != nil {
		fmt.Fprintln(os.Stderr, "acctest: setting up GCP credentials file:", err)
		os.Exit(1)
	}
	code := m.Run()
	cleanup()
	os.Exit(code)
}
