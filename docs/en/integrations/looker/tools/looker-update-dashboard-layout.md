---
title: "looker-update-dashboard-layout"
type: docs
weight: 1
description: >
  "looker-update-dashboard-layout" updates an existing Looker dashboard layout (tab), switching layout types or active state.
---

## About

The `looker-update-dashboard-layout` tool updates an existing Looker dashboard layout (`PATCH /api/4.0/dashboard_layouts/:id`). It allows changing the layout type, toggling whether the layout is active, or updating the layout display label.

When publishing Looker Data Applications, setting `type: "extension"` configures the layout as a frameless, 100% viewport container for hosting a single Data App Extension tile without 24-column grid constraints.

## Compatible Sources

{{< compatible-sources >}}

## Example

```yaml
kind: tool
name: update_dashboard_layout
type: looker-update-dashboard-layout
source: looker-source
description: |
  This tool updates an existing Looker dashboard layout (`PATCH /api/4.0/dashboard_layouts/:id`).
  It allows changing the layout type, toggling whether the layout is active, or updating the tab label.

  DATA APP FULL-BLEED LAYOUT:
  Setting `type: "extension"` configures the layout as a frameless, 100% viewport container
  for hosting a single Data App Extension tile without 24-column grid constraints.

  Parameters:
  - dashboard_layout_id (required): The ID of the dashboard layout to update.
  - type (optional): Layout type: 'newspaper' (responsive 24-column grid), 'extension' (frameless full-bleed single extension tile), or 'grid'.
  - active (optional): Whether this layout is the active layout for the dashboard.
  - label (optional): Display label for this layout tab.
```

## Reference

| **field**   | **type** | **required** | **description**                                    |
|:------------|:--------:|:------------:|----------------------------------------------------|
| type        | string   | true         | Must be "looker-update-dashboard-layout".          |
| source      | string   | true         | Name of the Looker source.                         |
| description | string   | true         | Description of the tool that is passed to the LLM. |
