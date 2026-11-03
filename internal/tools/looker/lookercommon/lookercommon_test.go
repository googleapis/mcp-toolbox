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

package lookercommon_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools/looker/lookercommon"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	"github.com/looker-open-source/sdk-codegen/go/rtl"
	v4 "github.com/looker-open-source/sdk-codegen/go/sdk/v4"
)

func TestExtractLookerFieldProperties(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	// Helper function to create string pointers
	stringPtr := func(s string) *string { return &s }
	stringArrayPtr := func(s []string) *[]string { return &s }
	boolPtr := func(b bool) *bool { return &b }

	tcs := []struct {
		desc   string
		fields []v4.LookmlModelExploreField
		want   []any
	}{
		{
			desc: "field with all properties including description",
			fields: []v4.LookmlModelExploreField{
				{
					Name:             stringPtr("dimension_name"),
					Type:             stringPtr("string"),
					Label:            stringPtr("Dimension Label"),
					LabelShort:       stringPtr("Dim Label"),
					Description:      stringPtr("This is a dimension description"),
					Suggestable:      boolPtr(true),
					SuggestExplore:   stringPtr("explore"),
					SuggestDimension: stringPtr("dimension"),
					Suggestions:      stringArrayPtr([]string{"foo", "bar", "baz"}),
					ValueFormat:      stringPtr("$#,##0.00"),
					ValueFormatName:  stringPtr("usd"),
				},
			},
			want: []any{
				map[string]any{
					"name":              "dimension_name",
					"type":              "string",
					"label":             "Dimension Label",
					"label_short":       "Dim Label",
					"description":       "This is a dimension description",
					"suggestable":       true,
					"suggest_explore":   "explore",
					"suggest_dimension": "dimension",
					"suggestions":       []string{"foo", "bar", "baz"},
					"value_format":      "$#,##0.00",
					"value_format_name": "usd",
				},
			},
		},
		{
			desc: "field with only value_format set",
			fields: []v4.LookmlModelExploreField{
				{
					Name:        stringPtr("measure_name"),
					Type:        stringPtr("number"),
					ValueFormat: stringPtr("0.00%"),
					// ValueFormatName is nil
				},
			},
			want: []any{
				map[string]any{
					"name":         "measure_name",
					"type":         "number",
					"value_format": "0.00%",
					// value_format_name should not be present in the map
				},
			},
		},
		{
			desc: "field with empty value formats",
			fields: []v4.LookmlModelExploreField{
				{
					Name:            stringPtr("measure_name"),
					Type:            stringPtr("number"),
					ValueFormat:     stringPtr(""),
					ValueFormatName: stringPtr(""),
				},
			},
			want: []any{
				map[string]any{
					"name": "measure_name",
					"type": "number",
					// neither value_format nor value_format_name should be present in the map
				},
			},
		},
		{
			desc: "field with only value_format_name set",
			fields: []v4.LookmlModelExploreField{
				{
					Name:            stringPtr("measure_name"),
					Type:            stringPtr("number"),
					ValueFormatName: stringPtr("percent_2"),
					// ValueFormat is nil
				},
			},
			want: []any{
				map[string]any{
					"name":              "measure_name",
					"type":              "number",
					"value_format_name": "percent_2",
					// value_format should not be present in the map
				},
			},
		},
		{
			desc: "field with missing description",
			fields: []v4.LookmlModelExploreField{
				{
					Name:       stringPtr("dimension_name"),
					Type:       stringPtr("string"),
					Label:      stringPtr("Dimension Label"),
					LabelShort: stringPtr("Dim Label"),
					// Description is nil
				},
			},
			want: []any{
				map[string]any{
					"name":        "dimension_name",
					"type":        "string",
					"label":       "Dimension Label",
					"label_short": "Dim Label",
					// description should not be present in the map
				},
			},
		},
		{
			desc: "field with only required fields",
			fields: []v4.LookmlModelExploreField{
				{
					Name: stringPtr("simple_dimension"),
					Type: stringPtr("number"),
				},
			},
			want: []any{
				map[string]any{
					"name": "simple_dimension",
					"type": "number",
				},
			},
		},
		{
			desc:   "empty fields list",
			fields: []v4.LookmlModelExploreField{},
			want:   []any{},
		},
		{
			desc: "multiple fields with mixed properties",
			fields: []v4.LookmlModelExploreField{
				{
					Name:        stringPtr("dim1"),
					Type:        stringPtr("string"),
					Label:       stringPtr("First Dimension"),
					Description: stringPtr("First dimension description"),
				},
				{
					Name:       stringPtr("dim2"),
					Type:       stringPtr("number"),
					LabelShort: stringPtr("Dim2"),
				},
			},
			want: []any{
				map[string]any{
					"name":        "dim1",
					"type":        "string",
					"label":       "First Dimension",
					"description": "First dimension description",
				},
				map[string]any{
					"name":        "dim2",
					"type":        "number",
					"label_short": "Dim2",
				},
			},
		},
	}

	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			got, err := lookercommon.ExtractLookerFieldProperties(ctx, &tc.fields, true)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatalf("incorrect result: diff %v", diff)
			}
		})
	}
}

func TestExtractLookerFieldPropertiesWithNilFields(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	got, err := lookercommon.ExtractLookerFieldProperties(ctx, nil, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []any{}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("incorrect result: diff %v", diff)
	}
}

func TestProcessQueryArgsStripsWrappingQuotes(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	tcs := []struct {
		desc       string
		filtersIn  map[string]any
		filtersOut map[string]any
	}{
		{
			desc:       "bare string value passed through unchanged",
			filtersIn:  map[string]any{"view.attribution_model": "first_touch"},
			filtersOut: map[string]any{"view.attribution_model": "first_touch"},
		},
		{
			desc:       "double-quoted value has wrapping quotes stripped",
			filtersIn:  map[string]any{"view.attribution_model": `"first_touch"`},
			filtersOut: map[string]any{"view.attribution_model": "first_touch"},
		},
		{
			desc:       "single-quoted value has wrapping quotes stripped",
			filtersIn:  map[string]any{"view.attribution_model": "'first_touch'"},
			filtersOut: map[string]any{"view.attribution_model": "first_touch"},
		},
		{
			desc:       "single-quoted key has wrapping quotes stripped",
			filtersIn:  map[string]any{"'view.field'": "value"},
			filtersOut: map[string]any{"view.field": "value"},
		},
		{
			desc:       "quoted key and quoted value are both stripped",
			filtersIn:  map[string]any{`"view.field"`: `"value"`},
			filtersOut: map[string]any{"view.field": "value"},
		},
		{
			desc:       "non-string values are not touched",
			filtersIn:  map[string]any{"view.threshold": 42, "view.enabled": true},
			filtersOut: map[string]any{"view.threshold": 42, "view.enabled": true},
		},
		{
			desc:       "non-comparable values are passed through without panic",
			filtersIn:  map[string]any{"view.ids": []any{"a", "b"}, "view.meta": map[string]any{"k": "v"}},
			filtersOut: map[string]any{"view.ids": []any{"a", "b"}, "view.meta": map[string]any{"k": "v"}},
		},
		{
			desc:       "single-character string is not mangled by the length check",
			filtersIn:  map[string]any{"view.code": "x"},
			filtersOut: map[string]any{"view.code": "x"},
		},
		{
			desc:       "mismatched wrapping characters are left alone",
			filtersIn:  map[string]any{"view.f": `"value'`},
			filtersOut: map[string]any{"view.f": `"value'`},
		},
	}

	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			params := parameters.ParamValues{
				{Name: "model", Value: "marketing"},
				{Name: "explore", Value: "cohort_marketing_performance"},
				{Name: "fields", Value: []any{"view.channel"}},
				{Name: "filters", Value: tc.filtersIn},
				{Name: "pivots", Value: []any{}},
				{Name: "sorts", Value: []any{}},
				{Name: "limit", Value: 10},
				{Name: "tz", Value: "Etc/UTC"},
			}
			wq, err := lookercommon.ProcessQueryArgs(ctx, params)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if wq.Filters == nil {
				t.Fatalf("expected non-nil Filters")
			}
			if diff := cmp.Diff(tc.filtersOut, *wq.Filters); diff != "" {
				t.Fatalf("incorrect filters: diff %v", diff)
			}
		})
	}
}

func TestEscapeFiltersForUnquotedParameters(t *testing.T) {
	unquoted := map[string]bool{
		"v.attribution_model_selector": true,
		"v.cohort_anchor_selector":     true,
		"v.period_type_selector":       true,
	}

	tcs := []struct {
		desc string
		in   map[string]any
		out  map[string]any
	}{
		{
			desc: "underscore in unquoted-parameter value is escaped",
			in:   map[string]any{"v.attribution_model_selector": "first_touch"},
			out:  map[string]any{"v.attribution_model_selector": "first^_touch"},
		},
		{
			desc: "multiple metacharacters escaped in one value",
			in:   map[string]any{"v.attribution_model_selector": "a_b%c,d"},
			out:  map[string]any{"v.attribution_model_selector": "a^_b^%c^,d"},
		},
		{
			desc: "already-escaped value passes through unchanged (idempotence)",
			in:   map[string]any{"v.cohort_anchor_selector": "signup^_date"},
			out:  map[string]any{"v.cohort_anchor_selector": "signup^_date"},
		},
		{
			desc: "mixed pre-escaped and unescaped metacharacters",
			in:   map[string]any{"v.attribution_model_selector": "first^_touch_v2"},
			out:  map[string]any{"v.attribution_model_selector": "first^_touch^_v2"},
		},
		{
			desc: "all four escape sequences pass through unchanged",
			in:   map[string]any{"v.attribution_model_selector": "a^_b^%c^,d^^e"},
			out:  map[string]any{"v.attribution_model_selector": "a^_b^%c^,d^^e"},
		},
		{
			desc: "lone caret followed by non-metachar is doubled",
			in:   map[string]any{"v.cohort_anchor_selector": "a^b"},
			out:  map[string]any{"v.cohort_anchor_selector": "a^^b"},
		},
		{
			desc: "trailing lone caret is doubled",
			in:   map[string]any{"v.cohort_anchor_selector": "tail^"},
			out:  map[string]any{"v.cohort_anchor_selector": "tail^^"},
		},
		{
			desc: "value with no metacharacters is unchanged",
			in:   map[string]any{"v.period_type_selector": "monthly"},
			out:  map[string]any{"v.period_type_selector": "monthly"},
		},
		{
			desc: "filter keyed to a non-parameter field is left alone",
			in:   map[string]any{"v.signup_date": "after 2026-01-01"},
			out:  map[string]any{"v.signup_date": "after 2026-01-01"},
		},
		{
			desc: "non-string values are not touched",
			in:   map[string]any{"v.attribution_model_selector": 42},
			out:  map[string]any{"v.attribution_model_selector": 42},
		},
		{
			desc: "mixed filters: unquoted is escaped, others pass through",
			in: map[string]any{
				"v.attribution_model_selector": "first_touch",
				"v.signup_date":                "after 2026-01-01",
				"v.user_count":                 ">= 100",
			},
			out: map[string]any{
				"v.attribution_model_selector": "first^_touch",
				"v.signup_date":                "after 2026-01-01",
				"v.user_count":                 ">= 100",
			},
		},
	}

	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			filters := map[string]any{}
			for k, v := range tc.in {
				filters[k] = v
			}
			wq := &v4.WriteQuery{Model: "m", View: "v", Filters: &filters}
			lookercommon.EscapeFiltersForUnquotedParameters(wq, unquoted)
			if diff := cmp.Diff(tc.out, *wq.Filters); diff != "" {
				t.Fatalf("incorrect filters: diff %v", diff)
			}
		})
	}
}

func TestEscapeFiltersForUnquotedParameters_NoopGuards(t *testing.T) {
	// Empty unquoted set: should not touch filters.
	filters := map[string]any{"v.x": "a_b"}
	wq := &v4.WriteQuery{Model: "m", View: "v", Filters: &filters}
	lookercommon.EscapeFiltersForUnquotedParameters(wq, map[string]bool{})
	if got := (*wq.Filters)["v.x"]; got != "a_b" {
		t.Fatalf("expected empty unquoted set to be a no-op, got %v", got)
	}

	// nil Filters pointer: must not panic.
	lookercommon.EscapeFiltersForUnquotedParameters(&v4.WriteQuery{}, map[string]bool{"v.x": true})

	// nil WriteQuery: must not panic.
	lookercommon.EscapeFiltersForUnquotedParameters(nil, map[string]bool{"v.x": true})
}

func TestRequestRunInlineQuery2(t *testing.T) {
	fields := make([]string, 1)
	fields[0] = "foo.bar"
	wq := v4.WriteQuery{
		Model:  "model",
		View:   "explore",
		Fields: &fields,
	}
	req2 := lookercommon.RequestRunInlineQuery2{
		Query: wq,
		RenderOpts: lookercommon.RenderOptions{
			Format: "json",
		},
		QueryApiClientCtx: lookercommon.QueryApiClientContext{
			Name: "MCP Toolbox",
		},
	}
	json, err := json.Marshal(req2)
	if err != nil {
		t.Fatalf("Could not marshall req2 as json")
	}
	got := string(json)
	want := `{"query":{"model":"model","view":"explore","fields":["foo.bar"]},"render_options":{"format":"json"},"query_api_client_context":{"name":"MCP Toolbox"}}`
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("incorrect result: diff %v", diff)
	}
}

func TestProcessQueryArgsWithFilterExpression(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	tcs := []struct {
		desc             string
		filterExpression any
		wantVal          *string
		wantErr          bool
	}{
		{
			desc:             "filter expression is nil",
			filterExpression: nil,
			wantVal:          nil,
			wantErr:          false,
		},
		{
			desc:             "filter expression is valid string",
			filterExpression: "matches_filter(${order.order_month}, `24 months`)",
			wantVal:          func() *string { s := "matches_filter(${order.order_month}, `24 months`)"; return &s }(),
			wantErr:          false,
		},
		{
			desc:             "filter expression is not a string",
			filterExpression: 123,
			wantVal:          nil,
			wantErr:          true,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			params := parameters.ParamValues{
				{Name: "model", Value: "marketing"},
				{Name: "explore", Value: "cohort_marketing_performance"},
				{Name: "fields", Value: []any{"view.channel"}},
				{Name: "filters", Value: map[string]any{}},
				{Name: "pivots", Value: []any{}},
				{Name: "sorts", Value: []any{}},
				{Name: "limit", Value: 10},
				{Name: "tz", Value: "Etc/UTC"},
			}
			if tc.filterExpression != nil {
				params = append(params, parameters.ParamValue{Name: "filter_expression", Value: tc.filterExpression})
			}
			wq, err := lookercommon.ProcessQueryArgs(ctx, params)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if wq.FilterExpression == nil && tc.wantVal != nil {
				t.Fatalf("expected FilterExpression %v, got nil", *tc.wantVal)
			}
			if wq.FilterExpression != nil && tc.wantVal == nil {
				t.Fatalf("expected FilterExpression nil, got %v", *wq.FilterExpression)
			}
			if wq.FilterExpression != nil && tc.wantVal != nil {
				if *wq.FilterExpression != *tc.wantVal {
					t.Fatalf("expected FilterExpression %v, got %v", *tc.wantVal, *wq.FilterExpression)
				}
			}
		})
	}
}

func TestProcessQueryArgsWithDynamicFields(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	tcs := []struct {
		desc          string
		dynamicFields any
		wantVal       *string
		wantErr       bool
	}{
		{
			desc:          "dynamic fields is nil",
			dynamicFields: nil,
			wantVal:       nil,
			wantErr:       false,
		},
		{
			desc: "dynamic fields is valid array of maps",
			dynamicFields: []any{
				map[string]any{
					"category":          "table_calculation",
					"expression":        "${order_items.total_sale_price} * 0.8",
					"label":             "test",
					"table_calculation": "test",
					"_type_hint":        "number",
				},
			},
			wantVal: func() *string {
				s := `[{"_type_hint":"number","category":"table_calculation","expression":"${order_items.total_sale_price} * 0.8","label":"test","table_calculation":"test"}]`
				return &s
			}(),
			wantErr: false,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			params := parameters.ParamValues{
				{Name: "model", Value: "marketing"},
				{Name: "explore", Value: "cohort_marketing_performance"},
				{Name: "fields", Value: []any{"view.channel"}},
				{Name: "filters", Value: map[string]any{}},
				{Name: "pivots", Value: []any{}},
				{Name: "sorts", Value: []any{}},
				{Name: "limit", Value: 10},
				{Name: "tz", Value: "Etc/UTC"},
			}
			if tc.dynamicFields != nil {
				params = append(params, parameters.ParamValue{Name: "dynamic_fields", Value: tc.dynamicFields})
			}
			wq, err := lookercommon.ProcessQueryArgs(ctx, params)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if wq.DynamicFields == nil && tc.wantVal != nil {
				t.Fatalf("expected DynamicFields %v, got nil", *tc.wantVal)
			}
			if wq.DynamicFields != nil && tc.wantVal == nil {
				t.Fatalf("expected DynamicFields nil, got %v", *wq.DynamicFields)
			}
			if wq.DynamicFields != nil && tc.wantVal != nil {
				var gotObj, wantObj any
				if err := json.Unmarshal([]byte(*wq.DynamicFields), &gotObj); err != nil {
					t.Fatalf("failed to unmarshal got dynamic fields: %v", err)
				}
				if err := json.Unmarshal([]byte(*tc.wantVal), &wantObj); err != nil {
					t.Fatalf("failed to unmarshal want dynamic fields: %v", err)
				}
				if diff := cmp.Diff(wantObj, gotObj); diff != "" {
					t.Fatalf("incorrect DynamicFields: diff %v", diff)
				}
			}
		})
	}
}

func TestStripDrillLinks(t *testing.T) {
	detailResp := map[string]any{
		"sql":             "SELECT * FROM orders",
		"applied_filters": map[string]any{"orders.status": "complete"},
		"from_cache":      true,
		"data": []any{
			map[string]any{
				"orders.status": map[string]any{
					"value":            "complete",
					"filterable_value": "complete",
					"links":            []any{map[string]any{"label": "Drill", "url": "/explore/x"}},
				},
				"orders.count": map[string]any{
					"2025": map[string]any{
						"value":            42,
						"rendered":         "42",
						"filterable_value": "42",
						"links":            []any{map[string]any{"label": "Show All 42", "url": "/explore/y"}},
					},
				},
			},
		},
		"pivots": []any{
			map[string]any{
				"key": "2025",
				"metadata": map[string]any{
					"orders.created_year": map[string]any{
						"value":            "2025",
						"filterable_value": "2025",
						"links":            []any{map[string]any{"label": "Drill Year", "url": "/explore/p"}},
					},
				},
			},
		},
		"totals_data": map[string]any{
			"orders.count": map[string]any{
				"value":            42,
				"filterable_value": "42",
				"links":            []any{map[string]any{"label": "Total Drill", "url": "/explore/z"}},
			},
		},
		"fields": map[string]any{
			"measures": []any{
				map[string]any{
					"name":                        "orders.count",
					"label":                       "Order Count",
					"drill_fields":                []any{"orders.id", "orders.created_date"},
					"sql":                         "${TABLE}.id",
					"can":                         map[string]any{"filter": true},
					"user_attribute_filter_types": []any{"number"},
					"available_custom_timeframes": []any{"day"},
					"suggest_dimension":           "orders.count",
				},
			},
		},
	}

	lookercommon.StripDrillLinks(detailResp)

	for _, topKey := range []string{"sql", "applied_filters", "from_cache"} {
		if _, exists := detailResp[topKey]; exists {
			t.Errorf("expected top-level %q to be stripped from detailResp", topKey)
		}
	}

	row := detailResp["data"].([]any)[0].(map[string]any)
	statusCell := row["orders.status"].(map[string]any)
	for _, k := range []string{"links", "filterable_value"} {
		if _, exists := statusCell[k]; exists {
			t.Errorf("expected %q to be stripped from flat cell, got %v", k, statusCell)
		}
	}
	if statusCell["value"] != "complete" {
		t.Errorf("expected value to be preserved, got %v", statusCell["value"])
	}

	pivotCell := row["orders.count"].(map[string]any)["2025"].(map[string]any)
	for _, k := range []string{"links", "filterable_value"} {
		if _, exists := pivotCell[k]; exists {
			t.Errorf("expected %q to be stripped from pivoted cell, got %v", k, pivotCell)
		}
	}
	if pivotCell["value"] != 42 {
		t.Errorf("expected pivoted cell value 42 to be preserved, got %v", pivotCell["value"])
	}

	pivotMeta := detailResp["pivots"].([]any)[0].(map[string]any)["metadata"].(map[string]any)["orders.created_year"].(map[string]any)
	for _, k := range []string{"links", "filterable_value"} {
		if _, exists := pivotMeta[k]; exists {
			t.Errorf("expected %q to be stripped from pivot metadata cell, got %v", k, pivotMeta)
		}
	}

	totalsCell := detailResp["totals_data"].(map[string]any)["orders.count"].(map[string]any)
	for _, k := range []string{"links", "filterable_value"} {
		if _, exists := totalsCell[k]; exists {
			t.Errorf("expected %q to be stripped from totals_data cell, got %v", k, totalsCell)
		}
	}

	measureField := detailResp["fields"].(map[string]any)["measures"].([]any)[0].(map[string]any)
	for _, strippedKey := range []string{"drill_fields", "sql", "can", "user_attribute_filter_types", "available_custom_timeframes", "suggest_dimension"} {
		if _, exists := measureField[strippedKey]; exists {
			t.Errorf("expected %q to be stripped from field metadata, got %v", strippedKey, measureField)
		}
	}
	if measureField["name"] != "orders.count" || measureField["label"] != "Order Count" {
		t.Errorf("expected essential field metadata to be preserved, got %v", measureField)
	}
}

func TestDataAppExtensionValidationHelpers(t *testing.T) {
	t.Run("GenerateDataAppExtensionName", func(t *testing.T) {
		got := lookercommon.GenerateDataAppExtensionName("Revenue & Margin Explorer (Q4)!")
		if !strings.HasPrefix(got, "data-app-revenue-margin-explorer-q4-") {
			t.Errorf("unexpected generated name prefix: %q", got)
		}
		if err := lookercommon.ValidateDataAppExtensionName(got); err != nil {
			t.Errorf("generated name %q failed validation: %v", got, err)
		}

		fallback := lookercommon.GenerateDataAppExtensionName("   !!!   ")
		if !strings.HasPrefix(fallback, "data-app-untitled-") {
			t.Errorf("expected untitled fallback prefix, got %q", fallback)
		}

		longLabel := strings.Repeat("a", 200)
		longName := lookercommon.GenerateDataAppExtensionName(longLabel)
		if err := lookercommon.ValidateDataAppExtensionName(longName); err != nil {
			t.Errorf("generated long-label name %q failed validation: %v", longName, err)
		}
	})

	t.Run("ValidateDataAppExtensionName", func(t *testing.T) {
		if err := lookercommon.ValidateDataAppExtensionName("valid_App-Name_01"); err != nil {
			t.Errorf("unexpected error for valid name: %v", err)
		}
		for _, bad := range []string{"", "has space", "bad/slash", strings.Repeat("x", 256)} {
			if err := lookercommon.ValidateDataAppExtensionName(bad); err == nil {
				t.Errorf("expected error for invalid name %q, got nil", bad)
			}
		}
	})

	t.Run("NormalizeDataAppExtensionID", func(t *testing.T) {
		id, err := lookercommon.NormalizeDataAppExtensionID("db::550e8400-e29b-41d4-a716-446655440000")
		if err != nil || id != "550e8400-e29b-41d4-a716-446655440000" {
			t.Fatalf("expected stripped UUID, got %q (err=%v)", id, err)
		}
		id, err = lookercommon.NormalizeDataAppExtensionID("550e8400-e29b-41d4-a716-446655440000")
		if err != nil || id != "550e8400-e29b-41d4-a716-446655440000" {
			t.Fatalf("expected raw UUID, got %q (err=%v)", id, err)
		}
		for _, bad := range []string{"", "db::", "../inline_extension/123", "id?dashboard_id=1", "id/bundle.js"} {
			if _, err := lookercommon.NormalizeDataAppExtensionID(bad); err == nil {
				t.Errorf("expected path/format error for %q, got nil", bad)
			}
		}
	})

	t.Run("ValidateDataAppExtensionCodeAndLabel", func(t *testing.T) {
		if err := lookercommon.ValidateDataAppExtensionLabel("My Data App"); err != nil {
			t.Errorf("unexpected label error: %v", err)
		}
		if err := lookercommon.ValidateDataAppExtensionLabel("   "); err == nil {
			t.Errorf("expected empty label error")
		}
		if err := lookercommon.ValidateDataAppExtensionLabel(strings.Repeat("é", 256)); err == nil {
			t.Errorf("expected >255 rune label error")
		}
		if err := lookercommon.ValidateDataAppExtensionCode("export default () => null;"); err != nil {
			t.Errorf("unexpected code error: %v", err)
		}
		if err := lookercommon.ValidateDataAppExtensionCode("   "); err == nil {
			t.Errorf("expected empty code error")
		}
		if err := lookercommon.ValidateDataAppExtensionCode(strings.Repeat("a", lookercommon.MaxDataAppExtensionCodeBytes+1)); err == nil {
			t.Errorf("expected >5MB code error")
		}
	})
}

func TestCreateAndUpdateDataAppExtensionTransport(t *testing.T) {
	var lastMethod, lastPath, lastRawQuery string
	var lastBody map[string]any

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/4.0/login" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "mock-token",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
			return
		}
		lastMethod = r.Method
		lastPath = r.URL.Path
		lastRawQuery = r.URL.RawQuery
		lastBody = nil
		_ = json.NewDecoder(r.Body).Decode(&lastBody)

		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":           "abc-123",
			"name":         "data-app-sales-abc",
			"label":        "Sales App",
			"user_id":      7,
			"code_size":    42,
			"mount_points": map[string]any{"dashboard_tile": true},
			"entitlements": map[string]any{
				"core_api_methods":  []string{"run_inline_query", "dashboard", "dashboard_element"},
				"external_api_urls": []string{},
			},
		})
	}))
	defer ts.Close()

	settings := rtl.ApiSettings{
		BaseUrl:      ts.URL,
		ApiVersion:   "4.0",
		ClientId:     "id",
		ClientSecret: "secret",
	}
	sdk := v4.NewLookerSDK(rtl.NewAuthSession(settings))

	name := "data-app-sales-abc"
	label := "Sales App"
	code := "console.log('hi');"
	created, err := lookercommon.CreateDataAppExtension(sdk, lookercommon.WriteDataAppExtension{
		Name:  &name,
		Label: &label,
		Code:  &code,
	}, &settings)
	if err != nil {
		t.Fatalf("CreateDataAppExtension failed: %v", err)
	}
	if lastMethod != http.MethodPost || lastPath != "/api/4.0/data_app_extension" {
		t.Fatalf("unexpected request %s %s", lastMethod, lastPath)
	}
	if created.ExtensionId != "db::abc-123" {
		t.Errorf("expected ExtensionId 'db::abc-123', got %q", created.ExtensionId)
	}
	sentCode, _ := lastBody["code"].(string)
	if !strings.Contains(sentCode, "window.__lookerDataAppMeta") || !strings.Contains(sentCode, "publishedAt") || !strings.HasSuffix(sentCode, "console.log('hi');") {
		t.Errorf("expected __lookerDataAppMeta header prepended to code, got %q", sentCode)
	}

	updatedLabel := "Sales App v2"
	updated, err := lookercommon.UpdateDataAppExtension(sdk, "db::abc-123", "99", lookercommon.WriteDataAppExtension{
		Label: &updatedLabel,
	}, &settings)
	if err != nil {
		t.Fatalf("UpdateDataAppExtension failed: %v", err)
	}
	if lastMethod != http.MethodPatch || lastPath != "/api/4.0/data_app_extension/abc-123" || lastRawQuery != "dashboard_id=99" {
		t.Fatalf("unexpected PATCH request: %s %s?%s", lastMethod, lastPath, lastRawQuery)
	}
	if _, hasName := lastBody["name"]; hasName {
		t.Errorf("expected omitted 'name' in PATCH body, got %v", lastBody)
	}
	if updated.ExtensionId != "db::abc-123" {
		t.Errorf("expected ExtensionId 'db::abc-123', got %q", updated.ExtensionId)
	}
}

func TestEnsureDataAppMeta(t *testing.T) {
	raw := "export default class MyApp {};"
	stamped := lookercommon.EnsureDataAppMeta(raw)
	if !strings.HasPrefix(stamped, "window.__lookerDataAppMeta = Object.assign(") {
		t.Fatalf("expected __lookerDataAppMeta prefix, got %q", stamped)
	}
	if !strings.HasSuffix(stamped, raw) {
		t.Fatalf("expected original code preserved at end, got %q", stamped)
	}
	if again := lookercommon.EnsureDataAppMeta(stamped); again != stamped {
		t.Fatalf("expected EnsureDataAppMeta to be idempotent, got %q", again)
	}
}

func TestConfigureExtensionDashboard(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var elementPatched, layoutPatched, dashPatched bool
	var gotLayoutBody, gotDashBody map[string]any

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/4.0/login":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "mock-token",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
		case r.Method == http.MethodPatch && r.URL.Path == "/api/4.0/dashboard_elements/1580":
			elementPatched = true
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "1580", "title_hidden": true})
		case r.Method == http.MethodGet && r.URL.Path == "/api/4.0/dashboards/233":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "233",
				"dashboard_layouts": []map[string]any{
					{"id": "369", "type": "newspaper", "active": true},
				},
			})
		case r.Method == http.MethodPatch && r.URL.Path == "/api/4.0/dashboard_layouts/369":
			layoutPatched = true
			_ = json.NewDecoder(r.Body).Decode(&gotLayoutBody)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "369", "type": "extension"})
		case r.Method == http.MethodPatch && r.URL.Path == "/api/4.0/dashboards/233":
			dashPatched = true
			_ = json.NewDecoder(r.Body).Decode(&gotDashBody)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "233"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	settings := rtl.ApiSettings{
		BaseUrl:      ts.URL,
		ApiVersion:   "4.0",
		ClientId:     "id",
		ClientSecret: "secret",
	}
	sdk := v4.NewLookerSDK(rtl.NewAuthSession(settings))
	hide := true
	lookercommon.ConfigureExtensionDashboard(ctx, sdk, "233", "1580", &hide, &settings)

	if !elementPatched {
		t.Errorf("expected PATCH /dashboard_elements/1580 to be called")
	}
	if !layoutPatched || gotLayoutBody["type"] != "extension" {
		t.Errorf("expected PATCH /dashboard_layouts/369 with type='extension', got %v", gotLayoutBody)
	}
	if !dashPatched {
		t.Fatalf("expected PATCH /dashboards/233 to be called")
	}
	appMap, ok := gotDashBody["appearance"].(map[string]any)
	if !ok {
		t.Fatalf("expected appearance object in PATCH /dashboards/233 body, got %v", gotDashBody)
	}
	if appMap["show_dashboard_header"] != false || appMap["page_side_margins"] != float64(0) || appMap["tile_space_between"] != float64(0) || appMap["tile_shadow"] != false {
		t.Errorf("unexpected appearance payload: %v", appMap)
	}
}
