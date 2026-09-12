---
title: "looker-make-look"
type: docs
weight: 1
description: >
  "looker-make-look" generates a Looker look in the users personal folder in
  Looker
---

## About

The `looker-make-look` creates a saved Look in the user's
Looker personal folder.

`looker-make-look` takes twelve parameters:

1. the `model`
2. the `explore`
3. the `fields` list
4. an optional set of `filters`
5. an optional set of `pivots`
6. an optional set of `sorts`
7. an optional `limit`
8. an optional `tz`
9. an optional `vis_config`
10. the `title`
11. an optional `description`
12. an optional `folder` id. If not provided, the user's default folder will be used.

## Compatible Sources

{{< compatible-sources >}}

## Example

```yaml
kind: tool
name: make_look
type: looker-make-look
source: looker-source
description: |
  This tool creates a new Look (saved query with visualization) in Looker.
  The Look will be saved in the user's personal folder, and its name must be unique.

  Required Parameters:
  - title: A unique title for the new Look.
  - description: A brief description of the Look's purpose.
  - model: The name of the LookML model (from `get_models`).
  - explore: The name of the explore (from `get_explores`).
  - fields: A list of field names (dimensions, measures, filters, or parameters) to include in the query.

  Optional Parameters:
  - pivots, filters, sorts, limit, tz: These parameters are identical
    to those described for the `query` tool.
  - vis_config: A JSON object defining the visualization settings for the Look.
    The structure and options are the same as for the `query_url` tool's `vis_config`.

  Output:
  A JSON object containing a link (`url`) to the newly created Look, along with its `id` and `slug`.
```

## Reference

| **field**   | **type** | **required** | **description**                                    |
|-------------|:--------:|:------------:|----------------------------------------------------|
| type        |  string  |     true     | Must be "looker-make-look"                         |
| source      |  string  |     true     | Name of the source the SQL should execute on.      |
| description |  string  |     true     | Description of the tool that is passed to the LLM. |
