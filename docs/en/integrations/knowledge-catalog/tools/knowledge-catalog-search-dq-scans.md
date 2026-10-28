---
title: "dataplex-search-dq-scans"
type: docs
weight: 1
description: >
  A "dataplex-search-dq-scans" tool allows to search for data quality scans based on the provided parameters.
aliases:
- /resources/tools/dataplex-search-dq-scans
- /integrations/dataplex/tools/dataplex-search-dq-scans/
---

## About

A `dataplex-search-dq-scans` tool returns data quality scans that match the given criteria.
## Compatible Sources

{{< compatible-sources >}}

## Requirements

### IAM Permissions

Dataplex uses [Identity and Access Management (IAM)][iam-overview] to control
user and group access to Dataplex resources. Toolbox will use your
[Application Default Credentials (ADC)][adc] to authorize and authenticate when
interacting with [Dataplex][dataplex-docs].

In addition to [setting the ADC for your server][set-adc], you need to ensure
the IAM identity has been given the correct IAM permissions for the tasks you
intend to perform. See [Dataplex Universal Catalog IAM permissions][iam-permissions]
and [Dataplex Universal Catalog IAM roles][iam-roles] for more information on
applying IAM permissions and roles to an identity.

[iam-overview]: https://cloud.google.com/dataplex/docs/iam-and-access-control
[adc]: https://cloud.google.com/docs/authentication#adc
[set-adc]: https://cloud.google.com/docs/authentication/provide-credentials-adc
[iam-permissions]: https://cloud.google.com/dataplex/docs/iam-permissions
[iam-roles]: https://cloud.google.com/dataplex/docs/iam-roles
[dataplex-docs]: https://cloud.google.com/dataplex

## Parameters

The `dataplex-search-dq-scans` tool accepts the following optional parameters:

| **field** | **type** | **required** | **description** |
| --------- | :------: | :----------: | --------------- |
| filter | string | false | Filter string to search/filter data quality scans (e.g. `display_name = "my-scan"`). |
| data_scan_id | string | false | The resource name of the data scan to filter by (`projects/{project}/locations/{locationId}/dataScans/{dataScanId}`). |
| table_name | string | false | The name of the table to filter by, mapping to `data.entity` (e.g. `//bigquery.googleapis.com/projects/P/datasets/D/tables/T`). |
| pageSize | integer | false | Number of returned data quality scans in the page. Defaults to 10. |
| orderBy | string | false | Specifies ordering of results. |

## Example

```yaml
kind: tool
name: search_dq_scans
type: dataplex-search-dq-scans
source: my-dataplex-source
description: Use this tool to search for data quality scans.
```

## Reference

| **field**   | **type** | **required** | **description**                                    |
|-------------|:--------:|:------------:|----------------------------------------------------|
| type        |  string  |     true     | Must be "dataplex-search-dq-scans".                |
| source      |  string  |     true     | Name of the source the tool should execute on.     |
| description |  string  |     true     | Description of the tool that is passed to the LLM. |
