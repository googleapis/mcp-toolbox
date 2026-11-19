// Copyright 2025 Google LLC
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

package dgraph

import "testing"

func TestEscapeRDFLiteral(t *testing.T) {
	tcs := []struct {
		desc string
		in   string
		want string
	}{
		{desc: "plain value", in: "hello", want: "hello"},
		{desc: "embedded double quote", in: `a"b`, want: `a\"b`},
		{desc: "backslash", in: `a\b`, want: `a\\b`},
		{desc: "newline", in: "a\nb", want: `a\nb`},
		{desc: "carriage return", in: "a\rb", want: `a\rb`},
		{desc: "tab", in: "a\tb", want: `a\tb`},
		{desc: "backslash before quote is not double-escaped", in: `a\"b`, want: `a\\\"b`},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			if got := escapeRDFLiteral(tc.in); got != tc.want {
				t.Fatalf("escapeRDFLiteral(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestEmbedParamsIntoMutationEscapesInjection(t *testing.T) {
	// A malicious parameter value containing a double quote must not be able to
	// break out of the RDF literal and inject additional triples.
	mutation := `_:node <name> $name .`
	paramsMap := map[string]interface{}{
		"$name": `evil" . _:node <isAdmin> "true`,
	}

	got := embedParamsIntoMutation(mutation, paramsMap)
	want := `_:node <name> "evil\" . _:node <isAdmin> \"true" .`

	if got != want {
		t.Fatalf("embedParamsIntoMutation did not escape the value\n got: %s\nwant: %s", got, want)
	}
}

func TestEmbedParamsIntoMutationPlainValue(t *testing.T) {
	mutation := `_:node <name> $name .`
	paramsMap := map[string]interface{}{"$name": "alice"}

	got := embedParamsIntoMutation(mutation, paramsMap)
	want := `_:node <name> "alice" .`

	if got != want {
		t.Fatalf("embedParamsIntoMutation(%q) = %s, want %s", mutation, got, want)
	}
}

// A shorter parameter key that is a prefix of a longer one ("$id" vs
// "$id_name") must not corrupt the longer key. The longest key is matched
// first at each position, so both are replaced with their own value.
func TestEmbedParamsIntoMutationPrefixKeys(t *testing.T) {
	mutation := `_:node <id> $id . _:node <name> $id_name .`
	paramsMap := map[string]interface{}{
		"$id":      "1",
		"$id_name": "bob",
	}

	got := embedParamsIntoMutation(mutation, paramsMap)
	want := `_:node <id> "1" . _:node <name> "bob" .`

	if got != want {
		t.Fatalf("embedParamsIntoMutation did not handle prefix keys\n got: %s\nwant: %s", got, want)
	}
}

// A single left-to-right pass means a substituted value that itself contains a
// parameter-key sequence ("$b") is never re-scanned and must survive verbatim.
func TestEmbedParamsIntoMutationValueContainingKey(t *testing.T) {
	mutation := `_:node <a> $a . _:node <b> $b .`
	paramsMap := map[string]interface{}{
		"$a": "$b",
		"$b": "x",
	}

	got := embedParamsIntoMutation(mutation, paramsMap)
	want := `_:node <a> "$b" . _:node <b> "x" .`

	if got != want {
		t.Fatalf("embedParamsIntoMutation corrupted a value containing a key\n got: %s\nwant: %s", got, want)
	}
}
