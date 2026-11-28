---
title: "looker-get-data-app-extension"
type: docs
weight: 1
description: >
  "looker-get-data-app-extension" retrieves metadata and published JavaScript ES
  module source code for an existing Looker Data App Extension.
---

## About

The `looker-get-data-app-extension` tool retrieves an existing Data App
Extension in Looker via `GET /api/4.0/data_app_extension/:data_app_extension_id`
and, when `include_code` is `true` (the default), also fetches its published
uncompressed JavaScript ES module source code via
`GET /api/4.0/inline_extension/:data_app_extension_id/bundle.js`.

This provides the read-back path for stateless agentic clients: inspect a Data
App dashboard with `looker-get-dashboard` to discover the mounted `extension_id`
(`db::<id>`), call `looker-get-data-app-extension` to read its metadata and
JavaScript source, and apply incremental edits with
`looker-update-data-app-extension`.

## Compatible Sources

{{< compatible-sources >}}

## Requirements

* The `inline_extensions` feature must be enabled on the Looker instance.
* The authenticated Looker user must have permission to read the Data App Extension metadata (`save_content` or `develop`) and, when `include_code` is `true`, access the published bundle (`access_data`).

## Parameters

| **parameter**           | **type** | **required** | **description**                                                                                  |
|:------------------------|:--------:|:------------:|--------------------------------------------------------------------------------------------------|
| `data_app_extension_id` |  string  |     true     | The ID of the Data App Extension to retrieve (accepts `<id>` or `db::<id>`).                     |
| `include_code`          | boolean  |    false     | Whether to also fetch and include the published JavaScript ES module source code (default true). |

## Example

```yaml
kind: tool
name: get_data_app_extension
type: looker-get-data-app-extension
source: looker-source
description: |
  This tool retrieves an existing Data App Extension in Looker (`GET /api/4.0/data_app_extension/:id`)
  and optionally fetches its published uncompressed JavaScript ES module source code
  (`GET /api/4.0/inline_extension/:id/bundle.js`).

  Use this tool after inspecting a Data App dashboard with `get_dashboard` (which returns `extension_id`, e.g. `"db::<id>"`,
  on `type: "extension"` elements) to read back the existing metadata and JavaScript source before applying
  incremental edits with `update_data_app_extension`.

  Required Parameters:
  - data_app_extension_id: The ID of the Data App Extension to retrieve (accepts `<id>` or `db::<id>`).

  Optional Parameters:
  - include_code: Whether to also fetch and include the published JavaScript ES module source code (defaults to `true`). Set to `false` to retrieve metadata only.

  Output:
  Returns the Data App Extension metadata JSON, including `id`, `extension_id` (`"db::<id>"`), `label`, `description`, `code_size`, and (when `include_code` is `true`) `code`.
```

## Output Format

Returns a JSON object representing the Data App Extension metadata (`id`,
`extension_id`, `name`, `label`, `description`, `user_id`, `mount_points`,
`entitlements`, `code_size`, `created_at`, `updated_at`, `can`) and, when
`include_code` is `true`, the published uncompressed JavaScript ES module source
in the `code` field.

## Reference

| **field**   | **type** | **required** | **description**                                    |
|:------------|:--------:|:------------:|----------------------------------------------------|
| type        |  string  |     true     | Must be "looker-get-data-app-extension".           |
| source      |  string  |     true     | Name of the Looker source to execute against.      |
| description |  string  |     true     | Description of the tool that is passed to the LLM. |
