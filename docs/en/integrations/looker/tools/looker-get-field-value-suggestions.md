---
title: "looker-get-field-value-suggestions"
type: docs
weight: 1
description: >
  A "looker-get-field-value-suggestions" tool retrieves distinct value suggestions
  for a given field in an explore.
---

## About

The `looker-get-field-value-suggestions` tool retrieves distinct value suggestions for a given field in an explore.

## Compatible Sources

{{< compatible-sources >}}

## Parameters

| **field** | **type** | **required** | **description**                                     |
| --------- | :------: | :----------: | --------------------------------------------------- |
| model     |  string  |     true     | The name of the LookML model.                       |
| explore   |  string  |     true     | The name of the explore containing the field.       |
| field     |  string  |     true     | The name of the field to get suggestions for.       |
| term      |  string  |    false     | Optional search term pattern to filter suggestions. |
| filters   |  object  |    false     | Optional filters to enable conditional suggestions. |

## Example

```yaml
kind: tool
name: get_field_value_suggestions
type: looker-get-field-value-suggestions
source: looker-source
description: |
  This tool retrieves distinct value suggestions for a field, facilitating accurate filtering in downstream queries.

  Required Parameters:
  - model: The name of the LookML model, obtained from `get_models`.
  - explore: The name of the explore containing the field, obtained from `get_explores`.
  - field: The name of the field to get suggestions for, obtained from `get_dimensions`.

  Optional Parameters:
  - term: Optional search term pattern to filter suggestions. Evaluated as `%term%`.
  - filters: Optional filters to enable conditional suggestions (restricting suggestions based on other field values), represented as a map of field names to filter expressions, e.g., `{"users.state": "CA", "users.age": ">=60"}`.

  Output:
  - A JSON object with a "suggestions" key containing an array of string values representing suggestions.
```

## Output Format

The output is a JSON object with a "suggestions" key containing an array of string values.

Example:

```json
{
  "suggestions": ["CA", "NY", "TX", "WA"]
}
```

## Reference

| **field**   | **type** | **required** | **description**                                    |
| ----------- | :------: | :----------: | -------------------------------------------------- |
| type        |  string  |     true     | Must be "looker-get-field-value-suggestions".      |
| source      |  string  |     true     | Name of the source Looker instance.                |
| description |  string  |     true     | Description of the tool that is passed to the LLM. |
