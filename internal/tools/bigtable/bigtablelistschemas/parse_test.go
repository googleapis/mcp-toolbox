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

package bigtablelistschemas

import (
	"reflect"
	"testing"
)

func TestParseColumnsFromQuery(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  []Column
	}{
		{
			name:  "basic AS",
			query: "SELECT _key, stats_summary['os_build'] AS os \nFROM analytics \nWHERE _key = 'phone#4c410523#20190501'",
			want:  []Column{{Name: "_key", Type: "BYTES"}, {Name: "os"}},
		},
		{
			name:  "multiple AS",
			query: "SELECT _key, stats_summary['os_build'] AS os, stats_summary['user_agent'] AS agent \nFROM analytics",
			want:  []Column{{Name: "_key", Type: "BYTES"}, {Name: "os"}, {Name: "agent"}},
		},
		{
			name:  "MAP_KEYS AS keys",
			query: "SELECT _key, MAP_KEYS(cell_plan) AS keys \nFROM analytics",
			want:  []Column{{Name: "_key", Type: "BYTES"}, {Name: "keys", Type: "ARRAY<BYTES>"}},
		},
		{
			name:  "select *",
			query: "SELECT * FROM myTable",
			want:  []Column{{Name: "*"}},
		},
		{
			name:  "multiple unaliased",
			query: "SELECT _key, cell_plan \nFROM analytics(with_history => TRUE, latest_n => 3)",
			want:  []Column{{Name: "_key", Type: "BYTES"}, {Name: "cell_plan"}},
		},
		{
			name:  "single unaliased",
			query: "SELECT address \nFROM myTable(as_of => TIMESTAMP('2022/01/10-13:14:00')) \nWHERE _key = 'user1'",
			want:  []Column{{Name: "address"}},
		},
		{
			name:  "nested brackets AS",
			query: "SELECT metrics['temperature'] AS temp_versioned \nFROM sensorReadings(\n    after => TIMESTAMP('2023/01/04-23:00:00'), \n    before => TIMESTAMP('2023/01/05-01:00:00')\n) \nWHERE _key LIKE 'sensorA%'",
			want:  []Column{{Name: "temp_versioned"}},
		},
		{
			name:  "deeply nested arrays AS",
			query: "SELECT \n    address['street'][0].value AS moved_to, \n    address['street'][1].value AS moved_from, \n    FORMAT_TIMESTAMP('%Y-%m-%d', address['street'][0].timestamp) AS moved_on\nFROM myTable(with_history => TRUE)",
			want:  []Column{{Name: "moved_to"}, {Name: "moved_from"}, {Name: "moved_on", Type: "STRING"}},
		},
		{
			name:  "NULLIF as lowercase",
			query: "SELECT NULLIF(session['identity'], 'anonymous') as user, session['agent'] \nFROM myTable",
			want:  []Column{{Name: "user"}, {Name: "session['agent']"}},
		},
		{
			name:  "TO_VECTOR32 AS",
			query: "SELECT _key, TO_VECTOR32(data['embedding']) AS embedding \nFROM myTable",
			want:  []Column{{Name: "_key", Type: "BYTES"}, {Name: "embedding", Type: "VECTOR32"}},
		},
		{
			name:  "double CAST struct property unaliased",
			query: "SELECT _key, (CAST(cf['product'] AS my_proto.foo.bar.Product)).product_name \nFROM myTable",
			want:  []Column{{Name: "_key", Type: "BYTES"}, {Name: "product_name"}},
		},
		{
			name:  "double CAST struct properties multi-line",
			query: "SELECT \n  (CAST(cf['product'] AS my_proto.foo.bar.Product)).product_name,\n  (CAST(cf['product'] AS my_proto.foo.bar.Product)).price\nFROM myTable;",
			want:  []Column{{Name: "product_name"}, {Name: "price"}},
		},
		{
			name:  "CAST string AS",
			query: "SELECT CAST(cf['name'] AS STRING) AS name \nFROM myTable",
			want:  []Column{{Name: "name", Type: "STRING"}},
		},
		{
			name:  "CAST bool to string",
			query: "SELECT CAST(TRUE AS STRING) AS str_bool;",
			want:  []Column{{Name: "str_bool", Type: "STRING"}},
		},
		{
			name:  "CAST numeric to string",
			query: "SELECT CAST(123.45 AS STRING) AS str_num;",
			want:  []Column{{Name: "str_num", Type: "STRING"}},
		},
		{
			name:  "CAST date to string",
			query: "SELECT CAST(CURRENT_DATE() AS STRING) AS str_date;",
			want:  []Column{{Name: "str_date", Type: "STRING"}},
		},
		{
			name:  "CAST string to int",
			query: "SELECT CAST(\"291\" AS INT64) AS int_val;",
			want:  []Column{{Name: "int_val", Type: "INT64"}},
		},
		{
			name:  "CAST string to bool",
			query: "SELECT CAST(\"true\" AS BOOL) AS bool_val;",
			want:  []Column{{Name: "bool_val", Type: "BOOL"}},
		},
		{
			name:  "CAST string to ts",
			query: "SELECT CAST(\"2020-06-02 17:00:53.110+00:00\" AS TIMESTAMP) AS ts_val;",
			want:  []Column{{Name: "ts_val", Type: "TIMESTAMP"}},
		},
		{
			name:  "SAFE_CONVERT bytes to string",
			query: "SELECT SAFE_CONVERT_BYTES_TO_STRING(cf['description']) AS safe_desc\nFROM myTable;",
			want:  []Column{{Name: "safe_desc", Type: "STRING"}},
		},
		{
			name:  "SAFE_CAST to INT64",
			query: "SELECT SAFE_CAST(cf['age_string'] AS INT64) AS age \nFROM myTable;",
			want:  []Column{{Name: "age", Type: "INT64"}},
		},
		{
			name:  "PARSE_TIMESTAMP",
			query: "SELECT PARSE_TIMESTAMP(\"%a %b %e %T %Y\", \"Thu Dec 25 07:30:00 2008\") AS parsed_ts;",
			want:  []Column{{Name: "parsed_ts", Type: "TIMESTAMP"}},
		},
		{
			name:  "PARSE_DATE",
			query: "SELECT PARSE_DATE(\"%Y%m%d\", \"20081225\") AS parsed_date;",
			want:  []Column{{Name: "parsed_date", Type: "DATE"}},
		},
		{
			name:  "FROM_BASE64",
			query: "SELECT FROM_BASE64('SGVsbG8=') AS decoded_bytes;",
			want:  []Column{{Name: "decoded_bytes", Type: "BYTES"}},
		},
		{
			name:  "TO_BASE64",
			query: "SELECT TO_BASE64(cf['payload']) AS base64_payload FROM myTable;",
			want:  []Column{{Name: "base64_payload", Type: "STRING"}},
		},
		{
			name:  "ARRAY_TO_STRING",
			query: "SELECT ARRAY_TO_STRING([\"apple\", \"banana\", \"cherry\"], \", \") AS fruits;",
			want:  []Column{{Name: "fruits", Type: "STRING"}},
		},
		{
			name:  "CODE_POINTS_TO_STRING",
			query: "SELECT CODE_POINTS_TO_STRING([65, 66, 67]) AS abc_string;",
			want:  []Column{{Name: "abc_string", Type: "STRING"}},
		},
		{
			name:  "TO_JSON_STRING",
			query: "SELECT TO_JSON_STRING(STRUCT(1 AS id, \"Alice\" AS name)) AS json_str;",
			want:  []Column{{Name: "json_str", Type: "STRING"}},
		},
		{
			name:  "comma in single quotes",
			query: "SELECT 'apple, orange' AS fruit, _key FROM myTable",
			want:  []Column{{Name: "fruit"}, {Name: "_key", Type: "BYTES"}},
		},
		{
			name:  "comma in double quotes",
			query: "SELECT \"apple, orange\" AS fruit, _key FROM myTable",
			want:  []Column{{Name: "fruit"}, {Name: "_key", Type: "BYTES"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseColumnsFromQuery(tt.query)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseColumnsFromQuery() = %v, want %v", got, tt.want)
			}
		})
	}
}
