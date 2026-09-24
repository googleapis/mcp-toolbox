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
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools/looker/lookercommon"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
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
