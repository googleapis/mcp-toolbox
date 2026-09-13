---
title: "bigquery-search-catalog"
type: docs
weight: 1
description: >
  A "bigquery-search-catalog" tool allows to search for entries based on the provided query.
---

## About

A `bigquery-search-catalog` tool returns all entries in Knowledge Catalog (e.g.
tables, views, models) with system=bigquery that matches given user query.

`bigquery-search-catalog` takes a required `query` parameter based on which
entries are filtered and returned to the user. It also optionally accepts
following parameters:

- `datasetIds` - The IDs of the bigquery dataset.
- `projectIds` - The IDs of the bigquery project.
- `types` - The type of the data. Accepted values are: CONNECTION, POLICY,
  DATASET, MODEL, ROUTINE, TABLE, VIEW.
- `pageSize` - Number of results in the search page. Defaults to `5`.

The tool's results are also constrained by the `allowedDatasets` restriction on
the `bigquery` source:

- **Without `allowedDatasets` restriction:** The search can return entries from
  any dataset in the project.
- **With `allowedDatasets` restriction:** Entries whose dataset is not in the
  allowed list are removed from the results before they are returned. Entries
  that are not scoped to a dataset (such as connections) are also excluded while
  a restriction is in effect.

## Compatible Sources

{{< compatible-sources >}}

## Requirements

### IAM Permissions

Bigquery uses [Identity and Access Management (IAM)][iam-overview] to control
user and group access to Knowledge Catalog (formerly known as Dataplex) resources. Toolbox will use your
[Application Default Credentials (ADC)][adc] to authorize and authenticate when
interacting with [Knowledge Catalog][dataplex-docs].

In addition to [setting the ADC for your server][set-adc], you need to ensure
the IAM identity has been given the correct IAM permissions for the tasks you
intend to perform. See [Knowledge Catalog IAM permissions][iam-permissions]
and [Knowledge Catalog IAM roles][iam-roles] for more information on
applying IAM permissions and roles to an identity.

[iam-overview]: https://cloud.google.com/dataplex/docs/iam-and-access-control
[adc]: https://cloud.google.com/docs/authentication#adc
[set-adc]: https://cloud.google.com/docs/authentication/provide-credentials-adc
[iam-permissions]: https://cloud.google.com/dataplex/docs/iam-permissions
[iam-roles]: https://cloud.google.com/dataplex/docs/iam-roles

## Example

```yaml
kind: tool
name: search_catalog
type: bigquery-search-catalog
source: bigquery-source
description: Use this tool to find tables, views, models, routines or connections.
```

## Reference

| **field**   |                  **type**                  | **required** | **description**                                                                                  |
|-------------|:------------------------------------------:|:------------:|--------------------------------------------------------------------------------------------------|
| type        |                   string                   |     true     | Must be "bigquery-search-catalog".                                                               |
| source      |                   string                   |     true     | Name of the source the tool should execute on.                                                   |
| description |                   string                   |     true     | Description of the tool that is passed to the LLM.                                               |
