---
title: "OrcaRouter Embedding"
type: docs
weight: 2
description: >
  Use OrcaRouter to route OpenAI-compatible embedding models from OpenAI,
  Google and others behind a single API key.
---

## About

[OrcaRouter](https://www.orcarouter.ai) is an OpenAI-compatible model routing
gateway. It exposes embedding models from OpenAI (`text-embedding-3-small`,
`text-embedding-3-large`, `text-embedding-ada-002`) and Google
(`gemini-embedding-001`, `gemini-embedding-2-preview`) behind a single endpoint
and API key, so you can switch embedding models without changing your Toolbox
configuration.

### Authentication

OrcaRouter uses a single API key for all models. Provide `apiKey` in the YAML
(or set the `ORCAROUTER_API_KEY` environment variable). Get an API key at
[orcarouter.ai](https://www.orcarouter.ai); keys start with `sk-orca-`.

## Behavior

### Automatic Vectorization

When a tool parameter is configured with `embeddedBy: <your-orcarouter-model-name>`,
the Toolbox intercepts the raw text input from the client and sends it to the
OrcaRouter `/embeddings` endpoint using the [OpenAI Embeddings API][openai-embeddings]
format. The resulting numerical array is then formatted before being passed to
your database source.

### Dimension Matching

The optional `dimension` field requests a specific output dimensionality and is
passed through to the model. It must match the expected size of your database
column (e.g., a `vector(3072)` column in PostgreSQL) and be supported by the
selected embedding model. If unset, the model's default dimensionality is used.

[openai-embeddings]: https://platform.openai.com/docs/api-reference/embeddings

## Example

```yaml
kind: embeddingModel
name: orcarouter-model
type: orcarouter
model: openai/text-embedding-3-small
apiKey: ${ORCAROUTER_API_KEY}
dimension: 1536
```

{{< notice tip >}} Use environment variable replacement with the format
${ENV_NAME} instead of hardcoding your secrets into the configuration file.
{{< /notice >}}

## Reference

| **field**   | **type** | **required** | **description**                                                                                                                                      |
| ----------- | :------: | :----------: | ---------------------------------------------------------------------------------------------------------------------------------------------------- |
| type        |  string  |     true     | Must be `orcarouter`.                                                                                                                                |
| model       |  string  |     true     | The OrcaRouter model ID to use (e.g., `openai/text-embedding-3-small`).                                                                              |
| apiKey      |  string  |    false     | OrcaRouter API key. If unset, `ORCAROUTER_API_KEY` is used.                                                                                          |
| baseUrl     |  string  |    false     | The gateway base URL. Defaults to `https://api.orcarouter.ai/v1`.                                                                                    |
| dimension   | integer  |    false     | The number of dimensions in the output vector (e.g., `1536`).                                                                                        |
