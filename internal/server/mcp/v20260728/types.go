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

package v20260728

import (
	"github.com/googleapis/mcp-toolbox/internal/server/mcp/jsonrpc"
	"github.com/googleapis/mcp-toolbox/internal/server/mcp/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

// SERVER_NAME is the server name used in Implementation.
const SERVER_NAME = "Toolbox"

// PROTOCOL_VERSION is the version of the MCP protocol in this package.
const PROTOCOL_VERSION = util.VERSION_20260728

// methods that are supported.
const (
	SERVER_DISCOVER = "server/discover"
	TOOLS_LIST      = "tools/list"
	TOOLS_CALL      = "tools/call"
	PROMPTS_LIST    = "prompts/list"
	PROMPTS_GET     = "prompts/get"
	GROUPS_LIST     = "groups/list"
	GROUPS_GET      = "groups/get"
)

/* Request Params */
type RequestParams struct {
	Meta *RequestMetaObject `json:"_meta"`
}

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
 */
type RequestMetaObject struct {
	// If specified, the caller is requesting out-of-band progress
	// notifications for this request (as represented by
	// notifications/progress). The value of this parameter is an
	// opaque token that will be attached to any subsequent
	// notifications. The receiver is not obligated to provide these
	// notifications.
	ProgressToken jsonrpc.ProgressToken `json:"progressToken,omitempty"`
	/**
	 * The MCP Protocol Version being used for this request. Required.
	 *
	 * For the HTTP transport, this value MUST match the `MCP-Protocol-Version`
	 * header; otherwise the server MUST return a `400 Bad Request`. If the
	 * server does not support the requested version, it MUST return an
	 * UnsupportedProtocolVersionError.
	 */
	ProtocolVersion string `json:"io.modelcontextprotocol/protocolVersion"`
	/**
	 * Identifies the client software making the request. Required.
	 *
	 * The Implementation schema requires `name` and `version`; other
	 * fields are optional.
	 */
	ClientInfo Implementation `json:"io.modelcontextprotocol/clientInfo"`
	/**
	 * The client's capabilities for this specific request. Required.
	 *
	 * Capabilities are declared per-request rather than once at initialization;
	 * an empty object means the client supports no optional capabilities.
	 * Servers MUST NOT infer capabilities from prior requests.
	 */
	MetaClientCapabilities *ClientCapabilities `json:"io.modelcontextprotocol/clientCapabilities"`
}

/**
 * Extends {@link MetaObject} with additional result-specific fields. All key naming rules from `MetaObject` apply.
 */
type ResultMetaObject struct {
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
	ServerInfo Implementation `json:"io.modelcontextprotocol/serverInfo,omitempty"`
}

// ClientCapabilities represents capabilities a client may support. Known
// capabilities are defined here, in this schema, but this is not a closed set: any
// client can define its own, additional capabilities.
type ClientCapabilities struct {
	// Experimental, non-standard capabilities that the client supports.
	Experimental map[string]any `json:"experimental,omitempty"`
	// Standard extensions that the client supports.
	Extensions map[string]any `json:"extensions,omitempty"`
	// Present if the client supports listing roots.
	Roots *ListChanged `json:"roots,omitempty"`
	// Present if the client supports sampling from an LLM.
	Sampling struct{} `json:"sampling,omitempty"`
}

/* Discovery */

/**
 * A request from the client asking the server to advertise its supported
 * protocol versions, capabilities, and other metadata. Servers **MUST**
 * implement `server/discover`. Clients **MAY** call it but are not required
 * to — version negotiation can also happen inline via per-request `_meta`.
 */
type DiscoverRequest struct {
	jsonrpc.Request
	Params RequestParams `json:"params,omitempty"`
}

// The result returned by the server for a {@link DiscoverRequest | server/discover} request.
type DiscoverResult struct {
	Result
	/**
	 * MCP Protocol Versions this server supports. The client should choose a
	 * version from this list for use in subsequent requests.
	 */
	SupportedVersions []string `json:"supportedVersions"`
	/**
	 * The capabilities of the server.
	 */
	Capabilities ServerCapabilities `json:"capabilities"`
	/**
	 * Natural-language guidance describing the server and its features.
	 *
	 * This can be used by clients to improve an LLM's understanding of
	 * available tools (e.g., by including it in a system prompt). It should
	 * focus on information that helps the model use the server effectively
	 * and should not duplicate information already in tool descriptions.
	 */
	Instructions string `json:"instructions,omitempty"`
}

// Base interface for metadata with name (identifier) and title (display name) properties.
type BaseMetadata struct {
	// Intended for programmatic or logical use, but used as a display name in past specs
	// or fallback (if title isn't present).
	Name string `json:"name"`
	// Intended for UI and end-user contexts — optimized to be human-readable and easily understood,
	//even by those unfamiliar with domain-specific terminology.
	//
	// If not provided, the name should be used for display (except for Tool,
	// where `annotations.title` should be given precedence over using `name`,
	// if present).
	Title string `json:"title,omitempty"`
}

// Implementation describes the name and version of an MCP implementation.
type Implementation struct {
	BaseMetadata
	Version string `json:"version"`
}

// ServerCapabilities represents capabilities that a server may support. Known
// capabilities are defined here, in this schema, but this is not a closed set: any
// server can define its own, additional capabilities.
type ServerCapabilities struct {
	Extensions map[string]any `json:"extensions,omitempty"`
	Tools      *ListChanged   `json:"tools,omitempty"`
	Prompts    *ListChanged   `json:"prompts,omitempty"`
}

// ListChange represents whether the server supports notification for changes to the capabilities.
type ListChanged struct {
	ListChanged *bool `json:"listChanged,omitempty"`
}

/* Empty result */

// EmptyResult represents a response that indicates success but carries no data.
type EmptyResult Result

/* Pagination */

// Cursor is an opaque token used to represent a cursor for pagination.
type Cursor string

// Common params for paginated requests.
type PaginatedRequest struct {
	jsonrpc.Request
	Params PaginatedRequestParams `json:"params,omitempty"`
}

type PaginatedRequestParams struct {
	RequestParams
	// An opaque token representing the current pagination position.
	// If provided, the server should return results starting after this cursor.
	Cursor Cursor `json:"cursor,omitempty"`
}

type PaginatedResult struct {
	// An opaque token representing the pagination position after the last returned result.
	// If present, there may be more results available.
	NextCursor Cursor `json:"nextCursor,omitempty"`
}

type cacheScope string

const (
	cacheScopePublic  cacheScope = "public"
	cacheScopePrivate cacheScope = "private"
)

// A result that supports a time-to-live (TTL) hint for client-side caching.
type CacheableResult struct {
	/**
	 * A hint from the server indicating how long (in milliseconds) the
	 * client MAY cache this response before re-fetching. Semantics are
	 * analogous to HTTP Cache-Control max-age.
	 *
	 * - If 0, The response SHOULD be considered immediately stale,
	 *   The client MAY re-fetch every time the result is needed.
	 * - If positive, the client SHOULD consider the result fresh for this many
	 *   milliseconds after receiving the response.
	 */
	TtlMs int `json:"ttlMs"`
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
	 * Toolbox currently default all as public.
	 */
	CacheScope cacheScope `json:"cacheScope"`
}

type resultType string

const (
	resultTypeComplete      resultType = "complete"       // the request completed successfully and the result contains the final content.
	resultTypeInputRequired resultType = "input_required" // the request is incomplete and the result contains an {@link InputRequiredResult} object
)

type Result struct {
	jsonrpc.Result
	/**
	* Indicates the type of the result, which allows the client to determine
	* how to parse the result object.
	*
	* Servers implementing this protocol version MUST include this field.
	* For backward compatibility, when a client receives a result from a
	* server implementing an earlier protocol version (which does not include
	* `resultType`), the client MUST treat the absent field as `"complete"`.
	 */
	ResultType resultType `json:"resultType"`
}

/* Tools */

// Sent from the client to request a list of tools the server has.
type ListToolsRequest struct {
	PaginatedRequest
}

// The server's response to a tools/list request from the client.
type ListToolsResult struct {
	Result
	PaginatedResult
	CacheableResult
	Tools []Tool `json:"tools"`
}

type Tool struct {
	BaseMetadata
	/**
	 * A human-readable description of the tool.
	 *
	 * This can be used by clients to improve the LLM's understanding of available tools. It can be thought of like a "hint" to the model.
	 */
	Description string `json:"description,omitempty"`
	// A JSON Schema object defining the expected parameters for the tool.
	ToolInputSchema   InputSchema  `json:"inputSchema,omitempty"`
	SecureInputSchema *InputSchema `json:"secureInputSchema,omitempty"`
	// Optional additional tool information.
	Annotations *ToolAnnotations `json:"annotations,omitempty"`
	// See [General fields: `_meta`](/specification/2025-11-25/basic/index#_meta) for notes on `_meta` usage.
	Metadata map[string]any `json:"_meta,omitempty"`
}

type InputSchema struct {
	Type       string                                     `json:"type"`
	Properties map[string]parameters.ParameterMcpManifest `json:"properties"`
	Required   []string                                   `json:"required"`
}

// Used by the client to invoke a tool provided by the server.
type CallToolRequest struct {
	jsonrpc.Request
	Params CallToolRequestParams `json:"params,omitempty"`
}

// Parameters for a `tools/call` request.
type CallToolRequestParams struct {
	RequestParams
	/**
	 * The name of the tool.
	 */
	Name string `json:"name"`
	/**
	 * Arguments to use for the tool call.
	 */
	Arguments map[string]any `json:"arguments,omitempty"`
	/**
	 * Secure arguments to use for the tool call.
	 */
	SecureArguments map[string]any `json:"secureArguments,omitempty"`
}

// The sender or recipient of messages and data in a conversation.
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// Base for objects that include optional annotations for the client.
// The client can use annotations to inform how objects are used or displayed
type Annotated struct {
	Annotations *struct {
		// Describes who the intended customer of this object or data is.
		// It can include multiple entries to indicate content useful for multiple
		// audiences (e.g., `["user", "assistant"]`).
		Audience []Role `json:"audience,omitempty"`
		// Describes how important this data is for operating the server.
		//
		// A value of 1 means "most important," and indicates that the data is
		// effectively required, while 0 means "least important," and indicates that
		// the data is entirely optional.
		//
		// @TJS-type number
		// @minimum 0
		// @maximum 1
		Priority float64 `json:"priority,omitempty"`
	} `json:"annotations,omitempty"`
}

// TextContent represents text provided to or from an LLM.
type TextContent struct {
	Annotated
	Type string `json:"type"`
	// The text content of the message.
	Text string `json:"text"`
}

// The server's response to a tool call.
//
// Any errors that originate from the tool SHOULD be reported inside the result
// object, with `isError` set to true, _not_ as an MCP protocol-level error
// response. Otherwise, the LLM would not be able to see that an error occurred
// and self-correct.
//
// However, any errors in _finding_ the tool, an error indicating that the
// server does not support tool calls, or any other exceptional conditions,
// should be reported as an MCP error response.
type CallToolResult struct {
	Result
	// Could be either a TextContent, ImageContent, or EmbeddedResources
	// For Toolbox, we will only be sending TextContent
	Content []TextContent `json:"content"`
	// Whether the tool call ended in an error.
	// If not set, this is assumed to be false (the call was successful).
	//
	// Any errors that originate from the tool SHOULD be reported inside the result
	// object, with `isError` set to true, _not_ as an MCP protocol-level error
	// response. Otherwise, the LLM would not be able to see that an error occurred
	// and self-correct.
	//
	// However, any errors in _finding_ the tool, an error indicating that the
	// server does not support tool calls, or any other exceptional conditions,
	// should be reported as an MCP error response.
	IsError bool `json:"isError,omitempty"`
	// An optional JSON object that represents the structured result of the tool call.
	StructuredContent map[string]any `json:"structuredContent,omitempty"`
}

// Additional properties describing a Tool to clients.
//
// NOTE: all properties in ToolAnnotations are **hints**.
// They are not guaranteed to provide a faithful description of
// tool behavior (including descriptive properties like `title`).
//
// Clients should never make tool use decisions based on ToolAnnotations
// received from untrusted servers.
type ToolAnnotations struct {
	// A human-readable title for the tool.
	Title string `json:"title,omitempty"`
	// If true, the tool does not modify its environment.
	// Default: false
	ReadOnlyHint *bool `json:"readOnlyHint,omitempty"`
	// If true, the tool may perform destructive updates to its environment.
	// If false, the tool performs only additive updates.
	// (This property is meaningful only when `readOnlyHint == false`)
	// Default: true
	DestructiveHint *bool `json:"destructiveHint,omitempty"`
	// If true, calling the tool repeatedly with the same arguments
	// will have no additional effect on the its environment.
	// (This property is meaningful only when `readOnlyHint == false`)
	// Default: false
	IdempotentHint *bool `json:"idempotentHint,omitempty"`
	// If true, this tool may interact with an "open world" of external
	// entities. If false, the tool's domain of interaction is closed.
	// For example, the world of a web search tool is open, whereas that
	// of a memory tool is not.
	// Default: true
	OpenWorldHint *bool `json:"openWorldHint,omitempty"`
}

/* Prompts */

// Sent from the client to request a list of prompts the server has.
type ListPromptsRequest struct {
	PaginatedRequest
}

// The server's response to a prompts/list request from the client.
type ListPromptsResult struct {
	Result
	PaginatedResult
	CacheableResult
	Prompts []Prompt `json:"prompts"`
}

// Used by the client to get a prompt provided by the server.
type GetPromptRequest struct {
	jsonrpc.Request
	Params GetPromptRequestParams `json:"params"`
}

// Parameters for a `prompts/get` request.
type GetPromptRequestParams struct {
	RequestParams
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

// The server's response to a prompts/get request from the client.
type GetPromptResult struct {
	Result
	Description string          `json:"description,omitempty"`
	Messages    []PromptMessage `json:"messages"`
}

// A prompt or prompt template that the server offers.
type Prompt struct {
	BaseMetadata
	// An optional description of what this prompt provides
	Description string `json:"description,omitempty"`
	// A list of arguments to use for templating the prompt.
	Arguments []PromptArgument `json:"arguments,omitempty"`
	// See [General fields: `_meta`](/specification/2025-11-25/basic/index#_meta) for notes on `_meta` usage.
	Metadata map[string]any `json:"_meta,omitempty"`
}

// Describes an argument that a prompt can accept.
type PromptArgument struct {
	BaseMetadata
	// A human-readable description of the argument.
	Description string `json:"description,omitempty"`
	// Whether this argument must be provided.
	Required bool `json:"required,omitempty"`
}

// Describes a message returned as part of a prompt.
type PromptMessage struct {
	Role    string      `json:"role"`
	Content TextContent `json:"content"`
}

/* Groups */

// ListGroupsRequest is sent from the client to request the list of groups the
// server has. It is not paginated: a server configures a bounded set of groups,
// so groups/list always returns all of them in one response.
type ListGroupsRequest struct {
	jsonrpc.Request
	Params RequestParams `json:"params,omitempty"`
}

// Group is a single entry in a groups/list response.
type Group struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// ListGroupsResult is the server's response to a groups/list request.
type ListGroupsResult struct {
	Result
	Groups []Group `json:"groups"`
}

// GetGroupRequest is sent from the client to request a single group's contents.
type GetGroupRequest struct {
	jsonrpc.Request
	Params GetGroupRequestParams `json:"params"`
}

// GetGroupRequestParams contains the parameters for a groups/get request.
type GetGroupRequestParams struct {
	RequestParams
	Name string `json:"name"`
}

// GetGroupResult is the server's response to a groups/get request: the group's
// tools and prompts. The description is intentionally omitted; it is exposed only
// through groups/list.
type GetGroupResult struct {
	Result
	CacheableResult
	Name    string   `json:"name"`
	Tools   []Tool   `json:"tools"`
	Prompts []Prompt `json:"prompts"`
}
