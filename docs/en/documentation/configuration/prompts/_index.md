---
title: "Prompts"
type: docs
weight: 8
description: >
   Prompts allow servers to provide structured messages and instructions for interacting with language models.
---

A `prompt` represents a reusable prompt template that can be retrieved and used
by MCP clients.

{{< notice note >}}
You can use [Groups](../groups/) to organize prompts into collections. When you connect to a group's endpoint, `prompts/list` returns only the prompts in that group. The default endpoint returns all prompts.
{{< /notice >}}

A Prompt is essentially a template for a message or a series of messages that
can be sent to a Large Language Model (LLM). The Toolbox server implements the
`prompts/list` and `prompts/get` methods from the [Model Context Protocol
(MCP)](https://modelcontextprotocol.io/docs/getting-started/intro)
specification, allowing clients to discover and retrieve these prompts.

```yaml
kind: prompt
name: code_review
description: "Asks the LLM to analyze code quality and suggest improvements."
messages:
  - content: "Please review the following code for quality, correctness, and potential improvements: \n\n{{.code}}"
arguments:
  - name: "code"
    description: "The code to review"
```

## Prompt Schema

| **field**   | **type**                       | **required** | **description**                                                          |
|-------------|--------------------------------|--------------|--------------------------------------------------------------------------|
| description | string                         | No           | A brief explanation of what the prompt does.                             |
| type        | string                         | No           | The type of prompt. Defaults to `"custom"`.                              |
| messages    | [][Message](#message-schema)   | Yes          | A list of one or more message objects that make up the prompt's content. |
| arguments   | [][Argument](#argument-schema) | No           | A list of arguments that can be interpolated into the prompt's content.  |

## Message Schema

| **field** | **type** | **required** | **description**                                                                                        |
|-----------|----------|--------------|--------------------------------------------------------------------------------------------------------|
| role      | string   | No           | The role of the sender. Can be `"user"` or `"assistant"`. Defaults to `"user"`.                        |
| content   | string   | Yes          | The text of the message. You can include placeholders for arguments using `{{.argument_name}}` syntax. |

## Argument Schema

An argument can be any [Parameter](../tools/_index.md#specifying-parameters)
type. If the `type` field is not specified, it will default to `string`.

## Usage with Gemini CLI

Prompts defined in your `tools.yaml` can be seamlessly integrated with the
Gemini CLI to create [custom slash
commands](https://github.com/google-gemini/gemini-cli/blob/main/docs/tools/mcp-server.md#mcp-prompts-as-slash-commands).
The workflow is as follows:

1. **Discovery:** When the Gemini CLI connects to your Toolbox server, it
   automatically calls `prompts/list` to discover all available prompts.

2. **Conversion:** Each discovered prompt is converted into a corresponding
   slash command. For example, a prompt named `code_review` becomes the
   `/code_review` command in the CLI.

3. **Execution:** You can execute the command as follows:

    ```bash
    /code_review --code="def hello():\n    print('world')"
    ```

4. **Interpolation:** Once all arguments are collected, the CLI calls prompts/get
   with your provided values to retrieve the final, interpolated prompt.
    Eg.

    ```bash
    Please review the following code for quality, correctness, and potential improvements: \ndef hello():\n    print('world')
    ```

5. **Response:** This completed prompt is then sent to the Gemini model, and the
   model's response is displayed back to you in the CLI.

## Types of prompts
