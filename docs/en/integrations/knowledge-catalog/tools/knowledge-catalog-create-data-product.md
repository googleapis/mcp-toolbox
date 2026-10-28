---
title: "dataplex-create-data-product"
type: docs
weight: 1
description: >
  A "dataplex-create-data-product" tool allows to create a new Data Product.
---

## About

A `dataplex-create-data-product` tool creates a new Data Product in Knowledge Catalog (formerly known as Dataplex). This is a long-running operation, and the tool returns immediately with the operation's location ID and operation ID.

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

The `dataplex-create-data-product` tool accepts the following parameters:

| **field**     | **type**         | **required** | **description**                                                                                                                                                                                            |
| ------------- | ---------------- | ------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| locationId    | string           | true         | The location ID (e.g. `us`, `us-central1`) where the Data Product should be created.                                                                                                                       |
| dataProductId | string           | false        | The unique ID of the Data Product to create. If not specified, the backend will auto-generate a unique ID.                                                                                                 |
| displayName   | string           | true         | The display name of the Data Product.                                                                                                                                                                      |
| description   | string           | false        | The description of the Data Product.                                                                                                                                                                       |
| ownerEmails   | array of strings | true         | The list of owner emails for the Data Product.                                                                                                                                                             |
| accessGroups  | array of objects | false        | List of access groups to associate with the Data Product. Each group object can contain: `id` (required), `displayName` (required), `description`, and at least one of `googleGroup` and `serviceAccount`. |

## Example

```yaml
kind: tool
name: create_data_product
type: dataplex-create-data-product
source: my-dataplex-source
description: Use this tool to create a Data Product.
```

## Reference

| **field**   | **type** | **required** | **description**                                    |
| ----------- | -------- | ------------ | -------------------------------------------------- |
| type        | string   | true         | Must be "dataplex-create-data-product".            |
| source      | string   | true         | Name of the source the tool should execute on.     |
| description | string   | true         | Description of the tool that is passed to the LLM. |
