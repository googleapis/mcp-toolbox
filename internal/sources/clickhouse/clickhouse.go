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

package clickhouse

import (
	"context"
	"database/sql"
	"encoding"
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"
	"strings"
	"time"

	_ "github.com/ClickHouse/clickhouse-go/v2"
	"github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/util/orderedmap"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	"go.opentelemetry.io/otel/trace"
)

const SourceType string = "clickhouse"

// validate interface
var _ sources.SourceConfig = Config{}

func init() {
	if !sources.Register(SourceType, newConfig) {
		panic(fmt.Sprintf("source type %q already registered", SourceType))
	}
}

func newConfig(ctx context.Context, name string, decoder *yaml.Decoder) (sources.SourceConfig, error) {
	actual := Config{Name: name}
	if err := decoder.DecodeContext(ctx, &actual); err != nil {
		return nil, err
	}
	return actual, nil
}

type Config struct {
	Name     string `yaml:"name" validate:"required"`
	Type     string `yaml:"type" validate:"required"`
	Host     string `yaml:"host" validate:"required"`
	Port     string `yaml:"port" validate:"required"`
	Database string `yaml:"database" validate:"required"`
	User     string `yaml:"user" validate:"required"`
	Password string `yaml:"password"`
	Protocol string `yaml:"protocol"`
	Secure   bool   `yaml:"secure"`
}

func (r Config) SourceConfigType() string {
	return SourceType
}

func (r Config) Initialize(ctx context.Context, tracer trace.Tracer) (sources.Source, error) {
	pool, err := initClickHouseConnectionPool(ctx, tracer, r.Name, r.Host, r.Port, r.User, r.Password, r.Database, r.Protocol, r.Secure)
	if err != nil {
		return nil, fmt.Errorf("unable to create pool: %w", err)
	}

	err = pool.PingContext(ctx)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("unable to connect successfully: %w", err)
	}

	s := &Source{
		Config: r,
		Pool:   pool,
	}
	return s, nil
}

var _ sources.Source = &Source{}

type Source struct {
	Config
	Pool *sql.DB
}

func (s *Source) IsReadOnly() bool {
	return false
}

func (s *Source) SourceType() string {
	return SourceType
}

func (s *Source) ToConfig() sources.SourceConfig {
	return s.Config
}

func (s *Source) ClickHousePool() *sql.DB {
	return s.Pool
}

func (s *Source) RunSQL(ctx context.Context, statement string, params parameters.ParamValues) (any, error) {
	var sliceParams []any
	if params != nil {
		sliceParams = params.AsSlice()
	}
	results, err := s.ClickHousePool().QueryContext(ctx, statement, sliceParams...)
	if err != nil {
		return nil, fmt.Errorf("unable to execute query: %w", err)
	}
	defer results.Close()

	cols, err := results.Columns()
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve rows column name: %w", err)
	}

	// create an array of values for each column, which can be re-used to scan each row
	rawValues := make([]any, len(cols))
	values := make([]any, len(cols))
	for i := range rawValues {
		values[i] = &rawValues[i]
	}

	colTypes, err := results.ColumnTypes()
	if err != nil {
		return nil, fmt.Errorf("unable to get column types: %w", err)
	}

	// The driver returns Array(UInt8) values, also inside arrays, maps and
	// tuples, as []uint8, which encoding/json would write as base64.
	hasUInt8Array := make([]bool, len(colTypes))
	for i, colType := range colTypes {
		hasUInt8Array[i] = strings.Contains(colType.DatabaseTypeName(), "Array(UInt8)")
	}

	out := []any{}
	for results.Next() {
		err := results.Scan(values...)
		if err != nil {
			return nil, fmt.Errorf("unable to parse row: %w", err)
		}
		// An ordered row keeps the columns in the order of the query.
		row := orderedmap.Row{}
		for i, name := range cols {
			// ClickHouse driver may return specific types that need handling
			switch colTypes[i].DatabaseTypeName() {
			case "String", "FixedString":
				if rawValues[i] != nil {
					// Handle potential []byte to string conversion if needed
					if b, ok := rawValues[i].([]byte); ok {
						row.Add(name, string(b))
					} else {
						row.Add(name, rawValues[i])
					}
				} else {
					row.Add(name, nil)
				}
			default:
				if hasUInt8Array[i] {
					row.Add(name, uint8ArraysToNumbers(rawValues[i]))
				} else {
					row.Add(name, rawValues[i])
				}
			}
		}
		out = append(out, row)
	}

	if err := results.Err(); err != nil {
		return nil, fmt.Errorf("errors encountered by results.Scan: %w", err)
	}

	return out, nil
}

// uint8ArraysToNumbers returns v with every []uint8 in it, at any depth of
// slices and maps, replaced by a list of numbers. Values that encode
// themselves to JSON (such as UUID, IPv6 or time.Time) are left unchanged, and
// a nil slice or map stays nil so that it still encodes to null.
func uint8ArraysToNumbers(v any) any {
	switch val := v.(type) {
	case nil, json.Marshaler, encoding.TextMarshaler:
		return v
	case []uint8:
		if val == nil {
			return v
		}
		nums := make([]int, len(val))
		for i, n := range val {
			nums[i] = int(n)
		}
		return nums
	}

	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Slice, reflect.Array:
		if rv.Kind() == reflect.Slice && rv.IsNil() {
			return v
		}
		elems := make([]any, rv.Len())
		for i := range elems {
			elems[i] = uint8ArraysToNumbers(rv.Index(i).Interface())
		}
		return elems
	case reflect.Map:
		if rv.IsNil() {
			return v
		}
		anyType := reflect.TypeFor[any]()
		m := reflect.MakeMapWithSize(reflect.MapOf(rv.Type().Key(), anyType), rv.Len())
		iter := rv.MapRange()
		for iter.Next() {
			elem := reflect.Zero(anyType)
			if converted := uint8ArraysToNumbers(iter.Value().Interface()); converted != nil {
				elem = reflect.ValueOf(converted)
			}
			m.SetMapIndex(iter.Key(), elem)
		}
		return m.Interface()
	}
	return v
}

func validateConfig(protocol string) error {
	validProtocols := map[string]bool{"http": true, "https": true}

	if protocol != "" && !validProtocols[protocol] {
		return fmt.Errorf("invalid protocol: %s, must be one of: http, https", protocol)
	}
	return nil
}

func initClickHouseConnectionPool(ctx context.Context, tracer trace.Tracer, name, host, port, user, pass, dbname, protocol string, secure bool) (*sql.DB, error) {
	//nolint:all // Reassigned ctx
	ctx, span := sources.InitConnectionSpan(ctx, tracer, SourceType, name)
	defer span.End()

	if protocol == "" {
		protocol = "https"
	}

	if err := validateConfig(protocol); err != nil {
		return nil, err
	}

	encodedUser := url.QueryEscape(user)
	encodedPass := url.QueryEscape(pass)

	var dsn string
	scheme := protocol
	if protocol == "http" && secure {
		scheme = "https"
	}
	dsn = fmt.Sprintf("%s://%s:%s@%s:%s/%s", scheme, encodedUser, encodedPass, host, port, dbname)
	if scheme == "https" {
		dsn += "?secure=true&skip_verify=false"
	}

	pool, err := sql.Open("clickhouse", dsn)
	if err != nil {
		return nil, fmt.Errorf("sql.Open: %w", err)
	}

	pool.SetMaxOpenConns(25)
	pool.SetMaxIdleConns(5)
	pool.SetConnMaxLifetime(5 * time.Minute)

	return pool, nil
}
