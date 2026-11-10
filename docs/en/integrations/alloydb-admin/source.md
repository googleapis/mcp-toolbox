---
title: "AlloyDB Admin Source"
linkTitle: "Source"
type: docs
weight: 1
description: "The \"alloydb-admin\" source provides a client for the AlloyDB API.\n"
no_list: true
---

## About

The `alloydb-admin` source provides a client to interact with the [Google
AlloyDB API](https://cloud.google.com/alloydb/docs/reference/rest). This allows
tools to perform administrative tasks on AlloyDB resources, such as managing
clusters, instances, and users.

Authentication can be handled in two ways:

1.  **Application Default Credentials (ADC):** By default, the source uses ADC
    to authenticate with the API.
2.  **Client-side OAuth:** If `useClientOAuth` is set to `true`, the source will
    expect an OAuth 2.0 access token to be provided by the client (e.g., a web
    browser) for each request.

## Available Tools

{{< list-tools >}}

## Example

```yaml
kind: source
name: my-alloydb-admin
type: alloydb-admin
---
kind: source
name: my-oauth-alloydb-admin
type: alloydb-admin
useClientOAuth: true
```

## Reference

| **field**      | **type** | **required** | **description**                                                                                                                                |
| -------------- | :------: | :----------: | ---------------------------------------------------------------------------------------------------------------------------------------------- |
| type           |  string  |     true     | Must be "alloydb-admin".                                                                                                                       |
| defaultProject |  string  |     false    | The Google Cloud project ID to use for AlloyDB infrastructure tools.                                                                           |
| useClientOAuth |  boolean |     false    | If true, the source will use client-side OAuth for authorization. Otherwise, it will use Application Default Credentials. Defaults to `false`. |
| readOnly       |  boolean |     false    | When set to `true`, suppresses write-capable admin tools. Default: `false`.                                                                    |
