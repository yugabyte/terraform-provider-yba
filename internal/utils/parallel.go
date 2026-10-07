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

package utils

import (
	"errors"
	"sync"
)

// UniverseParallelism bounds how many universes RunParallel works on at once,
// so a customer with many universes does not flood YBA with requests and
// tasks.
const UniverseParallelism = 8

// RunParallel calls fn(i) for every i in [0, n), at most limit calls at a
// time, and waits for all of them. A failed call does not stop the others:
// operations on different universes are independent, and a cancelled wait
// would leave a YBA task running unobserved. It returns every error, joined
// in index order. Callers write per-item results into a slice indexed by i.
func RunParallel(n, limit int, fn func(i int) error) error {
	errs := make([]error, n)
	sem := make(chan struct{}, max(limit, 1))
	var wg sync.WaitGroup
	for i := range n {
		sem <- struct{}{}
		wg.Go(func() {
			defer func() { <-sem }()
			errs[i] = fn(i)
		})
	}
	wg.Wait()
	return errors.Join(errs...)
}
