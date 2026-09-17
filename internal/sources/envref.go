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

package sources

import (
	"context"
	"os"
	"regexp"
	"slices"
	"sort"

	"github.com/goccy/go-yaml"
)

type sourceDocKey struct{}

type unresolvedNamesKey struct{}

// WithUnresolvedEnvVars records the variables the config parser left in place
// because they were unset. Only these are treated as references later: a
// "${...}" can just as easily arrive inside the value of a variable that was
// set, and that is data, not something waiting to be resolved.
func WithUnresolvedEnvVars(ctx context.Context, names []string) context.Context {
	if len(names) == 0 {
		return ctx
	}
	return context.WithValue(ctx, unresolvedNamesKey{}, names)
}

// UnresolvedEnvVarsFromContext returns the variables left unresolved.
func UnresolvedEnvVarsFromContext(ctx context.Context) []string {
	names, _ := ctx.Value(unresolvedNamesKey{}).([]string)
	return names
}

// WithSourceDoc returns ctx carrying a source's configuration as written. It is
// set on the context Initialize runs under, which a source already hands to
// NewConnectOnce, so nothing has to reach the source itself.
func WithSourceDoc(ctx context.Context, doc map[string]any) context.Context {
	if doc == nil {
		return ctx
	}
	return context.WithValue(ctx, sourceDocKey{}, doc)
}

// SourceDocFromContext returns the configuration as written, or nil for a
// source built outside InitializeConfigs.
func SourceDocFromContext(ctx context.Context) map[string]any {
	doc, _ := ctx.Value(sourceDocKey{}).(map[string]any)
	return doc
}

// envReference matches a ${VAR} the config parser left in place because the
// variable was unset. A reference carrying a default is never left behind, so
// only the bare form appears here.
var envReference = regexp.MustCompile(`\$\{(\w+)\}`)

// UnresolvedKeys reports the keys of a source document whose value still names
// an environment variable, anywhere within it. Sorted, so messages built from
// it do not reorder between runs.
func UnresolvedKeys(doc map[string]any, names []string) []string {
	if len(names) == 0 {
		return nil
	}
	var keys []string
	for k, v := range doc {
		if hasEnvReference(v, names) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys
}

func hasEnvReference(v any, names []string) bool {
	switch t := v.(type) {
	case string:
		for _, match := range envReference.FindAllStringSubmatch(t, -1) {
			if slices.Contains(names, match[1]) {
				return true
			}
		}
	case map[string]any:
		for _, e := range t {
			if hasEnvReference(e, names) {
				return true
			}
		}
	case []any:
		for _, e := range t {
			if hasEnvReference(e, names) {
				return true
			}
		}
	}
	return false
}

// ResolveDoc returns a copy of doc with every environment variable reference
// replaced by its current value, and the names of any variables still unset.
// A doc with no references is returned unchanged.
func ResolveDoc(doc map[string]any, names []string) (map[string]any, []string) {
	var unset []string
	resolved := make(map[string]any, len(doc))
	for k, v := range doc {
		resolved[k] = resolveValue(v, names, &unset)
	}
	sort.Strings(unset)
	return resolved, unset
}

func resolveValue(v any, names []string, unset *[]string) any {
	switch t := v.(type) {
	case string:
		if !hasEnvReference(t, names) {
			return v
		}
		substituted := envReference.ReplaceAllStringFunc(t, func(ref string) string {
			name := envReference.FindStringSubmatch(ref)[1]
			if !slices.Contains(names, name) {
				return ref
			}
			value, found := os.LookupEnv(name)
			if !found {
				if !slices.Contains(*unset, name) {
					*unset = append(*unset, name)
				}
				return ref
			}
			return value
		})
		return retype(substituted)
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, e := range t {
			out[k] = resolveValue(e, names, unset)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = resolveValue(e, names, unset)
		}
		return out
	}
	return v
}

// retype gives a substituted value the type it would have had if the variable
// had been set while the file was first read. Substitution there happens in the
// YAML text, so "5432" becomes a number and "true" a bool before any field sees
// it; a value resolved later has to travel the same route to reach the same
// field.
func retype(s string) any {
	var v any
	if err := yaml.Unmarshal([]byte(s), &v); err != nil {
		return s
	}
	if v == nil {
		return s
	}
	return v
}
