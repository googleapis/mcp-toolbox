---
title: "Cloud SQL Admin Source"
type: docs
linkTitle: "Source"
weight: 1
description: "A \"cloud-sql-admin\" source provides a client for the Cloud SQL Admin API.\n"
no_list: true
---

## About

The `cloud-sql-admin` source provides a client to interact with the [Google
Cloud SQL Admin API](https://cloud.google.com/sql/docs/mysql/admin-api). This
allows tools to perform administrative tasks on Cloud SQL instances, such as
creating users and databases.

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
name: my-cloud-sql-admin
type: cloud-sql-admin
---
kind: source
name: my-oauth-cloud-sql-admin
type: cloud-sql-admin
useClientOAuth: true
```

## Reference

| **field**      | **type** | **required** | **description**                                                                                                                                |
| -------------- | :------: | :----------: | ---------------------------------------------------------------------------------------------------------------------------------------------- |
| type           |  string  |     true     | Must be "cloud-sql-admin".                                                                                                                     |
| defaultProject |  string  |     false    | The Google Cloud project ID to use for Cloud SQL infrastructure tools.                                                                         |
| useClientOAuth |  boolean |     false    | If true, the source will use client-side OAuth for authorization. Otherwise, it will use Application Default Credentials. Defaults to `false`. |
| readOnly       |  boolean |     false    | When set to `true`, suppresses write-capable admin tools. Default: `false`.                                                                    |
