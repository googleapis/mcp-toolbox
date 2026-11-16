---
title: "looker-import-dashboard-from-lookml"
type: docs
weight: 1
description: >
  "looker-import-dashboard-from-lookml" imports a LookML dashboard YAML definition
  to create or overwrite a user-defined dashboard in Looker.
---

## About

The `looker-import-dashboard-from-lookml` tool imports a LookML dashboard YAML
definition to create or overwrite a user-defined dashboard in Looker.

If a dashboard already exists with the YAML-defined `preferred_slug`, it will be
overwritten. Otherwise, a new dashboard will be created.

`looker-import-dashboard-from-lookml` takes two parameters:

1. `lookml` (required): The LookML YAML definition of the dashboard to import.
2. `folder` (optional): The folder ID where the dashboard will be created. If not provided, the user's personal folder will be used for new dashboards, and existing folders are preserved when overwriting via `preferred_slug`.

## Compatible Sources

{{< compatible-sources >}}

## Example

```yaml
kind: tool
name: import_dashboard_from_lookml
type: looker-import-dashboard-from-lookml
source: looker-source
description: |
  This tool imports a LookML dashboard YAML definition to create or overwrite a
  user-defined dashboard in Looker.

  If a dashboard already exists with the YAML-defined `preferred_slug`, it will
  be overwritten. Otherwise, a new dashboard will be created.

  Required Parameters:
  - lookml (required): Valid LookML dashboard YAML string defining the dashboard, filters, and elements.

  Optional Parameters:
  - folder (optional): Destination folder ID. If omitted, new dashboards are placed in the user's personal folder and overwritten dashboards remain in their existing folder.

  Output:
  A JSON object containing a link (`url`) to the imported dashboard and its unique `id`.
```

## Reference

| **field**   | **type** | **required** | **description**                                    |
|-------------|:--------:|:------------:|----------------------------------------------------|
| type        |  string  |     true     | Must be "looker-import-dashboard-from-lookml"      |
| source      |  string  |     true     | Name of the Looker source to execute against.      |
| description |  string  |     true     | Description of the tool that is passed to the LLM. |
