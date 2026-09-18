---
title: "looker-list-conversation-messages"
type: docs
weight: 1
description: >
  "looker-list-conversation-messages" lists all messages in a Looker Conversational Analytics conversation.
---

## About

The `looker-list-conversation-messages` tool allows listing all messages in a conversation.

```json
{
  "name": "looker-list-conversation-messages",
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
name: list_conversation_messages
type: looker-list-conversation-messages
source: my-looker-instance
description: |
  List messages in a conversation.
  - `conversation_id` (string): The ID of the conversation.
```

## Reference

| **field**   | **type** | **required** | **description**                                    |
|-------------|:--------:|:------------:|----------------------------------------------------|
| type        |  string  |     true     | Must be "looker-list-conversation-messages".       |
| source      |  string  |     true     | Name of the Looker source.                         |
| description |  string  |     true     | Description of the tool that is passed to the LLM. |
