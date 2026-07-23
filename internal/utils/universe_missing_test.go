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
	"fmt"
	"net/http"
	"testing"
)

func TestIsUniverseMissing(t *testing.T) {
	cases := []struct {
		name string
		resp *http.Response
		err  error
		want bool
	}{
		{"nil response and error", nil, nil, false},
		{"404", &http.Response{StatusCode: http.StatusNotFound}, errors.New("404"), true},
		{
			"400 cannot find universe",
			&http.Response{StatusCode: http.StatusBadRequest},
			&HTTPResponseError{StatusCode: 400, Body: "Cannot find universe 5f0e4c93"},
			true,
		},
		{
			"500 does not exist",
			&http.Response{StatusCode: http.StatusInternalServerError},
			&HTTPResponseError{StatusCode: 500, Body: "Universe 5f0e4c93 does not exist"},
			true,
		},
		{
			"unrelated 400",
			&http.Response{StatusCode: http.StatusBadRequest},
			&HTTPResponseError{StatusCode: 400, Body: "some validation failure"},
			false,
		},
		{
			"plain error carries no body",
			&http.Response{StatusCode: http.StatusBadRequest},
			errors.New("Cannot find universe 5f0e4c93"),
			false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsUniverseMissing(tc.resp, tc.err); got != tc.want {
				t.Errorf("IsUniverseMissing = %v want %v", got, tc.want)
			}
		})
	}
}

// TestErrUniverseMissingUnwraps guards the sentinel contract: callers wrap it
// with context and CRUD code branches via errors.Is.
func TestErrUniverseMissingUnwraps(t *testing.T) {
	wrapped := fmt.Errorf("universe 5f0e4c93: %w", ErrUniverseMissing)
	if !errors.Is(wrapped, ErrUniverseMissing) {
		t.Error("errors.Is(wrapped, ErrUniverseMissing) = false")
	}
}
