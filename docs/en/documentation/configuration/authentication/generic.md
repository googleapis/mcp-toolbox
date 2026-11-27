---
title: "Generic OIDC Auth"
type: docs
weight: 2
description: >
  Use a Generic OpenID Connect (OIDC) provider for OAuth 2.0 flow and token
  lifecycle.
---

## Getting Started

The Generic Auth Service allows you to integrate with any OpenID Connect (OIDC)
compliant identity provider (IDP). It discovers the JWKS (JSON Web Key Set) URL
either through the provider's `/.well-known/openid-configuration` endpoint or
directly via the provided `authorizationServer`.

To configure this auth service, you need to provide the `audience` (the expected `aud` claim in the token), the `authorizationServer` of your identity provider, and optionally a list of `scopesRequired` that must be present in the token's claims.

> [!NOTE]
> The only time the `aud` claim matches the `client_id` is inside an ID Token (a concept from OpenID Connect used to verify a user's identity). Because an ID token is intended to be consumed by the client application itself, the client is the audience.

## Usage Modes

The Generic Auth Service supports two distinct modes of operation:

### 1. Toolbox Auth

This mode is used for Toolbox's native authentication/authorization features. It
is active when you reference the auth service in a tool's configuration and
`mcpEnabled` is set to false.

- **Header**: Expects the token in a custom header matching `<name>_token`
  (e.g., `my-generic-auth_token`).
- **Token Type**: Only supports **JWT** (OIDC) tokens.
- **Usage**: Used for [Authenticated Parameters][auth-params] and [Authorized
  Invocations][auth-invoke].

#### Token Validation

When a request is received in this mode, the service will:

1. Extract the token from the `<name>_token` header.
2. Treat it as a JWT (opaque tokens are not supported in this mode).
3. Validates signature using JWKS fetched from `authorizationServer`.
4. Verifies expiration (`exp`) and audience (`aud`).
5. Verifies required scopes in `scope` claim.

#### Example

```yaml
kind: authService
name: my-generic-auth
type: generic
audience: ${YOUR_OIDC_AUDIENCE}
authorizationServer: https://your-idp.example.com
# mcpEnabled: false
scopesRequired:
  - read
  - write
```

#### Tool Usage Example

To use this auth service for **Authenticated Parameters** or **Authorized
Invocations**, reference it in your tool configuration:

```yaml
kind: tool
name: secure_query
type: postgres-sql
source: my-pg-instance
statement: |
  SELECT * FROM data WHERE user_id = $1
parameters:
  - name: user_id
    type: strings
    description: Auto-populated from token
    authServices:
      - name: my-generic-auth
        field: sub # Extract 'sub' claim from JWT
authRequired:
  - my-generic-auth # Require valid token for invocation
```

### 2. MCP Authorization

This mode enforces global authentication for all MCP endpoints. It is active
when `mcpEnabled` is set to `true` in the auth service configuration.

- **Header**: Expects the token in the standard `Authorization: Bearer <token>`
  header.
- **Token Type**: Supports both **JWT** and **Opaque** tokens.
- **Usage**: Used to secure the entire MCP server.

#### Token Validation

When a request is received in this mode, the service will:

1. Extract the token from the `Authorization` header after `Bearer ` prefix.
2. Determine if the token is a JWT or an opaque token based on format (JWTs
   contain exactly two dots), unless `forceIntrospection` is set to `true` (see
   below), in which case every token is always treated as opaque.
3. For **JWTs**:
   - Validates signature using JWKS fetched from `authorizationServer`.
   - Verifies expiration (`exp`) and audience (`aud`).
   - Verifies required scopes in `scope` claim.
4. For **Opaque Tokens**:
   - Calls the introspection endpoint (either configured via `introspectionEndpoint`
     or discovered from the `authorizationServer`'s OIDC configuration).
   - Verifies expiration (`exp`) and audience (`aud` or `"audience"` fallback).
   - Verifies required scopes in `scope` field.

#### Example

```yaml
kind: authService
name: my-generic-auth
type: generic
audience: ${YOUR_TOKEN_AUDIENCE}
authorizationServer: https://your-idp.example.com
mcpEnabled: true
scopesRequired:
  - read
  - write
```

#### Forcing Introspection-Only Validation

Some identity providers issue JWT-formatted access tokens but still want
token revocation enforced via introspection (RFC 7662) rather than trusting a
locally-cached JWKS signature check.  Set `forceIntrospection: true` to always
validate tokens via the introspection endpoint, even when the token is JWT-shaped.

#### Authenticating to the Introspection Endpoint

Some identity providers require the caller to authenticate itself when
hitting the introspection endpoint (RFC 7662 client authentication), typically
via an `Authorization: Basic <base64(client_id:client_secret)>` header.  Since
a secret is required, these are set as environment variables instead:

```bash
export INTROSPECTION_CLIENT_ID="client_id"
export INTROSPECTION_CLIENT_SECRET="client_secret"
```

If both are set, an HTTP Basic Authorization header is added to every introspection
request it makes.  If neither are set, no Authorization header is added.  If only one is set,
a warning is logged and is treated as if neither were set.

#### Google Authentication Note

> [!WARNING]
> Do not configure Google's tokeninfo endpoint (`https://oauth2.googleapis.com/tokeninfo`) using `type: generic`. Because the generic OIDC service strictly enforces the presence and validity of the `active` claim (RFC 7662), and Google's tokeninfo endpoint does not return this claim, validation will fail.
>
> To authenticate with Google tokens, use the native [Google Sign-In](./google.md) auth service (`type: google`) instead, which natively handles Google's endpoints and token formats.

#### Okta OIDC Configuration Example

To secure your MCP server or tools using Okta as the identity provider:

```yaml
kind: authService
name: okta-auth
type: generic
audience: api://default # Or your custom Okta audience
authorizationServer: https://your-subdomain.okta.com/oauth2/default
mcpEnabled: true
scopesRequired:
  - openid
  - profile
```

> [!NOTE]
> If you are using Okta's Org Authorization Server (instead of a Custom Authorization Server), your `authorizationServer` URL will be `https://your-subdomain.okta.com`.

#### Tool-Level Scopes

When using MCP Authorization (with `mcpEnabled: true` in the auth service), you can enforce granular tool-level scope authorization by specifying the `scopesRequired` field in the tool configuration.

This ensures that a client can only invoke the tool if their authorization token contains all the specified scopes.

```yaml
kind: tool
name: update_flight_status
type: postgres-sql
source: my-pg-instance
statement: |
  UPDATE flights SET status = $1 WHERE flight_number = $2
description: Update flight status
authRequired:
  - my-generic-auth
scopesRequired:
  - execute:sql
  - write:flights
```

If a client attempts to invoke this tool without the required scopes, the server will return an HTTP 403 Forbidden response with a `WWW-Authenticate` header challenge indicating the missing scopes, as per the MCP Auth specification.

{{< notice tip >}} Use environment variable replacement with the format
${ENV_NAME} instead of hardcoding your secrets into the configuration file.
{{< /notice >}}

[auth-invoke]: ../tools/_index.md#authorized-invocations
[auth-params]: ../tools/_index.md#authenticated-parameters
[mcp-auth]: https://modelcontextprotocol.io/specification/2025-11-25/basic/authorization

## Reference

| **field**              | **type** | **required** | **description**                                                                                                                                                                                       |
| ---------------------- | :------: | :----------: | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| type                   |  string  |     true     | Must be "generic".                                                                                                                                                                                    |
| audience               |  string  |     true     | The expected audience (`aud` claim) in the token. This ensures the token was minted specifically for your application. See [Getting Started](#getting-started) for details on OIDC audience matching. |
| authorizationServer    |  string  |     true     | The base URL of your OIDC provider. The service will append `/.well-known/openid-configuration` to discover the JWKS URI. HTTP is allowed but logs a warning.                                         |
| mcpEnabled             |   bool   |    false     | Indicates if MCP endpoint authentication should be applied. Defaults to false.                                                                                                                        |
| scopesRequired         | []string |    false     | A list of required scopes that must be present in the token's `scope` claim to be considered valid. Disallowed if `mcpEnabled` is false.                                                             |
| introspectionEndpoint  |  string  |    false     | Optional override for the token introspection URL. Useful if the provider does not list it in OIDC discovery (e.g., Google). Disallowed if `mcpEnabled` is false.                                   |
| introspectionMethod    |  string  |    false     | HTTP method to use for introspection. Defaults to "POST". Set to "GET" for providers like Google. Disallowed if `mcpEnabled` is false.                                                               |
| introspectionParamName |  string  |    false     | Parameter name for the token in the introspection request. Defaults to "token". Set to "access_token" for Google. Disallowed if `mcpEnabled` is false.                                               |
| forceIntrospection     |  bool    |    false     | If true, always validates tokens via the introspection endpoint, even if the token is JWT-shaped. Disallowed if `mcpEnabled` is false.                                               |
