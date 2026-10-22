---
title: "Snowflake Source"
linkTitle: "Source"
type: docs
weight: 1
description: >
  Snowflake is a cloud-based data platform.
no_list: true
---

## About

[Snowflake][sf-docs] is a cloud data platform that provides a data warehouse-as-a-service designed for the cloud.

[sf-docs]: https://docs.snowflake.com/



## Available Tools

{{< list-tools >}}

## Requirements

### Database User

You will need to create a Snowflake user to login to the database with. This
source supports password authentication and [key-pair
authentication][sf-keypair].

[sf-keypair]: https://docs.snowflake.com/en/user-guide/key-pair-auth

## Example

```yaml
kind: source
name: my-sf-source
type: snowflake
account: ${SNOWFLAKE_ACCOUNT}
user: ${SNOWFLAKE_USER}
password: ${SNOWFLAKE_PASSWORD}
database: ${SNOWFLAKE_DATABASE}
schema: ${SNOWFLAKE_SCHEMA}
warehouse: ${SNOWFLAKE_WAREHOUSE}
role: ${SNOWFLAKE_ROLE}
```

To use key-pair authentication, supply a private key instead of a password.
Encrypted and unencrypted PKCS#8 keys are both supported; omit
`privateKeyPassphrase` for an unencrypted key.

```yaml
kind: source
name: my-sf-source
type: snowflake
account: ${SNOWFLAKE_ACCOUNT}
user: ${SNOWFLAKE_USER}
privateKeyPath: ${SNOWFLAKE_PRIVATE_KEY_PATH}
privateKeyPassphrase: ${SNOWFLAKE_PRIVATE_KEY_PASSPHRASE}
database: ${SNOWFLAKE_DATABASE}
schema: ${SNOWFLAKE_SCHEMA}
```

{{< notice tip >}}
Use environment variable replacement with the format ${ENV_NAME}
instead of hardcoding your secrets into the configuration file.
{{< /notice >}}

## Reference

| **field**            | **type** | **required** | **description**                                                                                  |
|----------------------|:--------:|:------------:|--------------------------------------------------------------------------------------------------|
| type                 |  string  |     true     | Must be "snowflake".                                                                             |
| account              |  string  |     true     | Your Snowflake account identifier.                                                               |
| user                 |  string  |     true     | Name of the Snowflake user to connect as (e.g. "my-sf-user").                                    |
| password             |  string  |     false    | Password of the Snowflake user (e.g. "my-password"). Required unless a private key is set.       |
| privateKey           |  string  |     false    | PEM-encoded PKCS#8 private key for key-pair authentication.                                      |
| privateKeyPath       |  string  |     false    | Path to a PEM-encoded PKCS#8 private key file (e.g. "/path/to/rsa_key.p8").                       |
| privateKeyPassphrase |  string  |     false    | Passphrase of the private key. Only required if the key is encrypted.                            |
| database             |  string  |     true     | Name of the Snowflake database to connect to (e.g. "my_db").                                     |
| schema               |  string  |     true     | Name of the schema to use (e.g. "my_schema").                                                    |
| warehouse            |  string  |     false    | The virtual warehouse to use. Defaults to "COMPUTE_WH".                                          |
| role                 |  string  |     false    | The security role to use. Defaults to "ACCOUNTADMIN".                                            |

Exactly one of `password`, `privateKey` or `privateKeyPath` must be set.