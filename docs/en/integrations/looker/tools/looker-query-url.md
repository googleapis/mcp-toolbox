---
title: "looker-query-url"
type: docs
weight: 1
description: >
  "looker-query-url" generates a url link to a Looker explore.
---

## About

The `looker-query-url` generates a url link to an explore in
Looker so the query can be investigated further.

`looker-query-url` takes eleven parameters:

1. the `model`
2. the `explore`
3. the `fields` list
4. an optional set of `filters`
5. an optional `filter_expression`
6. an optional `dynamic_fields`
7. an optional set of `pivots`
8. an optional set of `sorts`
9. an optional `limit`
10. an optional `tz`
11. an optional `vis_config`

## Compatible Sources

{{< compatible-sources >}}

## Example

```yaml
kind: tool
name: query_url
type: looker-query-url
source: looker-source
description: |
  This tool generates a shareable URL for a Looker query, allowing users to
  explore the query further within the Looker UI. It returns the generated URL,
  along with the `query_id` and `slug`.

  Parameters:
  All query parameters (e.g., `model`, `explore`, `fields`, `pivots`,
  `filters`, `filter_expression`, `dynamic_fields`, `sorts`, `limit`, `tz`) are the same as the `query` tool.

  Additionally, it accepts an optional `vis_config` parameter:
  - vis_config (optional): A JSON object that controls the default visualization
    settings for the generated query.

  vis_config Details:
  The `vis_config` object supports a wide range of properties for various chart types.
  Here are some notes on making visualizations.

  ### Cartesian Charts (Area, Bar, Column, Line, Scatter)

  These chart types share a large number of configuration options.

  **General**
  *   `type`: The type of visualization (`looker_area`, `looker_bar`, `looker_column`, `looker_line`, `looker_scatter`).
  *   `series_types`: Override the chart type for individual series.
  *   `show_view_names`: Display view names in labels and tooltips (`true`/`false`).
  *   `series_labels`: Provide custom names for series.

  **Styling & Colors**
  *   `colors`: An array of color values to be used for the chart series.
  *   `series_colors`: A mapping of series names to specific color values.
  *   `color_application`: Advanced controls for color palette application (collection, palette, reverse, etc.).
  *   `font_size`: Font size for labels (e.g., '12px').

  **Legend**
  *   `hide_legend`: Show or hide the chart legend (`true`/`false`).
  *   `legend_position`: Placement of the legend (`'center'`, `'left'`, `'right'`).

  **Axes**
  *   `swap_axes`: Swap the X and Y axes (`true`/`false`).
  *   `x_axis_scale`: Scale of the x-axis (`'auto'`, `'ordinal'`, `'linear'`, `'time'`).
  *   `x_axis_reversed`, `y_axis_reversed`: Reverse the direction of an axis (`true`/`false`).
  *   `x_axis_gridlines`, `y_axis_gridlines`: Display gridlines for an axis (`true`/`false`).
  *   `show_x_axis_label`, `show_y_axis_label`: Show or hide the axis title (`true`/`false`).
  *   `show_x_axis_ticks`, `show_y_axis_ticks`: Show or hide axis tick marks (`true`/`false`).
  *   `x_axis_label`, `y_axis_label`: Set a custom title for an axis.
  *   `x_axis_datetime_label`: A format string for datetime labels on the x-axis (e.g., `'%Y-%m'`).
  *   `x_padding_left`, `x_padding_right`: Adjust padding on the ends of the x-axis.
  *   `x_axis_label_rotation`, `x_axis_label_rotation_bar`: Set rotation for x-axis labels.
  *   `x_axis_zoom`, `y_axis_zoom`: Enable zooming on an axis (`true`/`false`).
  *   `y_axes`: An array of configuration objects for multiple y-axes.

  **Data & Series**
  *   `stacking`: How to stack series (`''` for none, `'normal'`, `'percent'`).
  *   `ordering`: Order of series in a stack (`'none'`, etc.).
  *   `limit_displayed_rows`: Enable or disable limiting the number of rows displayed (`true`/`false`).
  *   `limit_displayed_rows_values`: Configuration for the row limit (e.g., `{ "first_last": "first", "show_hide": "show", "num_rows": 10 }`).
  *   `discontinuous_nulls`: How to render null values in line charts (`true`/`false`).
  *   `point_style`: Style for points on line and area charts (`'none'`, `'circle'`, `'circle_outline'`).
  *   `series_point_styles`: Override point styles for individual series.
  *   `interpolation`: Line interpolation style (`'linear'`, `'monotone'`, `'step'`, etc.).
  *   `show_value_labels`: Display values on data points (`true`/`false`).
  *   `label_value_format`: A format string for value labels.
  *   `show_totals_labels`: Display total labels on stacked charts (`true`/`false`).
  *   `totals_color`: Color for total labels.
  *   `show_silhouette`: Display a "silhouette" of hidden series in stacked charts (`true`/`false`).
  *   `hidden_series`: An array of series names to hide from the visualization.

  **Scatter/Bubble Specific**
  *   `size_by_field`: The field used to determine the size of bubbles.
  *   `color_by_field`: The field used to determine the color of bubbles.
  *   `plot_size_by_field`: Whether to display the size-by field in the legend.
  *   `cluster_points`: Group nearby points into clusters (`true`/`false`).
  *   `quadrants_enabled`: Display quadrants on the chart (`true`/`false`).
  *   `quadrant_properties`: Configuration for quadrant labels and colors.
  *   `custom_quadrant_value_x`, `custom_quadrant_value_y`: Set quadrant boundaries as a percentage.
  *   `custom_quadrant_point_x`, `custom_quadrant_point_y`: Set quadrant boundaries to a specific value.

  **Miscellaneous**
  *   `reference_lines`: Configuration for displaying reference lines.
  *   `trend_lines`: Configuration for displaying trend lines.
  *   `trellis`: Configuration for creating trellis (small multiple) charts.
  *   `crossfilterEnabled`, `crossfilters`: Configuration for cross-filtering interactions.

  ### Boxplot

  *   Inherits most of the Cartesian chart options.
  *   `type`: Must be `looker_boxplot`.

  ### Funnel

  *   `type`: Must be `looker_funnel`.
  *   `orientation`: How data is read (`'automatic'`, `'dataInRows'`, `'dataInColumns'`).
  *   `percentType`: How percentages are calculated (`'percentOfMaxValue'`, `'percentOfPriorRow'`).
  *   `labelPosition`, `valuePosition`, `percentPosition`: Placement of labels (`'left'`, `'right'`, `'inline'`, `'hidden'`).
  *   `labelColor`, `labelColorEnabled`: Set a custom color for labels.
  *   `labelOverlap`: Allow labels to overlap (`true`/`false`).
  *   `barColors`: An array of colors for the funnel steps.
  *   `color_application`: Advanced color palette controls.
  *   `crossfilterEnabled`, `crossfilters`: Configuration for cross-filtering.

  ### Pie / Donut

  *   `type`: Must be `looker_pie`.
  *   `value_labels`: Where to display values (`'legend'`, `'labels'`).
  *   `label_type`: The format of data labels (`'labPer'`, `'labVal'`, `'lab'`, `'val'`, `'per'`).
  *   `start_angle`, `end_angle`: The start and end angles of the pie chart.
  *   `inner_radius`: The inner radius, used to create a donut chart.
  *   `series_colors`, `series_labels`: Override colors and labels for specific slices.
  *   `color_application`: Advanced color palette controls.
  *   `crossfilterEnabled`, `crossfilters`: Configuration for cross-filtering.
  *   `advanced_vis_config`: A string containing JSON for advanced Highcharts configuration.

  ### Waterfall

  *   Inherits most of the Cartesian chart options.
  *   `type`: Must be `looker_waterfall`.
  *   `up_color`: Color for positive (increasing) values.
  *   `down_color`: Color for negative (decreasing) values.
  *   `total_color`: Color for the total bar.

  ### Word Cloud

  *   `type`: Must be `looker_wordcloud`.
  *   `rotation`: Enable random word rotation (`true`/`false`).
  *   `colors`: An array of colors for the words.
  *   `color_application`: Advanced color palette controls.
  *   `crossfilterEnabled`, `crossfilters`: Configuration for cross-filtering.

  These are some sample vis_config settings.

  A bar chart -
  {{
    "defaults_version": 1,
    "label_density": 25,
    "legend_position": "center",
    "limit_displayed_rows": false,
    "ordering": "none",
    "plot_size_by_field": false,
    "point_style": "none",
    "show_null_labels": false,
    "show_silhouette": false,
    "show_totals_labels": false,
    "show_value_labels": false,
    "show_view_names": false,
    "show_x_axis_label": true,
    "show_x_axis_ticks": true,
    "show_y_axis_labels": true,
    "show_y_axis_ticks": true,
    "stacking": "normal",
    "totals_color": "#808080",
    "trellis": "",
    "type": "looker_bar",
    "x_axis_gridlines": false,
    "x_axis_reversed": false,
    "x_axis_scale": "auto",
    "x_axis_zoom": true,
    "y_axis_combined": true,
    "y_axis_gridlines": true,
    "y_axis_reversed": false,
    "y_axis_scale_mode": "linear",
    "y_axis_tick_density": "default",
    "y_axis_tick_density_custom": 5,
    "y_axis_zoom": true
  }}

  A column chart with an option advanced_vis_config -
  {{
    "advanced_vis_config": "{ chart: { type: 'pie', spacingBottom: 50, spacingLeft: 50, spacingRight: 50, spacingTop: 50, }, legend: { enabled: false, }, plotOptions: { pie: { dataLabels: { enabled: true, format: '\u003cb\u003e{key}\u003c/b\u003e\u003cspan style=\"font-weight: normal\"\u003e - {percentage:.2f}%\u003c/span\u003e', }, showInLegend: false, }, }, series: [], }",
    "colors": [
      "grey"
    ],
    "defaults_version": 1,
    "hidden_fields": [],
    "label_density": 25,
    "legend_position": "center",
    "limit_displayed_rows": false,
    "note_display": "below",
    "note_state": "collapsed",
    "note_text": "Unsold inventory only",
    "ordering": "none",
    "plot_size_by_field": false,
    "point_style": "none",
    "series_colors": {},
    "show_null_labels": false,
    "show_silhouette": false,
    "show_totals_labels": false,
    "show_value_labels": true,
    "show_view_names": false,
    "show_x_axis_label": true,
    "show_x_axis_ticks": true,
    "show_y_axis_labels": true,
    "show_y_axis_ticks": true,
    "stacking": "normal",
    "totals_color": "#808080",
    "trellis": "",
    "type": "looker_column",
    "x_axis_gridlines": false,
    "x_axis_reversed": false,
    "x_axis_scale": "auto",
    "x_axis_zoom": true,
    "y_axes": [],
    "y_axis_combined": true,
    "y_axis_gridlines": true,
    "y_axis_reversed": false,
    "y_axis_scale_mode": "linear",
    "y_axis_tick_density": "default",
    "y_axis_tick_density_custom": 5,
    "y_axis_zoom": true
  }}

  A line chart -
  {{
    "defaults_version": 1,
    "hidden_pivots": {},
    "hidden_series": [],
    "interpolation": "linear",
    "label_density": 25,
    "legend_position": "center",
    "limit_displayed_rows": false,
    "plot_size_by_field": false,
    "point_style": "none",
    "series_types": {},
    "show_null_points": true,
    "show_value_labels": false,
    "show_view_names": false,
    "show_x_axis_label": true,
    "show_x_axis_ticks": true,
    "show_y_axis_labels": true,
    "show_y_axis_ticks": true,
    "stacking": "",
    "trellis": "",
    "type": "looker_line",
    "x_axis_gridlines": false,
    "x_axis_reversed": false,
    "x_axis_scale": "auto",
    "y_axis_combined": true,
    "y_axis_gridlines": true,
    "y_axis_reversed": false,
    "y_axis_scale_mode": "linear",
    "y_axis_tick_density": "default",
    "y_axis_tick_density_custom": 5
  }}

  An area chart -
  {{
    "defaults_version": 1,
    "interpolation": "linear",
    "label_density": 25,
    "legend_position": "center",
    "limit_displayed_rows": false,
    "plot_size_by_field": false,
    "point_style": "none",
    "series_types": {},
    "show_null_points": true,
    "show_silhouette": false,
    "show_totals_labels": false,
    "show_value_labels": false,
    "show_view_names": false,
    "show_x_axis_label": true,
    "show_x_axis_ticks": true,
    "show_y_axis_labels": true,
    "show_y_axis_ticks": true,
    "stacking": "normal",
    "totals_color": "#808080",
    "trellis": "",
    "type": "looker_area",
    "x_axis_gridlines": false,
    "x_axis_reversed": false,
    "x_axis_scale": "auto",
    "x_axis_zoom": true,
    "y_axis_combined": true,
    "y_axis_gridlines": true,
    "y_axis_reversed": false,
    "y_axis_scale_mode": "linear",
    "y_axis_tick_density": "default",
    "y_axis_tick_density_custom": 5,
    "y_axis_zoom": true
  }}

  A scatter plot -
  {{
    "cluster_points": false,
    "custom_quadrant_point_x": 5,
    "custom_quadrant_point_y": 5,
    "custom_value_label_column": "",
    "custom_x_column": "",
    "custom_y_column": "",
    "defaults_version": 1,
    "hidden_fields": [],
    "hidden_pivots": {},
    "hidden_points_if_no": [],
    "hidden_series": [],
    "interpolation": "linear",
    "label_density": 25,
    "legend_position": "center",
    "limit_displayed_rows": false,
    "limit_displayed_rows_values": {
      "first_last": "first",
      "num_rows": 0,
      "show_hide": "hide"
    },
    "plot_size_by_field": false,
    "point_style": "circle",
    "quadrant_properties": {
      "0": {
        "color": "",
        "label": "Quadrant 1"
      },
      "1": {
        "color": "",
        "label": "Quadrant 2"
      },
      "2": {
        "color": "",
        "label": "Quadrant 3"
      },
      "3": {
        "color": "",
        "label": "Quadrant 4"
      }
    },
    "quadrants_enabled": false,
    "series_labels": {},
    "series_types": {},
    "show_null_points": false,
    "show_value_labels": false,
    "show_view_names": true,
    "show_x_axis_label": true,
    "show_x_axis_ticks": true,
    "show_y_axis_labels": true,
    "show_y_axis_ticks": true,
    "size_by_field": "roi",
    "stacking": "normal",
    "swap_axes": true,
    "trellis": "",
    "type": "looker_scatter",
    "x_axis_gridlines": false,
    "x_axis_reversed": false,
    "x_axis_scale": "auto",
    "x_axis_zoom": true,
    "y_axes": [
      {
        "label": "",
        "orientation": "bottom",
        "series": [
          {
            "axisId": "Channel_0 - average_of_roi_first",
            "id": "Channel_0 - average_of_roi_first",
            "name": "Channel_0"
          },
          {
            "axisId": "Channel_1 - average_of_roi_first",
            "id": "Channel_1 - average_of_roi_first",
            "name": "Channel_1"
          },
          {
            "axisId": "Channel_2 - average_of_roi_first",
            "id": "Channel_2 - average_of_roi_first",
            "name": "Channel_2"
          },
          {
            "axisId": "Channel_3 - average_of_roi_first",
            "id": "Channel_3 - average_of_roi_first",
            "name": "Channel_3"
          },
          {
            "axisId": "Channel_4 - average_of_roi_first",
            "id": "Channel_4 - average_of_roi_first",
            "name": "Channel_4"
          }
        ],
        "showLabels": true,
        "showValues": true,
        "tickDensity": "custom",
        "tickDensityCustom": 100,
        "type": "linear",
        "unpinAxis": false
      }
    ],
    "y_axis_combined": true,
    "y_axis_gridlines": true,
    "y_axis_reversed": false,
    "y_axis_scale_mode": "linear",
    "y_axis_tick_density": "default",
    "y_axis_tick_density_custom": 5,
    "y_axis_zoom": true
  }}

  A single record visualization -
  {{
    "defaults_version": 1,
    "show_view_names": false,
    "type": "looker_single_record"
  }}

  A single value visualization -
  {{
    "comparison_reverse_colors": false,
    "comparison_type": "value",                                                                                                                                            "conditional_formatting_include_nulls": false,                                                                                                                         "conditional_formatting_include_totals": false,
    "custom_color": "#1A73E8",
    "custom_color_enabled": true,
    "defaults_version": 1,
    "enable_conditional_formatting": false,
    "series_types": {},
    "show_comparison": false,
    "show_comparison_label": true,
    "show_single_value_title": true,
    "single_value_title": "Total Clicks",
    "type": "single_value"
  }}

  A Pie chart -
  {{
    "defaults_version": 1,
    "label_density": 25,
    "label_type": "labPer",
    "legend_position": "center",
    "limit_displayed_rows": false,
    "ordering": "none",
    "plot_size_by_field": false,
    "point_style": "none",
    "series_types": {},
    "show_null_labels": false,
    "show_silhouette": false,
    "show_totals_labels": false,
    "show_value_labels": false,
    "show_view_names": false,
    "show_x_axis_label": true,
    "show_x_axis_ticks": true,
    "show_y_axis_labels": true,
    "show_y_axis_ticks": true,
    "stacking": "",
    "totals_color": "#808080",
    "trellis": "",
    "type": "looker_pie",
    "value_labels": "legend",
    "x_axis_gridlines": false,
    "x_axis_reversed": false,
    "x_axis_scale": "auto",
    "y_axis_combined": true,
    "y_axis_gridlines": true,
    "y_axis_reversed": false,
    "y_axis_scale_mode": "linear",
    "y_axis_tick_density": "default",
    "y_axis_tick_density_custom": 5
  }}

  The result is a JSON object with the id, slug, the url, and
  the long_url.
```

## Reference

| **field**   | **type** | **required** | **description**                                    |
|-------------|:--------:|:------------:|----------------------------------------------------|
| type        |  string  |     true     | Must be "looker-query-url"                         |
| source      |  string  |     true     | Name of the source the SQL should execute on.      |
| description |  string  |     true     | Description of the tool that is passed to the LLM. |
