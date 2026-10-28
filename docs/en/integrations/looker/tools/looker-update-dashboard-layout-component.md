---
title: "looker-update-dashboard-layout-component"
type: docs
weight: 1
description: >
  "looker-update-dashboard-layout-component" updates a dashboard layout component.
---

## About

The `looker-update-dashboard-layout-component` tool updates a dashboard layout component, allowing you to move a tile to a different tab (layout) and/or change its position and size.

## Compatible Sources

{{< compatible-sources >}}

## Example

```yaml
kind: tool
name: update_dashboard_layout_component
type: looker-update-dashboard-layout-component
source: looker-source
description: |
  This tool updates a dashboard layout component, allowing you to move a tile
  to a different tab (layout) and/or change its position and size.

  Parameters:
  - dashboard_layout_component_id (required): The ID of the component to update.
  - dashboard_layout_id (optional): The ID of the target layout (tab) to move to.
  - row (optional): The new row position.
  - column (optional): The new column position.
  - width (optional): The new width.
  - height (optional): The new height.
```

## Reference

| **field**   | **type** | **required** | **description**                                    |
|:------------|:--------:|:------------:|----------------------------------------------------|
| type        | string   | true         | Must be "looker-update-dashboard-layout-component". |
| source      | string   | true         | Name of the Looker source.                         |
| description | string   | true         | Description of the tool that is passed to the LLM. |
