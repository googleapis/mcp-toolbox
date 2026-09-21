// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

/**
 * This file is automatically vendored from the Model Context Protocol specification.
 *
 * Source: https://github.com/modelcontextprotocol/modelcontextprotocol/blob/f7e99af6417ec978233d8e27e1ac878b12106542/schema/draft/schema.ts
 * Last updated from commit: f7e99af6417ec978233d8e27e1ac878b12106542
 *
 * Immutable 2026-07-28 snapshot. Do not edit; update schema/draft/ instead
 */
/* JSON types */

/**
 * @category Common Types
 */
export type JSONValue =
  string | number | boolean | null | JSONObject | JSONArray;

/**
 * @category Common Types
 */
export type JSONObject = { [key: string]: JSONValue };

/**
 * @category Common Types
 */
export type JSONArray = JSONValue[];

/* JSON-RPC types */

/**
 * Refers to any valid JSON-RPC object that can be decoded off the wire, or encoded to be sent.
 *
 * @category JSON-RPC
 */
export type JSONRPCMessage =
  JSONRPCRequest | JSONRPCNotification | JSONRPCResponse;

/** @internal */
export const LATEST_PROTOCOL_VERSION = "2026-07-28";
/** @internal */
export const JSONRPC_VERSION = "2.0";

/**
 * Represents the contents of a `_meta` field, which clients and servers use to attach additional metadata to their interactions.
 *
 * Certain key names are reserved by MCP for protocol-level metadata; implementations MUST NOT make assumptions about values at these keys. Additionally, specific schema definitions may reserve particular names for purpose-specific metadata, as declared in those definitions.
 *
 * Valid keys have two segments:
 *
 * **Prefix:**
 * - Optional — if specified, MUST be a series of _labels_ separated by dots (`.`), followed by a slash (`/`).
 * - Labels MUST start with a letter and end with a letter or digit. Interior characters may be letters, digits, or hyphens (`-`).
 * - Implementations SHOULD use reverse DNS notation (e.g., `com.example/` rather than `example.com/`).
 * - Any prefix where the second label is `modelcontextprotocol` or `mcp` is **reserved** for MCP use. For example: `io.modelcontextprotocol/`, `dev.mcp/`, `org.modelcontextprotocol.api/`, and `com.mcp.tools/` are all reserved. However, `com.example.mcp/` is NOT reserved, as the second label is `example`.
 *
 * **Name:**
 * - Unless empty, MUST start and end with an alphanumeric character (`[a-z0-9A-Z]`).
 * - Interior characters may be alphanumeric, hyphens (`-`), underscores (`_`), or dots (`.`).
 *
 * @see [General fields: `_meta`](/specification/draft/basic/index#meta) for more details.
 * @category Common Types
 */
export type MetaObject = Record<string, unknown>;

/**
 * Extends {@link MetaObject} with additional request-specific fields. All key naming rules from `MetaObject` apply.
 *
 * @see {@link MetaObject} for key naming rules and reserved prefixes.
 * @see [General fields: `_meta`](/specification/draft/basic/index#meta) for more details.
 * @category Common Types
 */
export interface RequestMetaObject extends MetaObject {
  /**
   * If specified, the caller is requesting out-of-band progress notifications for this request (as represented by {@link ProgressNotification | notifications/progress}). The value of this parameter is an opaque token that will be attached to any subsequent notifications. The receiver is not obligated to provide these notifications.
   */
  progressToken?: ProgressToken;
  /**
   * The MCP Protocol Version being used for this request. Required.
   *
   * For the HTTP transport, this value MUST match the `MCP-Protocol-Version`
   * header; otherwise the server MUST return a `400 Bad Request`. If the
   * server does not support the requested version, it MUST return an
   * {@link UnsupportedProtocolVersionError}.
   */
  "io.modelcontextprotocol/protocolVersion": string;
  /**
   * Identifies the client software making the request. Clients SHOULD
   * include this field on every request unless specifically configured not
   * to do so.
   *
   * The {@link Implementation} schema requires `name` and `version`; other
   * fields are optional.
   *
   * The value is self-reported by the client and is not verified by the
   * protocol. It is intended for display, logging, and debugging. Servers
   * SHOULD NOT use it to change their behavior, and SHOULD NOT rely on it for
   * security decisions.
   */
  "io.modelcontextprotocol/clientInfo"?: Implementation;
  /**
   * The client's capabilities for this specific request. Required.
   *
   * Capabilities are declared per-request rather than once at initialization;
   * an empty object means the client supports no optional capabilities.
   * Servers MUST NOT infer capabilities from prior requests.
   */
  "io.modelcontextprotocol/clientCapabilities": ClientCapabilities;
  /**
   * The desired log level for this request. Optional.
   *
   * If absent, the server MUST NOT send any {@link LoggingMessageNotification | notifications/message}
   * notifications for this request. The client opts in to log messages by
   * explicitly setting a level. Replaces the former `logging/setLevel` RPC.
   *
   * @deprecated Deprecated as of protocol version 2026-07-28 (SEP-2577).
   * Remains in the specification for at least twelve months; see the
   * deprecated features registry.
   */
  "io.modelcontextprotocol/logLevel"?: LoggingLevel;
}

/**
 * Extends {@link MetaObject} with additional notification-specific fields. All key naming rules from `MetaObject` apply.
 *
 * @see {@link MetaObject} for key naming rules and reserved prefixes.
 * @see [General fields: `_meta`](/specification/draft/basic/index#meta) for more details.
 * @category Common Types
 */
export interface NotificationMetaObject extends MetaObject {
  /**
   * Identifies the subscription stream a notification was delivered on. The
   * server MUST include this key on every notification delivered via a
   * {@link SubscriptionsListenRequest | subscriptions/listen} stream, so the
   * client can correlate the notification with the originating subscription.
   * The key is absent on notifications not delivered via a subscription
   * stream (e.g. progress notifications for an in-flight request), which is
   * why it is optional here.
   *
   * The value is the JSON-RPC ID of the `subscriptions/listen` request that
   * opened the stream.
   */
  "io.modelcontextprotocol/subscriptionId"?: RequestId;
}

/**
 * Extends {@link MetaObject} with additional result-specific fields. All key naming rules from `MetaObject` apply.
 *
 * @see {@link MetaObject} for key naming rules and reserved prefixes.
 * @see [General fields: `_meta`](/specification/draft/basic/index#meta) for more details.
 * @category Common Types
 */
export interface ResultMetaObject extends MetaObject {
  /**
   * Identifies the server software producing the response. Servers SHOULD
   * include this field on every response unless specifically configured not
   * to do so.
   *
   * The {@link Implementation} schema requires `name` and `version`; other
   * fields are optional.
   *
   * The value is self-reported by the server and is not verified by the
   * protocol. It is intended for display, logging, and debugging. Clients
   * SHOULD NOT use it to change their behavior, and SHOULD NOT rely on it for
   * security decisions.
   */
  "io.modelcontextprotocol/serverInfo"?: Implementation;
}

/**
 * A progress token, used to associate progress notifications with the original request.
 *
 * @category Common Types
 */
export type ProgressToken = string | number;

/**
 * An opaque token used to represent a cursor for pagination.
 *
 * @category Common Types
 */
export type Cursor = string;

/**
 * Common params for any request.
 *
 * @category Common Types
 */
export interface RequestParams {
  _meta: RequestMetaObject;
}

/** @internal */
export interface Request {
  method: string;
  // Allow unofficial extensions of `Request.params` without impacting `RequestParams`.
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  params?: { [key: string]: any };
}

/**
 * Common params for any notification.
 *
 * @category Common Types
 */
export interface NotificationParams {
  _meta?: NotificationMetaObject;
}

/** @internal */
export interface Notification {
  method: string;
  // Allow unofficial extensions of `Notification.params` without impacting `NotificationParams`.
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  params?: { [key: string]: any };
}

/**
 * Indicates the type of a {@link Result} object, allowing the client to
 * determine how to parse the response.
 *
 * complete - the request completed successfully and the result contains the final content.
 * input_required - the request requires additional input and the result contains an {@link InputRequiredResult} object with instructions for the client to provide additional input before retrying the original request.
 * @category Common Types
 */
export type ResultType = "complete" | "input_required" | string;

/**
 * Common result fields.
 *
 * @category Common Types
 */
export interface Result {
  _meta?: ResultMetaObject;
  /**
   * Indicates the type of the result, which allows the client to determine
   * how to parse the result object.
   *
   * Servers implementing this protocol version MUST include this field.
   * For backward compatibility, when a client receives a result from a
   * server implementing an earlier protocol version (which does not include
   * `resultType`), the client MUST treat the absent field as `"complete"`.
   */
  resultType: ResultType;
  [key: string]: unknown;
}

/**
 * @category Errors
 */
export interface Error {
  /**
   * The error type that occurred.
   */
  code: number;
  /**
   * A short description of the error. The message SHOULD be limited to a concise single sentence.
   */
  message: string;
  /**
   * Additional information about the error. The value of this member is defined by the sender (e.g. detailed error information, nested errors etc.).
   */
  data?: unknown;
}

/**
 * A uniquely identifying ID for a request in JSON-RPC.
 *
 * @category Common Types
 */
export type RequestId = string | number;

/**
 * A request that expects a response.
 *
 * @category JSON-RPC
 */
export interface JSONRPCRequest extends Request {
  jsonrpc: typeof JSONRPC_VERSION;
  id: RequestId;
}

/**
 * A notification which does not expect a response.
 *
 * @category JSON-RPC
 */
export interface JSONRPCNotification extends Notification {
  jsonrpc: typeof JSONRPC_VERSION;
}

/**
 * A successful (non-error) response to a request.
 *
 * @category JSON-RPC
 */
export interface JSONRPCResultResponse {
  jsonrpc: typeof JSONRPC_VERSION;
  id: RequestId;
  result: Result;
}

/**
 * A response to a request that indicates an error occurred.
 *
 * @category JSON-RPC
 */
export interface JSONRPCErrorResponse {
  jsonrpc: typeof JSONRPC_VERSION;
  id?: RequestId;
  error: Error;
}

/**
 * A response to a request, containing either the result or error.
 *
 * @category JSON-RPC
 */
export type JSONRPCResponse = JSONRPCResultResponse | JSONRPCErrorResponse;

// Standard JSON-RPC error codes
export const PARSE_ERROR = -32700;
export const INVALID_REQUEST = -32600;
export const METHOD_NOT_FOUND = -32601;
export const INVALID_PARAMS = -32602;
export const INTERNAL_ERROR = -32603;

/**
 * A JSON-RPC error indicating that invalid JSON was received by the server. This error is returned when the server cannot parse the JSON text of a message.
 *
 * @see {@link https://www.jsonrpc.org/specification#error_object | JSON-RPC 2.0 Error Object}
 *
 * @example Invalid JSON
 * {@includeCode ./examples/ParseError/invalid-json.json}
 *
 * @category Errors
 */
export interface ParseError extends Error {
  code: typeof PARSE_ERROR;
}

/**
 * A JSON-RPC error indicating that the request is not a valid request object. This error is returned when the message structure does not conform to the JSON-RPC 2.0 specification requirements for a request (e.g., missing required fields like `jsonrpc` or `method`, or using invalid types for these fields).
 *
 * @see {@link https://www.jsonrpc.org/specification#error_object | JSON-RPC 2.0 Error Object}
 *
 * @category Errors
 */
export interface InvalidRequestError extends Error {
  code: typeof INVALID_REQUEST;
}

/**
 * A JSON-RPC error indicating that the requested method does not exist or is not available.
 *
 * In MCP, a server returns this error when a client invokes a method the server does not implement — either a genuinely unknown method, or one gated behind a server capability the server did not advertise (e.g., calling `prompts/list` when the `prompts` capability was not advertised).
 *
 * A request that requires a client capability the client did not declare is signalled instead by {@link MissingRequiredClientCapabilityError} (`-32021`).
 *
 * @see {@link https://www.jsonrpc.org/specification#error_object | JSON-RPC 2.0 Error Object}
 *
 * @example Prompts not supported
 * {@includeCode ./examples/MethodNotFoundError/prompts-not-supported.json}
 *
 * @category Errors
 */
export interface MethodNotFoundError extends Error {
  code: typeof METHOD_NOT_FOUND;
}

/**
 * A JSON-RPC error indicating that the method parameters are invalid or malformed.
 *
 * In MCP, this error is returned in various contexts when request parameters fail validation:
 *
 * - **Tools**: Unknown tool name or invalid tool arguments
 * - **Prompts**: Unknown prompt name or missing required arguments
 * - **Pagination**: Invalid or expired cursor values
 * - **Logging**: Invalid log level
 * - **Elicitation**: Server requests an elicitation mode not declared in client capabilities
 * - **Sampling**: Missing tool result or tool results mixed with other content
 *
 * @see {@link https://www.jsonrpc.org/specification#error_object | JSON-RPC 2.0 Error Object}
 *
 * @example Unknown tool
 * {@includeCode ./examples/InvalidParamsError/unknown-tool.json}
 *
 * @example Invalid tool arguments
 * {@includeCode ./examples/InvalidParamsError/invalid-tool-arguments.json}
 *
 * @example Unknown prompt
 * {@includeCode ./examples/InvalidParamsError/unknown-prompt.json}
 *
 * @example Invalid cursor
 * {@includeCode ./examples/InvalidParamsError/invalid-cursor.json}
 *
 * @category Errors
 */
export interface InvalidParamsError extends Error {
  code: typeof INVALID_PARAMS;
}

/**
 * A JSON-RPC error indicating that an internal error occurred on the receiver. This error is returned when the receiver encounters an unexpected condition that prevents it from fulfilling the request.
 *
 * @see {@link https://www.jsonrpc.org/specification#error_object | JSON-RPC 2.0 Error Object}
 *
 * @example Unexpected error
 * {@includeCode ./examples/InternalError/unexpected-error.json}
 *
 * @category Errors
 */
export interface InternalError extends Error {
  code: typeof INTERNAL_ERROR;
}

/*
 * MCP error codes.
 *
 * JSON-RPC 2.0 reserves `-32000` to `-32099` for implementation-defined
 * server errors. MCP partitions that range:
 *
 * - `-32000` to `-32019`: implementation-defined. Existing SDKs and
 *   implementations use codes here for their own purposes; the specification
 *   will never define codes in this sub-range, and receivers must not assign
 *   cross-implementation semantics to them.
 * - `-32020` to `-32099`: reserved for error codes defined by the MCP
 *   specification. Every code allocated here is recorded in this file.
 *   Codes are allocated sequentially starting at `-32020` and proceeding
 *   toward `-32099`.
 *
 * Codes defined by earlier protocol versions remain reserved and are never
 * reused: `-32002` (resource not found, 2025-11-25 and earlier; replaced by
 * `-32602`) and `-32042` (URL elicitation required, 2025-11-25 only).
 */

/**
 * Error code returned when the HTTP headers of a request do not match the
 * corresponding values in the request body, or required headers are
 * missing or malformed.
 *
 * @category Errors
 */
export const HEADER_MISMATCH = -32020;

/**
 * Error code returned when a server requires a client capability that was
 * not declared in the request's `clientCapabilities`.
 *
 * @category Errors
 */
export const MISSING_REQUIRED_CLIENT_CAPABILITY = -32021;

/**
 * Error code returned when the request's protocol version is not supported
 * by the server.
 *
 * @category Errors
 */
export const UNSUPPORTED_PROTOCOL_VERSION = -32022;

/**
 * Returned when a server rejects a request because the values in the HTTP
 * headers do not match the corresponding values in the request body, or
 * because required headers are missing or malformed. For HTTP, the response
 * status code MUST be `400 Bad Request`.
 *
 * @example Header mismatch
 * {@includeCode ./examples/HeaderMismatchError/header-mismatch.json}
 *
 * @category Errors
 */
export interface HeaderMismatchError extends Omit<
  JSONRPCErrorResponse,
  "error"
> {
  error: Error & {
    code: typeof HEADER_MISMATCH;
  };
}

/**
 * Returned when the request's protocol version is unknown to the server or
 * unsupported (e.g., a known experimental or draft version the server has
 * chosen not to implement). For HTTP, the response status code MUST be
 * `400 Bad Request`.
 *
 * @example Unsupported protocol version
 * {@includeCode ./examples/UnsupportedProtocolVersionError/unsupported-version.json}
 *
 * @category Errors
 */
export interface UnsupportedProtocolVersionError extends Omit<
  JSONRPCErrorResponse,
  "error"
> {
  error: Error & {
    code: typeof UNSUPPORTED_PROTOCOL_VERSION;
    data: {
      /**
       * Protocol versions the server supports. The client should choose a
       * mutually supported version from this list and retry.
       */
      supported: string[];
      /**
       * The protocol version that was requested by the client.
       */
      requested: string;
    };
  };
}

/**
 * Returned when processing a request requires a capability the client did not
 * declare in `clientCapabilities`. For HTTP, the response status code MUST be
 * `400 Bad Request`.
 *
 * @example Missing elicitation capability
 * {@includeCode ./examples/MissingRequiredClientCapabilityError/missing-elicitation-capability.json}
 *
 * @category Errors
 */
export interface MissingRequiredClientCapabilityError extends Omit<
  JSONRPCErrorResponse,
  "error"
> {
  error: Error & {
    code: typeof MISSING_REQUIRED_CLIENT_CAPABILITY;
    data: {
      /**
       * The capabilities the server requires from the client to process this request.
       */
      requiredCapabilities: ClientCapabilities;
    };
  };
}

/* Empty result */
/**
 * A result that indicates success but carries no data.
 *
 * @category Common Types
 */
export type EmptyResult = Result;

/** @internal */
export type InputRequest =
  CreateMessageRequest | ListRootsRequest | ElicitRequest;

/** @internal */
export type InputResponse =
  CreateMessageResult | ListRootsResult | ElicitResult;

/**
 * A map of server-initiated requests that the client must fulfill.
 * Keys are server-assigned identifiers; values are the request objects.
 *
 * @example Elicitation and sampling input requests
 * {@includeCode ./examples/InputRequests/elicitation-and-sampling-input-requests.json}
 *
 * @category Multi Round-Trip
 */
export interface InputRequests {
  [key: string]: InputRequest;
}

/**
 * A map of client responses to server-initiated requests.
 * Keys correspond to the keys in the {@link InputRequests} map;
 * values are the client's result for each request.
 *
 * @example Elicitation and sampling input responses
 * {@includeCode ./examples/InputResponses/elicitation-and-sampling-input-responses.json}
 *
 * @category Multi Round-Trip
 */
export interface InputResponses {
  [key: string]: InputResponse;
}

/**
 * An InputRequiredResult sent by the server to indicate that additional input is needed
 * before the request can be completed.
 *
 * At least one of `inputRequests` or `requestState` MUST be present.
 * @example InputRequiredResult with elicitation and sampling input requests and request state
 * {@includeCode ./examples/InputRequiredResult/input-required-result-with-elicitation-and-sampling-and-request-state.json}
 *
 * @example InputRequiredResult with request state only (load shedding)
 * {@includeCode ./examples/InputRequiredResult/input-required-result-with-request-state-only.json}
 *
 * @category Multi Round-Trip
 */
export interface InputRequiredResult extends Result {
  /* Requests issued by the server that must be complete before the
   * client can retry the original request.
   */
  inputRequests?: InputRequests;
  /* Request state to be passed back to the server when the client
   * retries the original request.
   * Note: The client must treat this as an opaque blob; it must not
   * interpret it in any way.
   */
  requestState?: string;
}

/* Request parameter type that includes input responses and request state.
 * These parameters may be included in any client-initiated request.
 */
export interface InputResponseRequestParams extends RequestParams {
  /* New field to carry the responses for the server's requests from the
   * InputRequiredResult message.  For each key in the response's inputRequests
   * field, the same key must appear here with the associated response.
   */
  inputResponses?: InputResponses;
  /* Request state passed back to the server from the client.
   */
  requestState?: string;
}

/* Cancellation */
/**
 * Parameters for a `notifications/cancelled` notification.
 *
 * @example User-requested cancellation
 * {@includeCode ./examples/CancelledNotificationParams/user-requested-cancellation.json}
 *
 * @category `notifications/cancelled`
 */
export interface CancelledNotificationParams extends NotificationParams {
  /**
   * The ID of the request to cancel.
   *
   * This MUST correspond to the ID of a request the client previously issued.
   */
  requestId: RequestId;

  /**
   * An optional string describing the reason for the cancellation. This MAY be logged or presented to the user.
   */
  reason?: string;
}

/**
 * This notification is sent by the client to indicate that it is cancelling a request it previously issued.
 *
 * On stdio, the server also sends this notification, solely to terminate a {@link SubscriptionsListenRequest | subscriptions/listen} stream: it references the ID of the `subscriptions/listen` request that opened the stream. Servers MUST NOT use this notification to cancel any other request.
 *
 * The request SHOULD still be in-flight, but due to communication latency, it is always possible that this notification MAY arrive after the request has already finished.
 *
 * This notification indicates that the result will be unused, so any associated processing SHOULD cease.
 *
 * @example User-requested cancellation
 * {@includeCode ./examples/CancelledNotification/user-requested-cancellation.json}
 *
 * @category `notifications/cancelled`
 */
export interface CancelledNotification extends JSONRPCNotification {
  method: "notifications/cancelled";
  params: CancelledNotificationParams;
}

/* Discovery */
/**
 * A request from the client asking the server to advertise its supported
 * protocol versions, capabilities, and other metadata. Servers **MUST**
 * implement `server/discover`. Clients **MAY** call it but are not required
 * to — version negotiation can also happen inline via per-request `_meta`.
 *
 * @example Discover request
 * {@includeCode ./examples/DiscoverRequest/server-discover-request.json}
 *
 * @category `server/discover`
 */
export interface DiscoverRequest extends JSONRPCRequest {
  method: "server/discover";
  params: RequestParams;
}

/**
 * The result returned by the server for a {@link DiscoverRequest | server/discover} request.
 *
 * @example Server capabilities discovery
 * {@includeCode ./examples/DiscoverResult/server-capabilities-discovery.json}
 *
 * @category `server/discover`
 */
export interface DiscoverResult extends CacheableResult {
  /**
   * MCP Protocol Versions this server supports. The client should choose a
   * version from this list for use in subsequent requests.
   */
  supportedVersions: string[];
  /**
   * The capabilities of the server.
   */
  capabilities: ServerCapabilities;
  /**
   * Natural-language guidance describing the server and its features.
   *
   * This can be used by clients to improve an LLM's understanding of
   * available tools (e.g., by including it in a system prompt). It should
   * focus on information that helps the model use the server effectively
   * and should not duplicate information already in tool descriptions.
   */
  instructions?: string;
}

/**
 * A successful response from the server for a {@link DiscoverRequest | server/discover} request.
 *
 * @example Discover result response
 * {@includeCode ./examples/DiscoverResultResponse/discover-result-response.json}
 *
 * @category `server/discover`
 */
export interface DiscoverResultResponse extends JSONRPCResultResponse {
  result: DiscoverResult;
}

/**
 * Capabilities a client may support. Known capabilities are defined here, in this schema, but this is not a closed set: any client can define its own, additional capabilities.
 *
 * @category `server/discover`
 */
export interface ClientCapabilities {
  /**
   * Experimental, non-standard capabilities that the client supports.
   */
  experimental?: { [key: string]: JSONObject };
  /**
   * Present if the client supports listing roots.
   *
   * @deprecated Deprecated as of protocol version 2026-07-28 (SEP-2577).
   * Remains in the specification for at least twelve months; see the
   * deprecated features registry.
   *
   * @example Roots — minimum baseline support
   * {@includeCode ./examples/ClientCapabilities/roots-minimum-baseline-support.json}
   */
  // eslint-disable-next-line @typescript-eslint/no-empty-object-type
  roots?: {};
  /**
   * Present if the client supports sampling from an LLM.
   *
   * @deprecated Deprecated as of protocol version 2026-07-28 (SEP-2577).
   * Remains in the specification for at least twelve months; see the
   * deprecated features registry.
   *
   * @example Sampling — minimum baseline support
   * {@includeCode ./examples/ClientCapabilities/sampling-minimum-baseline-support.json}
   *
   * @example Sampling — tool use support
   * {@includeCode ./examples/ClientCapabilities/sampling-tool-use-support.json}
   *
   * @example Sampling — context inclusion support (deprecated)
   * {@includeCode ./examples/ClientCapabilities/sampling-context-inclusion-support-deprecated.json}
   */
  sampling?: {
    /**
     * Whether the client supports context inclusion via `includeContext` parameter.
     * If not declared, servers SHOULD only use `includeContext: "none"` (or omit it).
     */
    context?: JSONObject;
    /**
     * Whether the client supports tool use via `tools` and `toolChoice` parameters.
     */
    tools?: JSONObject;
  };
  /**
   * Present if the client supports elicitation from the server.
   *
   * @example Elicitation — form and URL mode support
   * {@includeCode ./examples/ClientCapabilities/elicitation-form-and-url-mode-support.json}
   *
   * @example Elicitation — form mode only (implicit)
   * {@includeCode ./examples/ClientCapabilities/elicitation-form-only-implicit.json}
   */
  elicitation?: {
    form?: JSONObject;
    url?: JSONObject;
  };

  /**
   * Optional MCP extensions that the client supports. Keys are extension identifiers
   * (e.g., "io.modelcontextprotocol/oauth-client-credentials"), and values are
   * per-extension settings objects. An empty object indicates support with no settings.
   *
   * Keys MUST follow the {@link MetaObject | `_meta` key naming rules}, with a
   * mandatory prefix.
   *
   * @example Extensions — MCP Apps (UI) extension with MIME type support
   * {@includeCode ./examples/ClientCapabilities/extensions-ui-mime-types.json}
   */
  extensions?: { [key: string]: JSONObject };
}

/**
 * Capabilities that a server may support. Known capabilities are defined here, in this schema, but this is not a closed set: any server can define its own, additional capabilities.
 *
 * @category `server/discover`
 */
export interface ServerCapabilities {
  /**
   * Experimental, non-standard capabilities that the server supports.
   */
  experimental?: { [key: string]: JSONObject };
  /**
   * Present if the server supports sending log messages to the client.
   *
   * @deprecated Deprecated as of protocol version 2026-07-28 (SEP-2577).
   * Remains in the specification for at least twelve months; see the
   * deprecated features registry.
   *
   * @example Logging — minimum baseline support
   * {@includeCode ./examples/ServerCapabilities/logging-minimum-baseline-support.json}
   */
  logging?: JSONObject;
  /**
   * Present if the server supports argument autocompletion suggestions.
   *
   * @example Completions — minimum baseline support
   * {@includeCode ./examples/ServerCapabilities/completions-minimum-baseline-support.json}
   */
  completions?: JSONObject;
  /**
   * Present if the server offers any prompt templates.
   *
   * @example Prompts — minimum baseline support
   * {@includeCode ./examples/ServerCapabilities/prompts-minimum-baseline-support.json}
   *
   * @example Prompts — list changed notifications
   * {@includeCode ./examples/ServerCapabilities/prompts-list-changed-notifications.json}
   */
  prompts?: {
    /**
     * Whether this server supports notifications for changes to the prompt list.
     */
    listChanged?: boolean;
  };
  /**
   * Present if the server offers any resources to read.
   *
   * @example Resources — minimum baseline support
   * {@includeCode ./examples/ServerCapabilities/resources-minimum-baseline-support.json}
   *
   * @example Resources — subscription to individual resource updates (only)
   * {@includeCode ./examples/ServerCapabilities/resources-subscription-to-individual-resource-updates-only.json}
   *
   * @example Resources — list changed notifications (only)
   * {@includeCode ./examples/ServerCapabilities/resources-list-changed-notifications-only.json}
   *
   * @example Resources — all notifications
   * {@includeCode ./examples/ServerCapabilities/resources-all-notifications.json}
   */
  resources?: {
    /**
     * Whether this server supports subscribing to resource updates.
     */
    subscribe?: boolean;
    /**
     * Whether this server supports notifications for changes to the resource list.
     */
    listChanged?: boolean;
  };
  /**
   * Present if the server offers any tools to call.
   *
   * @example Tools — minimum baseline support
   * {@includeCode ./examples/ServerCapabilities/tools-minimum-baseline-support.json}
   *
   * @example Tools — list changed notifications
   * {@includeCode ./examples/ServerCapabilities/tools-list-changed-notifications.json}
   */
  tools?: {
    /**
     * Whether this server supports notifications for changes to the tool list.
     */
    listChanged?: boolean;
  };
  /**
   * Optional MCP extensions that the server supports. Keys are extension identifiers
   * (e.g., "io.modelcontextprotocol/tasks"), and values are per-extension settings
   * objects. An empty object indicates support with no settings.
   *
   * Keys MUST follow the {@link MetaObject | `_meta` key naming rules}, with a
   * mandatory prefix.
   *
   * @example Extensions — Tasks extension support
   * {@includeCode ./examples/ServerCapabilities/extensions-tasks.json}
   */
  extensions?: { [key: string]: JSONObject };
}

/**
 * An optionally-sized icon that can be displayed in a user interface.
 *
 * @category Common Types
 */
export interface Icon {
  /**
   * A standard URI pointing to an icon resource. May be an HTTP/HTTPS URL or a
   * `data:` URI with Base64-encoded image data.
   *
   * Consumers SHOULD take steps to ensure URLs serving icons are from the
   * same domain as the client/server or a trusted domain.
   *
   * Consumers SHOULD take appropriate precautions when consuming SVGs as they can contain
   * executable JavaScript.
   *
   * @format uri
   */
  src: string;

  /**
   * Optional MIME type override if the source MIME type is missing or generic.
   * For example: `"image/png"`, `"image/jpeg"`, or `"image/svg+xml"`.
   */
  mimeType?: string;

  /**
   * Optional array of strings that specify sizes at which the icon can be used.
   * Each string should be in WxH format (e.g., `"48x48"`, `"96x96"`) or `"any"` for scalable formats like SVG.
   *
   * If not provided, the client should assume that the icon can be used at any size.
   */
  sizes?: string[];

  /**
   * Optional specifier for the theme this icon is designed for. `"light"` indicates
   * the icon is designed to be used with a light background, and `"dark"` indicates
   * the icon is designed to be used with a dark background.
   *
   * If not provided, the client should assume the icon can be used with any theme.
   */
  theme?: "light" | "dark";
}

/**
 * Base interface to add `icons` property.
 *
 * @internal
 */
export interface Icons {
  /**
   * Optional set of sized icons that the client can display in a user interface.
   *
   * Clients that support rendering icons MUST support at least the following MIME types:
   * - `image/png` - PNG images (safe, universal compatibility)
   * - `image/jpeg` (and `image/jpg`) - JPEG images (safe, universal compatibility)
   *
   * Clients that support rendering icons SHOULD also support:
   * - `image/svg+xml` - SVG images (scalable but requires security precautions)
   * - `image/webp` - WebP images (modern, efficient format)
   */
  icons?: Icon[];
}

/**
 * Base interface for metadata with name (identifier) and title (display name) properties.
 *
 * @internal
 */
export interface BaseMetadata {
  /**
   * Intended for programmatic or logical use, but used as a display name in past specs or fallback (if title isn't present).
   */
  name: string;

  /**
   * Intended for UI and end-user contexts — optimized to be human-readable and easily understood,
   * even by those unfamiliar with domain-specific terminology.
   *
   * If not provided, the name should be used for display (except for {@link Tool},
   * where `annotations.title` should be given precedence over using `name`,
   * if present).
   */
  title?: string;
}

/**
 * Describes the MCP implementation.
 *
 * @category `server/discover`
 */
export interface Implementation extends BaseMetadata, Icons {
  /**
   * The version of this implementation.
   */
  version: string;

  /**
   * An optional human-readable description of what this implementation does.
   *
   * This can be used by clients or servers to provide context about their purpose
   * and capabilities. For example, a server might describe the types of resources
   * or tools it provides, while a client might describe its intended use case.
   */
  description?: string;

  /**
   * An optional URL of the website for this implementation.
   *
   * @format uri
   */
  websiteUrl?: string;
}

/* Progress notifications */

/**
 * Parameters for a {@link ProgressNotification | notifications/progress} notification.
 *
 * @example Progress message
 * {@includeCode ./examples/ProgressNotificationParams/progress-message.json}
 *
 * @category `notifications/progress`
 */
export interface ProgressNotificationParams extends NotificationParams {
  /**
   * The progress token which was given in the initial request, used to associate this notification with the request that is proceeding.
   */
  progressToken: ProgressToken;
  /**
   * The progress thus far. This should increase every time progress is made, even if the total is unknown.
   *
   * @TJS-type number
   */
  progress: number;
  /**
   * Total number of items to process (or total progress required), if known.
   *
   * @TJS-type number
   */
  total?: number;
  /**
   * An optional message describing the current progress.
   */
  message?: string;
}

/**
 * An out-of-band notification used to inform the receiver of a progress update for a long-running request.
 *
 * @example Progress message
 * {@includeCode ./examples/ProgressNotification/progress-message.json}
 *
 * @category `notifications/progress`
 */
export interface ProgressNotification extends JSONRPCNotification {
  method: "notifications/progress";
  params: ProgressNotificationParams;
}

/* Pagination */
/**
 * Common params for paginated requests.
 *
 * @example List request with cursor
 * {@includeCode ./examples/PaginatedRequestParams/list-with-cursor.json}
 *
 * @category Common Types
 */
export interface PaginatedRequestParams extends RequestParams {
  /**
   * An opaque token representing the current pagination position.
   * If provided, the server should return results starting after this cursor.
   */
  cursor?: Cursor;
}

/** @internal */
export interface PaginatedRequest extends JSONRPCRequest {
  params: PaginatedRequestParams;
}

/** @internal */
export interface PaginatedResult extends Result {
  /**
   * An opaque token representing the pagination position after the last returned result.
   * If present, there may be more results available.
   */
  nextCursor?: Cursor;
}

/**
 * A result that supports a time-to-live (TTL) hint for client-side caching.
 *
 * @internal
 */
export interface CacheableResult extends Result {
  /**
   * A hint from the server indicating how long (in milliseconds) the
   * client MAY cache this response before re-fetching. Semantics are
   * analogous to HTTP Cache-Control max-age.
   *
   * - If 0, The response SHOULD be considered immediately stale,
   *   The client MAY re-fetch every time the result is needed.
   * - If positive, the client SHOULD consider the result fresh for this many
   *   milliseconds after receiving the response.
   *
   * @minimum 0
   */
  ttlMs: number;

  /**
   * Indicates the intended scope of the cached response, analogous to HTTP
   * `Cache-Control: public` vs `Cache-Control: private`.
   *
   * - `"public"`: The response does not contain user-specific data. Any
   *   client or intermediary (e.g., shared gateway, caching proxy) MAY cache
   *   the response and serve it across authorization contexts.
   * - `"private"`: The response MAY be cached and reused only within the
   *   same authorization context. Caches MUST NOT be shared across
   *   authorization contexts (e.g., a different access token requires a
   *   different cache).
   *
   */
  cacheScope: "public" | "private";
}

/* Resources */
/**
 * Sent from the client to request a list of resources the server has.
 *
 * @example List resources request
 * {@includeCode ./examples/ListResourcesRequest/list-resources-request.json}
 *
 * @category `resources/list`
 */
export interface ListResourcesRequest extends PaginatedRequest {
  method: "resources/list";
}

/**
 * The result returned by the server for a {@link ListResourcesRequest | resources/list} request.
 *
 * @example Resources list with cursor and TTL
 * {@includeCode ./examples/ListResourcesResult/resources-list-with-cursor-and-ttl.json}
 *
 * @category `resources/list`
 */
export interface ListResourcesResult extends PaginatedResult, CacheableResult {
  resources: Resource[];
}

/**
 * A successful response from the server for a {@link ListResourcesRequest | resources/list} request.
 *
 * @example List resources result response
 * {@includeCode ./examples/ListResourcesResultResponse/list-resources-result-response.json}
 *
 * @category `resources/list`
 */
export interface ListResourcesResultResponse extends JSONRPCResultResponse {
  result: ListResourcesResult;
}

/**
 * Sent from the client to request a list of resource templates the server has.
 *
 * @example List resource templates request
 * {@includeCode ./examples/ListResourceTemplatesRequest/list-resource-templates-request.json}
 *
 * @category `resources/templates/list`
 */
export interface ListResourceTemplatesRequest extends PaginatedRequest {
  method: "resources/templates/list";
}

/**
 * The result returned by the server for a {@link ListResourceTemplatesRequest | resources/templates/list} request.
 *
 * @example Resource templates list with cursor and TTL
 * {@includeCode ./examples/ListResourceTemplatesResult/resource-templates-list-with-cursor-and-ttl.json}
 *
 * @category `resources/templates/list`
 */
export interface ListResourceTemplatesResult
  extends PaginatedResult, CacheableResult {
  resourceTemplates: ResourceTemplate[];
}

/**
 * A successful response from the server for a {@link ListResourceTemplatesRequest | resources/templates/list} request.
 *
 * @example List resource templates result response
 * {@includeCode ./examples/ListResourceTemplatesResultResponse/list-resource-templates-result-response.json}
 *
 * @category `resources/templates/list`
 */
export interface ListResourceTemplatesResultResponse extends JSONRPCResultResponse {
  result: ListResourceTemplatesResult;
}

/**
 * Common params for resource-related requests.
 *
 * @internal
 */
export interface ResourceRequestParams extends RequestParams {
  /**
   * The URI of the resource. The URI can use any protocol; it is up to the server how to interpret it.
   *
   * @format uri
   */
  uri: string;
}

/**
 * Parameters for a `resources/read` request.
 *
 * @category `resources/read`
 */
export interface ReadResourceRequestParams
  extends ResourceRequestParams, InputResponseRequestParams {}

/**
 * Sent from the client to the server, to read a specific resource URI.
 *
 * @example Read resource request
 * {@includeCode ./examples/ReadResourceRequest/read-resource-request.json}
 *
 * @category `resources/read`
 */
export interface ReadResourceRequest extends JSONRPCRequest {
  method: "resources/read";
  params: ReadResourceRequestParams;
}

/**
 * The result returned by the server for a {@link ReadResourceRequest | resources/read} request.
 *
 * @example File resource contents
 * {@includeCode ./examples/ReadResourceResult/file-resource-contents.json}
 *
 * @category `resources/read`
 */
export interface ReadResourceResult extends CacheableResult {
  contents: (TextResourceContents | BlobResourceContents)[];
}

/**
 * A successful response from the server for a {@link ReadResourceRequest | resources/read} request.
 *
 * @example Read resource result response
 * {@includeCode ./examples/ReadResourceResultResponse/read-resource-result-response.json}
 *
 * @example Read resource result response with TTL
 * {@includeCode ./examples/ReadResourceResultResponse/read-resource-result-response-with-ttl.json}
 *
 * @category `resources/read`
 */
export interface ReadResourceResultResponse extends JSONRPCResultResponse {
  result: ReadResourceResult | InputRequiredResult;
}

/**
 * An optional notification from the server to the client, informing it that the list of resources it can read from has changed. This is only delivered on a {@link SubscriptionsListenRequest | subscriptions/listen} stream when the client requested it via the `resourcesListChanged` filter field.
 *
 * @example Resources list changed
 * {@includeCode ./examples/ResourceListChangedNotification/resources-list-changed.json}
 *
 * @category `notifications/resources/list_changed`
 */
export interface ResourceListChangedNotification extends JSONRPCNotification {
  method: "notifications/resources/list_changed";
  params?: NotificationParams;
}

/**
 * The set of notification types a client may opt in to on a
 * {@link SubscriptionsListenRequest | subscriptions/listen} request.
 *
 * Each notification type is **opt-in**; the server **MUST NOT** send
 * notification types the client has not explicitly requested here.
 *
 * @category `subscriptions/listen`
 */
export interface SubscriptionFilter {
  /**
   * If true, receive {@link ToolListChangedNotification | notifications/tools/list_changed}.
   */
  toolsListChanged?: boolean;
  /**
   * If true, receive {@link PromptListChangedNotification | notifications/prompts/list_changed}.
   */
  promptsListChanged?: boolean;
  /**
   * If true, receive {@link ResourceListChangedNotification | notifications/resources/list_changed}.
   */
  resourcesListChanged?: boolean;
  /**
   * Subscribe to {@link ResourceUpdatedNotification | notifications/resources/updated} for these resource URIs.
   * Replaces the former `resources/subscribe` RPC.
   */
  resourceSubscriptions?: string[];
}

/**
 * Parameters for a {@link SubscriptionsListenRequest | subscriptions/listen} request.
 *
 * @category `subscriptions/listen`
 */
export interface SubscriptionsListenRequestParams extends RequestParams {
  /**
   * The notifications the client opts in to on this stream. The server
   * **MUST NOT** send notification types the client has not explicitly
   * requested.
   */
  notifications: SubscriptionFilter;
}

/**
 * Sent from the client to open a long-lived channel for receiving notifications
 * outside the context of a specific request. Replaces the previous HTTP GET
 * endpoint and ensures consistent behavior between HTTP and STDIO.
 *
 * @example Listen for tools and resource list changes
 * {@includeCode ./examples/SubscriptionsListenRequest/listen-for-list-changes.json}
 *
 * @category `subscriptions/listen`
 */
export interface SubscriptionsListenRequest extends JSONRPCRequest {
  method: "subscriptions/listen";
  params: SubscriptionsListenRequestParams;
}

/**
 * Extends {@link ResultMetaObject} with the subscription-stream identifier carried by a
 * {@link SubscriptionsListenResult}. All key naming rules from `MetaObject` apply.
 *
 * @see {@link MetaObject} for key naming rules and reserved prefixes.
 * @category `subscriptions/listen`
 */
export interface SubscriptionsListenResultMetaObject extends ResultMetaObject {
  /**
   * Identifies the subscription stream this response closes, so the client can
   * correlate it with the originating subscription — mirroring the same key on
   * the stream's notifications. The value is the JSON-RPC ID of the
   * `subscriptions/listen` request that opened the stream (and equals this
   * response's `id`).
   */
  "io.modelcontextprotocol/subscriptionId": RequestId;
}

/**
 * The response to a {@link SubscriptionsListenRequest | subscriptions/listen}
 * request, signalling that the subscription has ended gracefully (for example,
 * during server shutdown). Because the listen stream is long-lived, this result
 * is sent only when the server tears the subscription down; an abrupt transport
 * close carries no response. The result body is otherwise empty.
 *
 * @example Subscription closed gracefully
 * {@includeCode ./examples/SubscriptionsListenResult/listen-closed.json}
 *
 * @category `subscriptions/listen`
 */
export interface SubscriptionsListenResult extends Result {
  _meta: SubscriptionsListenResultMetaObject;
}

/**
 * A successful response from the server for a {@link SubscriptionsListenRequest | subscriptions/listen}
 * request, sent when the server tears the subscription down gracefully.
 *
 * @example Subscription closed gracefully response
 * {@includeCode ./examples/SubscriptionsListenResultResponse/listen-closed-response.json}
 *
 * @category `subscriptions/listen`
 */
export interface SubscriptionsListenResultResponse extends JSONRPCResultResponse {
  result: SubscriptionsListenResult;
}

/**
 * Parameters for a {@link SubscriptionsAcknowledgedNotification | notifications/subscriptions/acknowledged} notification.
 *
 * @category `notifications/subscriptions/acknowledged`
 */
export interface SubscriptionsAcknowledgedNotificationParams extends NotificationParams {
  /**
   * The subset of requested notification types the server agreed to honor.
   * Only includes notification types the server actually supports; if the
   * client requested an unsupported type (e.g., `promptsListChanged` when
   * the server has no prompts), it is omitted from this set.
   */
  notifications: SubscriptionFilter;
}

/**
 * Sent by the server to acknowledge that a
 * {@link SubscriptionsListenRequest | subscriptions/listen} subscription has been
 * established and to report which notification types it agreed to honor.
 *
 * This notification MUST be the first message the server sends carrying the
 * subscription's ID in `io.modelcontextprotocol/subscriptionId`. The server MUST
 * NOT send any notification on the subscription before acknowledging it. On
 * stdio, where every subscription shares one channel, this ordering is defined
 * per subscription ID and not per channel: messages belonging to other
 * subscriptions MAY be interleaved before it.
 *
 * @example Listen acknowledged
 * {@includeCode ./examples/SubscriptionsAcknowledgedNotification/listen-acknowledged.json}
 *
 * @category `notifications/subscriptions/acknowledged`
 */
export interface SubscriptionsAcknowledgedNotification extends JSONRPCNotification {
  method: "notifications/subscriptions/acknowledged";
  params: SubscriptionsAcknowledgedNotificationParams;
}

/**
 * Parameters for a `notifications/resources/updated` notification.
 *
 * @example File resource updated
 * {@includeCode ./examples/ResourceUpdatedNotificationParams/file-resource-updated.json}
 *
 * @category `notifications/resources/updated`
 */
export interface ResourceUpdatedNotificationParams extends NotificationParams {
  /**
   * The URI of the resource that has been updated. This might be a sub-resource of the one that the client actually subscribed to.
   *
   * @format uri
   */
  uri: string;
}

/**
 * A notification from the server to the client, informing it that a resource has changed and may need to be read again. This is only sent for resources the client opted in to via the `resourceSubscriptions` field of a {@link SubscriptionsListenRequest | subscriptions/listen} request.
 *
 * @example File resource updated notification
 * {@includeCode ./examples/ResourceUpdatedNotification/file-resource-updated-notification.json}
 *
 * @category `notifications/resources/updated`
 */
export interface ResourceUpdatedNotification extends JSONRPCNotification {
  method: "notifications/resources/updated";
  params: ResourceUpdatedNotificationParams;
}

/**
 * A known resource that the server is capable of reading.
 *
 * @example File resource with annotations
 * {@includeCode ./examples/Resource/file-resource-with-annotations.json}
 *
 * @category `resources/list`
 */
export interface Resource extends BaseMetadata, Icons {
  /**
   * The URI of this resource.
   *
   * @format uri
   */
  uri: string;

  /**
   * A description of what this resource represents.
   *
   * This can be used by clients to improve the LLM's understanding of available resources. It can be thought of like a "hint" to the model.
   */
  description?: string;

  /**
   * The MIME type of this resource, if known.
   */
  mimeType?: string;

  /**
   * Optional annotations for the client.
   */
  annotations?: Annotations;

  /**
   * The size of the raw resource content, in bytes (i.e., before base64 encoding or any tokenization), if known.
   *
   * This can be used by Hosts to display file sizes and estimate context window usage.
   */
  size?: number;

  _meta?: MetaObject;
}

/**
 * A template description for resources available on the server.
 *
 * @category `resources/templates/list`
 */
export interface ResourceTemplate extends BaseMetadata, Icons {
  /**
   * A URI template (according to RFC 6570) that can be used to construct resource URIs.
   *
   * @format uri-template
   */
  uriTemplate: string;

  /**
   * A description of what this template is for.
   *
   * This can be used by clients to improve the LLM's understanding of available resources. It can be thought of like a "hint" to the model.
   */
  description?: string;

  /**
   * The MIME type for all resources that match this template. This should only be included if all resources matching this template have the same type.
   */
  mimeType?: string;

  /**
   * Optional annotations for the client.
   */
  annotations?: Annotations;

  _meta?: MetaObject;
}

/**
 * The contents of a specific resource or sub-resource.
 *
 * @internal
 */
export interface ResourceContents {
  /**
   * The URI of this resource.
   *
   * @format uri
   */
  uri: string;
  /**
   * The MIME type of this resource, if known.
   */
  mimeType?: string;

  _meta?: MetaObject;
}

/**
 * @example Text file contents
 * {@includeCode ./examples/TextResourceContents/text-file-contents.json}
 *
 * @category Content
 */
export interface TextResourceContents extends ResourceContents {
  /**
   * The text of the item. This must only be set if the item can actually be represented as text (not binary data).
   */
  text: string;
}

/**
 * @example Image file contents
 * {@includeCode ./examples/BlobResourceContents/image-file-contents.json}
 *
 * @category Content
 */
export interface BlobResourceContents extends ResourceContents {
  /**
   * A base64-encoded string representing the binary data of the item.
   *
   * @format byte
   */
  blob: string;
}

/* Prompts */
/**
 * Sent from the client to request a list of prompts and prompt templates the server has.
 *
 * @example List prompts request
 * {@includeCode ./examples/ListPromptsRequest/list-prompts-request.json}
 *
 * @category `prompts/list`
 */
export interface ListPromptsRequest extends PaginatedRequest {
  method: "prompts/list";
}

/**
 * The result returned by the server for a {@link ListPromptsRequest | prompts/list} request.
 *
 * @example Prompts list with cursor and TTL
 * {@includeCode ./examples/ListPromptsResult/prompts-list-with-cursor-and-ttl.json}
 *
 * @category `prompts/list`
 */
export interface ListPromptsResult extends PaginatedResult, CacheableResult {
  prompts: Prompt[];
}

/**
 * A successful response from the server for a {@link ListPromptsRequest | prompts/list} request.
 *
 * @example List prompts result response
 * {@includeCode ./examples/ListPromptsResultResponse/list-prompts-result-response.json}
 *
 * @category `prompts/list`
 */
export interface ListPromptsResultResponse extends JSONRPCResultResponse {
  result: ListPromptsResult;
}

/**
 * Parameters for a `prompts/get` request.
 *
 * @example Get code review prompt
 * {@includeCode ./examples/GetPromptRequestParams/get-code-review-prompt.json}
 *
 * @category `prompts/get`
 */
export interface GetPromptRequestParams extends InputResponseRequestParams {
  /**
   * The name of the prompt or prompt template.
   */
  name: string;
  /**
   * Arguments to use for templating the prompt.
   */
  arguments?: { [key: string]: string };
}

/**
 * Used by the client to get a prompt provided by the server.
 *
 * @example Get prompt request
 * {@includeCode ./examples/GetPromptRequest/get-prompt-request.json}
 *
 * @category `prompts/get`
 */
export interface GetPromptRequest extends JSONRPCRequest {
  method: "prompts/get";
  params: GetPromptRequestParams;
}

/**
 * The result returned by the server for a {@link GetPromptRequest | prompts/get} request.
 *
 * @example Code review prompt
 * {@includeCode ./examples/GetPromptResult/code-review-prompt.json}
 *
 * @category `prompts/get`
 */
export interface GetPromptResult extends Result {
  /**
   * An optional description for the prompt.
   */
  description?: string;
  messages: PromptMessage[];
}

/**
 * A successful response from the server for a {@link GetPromptRequest | prompts/get} request.
 *
 * @example Get prompt result response
 * {@includeCode ./examples/GetPromptResultResponse/get-prompt-result-response.json}
 *
 * @category `prompts/get`
 */
export interface GetPromptResultResponse extends JSONRPCResultResponse {
  result: GetPromptResult | InputRequiredResult;
}

/**
 * A prompt or prompt template that the server offers.
 *
 * @category `prompts/list`
 */
export interface Prompt extends BaseMetadata, Icons {
  /**
   * An optional description of what this prompt provides
   */
  description?: string;

  /**
   * A list of arguments to use for templating the prompt.
   */
  arguments?: PromptArgument[];

  _meta?: MetaObject;
}

/**
 * Describes an argument that a prompt can accept.
 *
 * @category `prompts/list`
 */
export interface PromptArgument extends BaseMetadata {
  /**
   * A human-readable description of the argument.
   */
  description?: string;
  /**
   * Whether this argument must be provided.
   */
  required?: boolean;
}

/**
 * The sender or recipient of messages and data in a conversation.
 *
 * @category Common Types
 */
export type Role = "user" | "assistant";

/**
 * Describes a message returned as part of a prompt.
 *
 * This is similar to {@link SamplingMessage}, but also supports the embedding of
 * resources from the MCP server.
 *
 * @category `prompts/get`
 */
export interface PromptMessage {
  role: Role;
  content: ContentBlock;
}

/**
 * A resource that the server is capable of reading, included in a prompt or tool call result.
 *
 * Note: resource links returned by tools are not guaranteed to appear in the results of {@link ListResourcesRequest | resources/list} requests.
 *
 * @example File resource link
 * {@includeCode ./examples/ResourceLink/file-resource-link.json}
 *
 * @category Content
 */
export interface ResourceLink extends Resource {
  type: "resource_link";
}

/**
 * The contents of a resource, embedded into a prompt or tool call result.
 *
 * It is up to the client how best to render embedded resources for the benefit
 * of the LLM and/or the user.
 *
 * @example Embedded file resource with annotations
 * {@includeCode ./examples/EmbeddedResource/embedded-file-resource-with-annotations.json}
 *
 * @category Content
 */
export interface EmbeddedResource {
  type: "resource";
  resource: TextResourceContents | BlobResourceContents;

  /**
   * Optional annotations for the client.
   */
  annotations?: Annotations;

  _meta?: MetaObject;
}
/**
 * An optional notification from the server to the client, informing it that the list of prompts it offers has changed. This is only delivered on a {@link SubscriptionsListenRequest | subscriptions/listen} stream when the client requested it via the `promptsListChanged` filter field.
 *
 * @example Prompts list changed
 * {@includeCode ./examples/PromptListChangedNotification/prompts-list-changed.json}
 *
 * @category `notifications/prompts/list_changed`
 */
export interface PromptListChangedNotification extends JSONRPCNotification {
  method: "notifications/prompts/list_changed";
  params?: NotificationParams;
}

/* Tools */
/**
 * Sent from the client to request a list of tools the server has.
 *
 * @example List tools request
 * {@includeCode ./examples/ListToolsRequest/list-tools-request.json}
 *
 * @category `tools/list`
 */
export interface ListToolsRequest extends PaginatedRequest {
  method: "tools/list";
}

/**
 * The result returned by the server for a {@link ListToolsRequest | tools/list} request.
 *
 * @example Tools list with cursor and TTL
 * {@includeCode ./examples/ListToolsResult/tools-list-with-cursor-and-ttl.json}
 *
 * @category `tools/list`
 */
export interface ListToolsResult extends PaginatedResult, CacheableResult {
  tools: Tool[];
}

/**
 * A successful response from the server for a {@link ListToolsRequest | tools/list} request.
 *
 * @example List tools result response
 * {@includeCode ./examples/ListToolsResultResponse/list-tools-result-response.json}
 *
 * @category `tools/list`
 */
export interface ListToolsResultResponse extends JSONRPCResultResponse {
  result: ListToolsResult;
}

/**
 * The result returned by the server for a {@link CallToolRequest | tools/call} request.
 *
 * @example Result with unstructured text
 * {@includeCode ./examples/CallToolResult/result-with-unstructured-text.json}
 *
 * @example Result with structured content
 * {@includeCode ./examples/CallToolResult/result-with-structured-content.json}
 *
 * @example Invalid tool input error
 * {@includeCode ./examples/CallToolResult/invalid-tool-input-error.json}
 *
 * @category `tools/call`
 */
export interface CallToolResult extends Result {
  /**
   * A list of content objects that represent the unstructured result of the tool call.
   */
  content: ContentBlock[];

  /**
   * An optional JSON value that represents the structured result of the tool call.
   *
   * This can be any JSON value (object, array, string, number, boolean, or null)
   * that conforms to the tool's outputSchema if one is defined.
   */
  structuredContent?: unknown;

  /**
   * Whether the tool call ended in an error.
   *
   * If not set, this is assumed to be false (the call was successful).
   *
   * Any errors that originate from the tool SHOULD be reported inside the result
   * object, with `isError` set to true, _not_ as an MCP protocol-level error
   * response. Otherwise, the LLM would not be able to see that an error occurred
   * and self-correct.
   *
   * However, any errors in _finding_ the tool, an error indicating that the
   * server does not support tool calls, or any other exceptional conditions,
   * should be reported as an MCP error response.
   */
  isError?: boolean;
}

/**
 * A successful response from the server for a {@link CallToolRequest | tools/call} request.
 *
 * @example Call tool result response
 * {@includeCode ./examples/CallToolResultResponse/call-tool-result-response.json}
 *
 * @category `tools/call`
 */
export interface CallToolResultResponse extends JSONRPCResultResponse {
  result: CallToolResult | InputRequiredResult;
}

/**
 * Parameters for a `tools/call` request.
 *
 * @example `get_weather` tool call params
 * {@includeCode ./examples/CallToolRequestParams/get-weather-tool-call-params.json}
 *
 * @example Tool call params with progress token
 * {@includeCode ./examples/CallToolRequestParams/tool-call-params-with-progress-token.json}
 *
 * @category `tools/call`
 */
export interface CallToolRequestParams extends InputResponseRequestParams {
  /**
   * The name of the tool.
   */
  name: string;
  /**
   * Arguments to use for the tool call.
   */
  arguments?: { [key: string]: unknown };
}

/**
 * Used by the client to invoke a tool provided by the server.
 *
 * @example Call tool request
 * {@includeCode ./examples/CallToolRequest/call-tool-request.json}
 *
 * @category `tools/call`
 */
export interface CallToolRequest extends JSONRPCRequest {
  method: "tools/call";
  params: CallToolRequestParams;
}

/**
 * An optional notification from the server to the client, informing it that the list of tools it offers has changed. This is only delivered on a {@link SubscriptionsListenRequest | subscriptions/listen} stream when the client requested it via the `toolsListChanged` filter field.
 *
 * @example Tools list changed
 * {@includeCode ./examples/ToolListChangedNotification/tools-list-changed.json}
 *
 * @category `notifications/tools/list_changed`
 */
export interface ToolListChangedNotification extends JSONRPCNotification {
  method: "notifications/tools/list_changed";
  params?: NotificationParams;
}

/**
 * Additional properties describing a {@link Tool} to clients.
 *
 * NOTE: all properties in `ToolAnnotations` are **hints**.
 * They are not guaranteed to provide a faithful description of
 * tool behavior (including descriptive properties like `title`).
 *
 * Clients should never make tool use decisions based on `ToolAnnotations`
 * received from untrusted servers.
 *
 * @category `tools/list`
 */
export interface ToolAnnotations {
  /**
   * A human-readable title for the tool.
   */
  title?: string;

  /**
   * If true, the tool does not modify its environment.
   *
   * Default: false
   */
  readOnlyHint?: boolean;

  /**
   * If true, the tool may perform destructive updates to its environment.
   * If false, the tool performs only additive updates.
   *
   * (This property is meaningful only when `readOnlyHint == false`)
   *
   * Default: true
   */
  destructiveHint?: boolean;

  /**
   * If true, calling the tool repeatedly with the same arguments
   * will have no additional effect on its environment.
   *
   * (This property is meaningful only when `readOnlyHint == false`)
   *
   * Default: false
   */
  idempotentHint?: boolean;

  /**
   * If true, this tool may interact with an "open world" of external
   * entities. If false, the tool's domain of interaction is closed.
   * For example, the world of a web search tool is open, whereas that
   * of a memory tool is not.
   *
   * Default: true
   */
  openWorldHint?: boolean;
}

/**
 * Definition for a tool the client can call.
 *
 * @example With default 2020-12 input schema
 * {@includeCode ./examples/Tool/with-default-2020-12-input-schema.json}
 *
 * @example With explicit draft-07 input schema
 * {@includeCode ./examples/Tool/with-explicit-draft-07-input-schema.json}
 *
 * @example With no parameters
 * {@includeCode ./examples/Tool/with-no-parameters.json}
 *
 * @example With output schema for structured content
 * {@includeCode ./examples/Tool/with-output-schema-for-structured-content.json}
 *
 * @category `tools/list`
 */
export interface Tool extends BaseMetadata, Icons {
  /**
   * A human-readable description of the tool.
   *
   * This can be used by clients to improve the LLM's understanding of available tools. It can be thought of like a "hint" to the model.
   */
  description?: string;

  /**
   * A JSON Schema object defining the expected parameters for the tool.
   *
   * Tool arguments are always JSON objects, so `type: "object"` is required at the root.
   * Beyond that, any JSON Schema 2020-12 keyword may appear alongside `type` — including
   * composition keywords (`oneOf`, `anyOf`, `allOf`, `not`), conditional keywords
   * (`if`/`then`/`else`), reference keywords (`$ref`, `$defs`, `$anchor`), and any other
   * standard validation or annotation keywords.
   *
   * Property schemas may carry an `x-mcp-header` annotation to mirror the
   * argument value into an HTTP header on the Streamable HTTP transport. See
   * the Streamable HTTP transport specification for the validity and
   * extraction rules.
   *
   * Defaults to JSON Schema 2020-12 when no explicit `$schema` is provided.
   */
  inputSchema: { $schema?: string; type: "object"; [key: string]: unknown };

  /**
   * An optional JSON Schema object defining the structure of the tool's output returned in
   * the structuredContent field of a {@link CallToolResult}. This can be any valid JSON Schema 2020-12.
   *
   * Defaults to JSON Schema 2020-12 when no explicit `$schema` is provided.
   */
  outputSchema?: { $schema?: string; [key: string]: unknown };

  /**
   * Optional additional tool information.
   *
   * Display name precedence order is: `title`, `annotations.title`, then `name`.
   */
  annotations?: ToolAnnotations;

  _meta?: MetaObject;
}

/* Logging */

/**
 * Parameters for a `notifications/message` notification.
 *
 * @deprecated Deprecated as of protocol version 2026-07-28 (SEP-2577).
 * Remains in the specification for at least twelve months; see the
 * deprecated features registry.
 *
 * @example Log database connection failed
 * {@includeCode ./examples/LoggingMessageNotificationParams/log-database-connection-failed.json}
 *
 * @category `notifications/message`
 */
export interface LoggingMessageNotificationParams extends NotificationParams {
  /**
   * The severity of this log message.
   */
  level: LoggingLevel;
  /**
   * An optional name of the logger issuing this message.
   */
  logger?: string;
  /**
   * The data to be logged, such as a string message or an object. Any JSON serializable type is allowed here.
   */
  data: unknown;
}

/**
 * JSONRPCNotification of a log message passed from server to client. The client opts in by setting `"io.modelcontextprotocol/logLevel"` in a request's `_meta`.
 *
 * @deprecated Deprecated as of protocol version 2026-07-28 (SEP-2577).
 * Remains in the specification for at least twelve months; see the
 * deprecated features registry.
 *
 * @example Log database connection failed
 * {@includeCode ./examples/LoggingMessageNotification/log-database-connection-failed.json}
 *
 * @category `notifications/message`
 */
export interface LoggingMessageNotification extends JSONRPCNotification {
  method: "notifications/message";
  params: LoggingMessageNotificationParams;
}

/**
 * The severity of a log message.
 *
 * These map to syslog message severities, as specified in RFC-5424:
 * https://datatracker.ietf.org/doc/html/rfc5424#section-6.2.1
 *
 * @deprecated Deprecated as of protocol version 2026-07-28 (SEP-2577).
 * Remains in the specification for at least twelve months; see the
 * deprecated features registry.
 *
 * @category Common Types
 */
export type LoggingLevel =
  | "debug"
  | "info"
  | "notice"
  | "warning"
  | "error"
  | "critical"
  | "alert"
  | "emergency";

/* Sampling */
/**
 * Parameters for a `sampling/createMessage` request.
 *
 * @example Basic request
 * {@includeCode ./examples/CreateMessageRequestParams/basic-request.json}
 *
 * @example Request with tools
 * {@includeCode ./examples/CreateMessageRequestParams/request-with-tools.json}
 *
 * @example Follow-up request with tool results
 * {@includeCode ./examples/CreateMessageRequestParams/follow-up-with-tool-results.json}
 *
 * @deprecated Deprecated as of protocol version 2026-07-28 (SEP-2577).
 * Remains in the specification for at least twelve months; see the
 * deprecated features registry.
 *
 * @category `sampling/createMessage`
 */
export interface CreateMessageRequestParams {
  messages: SamplingMessage[];
  /**
   * The server's preferences for which model to select. The client MAY ignore these preferences.
   */
  modelPreferences?: ModelPreferences;
  /**
   * An optional system prompt the server wants to use for sampling. The client MAY modify or omit this prompt.
   */
  systemPrompt?: string;
  /**
   * A request to include context from one or more MCP servers (including the caller), to be attached to the prompt.
   * The client MAY ignore this request.
   *
   * Default is `"none"`. The values `"thisServer"` and `"allServers"` are deprecated (SEP-2596): servers SHOULD
   * omit this field or use `"none"`, and SHOULD only use the deprecated values if the client declares
   * {@link ClientCapabilities.sampling.context}.
   *
   * @deprecated The `"thisServer"` and `"allServers"` values are deprecated as of protocol version 2025-11-25
   * (SEP-2596) and will be removed no later than the Sampling feature itself (SEP-2577). Omit this field or use `"none"`.
   */
  includeContext?: "none" | "thisServer" | "allServers";
  /**
   * @TJS-type number
   */
  temperature?: number;
  /**
   * The requested maximum number of tokens to sample (to prevent runaway completions).
   *
   * The client MAY choose to sample fewer tokens than the requested maximum.
   */
  maxTokens: number;
  stopSequences?: string[];
  /**
   * Optional metadata to pass through to the LLM provider. The format of this metadata is provider-specific.
   */
  metadata?: JSONObject;
  /**
   * Tools that the model may use during generation.
   * The client MUST return an error if this field is provided but {@link ClientCapabilities.sampling.tools} is not declared.
   */
  tools?: Tool[];
  /**
   * Controls how the model uses tools.
   * The client MUST return an error if this field is provided but {@link ClientCapabilities.sampling.tools} is not declared.
   * Default is `{ mode: "auto" }`.
   */
  toolChoice?: ToolChoice;
}

/**
 * Controls tool selection behavior for sampling requests.
 *
 * @deprecated Deprecated as of protocol version 2026-07-28 (SEP-2577).
 * Remains in the specification for at least twelve months; see the
 * deprecated features registry.
 *
 * @category `sampling/createMessage`
 */
export interface ToolChoice {
  /**
   * Controls the tool use ability of the model:
   * - `"auto"`: Model decides whether to use tools (default)
   * - `"required"`: Model MUST use at least one tool before completing
   * - `"none"`: Model MUST NOT use any tools
   */
  mode?: "auto" | "required" | "none";
}

/**
 * A request from the server to sample an LLM via the client. The client has full discretion over which model to select. The client should also inform the user before beginning sampling, to allow them to inspect the request (human in the loop) and decide whether to approve it.
 *
 * @example Sampling request
 * {@includeCode ./examples/CreateMessageRequest/sampling-request.json}
 *
 * @deprecated Deprecated as of protocol version 2026-07-28 (SEP-2577).
 * Remains in the specification for at least twelve months; see the
 * deprecated features registry.
 *
 * @category `sampling/createMessage`
 */
export interface CreateMessageRequest {
  method: "sampling/createMessage";
  params: CreateMessageRequestParams;
}

/**
 * The result returned by the client for a {@link CreateMessageRequest | sampling/createMessage} request.
 * The client should inform the user before returning the sampled message, to allow them
 * to inspect the response (human in the loop) and decide whether to allow the server to see it.
 *
 * @example Text response
 * {@includeCode ./examples/CreateMessageResult/text-response.json}
 *
 * @example Tool use response
 * {@includeCode ./examples/CreateMessageResult/tool-use-response.json}
 *
 * @example Final response after tool use
 * {@includeCode ./examples/CreateMessageResult/final-response.json}
 *
 * @deprecated Deprecated as of protocol version 2026-07-28 (SEP-2577).
 * Remains in the specification for at least twelve months; see the
 * deprecated features registry.
 *
 * @category `sampling/createMessage`
 */
export interface CreateMessageResult extends SamplingMessage {
  /**
   * The name of the model that generated the message.
   */
  model: string;

  /**
   * The reason why sampling stopped, if known.
   *
   * Standard values:
   * - `"endTurn"`: Natural end of the assistant's turn
   * - `"stopSequence"`: A stop sequence was encountered
   * - `"maxTokens"`: Maximum token limit was reached
   * - `"toolUse"`: The model wants to use one or more tools
   *
   * This field is an open string to allow for provider-specific stop reasons.
   */
  stopReason?: "endTurn" | "stopSequence" | "maxTokens" | "toolUse" | string;
}

/**
 * Describes a message issued to or received from an LLM API.
 *
 * @example Single content block
 * {@includeCode ./examples/SamplingMessage/single-content-block.json}
 *
 * @example Multiple content blocks
 * {@includeCode ./examples/SamplingMessage/multiple-content-blocks.json}
 *
 * @deprecated Deprecated as of protocol version 2026-07-28 (SEP-2577).
 * Remains in the specification for at least twelve months; see the
 * deprecated features registry.
 *
 * @category `sampling/createMessage`
 */
export interface SamplingMessage {
  role: Role;
  content: SamplingMessageContentBlock | SamplingMessageContentBlock[];
  _meta?: MetaObject;
}

/**
 * @deprecated Deprecated as of protocol version 2026-07-28 (SEP-2577).
 * Remains in the specification for at least twelve months; see the
 * deprecated features registry.
 *
 * @category `sampling/createMessage`
 */
export type SamplingMessageContentBlock =
  | TextContent
  | ImageContent
  | AudioContent
  | ToolUseContent
  | ToolResultContent;

/**
 * Optional annotations for the client. The client can use annotations to inform how objects are used or displayed
 *
 * @category Common Types
 */
export interface Annotations {
  /**
   * Describes who the intended audience of this object or data is.
   *
   * It can include multiple entries to indicate content useful for multiple audiences (e.g., `["user", "assistant"]`).
   */
  audience?: Role[];

  /**
   * Describes how important this data is for operating the server.
   *
   * A value of 1 means "most important," and indicates that the data is
   * effectively required, while 0 means "least important," and indicates that
   * the data is entirely optional.
   *
   * @TJS-type number
   * @minimum 0
   * @maximum 1
   */
  priority?: number;

  /**
   * The moment the resource was last modified, as an ISO 8601 formatted string.
   *
   * Should be an ISO 8601 formatted string (e.g., "2025-01-12T15:00:58Z").
   *
   * Examples: last activity timestamp in an open file, timestamp when the resource
   * was attached, etc.
   */
  lastModified?: string;
}

/**
 * @category Content
 */
export type ContentBlock =
  TextContent | ImageContent | AudioContent | ResourceLink | EmbeddedResource;

/**
 * Text provided to or from an LLM.
 *
 * @example Text content
 * {@includeCode ./examples/TextContent/text-content.json}
 *
 * @category Content
 */
export interface TextContent {
  type: "text";

  /**
   * The text content of the message.
   */
  text: string;

  /**
   * Optional annotations for the client.
   */
  annotations?: Annotations;

  _meta?: MetaObject;
}

/**
 * An image provided to or from an LLM.
 *
 * @example `image/png` content with annotations
 * {@includeCode ./examples/ImageContent/image-png-content-with-annotations.json}
 *
 * @category Content
 */
export interface ImageContent {
  type: "image";

  /**
   * The base64-encoded image data.
   *
   * @format byte
   */
  data: string;

  /**
   * The MIME type of the image. Different providers may support different image types.
   */
  mimeType: string;

  /**
   * Optional annotations for the client.
   */
  annotations?: Annotations;

  _meta?: MetaObject;
}

/**
 * Audio provided to or from an LLM.
 *
 * @example `audio/wav` content
 * {@includeCode ./examples/AudioContent/audio-wav-content.json}
 *
 * @category Content
 */
export interface AudioContent {
  type: "audio";

  /**
   * The base64-encoded audio data.
   *
   * @format byte
   */
  data: string;

  /**
   * The MIME type of the audio. Different providers may support different audio types.
   */
  mimeType: string;

  /**
   * Optional annotations for the client.
   */
  annotations?: Annotations;

  _meta?: MetaObject;
}

/**
 * A request from the assistant to call a tool.
 *
 * @example `get_weather` tool use
 * {@includeCode ./examples/ToolUseContent/get-weather-tool-use.json}
 *
 * @deprecated Deprecated as of protocol version 2026-07-28 (SEP-2577).
 * Remains in the specification for at least twelve months; see the
 * deprecated features registry.
 *
 * @category `sampling/createMessage`
 */
export interface ToolUseContent {
  type: "tool_use";

  /**
   * A unique identifier for this tool use.
   *
   * This ID is used to match tool results to their corresponding tool uses.
   */
  id: string;

  /**
   * The name of the tool to call.
   */
  name: string;

  /**
   * The arguments to pass to the tool, conforming to the tool's input schema.
   */
  input: { [key: string]: unknown };

  /**
   * Optional metadata about the tool use. Clients SHOULD preserve this field when
   * including tool uses in subsequent sampling requests to enable caching optimizations.
   */
  _meta?: MetaObject;
}

/**
 * The result of a tool use, provided by the user back to the assistant.
 *
 * @example `get_weather` tool result
 * {@includeCode ./examples/ToolResultContent/get-weather-tool-result.json}
 *
 * @deprecated Deprecated as of protocol version 2026-07-28 (SEP-2577).
 * Remains in the specification for at least twelve months; see the
 * deprecated features registry.
 *
 * @category `sampling/createMessage`
 */
export interface ToolResultContent {
  type: "tool_result";

  /**
   * The ID of the tool use this result corresponds to.
   *
   * This MUST match the ID from a previous {@link ToolUseContent}.
   */
  toolUseId: string;

  /**
   * The unstructured result content of the tool use.
   *
   * This has the same format as {@link CallToolResult.content} and can include text, images,
   * audio, resource links, and embedded resources.
   */
  content: ContentBlock[];

  /**
   * An optional structured result value.
   *
   * This can be any JSON value (object, array, string, number, boolean, or null).
   * If the tool defined an {@link Tool.outputSchema}, this SHOULD conform to that schema.
   */
  structuredContent?: unknown;

  /**
   * Whether the tool use resulted in an error.
   *
   * If true, the content typically describes the error that occurred.
   * Default: false
   */
  isError?: boolean;

  /**
   * Optional metadata about the tool result. Clients SHOULD preserve this field when
   * including tool results in subsequent sampling requests to enable caching optimizations.
   */
  _meta?: MetaObject;
}

/**
 * The server's preferences for model selection, requested of the client during sampling.
 *
 * Because LLMs can vary along multiple dimensions, choosing the "best" model is
 * rarely straightforward.  Different models excel in different areas—some are
 * faster but less capable, others are more capable but more expensive, and so
 * on. This interface allows servers to express their priorities across multiple
 * dimensions to help clients make an appropriate selection for their use case.
 *
 * These preferences are always advisory. The client MAY ignore them. It is also
 * up to the client to decide how to interpret these preferences and how to
 * balance them against other considerations.
 *
 * @example With hints and priorities
 * {@includeCode ./examples/ModelPreferences/with-hints-and-priorities.json}
 *
 * @deprecated Deprecated as of protocol version 2026-07-28 (SEP-2577).
 * Remains in the specification for at least twelve months; see the
 * deprecated features registry.
 *
 * @category `sampling/createMessage`
 */
export interface ModelPreferences {
  /**
   * Optional hints to use for model selection.
   *
   * If multiple hints are specified, the client MUST evaluate them in order
   * (such that the first match is taken).
   *
   * The client SHOULD prioritize these hints over the numeric priorities, but
   * MAY still use the priorities to select from ambiguous matches.
   */
  hints?: ModelHint[];

  /**
   * How much to prioritize cost when selecting a model. A value of 0 means cost
   * is not important, while a value of 1 means cost is the most important
   * factor.
   *
   * @TJS-type number
   * @minimum 0
   * @maximum 1
   */
  costPriority?: number;

  /**
   * How much to prioritize sampling speed (latency) when selecting a model. A
   * value of 0 means speed is not important, while a value of 1 means speed is
   * the most important factor.
   *
   * @TJS-type number
   * @minimum 0
   * @maximum 1
   */
  speedPriority?: number;

  /**
   * How much to prioritize intelligence and capabilities when selecting a
   * model. A value of 0 means intelligence is not important, while a value of 1
   * means intelligence is the most important factor.
   *
   * @TJS-type number
   * @minimum 0
   * @maximum 1
   */
  intelligencePriority?: number;
}

/**
 * Hints to use for model selection.
 *
 * Keys not declared here are currently left unspecified by the spec and are up
 * to the client to interpret.
 *
 * @deprecated Deprecated as of protocol version 2026-07-28 (SEP-2577).
 * Remains in the specification for at least twelve months; see the
 * deprecated features registry.
 *
 * @category `sampling/createMessage`
 */
export interface ModelHint {
  /**
   * A hint for a model name.
   *
   * The client SHOULD treat this as a substring of a model name; for example:
   *  - `claude-3-5-sonnet` should match `claude-3-5-sonnet-20241022`
   *  - `sonnet` should match `claude-3-5-sonnet-20241022`, `claude-3-sonnet-20240229`, etc.
   *  - `claude` should match any Claude model
   *
   * The client MAY also map the string to a different provider's model name or a different model family, as long as it fills a similar niche; for example:
   *  - `gemini-1.5-flash` could match `claude-3-haiku-20240307`
   */
  name?: string;
}

/* Autocomplete */
/**
 * Parameters for a `completion/complete` request.
 *
 * @category `completion/complete`
 *
 * @example Prompt argument completion
 * {@includeCode ./examples/CompleteRequestParams/prompt-argument-completion.json}
 *
 * @example Prompt argument completion with context
 * {@includeCode ./examples/CompleteRequestParams/prompt-argument-completion-with-context.json}
 */
export interface CompleteRequestParams extends RequestParams {
  ref: PromptReference | ResourceTemplateReference;
  /**
   * The argument's information
   */
  argument: {
    /**
     * The name of the argument
     */
    name: string;
    /**
     * The value of the argument to use for completion matching.
     */
    value: string;
  };

  /**
   * Additional, optional context for completions
   */
  context?: {
    /**
     * Previously-resolved variables in a URI template or prompt.
     */
    arguments?: { [key: string]: string };
  };
}

/**
 * A request from the client to the server, to ask for completion options.
 *
 * @example Completion request
 * {@includeCode ./examples/CompleteRequest/completion-request.json}
 *
 * @category `completion/complete`
 */
export interface CompleteRequest extends JSONRPCRequest {
  method: "completion/complete";
  params: CompleteRequestParams;
}

/**
 * The result returned by the server for a {@link CompleteRequest | completion/complete} request.
 *
 * @category `completion/complete`
 *
 * @example Single completion value
 * {@includeCode ./examples/CompleteResult/single-completion-value.json}
 *
 * @example Multiple completion values with more available
 * {@includeCode ./examples/CompleteResult/multiple-completion-values-with-more-available.json}
 */
export interface CompleteResult extends Result {
  completion: {
    /**
     * An array of completion values. Must not exceed 100 items.
     *
     * @maxItems 100
     */
    values: string[];
    /**
     * The total number of completion options available. This can exceed the number of values actually sent in the response.
     */
    total?: number;
    /**
     * Indicates whether there are additional completion options beyond those provided in the current response, even if the exact total is unknown.
     */
    hasMore?: boolean;
  };
}

/**
 * A successful response from the server for a {@link CompleteRequest | completion/complete} request.
 *
 * @example Completion result response
 * {@includeCode ./examples/CompleteResultResponse/completion-result-response.json}
 *
 * @category `completion/complete`
 */
export interface CompleteResultResponse extends JSONRPCResultResponse {
  result: CompleteResult;
}

/**
 * A reference to a resource or resource template definition.
 *
 * @category `completion/complete`
 */
export interface ResourceTemplateReference {
  type: "ref/resource";
  /**
   * The URI or URI template of the resource.
   *
   * @format uri-template
   */
  uri: string;
}

/**
 * Identifies a prompt.
 *
 * @category `completion/complete`
 */
export interface PromptReference extends BaseMetadata {
  type: "ref/prompt";
}

/* Roots */
/**
 * Sent from the server to request a list of root URIs from the client. Roots allow
 * servers to ask for specific directories or files to operate on. A common example
 * for roots is providing a set of repositories or directories a server should operate
 * on.
 *
 * This request is typically used when the server needs to understand the file system
 * structure or access specific locations that the client has permission to read from.
 *
 * @example List roots request
 * {@includeCode ./examples/ListRootsRequest/list-roots-request.json}
 *
 * @deprecated Deprecated as of protocol version 2026-07-28 (SEP-2577).
 * Remains in the specification for at least twelve months; see the
 * deprecated features registry.
 *
 * @category `roots/list`
 */
export interface ListRootsRequest {
  method: "roots/list";
  params?: {
    _meta?: MetaObject;
  };
}

/**
 * The result returned by the client for a {@link ListRootsRequest | roots/list} request.
 * This result contains an array of {@link Root} objects, each representing a root directory
 * or file that the server can operate on.
 *
 * @example Single root directory
 * {@includeCode ./examples/ListRootsResult/single-root-directory.json}
 *
 * @example Multiple root directories
 * {@includeCode ./examples/ListRootsResult/multiple-root-directories.json}
 *
 * @deprecated Deprecated as of protocol version 2026-07-28 (SEP-2577).
 * Remains in the specification for at least twelve months; see the
 * deprecated features registry.
 *
 * @category `roots/list`
 */
export interface ListRootsResult {
  roots: Root[];
}

/**
 * Represents a root directory or file that the server can operate on.
 *
 * @example Project directory root
 * {@includeCode ./examples/Root/project-directory.json}
 *
 * @deprecated Deprecated as of protocol version 2026-07-28 (SEP-2577).
 * Remains in the specification for at least twelve months; see the
 * deprecated features registry.
 *
 * @category `roots/list`
 */
export interface Root {
  /**
   * The URI identifying the root. This *must* start with `file://` for now.
   * This restriction may be relaxed in future versions of the protocol to allow
   * other URI schemes.
   *
   * @format uri
   */
  uri: string;
  /**
   * An optional name for the root. This can be used to provide a human-readable
   * identifier for the root, which may be useful for display purposes or for
   * referencing the root in other parts of the application.
   */
  name?: string;

  _meta?: MetaObject;
}

/**
 * The parameters for a request to elicit non-sensitive information from the user via a form in the client.
 *
 * @example Elicit single field
 * {@includeCode ./examples/ElicitRequestFormParams/elicit-single-field.json}
 *
 * @example Elicit multiple fields
 * {@includeCode ./examples/ElicitRequestFormParams/elicit-multiple-fields.json}
 *
 * @category `elicitation/create`
 */
export interface ElicitRequestFormParams {
  /**
   * The elicitation mode.
   */
  mode?: "form";

  /**
   * The message to present to the user describing what information is being requested.
   */
  message: string;

  /**
   * A restricted subset of JSON Schema.
   * Only top-level properties are allowed, without nesting.
   */
  requestedSchema: {
    $schema?: string;
    type: "object";
    properties: {
      [key: string]: PrimitiveSchemaDefinition;
    };
    required?: string[];
  };
}

/**
 * The parameters for a request to elicit information from the user via a URL in the client.
 *
 * @example Elicit sensitive data
 * {@includeCode ./examples/ElicitRequestURLParams/elicit-sensitive-data.json}
 *
 * @category `elicitation/create`
 */
export interface ElicitRequestURLParams {
  /**
   * The elicitation mode.
   */
  mode: "url";

  /**
   * The message to present to the user explaining why the interaction is needed.
   */
  message: string;

  /**
   * The URL that the user should navigate to.
   *
   * @format uri
   */
  url: string;
}

/**
 * The parameters for a request to elicit additional information from the user via the client.
 *
 * @category `elicitation/create`
 */
export type ElicitRequestParams =
  ElicitRequestFormParams | ElicitRequestURLParams;

/**
 * A request from the server to elicit additional information from the user via the client.
 *
 * @example Elicitation request
 * {@includeCode ./examples/ElicitRequest/elicitation-request.json}
 *
 * @category `elicitation/create`
 */
export interface ElicitRequest {
  method: "elicitation/create";
  params: ElicitRequestParams;
}

/**
 * Restricted schema definitions that only allow primitive types
 * without nested objects or arrays.
 *
 * @category `elicitation/create`
 */
export type PrimitiveSchemaDefinition =
  StringSchema | NumberSchema | BooleanSchema | EnumSchema;

/**
 * @example Email input schema
 * {@includeCode ./examples/StringSchema/email-input-schema.json}
 *
 * @category `elicitation/create`
 */
export interface StringSchema {
  type: "string";
  title?: string;
  description?: string;
  minLength?: number;
  maxLength?: number;
  format?: "email" | "uri" | "date" | "date-time";
  default?: string;
}

/**
 * @example Number input schema
 * {@includeCode ./examples/NumberSchema/number-input-schema.json}
 *
 * @category `elicitation/create`
 */
export interface NumberSchema {
  type: "number" | "integer";
  title?: string;
  description?: string;
  /**
   * @TJS-type number
   */
  minimum?: number;
  /**
   * @TJS-type number
   */
  maximum?: number;
  /**
   * @TJS-type number
   */
  default?: number;
}

/**
 * @example Boolean input schema
 * {@includeCode ./examples/BooleanSchema/boolean-input-schema.json}
 *
 * @category `elicitation/create`
 */
export interface BooleanSchema {
  type: "boolean";
  title?: string;
  description?: string;
  default?: boolean;
}

/**
 * Schema for single-selection enumeration without display titles for options.
 *
 * @example Color select schema
 * {@includeCode ./examples/UntitledSingleSelectEnumSchema/color-select-schema.json}
 *
 * @category `elicitation/create`
 */
export interface UntitledSingleSelectEnumSchema {
  type: "string";
  /**
   * Optional title for the enum field.
   */
  title?: string;
  /**
   * Optional description for the enum field.
   */
  description?: string;
  /**
   * Array of enum values to choose from.
   */
  enum: string[];
  /**
   * Optional default value.
   */
  default?: string;
}

/**
 * Schema for single-selection enumeration with display titles for each option.
 *
 * @example Titled color select schema
 * {@includeCode ./examples/TitledSingleSelectEnumSchema/titled-color-select-schema.json}
 *
 * @category `elicitation/create`
 */
export interface TitledSingleSelectEnumSchema {
  type: "string";
  /**
   * Optional title for the enum field.
   */
  title?: string;
  /**
   * Optional description for the enum field.
   */
  description?: string;
  /**
   * Array of enum options with values and display labels.
   */
  oneOf: Array<{
    /**
     * The enum value.
     */
    const: string;
    /**
     * Display label for this option.
     */
    title: string;
  }>;
  /**
   * Optional default value.
   */
  default?: string;
}

/**
 * @category `elicitation/create`
 */
// Combined single selection enumeration
export type SingleSelectEnumSchema =
  UntitledSingleSelectEnumSchema | TitledSingleSelectEnumSchema;

/**
 * Schema for multiple-selection enumeration without display titles for options.
 *
 * @example Color multi-select schema
 * {@includeCode ./examples/UntitledMultiSelectEnumSchema/color-multi-select-schema.json}
 *
 * @category `elicitation/create`
 */
export interface UntitledMultiSelectEnumSchema {
  type: "array";
  /**
   * Optional title for the enum field.
   */
  title?: string;
  /**
   * Optional description for the enum field.
   */
  description?: string;
  /**
   * Minimum number of items to select.
   */
  minItems?: number;
  /**
   * Maximum number of items to select.
   */
  maxItems?: number;
  /**
   * Schema for the array items.
   */
  items: {
    type: "string";
    /**
     * Array of enum values to choose from.
     */
    enum: string[];
  };
  /**
   * Optional default value.
   */
  default?: string[];
}

/**
 * Schema for multiple-selection enumeration with display titles for each option.
 *
 * @example Titled color multi-select schema
 * {@includeCode ./examples/TitledMultiSelectEnumSchema/titled-color-multi-select-schema.json}
 *
 * @category `elicitation/create`
 */
export interface TitledMultiSelectEnumSchema {
  type: "array";
  /**
   * Optional title for the enum field.
   */
  title?: string;
  /**
   * Optional description for the enum field.
   */
  description?: string;
  /**
   * Minimum number of items to select.
   */
  minItems?: number;
  /**
   * Maximum number of items to select.
   */
  maxItems?: number;
  /**
   * Schema for array items with enum options and display labels.
   */
  items: {
    /**
     * Array of enum options with values and display labels.
     */
    anyOf: Array<{
      /**
       * The constant enum value.
       */
      const: string;
      /**
       * Display title for this option.
       */
      title: string;
    }>;
  };
  /**
   * Optional default value.
   */
  default?: string[];
}

/**
 * @category `elicitation/create`
 */
// Combined multiple selection enumeration
export type MultiSelectEnumSchema =
  UntitledMultiSelectEnumSchema | TitledMultiSelectEnumSchema;

/**
 * Use {@link TitledSingleSelectEnumSchema} instead.
 * This interface will be removed in a future version.
 *
 * @category `elicitation/create`
 */
export interface LegacyTitledEnumSchema {
  type: "string";
  title?: string;
  description?: string;
  enum: string[];
  /**
   * (Legacy) Display names for enum values.
   * Non-standard according to JSON schema 2020-12.
   */
  enumNames?: string[];
  default?: string;
}

/**
 * @category `elicitation/create`
 */
// Union type for all enum schemas
export type EnumSchema =
  SingleSelectEnumSchema | MultiSelectEnumSchema | LegacyTitledEnumSchema;

/**
 * The result returned by the client for an {@link ElicitRequest| elicitation/create} request.
 *
 * @example Input single field
 * {@includeCode ./examples/ElicitResult/input-single-field.json}
 *
 * @example Input multiple fields
 * {@includeCode ./examples/ElicitResult/input-multiple-fields.json}
 *
 * @example Accept URL mode (no content)
 * {@includeCode ./examples/ElicitResult/accept-url-mode-no-content.json}
 *
 * @category `elicitation/create`
 */
export interface ElicitResult {
  /**
   * The user action in response to the elicitation.
   * - `"accept"`: User submitted the form/confirmed the action
   * - `"decline"`: User explicitly declined the action
   * - `"cancel"`: User dismissed without making an explicit choice
   */
  action: "accept" | "decline" | "cancel";

  /**
   * The submitted form data, only present when action is `"accept"` and mode was `"form"`.
   * Contains values matching the requested schema.
   * Omitted for out-of-band mode responses.
   */
  content?: { [key: string]: string | number | boolean | string[] };
}

/* Client messages */
/** @internal */
export type ClientRequest =
  | DiscoverRequest
  | CompleteRequest
  | GetPromptRequest
  | ListPromptsRequest
  | ListResourcesRequest
  | ListResourceTemplatesRequest
  | ReadResourceRequest
  | SubscriptionsListenRequest
  | CallToolRequest
  | ListToolsRequest;

/** @internal */
export type ClientNotification = CancelledNotification;

/** @internal */
export type ClientResult = EmptyResult;

/* Server messages */

/** @internal */
export type ServerNotification =
  | CancelledNotification
  | ProgressNotification
  | LoggingMessageNotification
  | ResourceUpdatedNotification
  | ResourceListChangedNotification
  | ToolListChangedNotification
  | PromptListChangedNotification
  | SubscriptionsAcknowledgedNotification;

/** @internal */
export type ServerResult =
  | EmptyResult
  | DiscoverResult
  | CompleteResult
  | GetPromptResult
  | ListPromptsResult
  | ListResourceTemplatesResult
  | ListResourcesResult
  | ReadResourceResult
  | SubscriptionsListenResult
  | CallToolResult
  | ListToolsResult
  | InputRequiredResult;
