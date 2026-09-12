---
title: "looker-get-conversation"
type: docs
weight: 1
description: >
  "looker-get-conversation" retrieves details of a specific Looker Conversational Analytics conversation.
---

## About

The `looker-get-conversation` tool allows retrieving details of a specific conversation.

```json
{
  "name": "looker-get-conversation",
  "parameters": {
    "conversation_id": "conv-123"
  }
}
```

## Compatible Sources

{{< compatible-sources >}}

## Example

```yaml
kind: tool
name: get_conversation
type: looker-get-conversation
source: my-looker-instance
description: |
  Retrieve a conversation.
  - `conversation_id` (string): The ID of the conversation.
```

## Reference

| **field**   | **type** | **required** | **description**                                    |
|-------------|:--------:|:------------:|----------------------------------------------------|
| type        |  string  |     true     | Must be "looker-get-conversation".                  |
| source      |  string  |     true     | Name of the Looker source.                         |
| description |  string  |     true     | Description of the tool that is passed to the LLM. |
