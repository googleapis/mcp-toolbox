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

package util

import (
	"bytes"
	"regexp"
)

// envReference matches a ${VAR} the config parser left in place because the
// variable was unset. A reference written with a default is always substituted,
// so only the bare form reaches here.
var envReference = regexp.MustCompile(`\$\{(\w+)\}`)

// HasEnvReference reports whether s still names an environment variable.
func HasEnvReference(s string) bool {
	return envReference.MatchString(s)
}

// IsEnvReferenceScalar reports whether a YAML scalar is nothing but an
// unresolved reference. Used to tell a field that has not been resolved yet
// from one holding a value that cannot be parsed.
func IsEnvReferenceScalar(b []byte) bool {
	trimmed := bytes.TrimSpace(b)
	trimmed = bytes.Trim(trimmed, `"'`)
	return envReference.Match(trimmed) && len(envReference.Find(trimmed)) == len(trimmed)
}

// ReplaceEnvReferences substitutes every reference in s using lookup, and
// reports the names lookup could not supply.
func ReplaceEnvReferences(s string, lookup func(name string) (string, bool)) (string, []string) {
	var unset []string
	out := envReference.ReplaceAllStringFunc(s, func(ref string) string {
		name := envReference.FindStringSubmatch(ref)[1]
		value, found := lookup(name)
		if !found {
			unset = append(unset, name)
			return ref
		}
		return value
	})
	return out, unset
}
