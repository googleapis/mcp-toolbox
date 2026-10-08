---
title: "Gemini Data Analytics Source"
type: docs
weight: 1
linkTitle: "Source"
description: >
  A "cloud-gemini-data-analytics" source provides a client for the Gemini Data Analytics API.
no_list: true
---

## About

The `cloud-gemini-data-analytics` source provides a client to interact with the [Gemini Data Analytics API](https://docs.cloud.google.com/gemini/docs/conversational-analytics-api/reference/rest). This allows tools to send natural language queries to the API.

Authentication can be handled in two ways:

1.  **Application Default Credentials (ADC) (Recommended):** By default, the source uses ADC to authenticate with the API. The Toolbox server will fetch the credentials from its running environment (server-side authentication). This is the recommended method.
2.  **Client-side OAuth:** If `useClientOAuth` is set to `true`, the source expects the authentication token to be provided by the caller when making a request to the Toolbox server (typically via an HTTP Bearer token). The Toolbox server will then forward this token to the underlying Gemini Data Analytics API calls.

## Available Tools

{{< list-tools >}}

## Example

```yaml
kind: source
name: my-gda-source
type: cloud-gemini-data-analytics
projectId: my-project-id
---
kind: source
name: my-oauth-gda-source
type: cloud-gemini-data-analytics
projectId: my-project-id
useClientOAuth: true
```

## Reference

| **field**      | **type** | **required** | **description**                                                                                                                                                              |
| -------------- | :------: | :----------: | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| type           |  string  |     true     | Must be "cloud-gemini-data-analytics".                                                                                                                                       |
| projectId      |  string  |     true     | The Google Cloud Project ID where the API is enabled.                                                                                                                        |
| useClientOAuth | boolean  |    false     | If true, the source uses the token provided by the caller (forwarded to the API). Otherwise, it uses server-side Application Default Credentials (ADC). Defaults to `false`. |
| readOnly       | boolean  |    false     | When set to `true`, suppresses write-capable tools (such as those that create, update, or delete data agents). Default: `false`.                                             |
