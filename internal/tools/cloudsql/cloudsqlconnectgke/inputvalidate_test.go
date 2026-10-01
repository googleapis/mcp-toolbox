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

package cloudsqlconnectgke

import (
	"strings"
	"testing"
)

func TestInputValidators(t *testing.T) {
	tcs := []struct {
		desc string
		fn   func(string) error
		ok   []string
		bad  []string
	}{
		{"project", func(v string) error { return validateProjectID(v, "cluster_project") }, []string{"my-proj-1"}, []string{"", "My-Proj", "1abcde", "a b", "google.com:proj"}},
		{"cluster", validateClusterName, []string{"prod", "a-1"}, []string{"", "Prod", "-a", strings.Repeat("a", 41)}},
		{"location", func(v string) error { return validateLocation(v, "cluster_location") }, []string{"us-central1", "us-central1-a"}, []string{"", "US", "us central1"}},
		{"k8s name", func(v string) error { return validateKubernetesName(v, "namespace") }, []string{"default", "my-ns"}, []string{"", "My_NS", "-x", strings.Repeat("a", 64)}},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			for _, v := range tc.ok {
				if err := tc.fn(v); err != nil {
					t.Errorf("%q: unexpected error %v", v, err)
				}
			}
			for _, v := range tc.bad {
				if err := tc.fn(v); err == nil {
					t.Errorf("%q: expected error", v)
				}
			}
		})
	}
}
