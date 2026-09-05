---
title: "Text"
type: docs
weight: 1
description: >
  Direct text resources embedded in your configuration.
---

Text resources allow you to define static, in-memory content directly within your configuration file. They are ideal for embedding reference tables, schemas, documentation notes, lookup dictionaries, or configuration data that should be supplied to an LLM or client as read-only context.

## Examples

### Basic Text Resource

Here is an example of a minimal text resource. If `uri` is omitted, Toolbox automatically defaults to `text://{name}`.

```yaml
kind: resource
name: country_codes
type: text
description: "ISO standard country codes and country names."
text: |
  US: United States
  CA: Canada
  GB: United Kingdom
  DE: Germany
  JP: Japan
```

### Custom URI and Markdown MIME Type

You can specify a custom URI scheme (such as `docs://`) and define a specific MIME type like `text/markdown`:

```yaml
kind: resource
name: architecture_doc
type: text
title: "System Architecture"
description: "High-level overview of our microservices architecture."
uri: "docs://architecture/overview"
mimeType: "text/markdown"
text: |
  # System Architecture Overview
  
  - **API Gateway**: Handles incoming client traffic and routing.
  - **Auth Service**: Issues and validates OAuth tokens.
  - **Data Service**: Interfaces with PostgreSQL and BigQuery storage.
```

### Resource with Annotations

You can attach annotations to specify audience and priority for MCP clients:

```yaml
kind: resource
name: order_status_codes
type: text
title: "Order Status Codes"
description: "Valid order status codes and definitions."
uri: "schema://orders/status-codes"
mimeType: "text/plain"
text: |
  PENDING   - Order created, awaiting payment confirmation.
  CONFIRMED - Payment authorized, order queued for fulfillment.
  SHIPPED   - Order dispatched with carrier tracking number.
  DELIVERED - Package received by customer.
  CANCELLED - Order cancelled prior to shipment.
annotations:
  priority: 0.8
  audience:
    - assistant
```

## Reference

### Text Resource Schema

| **field**     | **type**                           | **required** | **description**                                                                                               |
|---------------|------------------------------------|--------------|---------------------------------------------------------------------------------------------------------------|
| `name`        | string                             | Yes          | Unique name for the resource.                                                                                 |
| `type`        | string                             | Yes          | Must be `"text"`.                                                                                             |
| `text`        | string                             | Yes          | The text content of the resource.                                                                             |
| `uri`         | string                             | No           | Unique URI identifying the resource. Defaults to `text://{name}` if omitted.                                   |
| `description` | string                             | No           | A brief explanation of the resource's purpose.                                                                |
| `title`       | string                             | No           | Human-readable title for client display.                                                                      |
| `mimeType`    | string                             | No           | MIME type of the text content. Defaults to `text/plain`.                                                      |
| `annotations` | [Annotations](./_index.md#annotations-schema) | No   | Metadata annotations (`priority`, `audience`, `lastModified`).                                                |

## Behaviors

- **Default URI Generation**: When `uri` is not specified, Toolbox generates `text://{name}`. All resource URIs on the server must be globally unique across all resources and templates.
- **Computed Size**: When queried via `resources/list`, the `size` field is automatically calculated as the byte length of the UTF-8 text string.
