---
title: "looker-get-explore"
description: "Tool to get detailed metadata for a single Looker explore"
---

## About
Tool to get detailed metadata for a single Looker explore.

{{< compatible-sources >}}

## Parameters
* `model`: The model containing the explore.
* `explore`: The explore to get metadata for.

## Example
```yaml
tools:
  - name: my_looker_get_explore
    type: looker-get-explore
    source: my_looker_source
```

## Output Format

The return type is a single map containing the explore's metadata:

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
            "field": "field_name",
            "value": "filter value"
        }
    ],
    "conditionally_filter": [
        {
            "field": "field_name",
            "value": "filter value"
        }
    ]
}
```
