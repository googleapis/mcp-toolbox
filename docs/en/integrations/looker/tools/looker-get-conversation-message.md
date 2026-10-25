---
title: "looker-get-conversation-message"
type: docs
weight: 1
description: >
  "looker-get-conversation-message" retrieves a specific message from a Looker Conversational Analytics conversation.
---

## About

The `looker-get-conversation-message` tool allows retrieving a specific message from a conversation.

```json
{
  "name": "looker-get-conversation-message",
  "parameters": {
    "conversation_id": "conv-123",
    "message_id": "msg-456"
  }
}
```

## Compatible Sources

{{< compatible-sources >}}

## Example

```yaml
kind: tool
name: get_conversation_message
type: looker-get-conversation-message
source: my-looker-instance
description: |
  Retrieve a specific message from a conversation.
  - `conversation_id` (string): The ID of the conversation.
  - `message_id` (string): The ID of the message.
```

## Reference

| **field**   | **type** | **required** | **description**                                    |
|-------------|:--------:|:------------:|----------------------------------------------------|
| type        |  string  |     true     | Must be "looker-get-conversation-message".         |
| source      |  string  |     true     | Name of the Looker source.                         |
| description |  string  |     true     | Description of the tool that is passed to the LLM. |
