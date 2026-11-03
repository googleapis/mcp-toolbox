---
title: "looker-add-dashboard-element"
type: docs
weight: 1
description: >
  "looker-add-dashboard-element" creates a dashboard element in the given dashboard.
---

## About

The `looker-add-dashboard-element` tool creates a new tile (element) within an existing Looker dashboard.
Tiles are added in the order this tool is called for a given `dashboard_id`.
It supports query-based tiles (`vis`, `data`), text/Markdown/Slate tiles (`text`), and extension/data app tiles (`extension`).

CRITICAL ORDER OF OPERATIONS:
1. Create the dashboard using `make_dashboard`.
2. Add any dashboard-level filters using `add_dashboard_filter`.
3. Then, add elements (tiles) using this tool.

DATA APP EXTENSION PUBLISHING LIFECYCLE:
To mount a Data App Extension on a dashboard:
1. Create or identify the target dashboard using `make_dashboard` (leave folder empty for personal folder) or `get_dashboards`.
2. Create the extension using `create_data_app_extension`, obtaining `extension_id` (`"db::<uuid>"`).
3. Mount the extension using this tool (`add_dashboard_element`) with `type: "extension"`, `extension_id: "db::<uuid>"`, and `title_hidden: true`.
4. Sizing: Use `update_dashboard_layout_component` to size the tile to full-bleed (`width: 24`, `height: 20`).

## Compatible Sources

{{< compatible-sources >}}

## Example

```yaml
kind: tool
name: add_dashboard_element
type: looker-add-dashboard-element
source: looker-source
description: |
  This tool creates a new tile (element) within an existing Looker dashboard.
  Tiles are added in the order this tool is called for a given `dashboard_id`.
  It supports query-based tiles (`vis`, `data`), text/Markdown/Slate tiles (`text`),
  and extension/data app tiles (`extension`).

  CRITICAL ORDER OF OPERATIONS:
  1. Create the dashboard using `make_dashboard`.
  2. Add any dashboard-level filters using `add_dashboard_filter`.
  3. Then, add elements (tiles) using this tool.

  DATA APP EXTENSION PUBLISHING LIFECYCLE:
  To mount a Data App Extension on a dashboard:
  1. Create or identify the target dashboard using `make_dashboard` (leave folder empty for personal folder) or `get_dashboards`.
  2. Create the extension using `create_data_app_extension`, obtaining `extension_id` (`"db::<uuid>"`).
  3. Mount the extension using this tool (`add_dashboard_element`) with `type: "extension"`, `extension_id: "db::<uuid>"`, and `title_hidden: true`.
  4. Sizing: Use `update_dashboard_layout_component` to size the tile to full-bleed (`width: 24`, `height: 20`).

  Required Parameters:
  - dashboard_id: The ID of the target dashboard, obtained from `make_dashboard`.
  - For query tiles (`type` omitted, `'vis'`, or `'data'`): `model`, `explore`, and `fields` are required to define the query for the tile.
  - For extension tiles (`type: 'extension'`): `extension_id` is required (e.g. `'db::<id>'` or `'app::<id>'`). For extension tiles, query fields (`model`, `explore`, `fields`) should be omitted as the extension manages its own queries via the Looker SDK.

  Optional Parameters:
  - type: The type of dashboard element (`'vis'`, `'data'`, `'text'`, or `'extension'`). Note that `'extension'` is automatically inferred when `extension_id` is provided; `'text'` is inferred when `model` and `explore` are empty and `body_text` or `title_text` is provided; otherwise `'vis'` when `vis_config` is non-empty and `'data'` otherwise.
  - extension_id: The ID of the extension (e.g. `'db::<id>'` from `create_data_app_extension` or `'app::<id>'` from LookML extensions) for `'extension'` tiles. For extension tiles, query fields (`model`, `explore`, `fields`) should be omitted as the extension manages its own queries via the Looker SDK.
  - title: An optional title for query-based or extension dashboard tiles.
  - title_text: The header title text for `'text'` tiles.
  - subtitle_text: The subtitle text displayed below the title.
  - body_text: The text, Markdown, HTML, or Slate JSON body content for `'text'` tiles.
  - rich_content_json: A JSON string containing properties for rich text (Slate) elements (e.g., `'{"format":"slate"}'`).
  - note_text: Note text attached to the tile.
  - note_display: Where the note appears on the tile (`'above'`, `'below'`, or `'hover'`).
  - note_state: The display state of the note (`'expanded'` or `'collapsed'`).
  - title_hidden: Whether to hide the tile title (`true`/`false`).
  - refresh_interval: Auto-refresh interval for the tile (e.g., `'1 hour'`).
  - pivots, filters, filter_expression, dynamic_fields, sorts, limit, tz: Query parameters inherited from the `query` tool to customize a query tile.
  - vis_config: A JSON object defining the visualization settings for this tile.
    The structure and options are the same as for the `query_url` tool's `vis_config`.

  Connecting to Dashboard Filters:
  A dashboard element can be connected to one or more dashboard filters (created with
  `add_dashboard_filter`). To do this, specify the `dashboard_filter_name` of the dashboard filter
  and the `field` from the element's query that the filter should apply to in `dashboard_filters`.
  The format for specifying the field is `view_name.field_name`.
```

## Reference

| **field**   | **type** | **required** | **description**                                    |
|:------------|:--------:|:------------:|----------------------------------------------------|
| type        | string   | true         | Must be "looker-add-dashboard-element".            |
| source      | string   | true         | Name of the source the SQL should execute on.      |
| description | string   | true         | Description of the tool that is passed to the LLM. |