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

package sources_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/sources/postgres"
	"go.opentelemetry.io/otel/trace/noop"
)

// pgDoc is a source configuration whose host was never resolved, as the config
// parser leaves it when the variable is unset and the caller tolerates that.
func pgDoc() map[string]any {
	return map[string]any{
		"name":     "pg",
		"type":     "postgres",
		"host":     "${TEST_PG_HOST}",
		"port":     "5432",
		"database": "mydb",
		"user":     "me",
		"password": "secret",
	}
}

// A field resolved after the source was built reaches the connection. This is
// the whole point of resolving at connect rather than at startup, and a test is
// the only place it is observable: nothing outside a process can change its
// environment, so in production the value is always the one startup would have
// read.
func TestDeferredFieldResolvesAtConnect(t *testing.T) {
	ctx := sources.WithUnresolvedEnvVars(
		sources.WithSourceDoc(context.Background(), pgDoc()), []string{"TEST_PG_HOST"})
	conn := sources.NewConnectOnce[string](ctx, "pg", postgres.SourceType,
		postgres.Config{Name: "pg"}, noop.NewTracerProvider().Tracer("test"))

	t.Setenv("TEST_PG_HOST", "resolved.example.com")

	got, err := conn.DoWithConfig(ctx, func(_ context.Context, sc sources.SourceConfig) (string, error) {
		return sc.(postgres.Config).Host, nil
	})
	if err != nil {
		t.Fatalf("DoWithConfig: %v", err)
	}
	if got != "resolved.example.com" {
		t.Errorf("Host = %q, want the value set after the source was built", got)
	}
}

// A variable still unset when the source connects names itself in the error,
// rather than surfacing as whatever the driver makes of an unresolved value.
func TestUnresolvedFieldNamesItsVariable(t *testing.T) {
	ctx := sources.WithUnresolvedEnvVars(
		sources.WithSourceDoc(context.Background(), pgDoc()), []string{"TEST_PG_HOST"})
	conn := sources.NewConnectOnce[string](ctx, "pg", postgres.SourceType,
		postgres.Config{Name: "pg"}, noop.NewTracerProvider().Tracer("test"))

	_, err := conn.DoWithConfig(ctx, func(context.Context, sources.SourceConfig) (string, error) {
		return "", nil
	})
	if err == nil {
		t.Fatal("expected an error while TEST_PG_HOST is unset")
	}
	if !strings.Contains(err.Error(), "TEST_PG_HOST is not set") {
		t.Errorf("error should name the variable, got: %v", err)
	}
}

// A source with nothing left to resolve connects with the config it was built
// from, without going back through its factory.
func TestFullyResolvedConfigIsReused(t *testing.T) {
	doc := pgDoc()
	doc["host"] = "literal.example.com"
	ctx := sources.WithSourceDoc(context.Background(), doc)
	built := postgres.Config{Name: "pg", Host: "literal.example.com"}
	conn := sources.NewConnectOnce[postgres.Config](ctx, "pg", postgres.SourceType,
		built, noop.NewTracerProvider().Tracer("test"))

	got, err := conn.DoWithConfig(ctx, func(_ context.Context, sc sources.SourceConfig) (postgres.Config, error) {
		return sc.(postgres.Config), nil
	})
	if err != nil {
		t.Fatalf("DoWithConfig: %v", err)
	}
	if diff := cmp.Diff(built, got); diff != "" {
		t.Errorf("config mismatch, want the one the source was built with (-want +got):\n%s", diff)
	}
}

// A value that merely looks like a reference is data. The parser records which
// variables it actually left behind, and only those are resolved later — a
// variable whose value happens to contain "${...}" must not strand the source.
func TestLiteralReferenceInValueIsNotDeferred(t *testing.T) {
	doc := pgDoc()
	doc["host"] = "literal.example.com"
	doc["password"] = "${NOT_A_VAR}"
	ctx := sources.WithSourceDoc(context.Background(), doc)

	built := postgres.Config{Name: "pg", Host: "literal.example.com", Password: "${NOT_A_VAR}"}
	conn := sources.NewConnectOnce[postgres.Config](ctx, "pg", postgres.SourceType,
		built, noop.NewTracerProvider().Tracer("test"))

	got, err := conn.DoWithConfig(ctx, func(_ context.Context, sc sources.SourceConfig) (postgres.Config, error) {
		return sc.(postgres.Config), nil
	})
	if err != nil {
		t.Fatalf("a literal ${...} should not block connecting: %v", err)
	}
	if diff := cmp.Diff(built, got); diff != "" {
		t.Errorf("config mismatch (-want +got):\n%s", diff)
	}
}
