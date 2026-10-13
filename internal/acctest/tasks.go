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

package acctest

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

// UniverseTaskWatch checks that the steps after Mark run no YBA task on the
// universe of Resource, the address of a yba_universe such as
// yba_universe.gcp. Use Mark and NoNewTask as step checks.
type UniverseTaskWatch struct {
	Resource string
	last     string
}

// Mark records the newest task of the universe.
func (w *UniverseTaskWatch) Mark(s *terraform.State) (err error) {
	w.last, err = w.latestTask(s)
	return err
}

// NoNewTask fails when the universe has a task newer than the one that Mark
// recorded.
func (w *UniverseTaskWatch) NoNewTask(s *terraform.State) error {
	latest, err := w.latestTask(s)
	if err == nil && latest != w.last {
		err = fmt.Errorf("task %s ran on %s, want none", latest, w.Resource)
	}
	return err
}

func (w *UniverseTaskWatch) latestTask(s *terraform.State) (string, error) {
	rs, ok := s.RootModule().Resources[w.Resource]
	if !ok {
		return "", fmt.Errorf("%s is not in the state", w.Resource)
	}
	tasks, _, err := APIClient.YugawareClient.CustomerTasksAPI.
		TasksList(context.Background(), APIClient.CustomerID).UUUID(rs.Primary.ID).Execute()
	if err != nil {
		return "", fmt.Errorf("listing tasks of universe %s: %w", rs.Primary.ID, err)
	}
	latest, at := "", time.Time{}
	for _, task := range tasks {
		if task.GetCreateTime().After(at) {
			latest, at = task.GetId(), task.GetCreateTime()
		}
	}
	return latest, nil
}
