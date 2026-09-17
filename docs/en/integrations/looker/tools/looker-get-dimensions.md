---
title: "looker-get-dimensions"
type: docs
weight: 1
description: >
  A "looker-get-dimensions" tool returns all the dimensions from a given explore
  in a given model in the source.

---

## About

A `looker-get-dimensions` tool returns all the dimensions from a given explore
in a given model in the source.

`looker-get-dimensions` accepts two parameters, the `model` and the `explore`.

## Compatible Sources

{{< compatible-sources >}}

## Example

```yaml
kind: tool
name: get_dimensions
type: looker-get-dimensions
source: looker-source
description: |
  This tool retrieves a list of dimensions defined within a specific Looker explore.
  Dimensions are non-aggregatable attributes or characteristics of your data
  (e.g., product name, order date, customer city) that can be used for grouping,
  filtering, or segmenting query results.

  Parameters:
  - model (required): The name of the LookML model, obtained from `get_models`.
  - explore (required): The name of the explore within the model, obtained from `get_explores`.

  Output Details:
  - If a dimension includes a `suggestions` field, its contents are valid values
    that can be used directly as filters for that dimension.
  - If a `suggest_explore` and `suggest_dimension` are provided, you can query
    that specified explore and dimension to retrieve a list of valid filter values.

```

The response is a json array with the following elements:

```json
{
  "name": "field name",
  "description": "field description",
  "type": "field type",
  "label": "field label",
  "label_short": "field short label",
  "tags": ["tags", ...],
  "synonyms": ["synonyms", ...],
  "suggestions": ["suggestion", ...],
  "suggest_explore": "explore",
  "suggest_dimension": "dimension"
}
```

## Reference

| **field**   | **type** | **required** | **description**                                    |
|-------------|:--------:|:------------:|----------------------------------------------------|
| type        |  string  |     true     | Must be "looker-get-dimensions".                   |
| source      |  string  |     true     | Name of the source the SQL should execute on.      |
| description |  string  |     true     | Description of the tool that is passed to the LLM. |
