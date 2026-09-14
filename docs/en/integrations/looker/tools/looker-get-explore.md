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
