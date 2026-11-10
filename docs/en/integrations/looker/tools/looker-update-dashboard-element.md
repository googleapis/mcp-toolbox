---
title: "looker-update-dashboard-element"
type: docs
weight: 1
description: >
  "looker-update-dashboard-element" updates an existing element in a Looker dashboard.
---

## About

The `looker-update-dashboard-element` tool updates an existing element (tile) in a Looker dashboard. It supports updating query-based tiles (`vis`, `data`) as well as text, Markdown/HTML, and Slate rich-text tiles (`text`), including notes, subtitles, title visibility, and refresh intervals.

## Compatible Sources

{{< compatible-sources >}}

## Example

```yaml
kind: tool
name: update_dashboard_element
type: looker-update-dashboard-element
source: looker-source
description: |
  This tool updates an existing element (tile) in a Looker dashboard.
  It supports updating query-based tiles (`vis`, `data`) as well as text, Markdown/HTML, and Slate rich-text tiles (`text`).

  Required Parameters:
  - dashboard_id: The ID of the dashboard containing the element.
  - dashboard_element_id: The ID of the element to update.
  - For query tiles (`type` omitted, `'vis'`, or `'data'`): `model`, `explore`, and `fields` define the data for the tile.

  Optional Parameters:
  - type: The type of dashboard element (`'vis'`, `'data'`, or `'text'`).
  - title: The new title for query-based dashboard tiles.
  - title_text: The header title text for `'text'` tiles.
  - subtitle_text: The subtitle text displayed below the title.
  - body_text: The text, Markdown, HTML, or Slate JSON body content for `'text'` tiles.
  - rich_content_json: A JSON string containing properties for rich text (Slate) elements (e.g., `'{"format":"slate"}'`).
  - note_text: Note text attached to the tile.
  - note_display: Where the note appears on the tile (`'above'`, `'below'`, or `'hover'`).
  - note_state: The display state of the note (`'expanded'` or `'collapsed'`).
  - title_hidden: Whether to hide the tile title (`true`/`false`).
  - refresh_interval: Auto-refresh interval for the tile (e.g., `'1 hour'`).
  - pivots, filters, filter_expression, dynamic_fields, sorts, limit, tz: Query parameters that customize a query tile.
  - vis_config: A JSON object defining the visualization settings.
  - dashboard_filters: An array of dashboard filters to connect to this element.
```

## Reference

| **field**   | **type** | **required** | **description**                                    |
|:------------|:--------:|:------------:|----------------------------------------------------|
| type        | string   | true         | Must be "looker-update-dashboard-element".        |
| source      | string   | true         | Name of the Looker source.                         |
| description | string   | true         | Description of the tool that is passed to the LLM. |
