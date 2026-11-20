---
title: "looker-publish-data-app"
type: docs
weight: 1
description: >
  "looker-publish-data-app" publishes a new full-bleed Looker Data Application or updates
  an existing one in-place in a single server-side call.
---

## About

The `looker-publish-data-app` tool is a composite tool that performs idempotent
create-or-update orchestration for full-bleed Looker Data Applications in a
single tool call:

* **Create flow (`dashboard_id` omitted):** Validates parameters and folder
  availability, creates the Data App Extension (`POST /api/4.0/data_app_extension`)
  before creating the dashboard (so invalid payloads never leave orphaned
  dashboards), creates the dashboard with full-bleed container appearance
  (`show_dashboard_header: false`, `page_side_margins: 0`,
  `tile_space_between: 0`, `tile_shadow: false`), mounts the `type: "extension"`
  dashboard element (`title_hidden: true`, optional `rich_content_json`), and
  switches the active layout to `type: "extension"`.
* **Update flow (`dashboard_id` provided):** Inspects the target dashboard to
  locate its `extension` element and active layout, updates the existing
  `db::<id>` extension in-place via
  `PATCH /api/4.0/data_app_extension/{id}?dashboard_id={dashboard_id}` (falling
  back to creating a new extension and repointing the element if the referenced
  extension was deleted), and converges full-bleed dashboard appearance,
  element metadata/`rich_content_json`, and `type: "extension"` layout.

## Compatible Sources

{{< compatible-sources >}}

## Requirements

* The `inline_extensions` feature must be enabled on the Looker instance.
* The authenticated Looker user must have `save_content` permission (and edit
  permissions on the target folder or shared dashboard).

## Parameters

| **parameter**       | **type** | **required** | **description**                                                                                                 |
|:--------------------|:--------:|:------------:|-----------------------------------------------------------------------------------------------------------------|
| `title`             |  string  |     true     | Display title for the Data App dashboard and extension label (up to 255 characters).                            |
| `code`              |  string  |     true     | Uncompressed JavaScript ES module source code payload (up to 5 MB). Must be a native ES module (`<script type="module">`), not CommonJS or an IIFE `bundle.js`. To connect to Looker's extension host (avoiding `"Cannot connect to extension"`), the module MUST complete the Looker Extension SDK handshake and mount into `document.body`: either import `{ DataApp, html }` from the Looker Data Apps runtime (`https://www.gstatic.com/looker-data-apps/versions/<version>/data-app.js`, e.g. `1.3.0`), extend `DataApp`, and end with the custom element registration guard (`if (!customElements.get(elementName)) { customElements.define(elementName, AppClass); } export default AppClass;`), or call `connectExtensionHost()` from `@looker/extension-sdk` and append the root element to `document.body`. Rewrite any relative imports (e.g. `/runtime/data-app.js`) to absolute HTTPS URLs. |
| `description`       |  string  |    false     | Optional description for the Data App dashboard and extension.                                                  |
| `folder`            |  string  |    false     | Optional target folder ID when creating a new dashboard. Defaults to the user's personal folder; ignored when `dashboard_id` is provided. |
| `dashboard_id`      |  string  |    false     | Optional existing dashboard ID to update in-place. When omitted, creates a new full-bleed Data App dashboard.   |
| `rich_content_json` |  string  |    false     | Optional JSON string (e.g. `'{"data_app_queries_version": 1, "data_app_queries": [...]}'`) to attach to the extension dashboard element. |

## Example

```yaml
kind: tool
name: publish_data_app
type: looker-publish-data-app
source: looker-source
description: |
  This composite tool publishes a new full-bleed Looker Data Application or updates an existing one in-place
  in a single call. Requires the `inline_extensions` feature to be enabled on the Looker instance.

  Required Parameters:
  - title: Display title for the Data App dashboard and extension label (up to 255 characters).
  - code: Uncompressed JavaScript ES module source code payload (up to 5 MB).

  Optional Parameters:
  - description: Dashboard and extension description.
  - folder: Target folder ID when creating a new dashboard.
  - dashboard_id: Existing dashboard ID to update in-place.
  - rich_content_json: Optional JSON string to attach to the extension dashboard element.

  Output:
  Returns `{dashboard_id, url, extension_id, dashboard_element_id, dashboard_layout_id}`.
```

## Output Format

Returns a JSON object with:

* `dashboard_id`: ID of the created or updated Looker dashboard.
* `url`: Full URL to the Looker dashboard.
* `extension_id`: Mounted Data App Extension ID (`"db::<id>"`).
* `dashboard_element_id`: ID of the mounted extension dashboard element.
* `dashboard_layout_id`: ID of the active `extension` dashboard layout.

## Reference

| **field**   | **type** | **required** | **description**                                    |
|:------------|:--------:|:------------:|----------------------------------------------------|
| type        |  string  |     true     | Must be "looker-publish-data-app".                 |
| source      |  string  |     true     | Name of the Looker source to execute against.      |
| description |  string  |     true     | Description of the tool that is passed to the LLM. |
