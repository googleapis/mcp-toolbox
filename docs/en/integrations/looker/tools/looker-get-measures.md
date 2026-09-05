---
title: "looker-get-measures"
type: docs
weight: 1
description: >
  A "looker-get-measures" tool returns all the measures from a given explore
  in a given model in the source.
---

## About

A `looker-get-measures` tool returns all the measures from a given explore
in a given model in the source.

`looker-get-measures` accepts two parameters, the `model` and the `explore`.

## Compatible Sources

{{< compatible-sources >}}

## Example

```yaml
kind: tool
name: get_measures
type: looker-get-measures
source: looker-source
description: |
  This tool retrieves a list of measures defined within a specific Looker explore.
  Measures are aggregatable metrics (e.g., total sales, average price, count of users)
  that are used for calculations and quantitative analysis in your queries.

  Parameters:
  - model (required): The name of the LookML model, obtained from `get_models`.
  - explore (required): The name of the explore within the model, obtained from `get_explores`.

  Output Details:
  - If a measure includes a `suggestions` field, its contents are valid values
    that can be used directly as filters for that measure.
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
| type        |  string  |     true     | Must be "looker-get-measures".                     |
| source      |  string  |     true     | Name of the source the SQL should execute on.      |
| description |  string  |     true     | Description of the tool that is passed to the LLM. |
