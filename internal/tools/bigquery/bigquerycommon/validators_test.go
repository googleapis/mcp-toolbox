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

package bigquerycommon_test

import (
	"testing"

	"github.com/googleapis/mcp-toolbox/internal/tools/bigquery/bigquerycommon"
)

func TestValidTableID(t *testing.T) {
	tcs := []struct {
		in    string
		valid bool
	}{
		// Allowed: dataset.table
		{"my_dataset.my_table", true},
		{"ds.t", true},
		{"ds_1.tbl_2", true},

		// Allowed: project.dataset.table
		{"proj.ds.tbl", true},
		{"PROJ.DS.TBL", true},
		{"my_project.my_dataset.my_table", true},

		// Rejected: hyphens (not valid in dataset/table IDs).
		{"my-project.my-dataset.my_table", false},
		{"my-project.my_dataset.my-table", false},
		{"my-project-123.dataset.table", true},

		// Rejected: only one component (no dot)
		{"my_dataset", false},
		{"", false},

		// Rejected: too many dots (4+ parts)
		{"a.b.c.d", false},

		// Rejected: injection characters
		{"dataset.table`", false},
		{"dataset.table` UNION ALL SELECT 1 --", false},
		{"dataset.table'; DROP TABLE x --", false},
		{"dataset.table\n", false},
		{"dataset.table ", false},
		{" dataset.table", false},
		{"dataset.table\t", false},

		// Rejected: backtick (closes identifier in SQL)
		{"ds.`table`", false},

		// Rejected: SQL metacharacters
		{"ds.table--", false},
		{"ds.table/*", false},
		{"ds.table;", false},
	}
	for _, tc := range tcs {
		if got := bigquerycommon.ValidTableID(tc.in); got != tc.valid {
			t.Errorf("ValidTableID(%q) = %v, want %v", tc.in, got, tc.valid)
		}
	}
}

func TestValidColumnName(t *testing.T) {
	tcs := []struct {
		in    string
		valid bool
	}{
		// Allowed: simple identifiers.
		{"sales", true},
		{"sales_col", true},
		{"_internal", true},
		{"Col1", true},
		{"A", true},
		{"is_test", true},
		{"timestamp_col", true},

		// Rejected: empty string.
		{"", false},

		// Rejected: leading digit.
		{"1col", false},

		// Rejected: SQL injection characters.
		{"col'", false},
		{"col`", false},
		{"col; DROP TABLE x", false},
		{"col UNION SELECT", false},
		{"col--", false},
		{"col/*", false},
		{"col(", false},
		{"col)", false},
		{"col/col2", false},

		// Rejected: whitespace.
		{"col name", false},
		{" col", false},
		{"col ", false},

		// Rejected: dots (not valid in unquoted column names).
		{"ds.col", false},
	}
	for _, tc := range tcs {
		if got := bigquerycommon.ValidColumnName(tc.in); got != tc.valid {
			t.Errorf("ValidColumnName(%q) = %v, want %v", tc.in, got, tc.valid)
		}
	}
}

func TestStripSingleQuotes(t *testing.T) {
	tcs := []struct {
		in   string
		want string
	}{
		{"'hello'", "hello"},
		{"hello", "hello"},
		{"'hello", "'hello"},
		{"hello'", "hello'"},
		{"''", ""},
		{"'", "'"},
		{"", ""},
		{"'a'", "a"},
		{"'abc'", "abc"},
		{"''abc''", "'abc'"},
	}
	for _, tc := range tcs {
		if got := bigquerycommon.StripSingleQuotes(tc.in); got != tc.want {
			t.Errorf("StripSingleQuotes(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestValidColumnParam(t *testing.T) {
	tcs := []struct {
		in    string
		valid bool
	}{
		{"'sales'", true},
		{"sales", true},
		{"'sales_col'", true},
		{"_internal", true},
		{"'1col'", false},
		{"col'", false},
		{"'col", false},
		{"'col; DROP TABLE x'", false},
		{"", false},
	}
	for _, tc := range tcs {
		if got := bigquerycommon.ValidColumnParam(tc.in); got != tc.valid {
			t.Errorf("ValidColumnParam(%q) = %v, want %v", tc.in, got, tc.valid)
		}
	}
}

func TestValidContributionMetricParam(t *testing.T) {
	tcs := []struct {
		in    string
		valid bool
	}{
		{"'metric'", true},
		{"metric", true},
		{"'metric's'", false},
		{"metric's", false},
		{"'metric", false},
		{"''metric''", false},
	}
	for _, tc := range tcs {
		if got := bigquerycommon.ValidContributionMetricParam(tc.in); got != tc.valid {
			t.Errorf("ValidContributionMetricParam(%q) = %v, want %v", tc.in, got, tc.valid)
		}
	}
}

func TestIsSystemResource(t *testing.T) {
	tcs := []struct {
		datasetID  string
		resourceID string
		want       bool
	}{
		{"AI", "FORECAST", true},
		{"ai", "forecast", true},
		{"AI", "GENERATE_TEXT", true},
		{"AI", "EXTRACT_ENTITY", true},
		{"AI", "SUMMARIZE", true},
		{"AI", "GENERATE", true},
		{"AI", "GENERATE_BOOL", true},
		{"AI", "GENERATE_INT", true},
		{"AI", "GENERATE_DOUBLE", true},
		{"AI", "GENERATE_TABLE", true},
		{"AI", "IF", true},
		{"AI", "CLASSIFY", true},
		{"AI", "SCORE", true},
		{"AI", "INVALID", false},
		{"ML", "GET_INSIGHTS", true},
		{"ml", "explain_predict", true},
		{"ML", "GENERATE_TEXT", true},
		{"ML", "DISTANCE", true},
		{"ML", "PREDICT", true},
		{"ml", "predict", true},
		{"ML", "GENERATE_EMBEDDING", true},
		{"ML", "TRANSLATE", true},
		{"ML", "UNDERSTAND_TEXT", true},
		{"ML", "ANNOTATE_IMAGE", true},
		{"ML", "TRANSCRIBE", true},
		{"ML", "PROCESS_DOCUMENT", true},
		{"ML", "DETECT_ANOMALIES", true},
		{"ML", "FORECAST", true},
		{"ML", "EVALUATE", true},
		{"ML", "TRAINING_INFO", true},
		{"ML", "FEATURE_INFO", true},
		{"ML", "WEIGHTS", true},
		{"ML", "CENTROIDS", true},
		{"ML", "CONFUSION_MATRIX", true},
		{"ML", "ROC_CURVE", true},
		{"ML", "ARIMA_EVALUATE", true},
		{"ML", "ARIMA_COEFFICIENTS", true},
		{"ML", "GLOBAL_EXPLAIN", true},
		{"ML", "RECOMMEND", true},
		{"ML", "TRANSFORM", true},
		{"ML", "PRINCIPAL_COMPONENTS", true},
		{"ML", "HOLIDAY", true},
		{"ML", "INVALID", false},
		{"OTHER", "FORECAST", false},
	}
	for _, tc := range tcs {
		if got := bigquerycommon.IsSystemResource(tc.datasetID, tc.resourceID); got != tc.want {
			t.Errorf("IsSystemResource(%q, %q) = %v, want %v", tc.datasetID, tc.resourceID, got, tc.want)
		}
	}
}

func TestBQTypeStringFromToolType(t *testing.T) {
	tcs := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"string", "STRING", false},
		{"integer", "INT64", false},
		{"float", "FLOAT64", false},
		{"boolean", "BOOL", false},
		{"map", "STRUCT", false},
		{"invalid", "", true},
	}
	for _, tc := range tcs {
		got, err := bigquerycommon.BQTypeStringFromToolType(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("BQTypeStringFromToolType(%q) expected error, got nil", tc.in)
			}
		} else {
			if err != nil {
				t.Errorf("BQTypeStringFromToolType(%q) unexpected error: %v", tc.in, err)
			}
			if got != tc.want {
				t.Errorf("BQTypeStringFromToolType(%q) = %q, want %q", tc.in, got, tc.want)
			}
		}
	}
}
