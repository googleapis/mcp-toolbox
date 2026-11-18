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

package redis

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

func TestReplaceCommandsParamsNonStringValues(t *testing.T) {
	params := parameters.Parameters{
		parameters.NewStringParameter("key", "the key"),
		parameters.NewIntParameter("ttl", "time to live in seconds"),
		parameters.NewFloatParameter("score", "member score"),
		parameters.NewBooleanParameter("flag", "a flag"),
		parameters.NewArrayParameter("ids", "member ids", parameters.NewIntParameter("id", "member id")),
	}
	commands := [][]string{
		{"EXPIRE", "$key", "$ttl"},
		{"ZADD", "myzset", "$score", "member"},
		{"SET", "$key", "$flag"},
		{"SADD", "myset", "$ids"},
	}
	paramValues := parameters.ParamValues{
		{Name: "key", Value: "k"},
		{Name: "ttl", Value: 60},
		{Name: "score", Value: 1.5},
		{Name: "flag", Value: true},
		{Name: "ids", Value: []any{1, 2}},
	}
	want := [][]any{
		{"EXPIRE", "k", "60"},
		{"ZADD", "myzset", "1.5", "member"},
		{"SET", "k", "true"},
		{"SADD", "myset", "1", "2"},
	}

	got, err := replaceCommandsParams(commands, params, paramValues)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("incorrect commands (-want +got):\n%s", diff)
	}
}
