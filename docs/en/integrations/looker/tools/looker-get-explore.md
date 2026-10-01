---
title: "looker-get-explore"
type: docs
weight: 1
description: >
  A "looker-get-explore" tool returns detailed metadata for a single explore
  in a given model from the source.

---

## About

A `looker-get-explore` tool returns detailed metadata for a single explore,
including query constraints such as `always_filter` and
`conditionally_filter`, as well as `tags` and whether the explore is `hidden`.

Use `looker-get-explores` to cheaply list all explores in a model, then use
`looker-get-explore` to fetch the full metadata for the specific explore you
intend to query. The two are kept separate because `looker-get-explores` calls
the lightweight `lookml_model` endpoint, which only returns a navigation
summary of each explore, while the constraint fields are only available from
the heavier `lookml_model_explore` endpoint. Returning them from
`looker-get-explores` would require one additional API call per explore.

If the source has `show_hidden_explores` set to `false`, requesting a hidden
explore returns an error.

## Compatible Sources

{{< compatible-sources >}}

## Parameters

| **parameter** | **type** | **required** | **description**                     |
|---------------|:--------:|:------------:|-------------------------------------|
| model         |  string  |     true     | The model containing the explore.   |
| explore       |  string  |     true     | The explore to get metadata for.    |

## Example

```yaml
kind: tool
name: get_explore
type: looker-get-explore
source: looker-source
description: |
  This tool retrieves detailed metadata about a specific Looker explore, including its
  constraints and rules (e.g., `always_filter`, `conditionally_filter`, `tags`).

  CRITICAL: You MUST use this tool to inspect the explore's metadata BEFORE generating
  or running a query. If an explore has mandatory filters defined (like `always_filter`
  or `conditionally_filter`), your query will fail unless you include them.

  Parameters:
  - model (required): The name of the LookML model, obtained from `get_models`.
  - explore (required): The name of the explore within the model, obtained from `get_explores`.
```

## Output Format

The return type is a single map containing the explore's metadata. Keys are
omitted when the explore does not define them.

```json
{
    "name": "explore name",
    "description": "explore description",
    "label": "explore label",
    "group_label": "group label",
    "hidden": false,
    "tags": ["tag1", "tag2"],
    "always_filter": [
        {
            "name": "view_name.field_name",
            "value": "filter value"
        }
    ],
    "conditionally_filter": [
        {
            "name": "view_name.field_name",
            "value": "filter value"
        }
    ]
}
```

## Reference

| **field**   | **type** | **required** | **description**                                    |
|-------------|:--------:|:------------:|----------------------------------------------------|
| type        |  string  |     true     | Must be "looker-get-explore".                      |
| source      |  string  |     true     | Name of the Looker source.                         |
| description |  string  |     true     | Description of the tool that is passed to the LLM. |
