---
title: "http"
type: docs
weight: 1
description: >
  A "http" tool sends out an HTTP request to an HTTP endpoint.
---


## About

The `http` tool allows you to make HTTP requests to APIs to retrieve data.
An HTTP request is the method by which a client communicates with a server to
retrieve or manipulate resources.
Toolbox allows you to configure the request URL, method, headers, query
parameters, and the request body for an HTTP Tool.

## Compatible Sources

{{< compatible-sources >}}

### URL

An HTTP request URL identifies the target the client wants to access.
Toolbox composes the request URL from 3 places:

1. The HTTP Source's `baseUrl`.
2. The HTTP Tool's `path` field.
3. The HTTP Tool's `pathParams` for dynamic path composed during Tool
   invocation.

For example, the following config allows you to reach different paths of the
same server using multiple Tools:

```yaml
kind: source
name: my-http-source
type: http
baseUrl: https://api.example.com
---
kind: tool
name: my-post-tool
type: http
source: my-http-source
method: POST
path: /update
description: Tool to update information to the example API
---
kind: tool
name: my-get-tool
type: http
source: my-http-source
method: GET
path: /search
description: Tool to search information from the example API
---
kind: tool
name: my-dynamic-path-tool
type: http
source: my-http-source
method: GET
path: /{{.myPathParam}}/search
description: Tool to reach endpoint based on the input to `myPathParam`
pathParams:
    - name: myPathParam
      type: string
      description: The dynamic path parameter

```

### Headers

An HTTP request header is a key-value pair sent by a client to a server,
providing additional information about the request, such as the client's
preferences, the request body content type, and other metadata.
Headers specified by the HTTP Tool are combined with its HTTP Source headers for
the resulting HTTP request, and override the Source headers in case of conflict.
The HTTP Tool allows you to specify headers in two different ways:

- Static headers can be specified using the `headers` field, and will be the
  same for every invocation:

```yaml
kind: tool
name: my-http-tool
type: http
source: my-http-source
method: GET
path: /search
description: Tool to search data from API
headers:
  Authorization: API_KEY
  Content-Type: application/json
```

- Dynamic headers can be specified as parameters in the `headerParams` field.
  The `name` of the `headerParams` will be used as the header key, and the value
  is determined by the LLM input upon Tool invocation:

```yaml
kind: tool
name: my-http-tool
type: http
source: my-http-source
method: GET
path: /search
description: some description
headerParams:
  - name: Content-Type # Example LLM input: "application/json"
    description: request content type
    type: string
```

#### Google access tokens from ADC

For Google APIs that accept an OAuth 2.0 access token, set
`sendGoogleAccessToken` to `true`. Toolbox obtains a token from
[Application Default Credentials][adc-doc] using the Google Cloud Platform
scope and sends it as a Bearer token in the `Authorization` header:

```yaml
kind: tool
name: my-google-api-tool
type: http
source: my-http-source
method: GET
path: /search
description: Search a Google API
sendGoogleAccessToken: true
```

When enabled, the ADC-generated `Authorization` header overrides a header with
the same name configured on the source, in `headers`, or in `headerParams`.
Only enable this option for trusted HTTPS endpoints to avoid exposing the
access token.

### Query parameters

Query parameters are key-value pairs appended to a URL after a question mark (?)
to provide additional information to the server for processing the request, like
filtering or sorting data.

- Static request query parameters should be specified in the `path` as part of
  the URL itself:

```yaml
kind: tool
name: my-http-tool
type: http
source: my-http-source
method: GET
path: /search?language=en&id=1
description: Tool to search for item with ID 1 in English
```

- Dynamic request query parameters should be specified as parameters in the
  `queryParams` section:

```yaml
kind: tool
name: my-http-tool
type: http
source: my-http-source
method: GET
path: /search
description: Tool to search for item with ID
queryParams:
  - name: id
    description: item ID
    type: integer
```

### Request body

The request body payload is a string that supports parameter replacement
following [Go template][go-template-doc]'s annotations.
The parameter names in the `requestBody` should be preceded by "." and enclosed
by double curly brackets "{{}}". The values will be populated into the request
body payload upon Tool invocation.

Example:

```yaml
kind: tool
name: my-http-tool
type: http
source: my-http-source
method: GET
path: /search
description: Tool to search for person with name and age
requestBody: |
  {
    "age": {{.age}},
    "name": "{{.name}}"
  }
bodyParams:
  - name: age
    description: age number
    type: integer
  - name: name
    description: name string
    type: string
```

#### Formatting Parameters

Some complex parameters (such as arrays) may require additional formatting to
match the expected output. For convenience, you can specify one of the following
pre-defined functions before the parameter name to format it:

##### JSON

The `json` keyword converts a parameter into a JSON format.

{{< notice note >}}
Using JSON may add quotes to the variable name for certain types (such as
strings).
{{< /notice >}}

Example:

```yaml
requestBody: |
  {
    "age": {{json .age}},
    "name": {{json .name}},
    "nickname": "{{json .nickname}}",
    "nameArray": {{json .nameArray}}
  }
```

will send the following output:

```yaml
{
  "age": 18,
  "name": "Katherine",
  "nickname": ""Kat"", # Duplicate quotes
  "nameArray": ["A", "B", "C"]
}
```

##### Path Escape

The `pathEscape` keyword escapes strings so they can be safely placed inside a URL path segment, replacing special characters like slashes `/` with `%2F`.

Example:

```yaml
path: /users/{{pathEscape .username}}/records
```

##### Query Escape

The `queryEscape` keyword escapes strings to be safely placed inside a URL query string, encoding spaces into `+` and special characters into percent-encoded values.

Example:

```yaml
path: /search?q={{queryEscape .query}}
```

## Example

```yaml
kind: tool
name: my-http-tool
type: http
source: my-http-source
method: GET
path: /search
description: some description
authRequired:
  - my-google-auth-service
  - other-auth-service
queryParams:
  - name: country
    description: some description
    type: string
requestBody: |
  {
    "age": {{.age}},
    "city": "{{.city}}"
  }
bodyParams:
  - name: age
    description: age number
    type: integer
  - name: city
    description: city string
    type: string
headers:
  Authorization: API_KEY
  Content-Type: application/json
headerParams:
  - name: Language
    description: language string
    type: string
```

## Reference

| **field**    |                **type**                 | **required** | **description**                                                                                                                                                                                                            |
|--------------|:---------------------------------------:|:------------:|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| type         |                 string                  |     true     | Must be "http".                                                                                                                                                                                                            |
| source       |                 string                  |     true     | Name of the source the HTTP request should be sent to.                                                                                                                                                                     |
| description  |                 string                  |     true     | Description of the tool that is passed to the LLM.                                                                                                                                                                         |
| path         |                 string                  |     true     | The path of the HTTP request. You can include static query parameters in the path string.                                                                                                                                  |
| method       |                 string                  |     true     | The HTTP method to use (e.g., GET, POST, PUT, DELETE).                                                                                                                                                                     |
| headers      |            map[string]string            |    false     | A map of headers to include in the HTTP request (overrides source headers).                                                                                                                                                |
| requestBody  |                 string                  |    false     | The request body payload. Use [go template][go-template-doc] with the parameter name as the placeholder (e.g., `{{.id}}` will be replaced with the value of the parameter that has name `id` in the `bodyParams` section). |
| queryParams  | [parameters](../../../documentation/configuration/tools/_index.md#specifying-parameters) |    false     | List of [parameters](../../../documentation/configuration/tools/_index.md#specifying-parameters) that will be inserted into the query string.                                                                                                                               |
| bodyParams   | [parameters](../../../documentation/configuration/tools/_index.md#specifying-parameters) |    false     | List of [parameters](../../../documentation/configuration/tools/_index.md#specifying-parameters) that will be inserted into the request body payload.                                                                                                                       |
| headerParams | [parameters](../../../documentation/configuration/tools/_index.md#specifying-parameters) |    false     | List of [parameters](../../../documentation/configuration/tools/_index.md#specifying-parameters) that will be inserted as the request headers.                                                                                                                              |
| sendGoogleAccessToken | boolean | false | Whether to obtain an OAuth 2.0 access token from Application Default Credentials using the Google Cloud Platform scope and send it as a Bearer token in the `Authorization` header. |

[go-template-doc]: <https://pkg.go.dev/text/template#pkg-overview>
[adc-doc]: <https://cloud.google.com/docs/authentication/application-default-credentials>
