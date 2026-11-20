// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mysqllistalllocks

import "testing"

func TestSelectListAllLocksStatement(t *testing.T) {
	tcs := []struct {
		version string
		want    string
	}{
		{version: "5.7.44", want: listAllLocksStatement57},
		{version: "5.7.44-log", want: listAllLocksStatement57},
		{version: "8.0.40", want: listAllLocksStatement8plus},
		{version: "8.4.3", want: listAllLocksStatement8plus},
		{version: "9.1.0", want: listAllLocksStatement8plus},
		{version: "10.6.20-MariaDB", want: listAllLocksStatement57},
		{version: "11.4.4-MariaDB-ubu2404-log", want: listAllLocksStatement57},
		{version: "12.0.2-mariadb", want: listAllLocksStatement57},
	}
	for _, tc := range tcs {
		t.Run(tc.version, func(t *testing.T) {
			if got := selectListAllLocksStatement(tc.version); got != tc.want {
				t.Errorf("selectListAllLocksStatement(%q) picked the wrong statement", tc.version)
			}
		})
	}
}
