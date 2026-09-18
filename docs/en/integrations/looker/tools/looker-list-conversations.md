---
title: "looker-list-conversations"
type: docs
weight: 1
description: >
  "looker-list-conversations" lists or searches Looker Conversational Analytics conversations.
---

## About

The `looker-list-conversations` tool allows listing or searching conversations.

```json
{
  "name": "looker-list-conversations",
  "parameters": {
    "name": "My Conversation",
    "limit": 10
  }
}
```

## Compatible Sources

{{< compatible-sources >}}

## Example

```yaml
kind: tool
name: list_conversations
type: looker-list-conversations
source: my-looker-instance
description: |
  List conversations.
  - `name` (string): Optional. Filter by name.
  - `agent_id` (string): Optional. Filter by agent ID.
  - `category` (string): Optional. Filter by category.
  - `limit` (integer): Optional. Limit results.
  - `offset` (integer): Optional. Offset results.
```

## Reference

| **field**   | **type** | **required** | **description**                                    |
|-------------|:--------:|:------------:|----------------------------------------------------|
| type        |  string  |     true     | Must be "looker-list-conversations".               |
| source      |  string  |     true     | Name of the Looker source.                         |
| description |  string  |     true     | Description of the tool that is passed to the LLM. |
