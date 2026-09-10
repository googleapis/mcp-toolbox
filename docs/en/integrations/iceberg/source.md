---
title: "Iceberg Source"
linkTitle: "Source"
type: docs
weight: 1
description: >
  Apache Iceberg is an open table format for huge analytic datasets, managed
  through a catalog.
no_list: true
---

## About

[Apache Iceberg][iceberg-docs] is an open table format for large analytic
datasets. An Iceberg [catalog][iceberg-catalog] tracks the namespaces and
tables in a warehouse and serves each table's metadata: its schema, partition
spec, sort order, properties, and snapshots.

This source connects to an Iceberg [REST catalog][iceberg-rest-spec] using
[apache/iceberg-go][iceberg-go] and exposes read-only catalog exploration
tools. It talks only to the catalog API, so it needs no query engine and never
reads data files from the warehouse.

[iceberg-docs]: https://iceberg.apache.org/
[iceberg-catalog]: https://iceberg.apache.org/terms/#catalog
[iceberg-rest-spec]: https://github.com/apache/iceberg/blob/main/open-api/rest-catalog-open-api.yaml
[iceberg-go]: https://github.com/apache/iceberg-go

## Available Tools

{{< list-tools >}}

## Requirements

### Iceberg REST Catalog

You need a reachable Iceberg REST catalog endpoint, such as a self-hosted REST
catalog service or a managed catalog that exposes the Iceberg REST API. If the
catalog requires authentication, provide a bearer token via `accessToken`.

## Example

```yaml
kind: source
name: my-iceberg-source
type: iceberg
catalog: rest
uri: https://catalog.example.com
warehouse: examplewh
accessToken: ${ICEBERG_ACCESS_TOKEN}  # Optional for catalogs without auth
```

{{< notice tip >}}
Use environment variable replacement with the format ${ENV_NAME}
instead of hardcoding your secrets into the configuration file.
{{< /notice >}}

## Reference

| **field**   | **type** | **required** | **description**                                                                                  |
| ----------- | :------: | :----------: | ------------------------------------------------------------------------------------------------ |
| type        |  string  |     true     | Must be "iceberg".                                                                               |
| catalog     |  string  |    false     | The catalog flavor. Only "rest" is currently supported (default: "rest").                        |
| uri         |  string  |     true     | The base URI of the REST catalog (e.g. "https://catalog.example.com").                           |
| warehouse   |  string  |    false     | The warehouse to use, for catalogs that serve more than one.                                     |
| accessToken |  string  |    false     | Static bearer token sent on every catalog request. Leave unset for catalogs that do not need it. |
