---
title: "looker-update-data-app-extension"
type: docs
weight: 1
description: >
  "looker-update-data-app-extension" updates an existing Data App
  Extension in Looker while preserving sandboxed mount points and entitlements.
---

## About

The `looker-update-data-app-extension` tool updates an existing
Data App Extension in Looker via `PATCH /api/4.0/data_app_extension/:data_app_extension_id`
using the Looker SDK transport. Looker preserves the server-enforced
dashboard-tile mount points and sandboxed core API entitlements across updates.

Updating `code` updates the underlying JavaScript ES module source code payload
in place. Any dashboards or tiles referencing `db::<id>` will immediately execute
the updated code on next view without needing to recreate or update the dashboard
element or modify dashboard layouts.

At least one updatable field (`label`, `code`, `description`, or `name`) must be
provided when invoking the tool.

## Compatible Sources

{{< compatible-sources >}}

## Requirements

* The `inline_extensions` feature must be enabled on the Looker instance.
* The authenticated Looker user must own the Data App Extension, have `manage_inline_extensions` permission, or supply a `dashboard_id` for a shared dashboard they have edit access to.

## Parameters

| **parameter**           | **type** | **required** | **description**                                                                                                             |
|:------------------------|:--------:|:------------:|-----------------------------------------------------------------------------------------------------------------------------|
| `data_app_extension_id` |  string  |     true     | The ID of the Data App Extension to update (accepts `<id>` or `db::<id>`).                                                  |
| `label`                 |  string  |    false     | Optional updated display label for the Data App Extension (up to 255 characters).                                           |
| `code`                  |  string  |    false     | Optional updated uncompressed JavaScript ES module source code payload (up to 5 MB). Must be a native ES module (`<script type="module">`), not CommonJS or an IIFE `bundle.js`. Relative imports (e.g. `/runtime/data-app.js`) must be rewritten to absolute CDN URLs (e.g. `'https://www.gstatic.com/looker-data-apps/versions/1.2.0/data-app.js'` and `'https://www.gstatic.com/looker-data-apps/vendor/lit/3.3.1/lit.mjs'`). Must end with custom element registration guard: `if (!customElements.get(elementName)) { customElements.define(elementName, AppClass); } export default AppClass;` |
| `description`           |  string  |    false     | Optional updated description of the Data App Extension.                                                                     |
| `name`                  |  string  |    false     | Optional updated unique system name matching `[a-zA-Z0-9-_]+`. Usually omitted on update to avoid unique-index collisions.  |
| `dashboard_id`          |  string  |    false     | Optional ID of the shared dashboard containing this Data App tile. Only needed when updating an extension mounted on a shared dashboard where the user relies on shared-folder contextual permissions. |

## Example

```yaml
kind: tool
name: update_data_app_extension
type: looker-update-data-app-extension
source: looker-source
description: |
  This tool updates an existing Data App Extension in Looker (`PATCH /api/4.0/data_app_extension/:id`)
  while preserving server-enforced dashboard-tile mount points and sandboxed entitlements.

  IN-PLACE CODE UPDATES:
  Updating `code` updates the underlying JavaScript ES module source code payload in place. Any dashboards
  or tiles referencing `db::<id>` will immediately execute the updated code on next view without needing
  to recreate or update the dashboard element or modify dashboard layouts.

  Required Parameters:
  - data_app_extension_id: The ID of the Data App Extension to update (accepts `<id>` or `db::<id>`).
  - At least one updatable field (`label`, `code`, `description`, or `name`) must be provided.

  Optional Parameters:
  - label: Updated user-friendly display label (up to 255 characters).
  - code: Updated uncompressed JavaScript ES module source code payload (up to 5 MB). Must be a native ES module (`<script type="module">`), not CommonJS or an IIFE `bundle.js`. Relative imports like `/runtime/data-app.js` used during local preview must be rewritten to absolute CDN URLs (e.g. `'https://www.gstatic.com/looker-data-apps/versions/1.2.0/data-app.js'` and `'https://www.gstatic.com/looker-data-apps/vendor/lit/3.3.1/lit.mjs'`). The module must end with the custom element registration guard: `if (!customElements.get(elementName)) { customElements.define(elementName, AppClass); } export default AppClass;`
  - description: Updated description of the Data App Extension.
  - name: Updated unique system identifier matching `[a-zA-Z0-9-_]+` (normally omitted to avoid unique-index collisions).
  - dashboard_id: ID of the shared dashboard containing this Data App tile. Only needed when updating an extension mounted on a shared dashboard where the user relies on shared-folder contextual permissions.

  Output:
  Returns the updated Data App Extension metadata JSON, including `id` and `extension_id` (`"db::<id>"`).
```

## Reference

| **field**   | **type** | **required** | **description**                                    |
|:------------|:--------:|:------------:|----------------------------------------------------|
| type        |  string  |     true     | Must be "looker-update-data-app-extension".        |
| source      |  string  |     true     | Name of the Looker source to execute against.      |
| description |  string  |     true     | Description of the tool that is passed to the LLM. |
