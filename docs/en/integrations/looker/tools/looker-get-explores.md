---
title: "looker-get-explores"
type: docs
weight: 1
description: >
  A "looker-get-explores" tool returns all explores
  for the given model from the source.

---

## About

A `looker-get-explores` tool returns all explores
for a given model from the source.

`looker-get-explores` accepts one parameter, the
`model` id.

The return type is an array of maps, each map is formatted like:

```json
{
    "name": "explore name",
    "description": "explore description",
    "label": "explore label",
    "group_label": "group label"
}
```

## Compatible Sources

{{< compatible-sources >}}

## Example

```yaml
kind: tool
name: get_explores
type: looker-get-explores
source: looker-source
description: |
  This tool retrieves a list of explores defined within a specific LookML model.
  Explores represent a curated view of your data, typically joining several
  tables together to allow for focused analysis on a particular subject area.
  The output provides details like the explore's `name` and `label`.

  Parameters:
  - model (required): The name of the LookML model, obtained from `get_models`.
```

## Reference

| **field**   | **type** | **required** | **description**                                    |
|-------------|:--------:|:------------:|----------------------------------------------------|
| type        |  string  |     true     | Must be "looker-get-explores".                     |
| source      |  string  |     true     | Name of the source the SQL should execute on.      |
| description |  string  |     true     | Description of the tool that is passed to the LLM. |
