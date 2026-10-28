---
title: "dataplex-update-data-product"
type: docs
weight: 1
description: >
  A "dataplex-update-data-product" tool updates metadata for an existing Data Product in Knowledge Catalog.
---

## About

A `dataplex-update-data-product` tool updates an existing Data Product in Knowledge Catalog (formerly known as Dataplex). This is a long-running operation, and the tool returns immediately with the operation's location ID and operation ID.

View the [Data Products guide][guide] for more information.

[guide]: https://docs.cloud.google.com/dataplex/docs/data-products-overview

## Compatible Sources

{{< compatible-sources >}}

## Requirements

### IAM Permissions

Knowledge Catalog uses [Identity and Access Management (IAM)][iam-overview] to control
user and group access to Knowledge Catalog resources. Toolbox will use your
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
[dataplex-docs]: https://cloud.google.com/dataplex

## Parameters

The `dataplex-update-data-product` tool accepts the following parameters:

| **field**     | **type**         | **required** | **description**                                                                                                                                                                                            |
| ------------- | ---------------- | ------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| locationId    | string           | true         | The location ID (e.g. `us`, `us-central1`) of the Data Product.                                                                                                                                            |
| dataProductId | string           | true         | The unique ID of the Data Product to update.                                                                                                                                                               |
| displayName   | string           | false        | The display name of the Data Product.                                                                                                                                                                      |
| description   | string           | false        | The description of the Data Product.                                                                                                                                                                       |
| ownerEmails   | array of strings | false        | The list of owner emails for the Data Product.                                                                                                                                                             |
| accessGroups  | array of objects | false        | List of access groups to associate with the Data Product. Each group object can contain: `id` (required), `displayName` (required), `description`, and at least one of `googleGroup` and `serviceAccount`. |
| updateMask    | array of strings | false        | List of paths indicating which fields to update (e.g., `displayName`, `description`, `ownerEmails`, `accessGroups`). If not specified, all fields provided will be updated.                                |

## Example

```yaml
kind: tool
name: update_data_product
type: dataplex-update-data-product
source: my-dataplex-source
description: Use this tool to update a Data Product.
```

## Reference

| **field**   | **type** | **required** | **description**                                    |
| ----------- | -------- | ------------ | -------------------------------------------------- |
| type        | string   | true         | Must be "dataplex-update-data-product".            |
| source      | string   | true         | Name of the source the tool should execute on.     |
| description | string   | true         | Description of the tool that is passed to the LLM. |
