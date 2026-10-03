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

package releases

import "time"

// Create/Update may stream multi-GB release tarballs to the YBA node and then
// wait while YBA synchronously hashes and untars each one, hence 3h.
const (
	releaseCreateTimeout = 3 * time.Hour
	releaseUpdateTimeout = 3 * time.Hour
	releaseDeleteTimeout = 1 * time.Hour
)
