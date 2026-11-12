---
title: "looker-create-data-app-extension"
type: docs
weight: 1
description: >
  "looker-create-data-app-extension" creates a Data App Extension
  in Looker with server-enforced dashboard-tile mount points and sandboxed entitlements.
---

## About

The `looker-create-data-app-extension` tool creates a Data App
Extension entity in Looker via `POST /api/4.0/data_app_extension` using the Looker SDK
transport. Looker enforces dashboard-tile-only mount points and a fixed
sandboxed core API entitlement profile on the created extension.

NOTE: This tool creates the Extension entity in Looker's database, but does NOT automatically create a dashboard or dashboard tile.

LIFECYCLE / ORDER OF OPERATIONS:
1. Create or identify the target dashboard using `make_dashboard` (leave folder empty for personal folder) or `get_dashboards`.
2. Switch active layout to full bleed using `update_dashboard_layout` (sets `type: "extension"`).
3. Configure full-bleed container appearance using `update_dashboard` (sets `show_dashboard_header: false`, `page_side_margins: 0`, `tile_space_between: 0`, `tile_shadow: false`).
4. Create the extension entity using this tool (`create_data_app_extension`), which returns `extension_id` (`"db::<uuid>"`).
5. Mount the extension onto the dashboard as a tile by calling `add_dashboard_element` with `type: "extension"`, `extension_id: "db::<uuid>"`, and `title_hidden: true`.

It returns the created extension metadata, including `id` and `extension_id`
(`"db::<id>"`) for mounting on a dashboard tile.

## Compatible Sources

{{< compatible-sources >}}

## Requirements

* The `inline_extensions` feature must be enabled on the Looker instance.
* The authenticated Looker user must have `save_content` permission.

## Parameters

| **parameter** | **type** | **required** | **description**                                                                                                 |
|:--------------|:--------:|:------------:|-----------------------------------------------------------------------------------------------------------------|
| `label`       |  string  |     true     | The user-friendly display label for the Data App Extension (up to 255 characters).                              |
| `code`        |  string  |     true     | The uncompressed JavaScript ES module source code payload (up to 5 MB). Must be a native ES module (`<script type="module">`), not CommonJS or an IIFE `bundle.js`. Relative imports (e.g. `/runtime/data-app.js`) must be rewritten to absolute CDN URLs (e.g. `'https://www.gstatic.com/looker-data-apps/versions/1.2.0/data-app.js'` and `'https://www.gstatic.com/looker-data-apps/vendor/lit/3.3.1/lit.mjs'`). Must end with custom element registration guard: `if (!customElements.get(elementName)) { customElements.define(elementName, AppClass); } export default AppClass;` |
| `description` |  string  |    false     | Optional description of the Data App Extension.                                                                 |
| `name`        |  string  |    false     | Optional unique system name matching `[a-zA-Z0-9-_]+`. Leave empty to auto-generate a collision-free name.      |

## Example

```yaml
kind: tool
name: create_data_app_extension
type: looker-create-data-app-extension
source: looker-source
description: |
  This tool creates a Data App Extension entity in Looker with
  server-enforced dashboard-tile mount points and sandboxed core API entitlements (`POST /api/4.0/data_app_extension`).
  Requires the `inline_extensions` feature to be enabled on the Looker instance.

  NOTE: This tool creates the Extension entity in Looker's database, but does NOT automatically create a dashboard or dashboard tile.

  LIFECYCLE / ORDER OF OPERATIONS:
  1. Create or identify the target dashboard using `make_dashboard` (leave folder empty for personal folder) or `get_dashboards`.
  2. Switch active layout to full bleed using `update_dashboard_layout` (sets `type: "extension"`).
  3. Configure full-bleed container appearance using `update_dashboard` (sets `show_dashboard_header: false`, `page_side_margins: 0`, `tile_space_between: 0`, `tile_shadow: false`).
  4. Create the extension entity using this tool (`create_data_app_extension`), which returns `extension_id` (`"db::<uuid>"`).
  5. Mount the extension onto the dashboard as a tile by calling `add_dashboard_element` with `type: "extension"`, `extension_id: "db::<uuid>"`, and `title_hidden: true`.

  Required Parameters:
  - label: The user-friendly display label for the Data App Extension (up to 255 characters).
  - code: The uncompressed JavaScript ES module source code payload (up to 5 MB). Must be a native ES module (`<script type="module">`), not CommonJS or an IIFE `bundle.js`. Relative imports like `/runtime/data-app.js` used during local preview must be rewritten to absolute CDN URLs (e.g. `'https://www.gstatic.com/looker-data-apps/versions/1.2.0/data-app.js'` and `'https://www.gstatic.com/looker-data-apps/vendor/lit/3.3.1/lit.mjs'`). The module must end with the custom element registration guard: `if (!customElements.get(elementName)) { customElements.define(elementName, AppClass); } export default AppClass;`

  Optional Parameters:
  - description: Brief description of the Data App Extension.
  - name: Unique system identifier matching `[a-zA-Z0-9-_]+`. Leave empty to auto-generate a collision-free `data-app-<slug>-<uuid>` name.

  Output:
  Returns the created Data App Extension metadata JSON, including `id` and `extension_id` (`"db::<id>"`) for dashboard tile mounting.
```

## Reference

| **field**   | **type** | **required** | **description**                                    |
|:------------|:--------:|:------------:|----------------------------------------------------|
| type        |  string  |     true     | Must be "looker-create-data-app-extension".        |
| source      |  string  |     true     | Name of the Looker source to execute against.      |
| description |  string  |     true     | Description of the tool that is passed to the LLM. |
