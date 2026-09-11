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

package v20251125

import (
	"github.com/googleapis/mcp-toolbox/internal/server/mcp/jsonrpc"
	"github.com/googleapis/mcp-toolbox/internal/server/mcp/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

// SERVER_NAME is the server name used in Implementation.
const SERVER_NAME = "Toolbox"

// PROTOCOL_VERSION is the version of the MCP protocol in this package.
const PROTOCOL_VERSION = util.VERSION_20251125

// methods that are supported.
const (
	INITIALIZE               = "initialize"
	PING                     = "ping"
	TOOLS_LIST               = "tools/list"
	TOOLS_CALL               = "tools/call"
	PROMPTS_LIST             = "prompts/list"
	PROMPTS_GET              = "prompts/get"
	RESOURCES_LIST           = "resources/list"
	RESOURCES_TEMPLATES_LIST = "resources/templates/list"
	RESOURCES_READ           = "resources/read"
)

/* Initialization */

// Params to define MCP Client during initialize request.
type InitializeParams struct {
	// The latest version of the Model Context Protocol that the client supports.
	// The client MAY decide to support older versions as well.
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ClientCapabilities `json:"capabilities"`
	ClientInfo      Implementation     `json:"clientInfo"`
}

// InitializeRequest is sent from the client to the server when it first
// connects, asking it to begin initialization.
type InitializeRequest struct {
	jsonrpc.Request
	Params InitializeParams `json:"params"`
}

// InitializeResult is sent after receiving an initialize request from the
// client.
type InitializeResult struct {
	jsonrpc.Result
	// The version of the Model Context Protocol that the server wants to use.
	// This may not match the version that the client requested. If the client cannot
	// support this version, it MUST disconnect.
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ServerCapabilities `json:"capabilities"`
	ServerInfo      Implementation     `json:"serverInfo"`
	// Instructions describing how to use the server and its features.
	//
	// This can be used by clients to improve the LLM's understanding of
	// available tools, resources, etc. It can be thought of like a "hint" to the model.
	// For example, this information MAY be added to the system prompt.
	Instructions string `json:"instructions,omitempty"`
}

// InitializedNotification is sent from the client to the server after
// initialization has finished.
type InitializedNotification struct {
	jsonrpc.Notification
}

// ListChanged represents whether the server supports notification for changes to the capabilities.
type ListChanged struct {
	ListChanged *bool `json:"listChanged,omitempty"`
}

// ClientCapabilities represents capabilities a client may support. Known
// capabilities are defined here, in this schema, but this is not a closed set: any
// client can define its own, additional capabilities.
type ClientCapabilities struct {
	// Experimental, non-standard capabilities that the client supports.
	Experimental map[string]interface{} `json:"experimental,omitempty"`
	// Present if the client supports listing roots.
	Roots *ListChanged `json:"roots,omitempty"`
	// Present if the client supports sampling from an LLM.
	Sampling struct{} `json:"sampling,omitempty"`
}

// ServerCapabilities represents capabilities that a server may support. Known
// capabilities are defined here, in this schema, but this is not a closed set: any
// server can define its own, additional capabilities.
type ServerCapabilities struct {
	Tools     *ListChanged `json:"tools,omitempty"`
	Prompts   *ListChanged `json:"prompts,omitempty"`
	Resources *struct {
		Subscribe   *bool `json:"subscribe,omitempty"`
		ListChanged *bool `json:"listChanged,omitempty"`
	} `json:"resources,omitempty"`
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

/* Empty result */

// EmptyResult represents a response that indicates success but carries no data.
type EmptyResult jsonrpc.Result

/* Pagination */

// Cursor is an opaque token used to represent a cursor for pagination.
type Cursor string

type PaginatedRequest struct {
	jsonrpc.Request
	Params struct {
		// An opaque token representing the current pagination position.
		// If provided, the server should return results starting after this cursor.
		Cursor Cursor `json:"cursor,omitempty"`
	} `json:"params,omitempty"`
}

type PaginatedResult struct {
	jsonrpc.Result
	// An opaque token representing the pagination position after the last returned result.
	// If present, there may be more results available.
	NextCursor Cursor `json:"nextCursor,omitempty"`
}

/* Tools */

// Sent from the client to request a list of tools the server has.
type ListToolsRequest struct {
	PaginatedRequest
}

// The server's response to a tools/list request from the client.
type ListToolsResult struct {
	PaginatedResult
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
	ToolInputSchema InputSchema `json:"inputSchema,omitempty"`
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
	Params struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments,omitempty"`
	} `json:"params,omitempty"`
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
	jsonrpc.Result
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
	PaginatedResult
	Prompts []Prompt `json:"prompts"`
}

// Used by the client to get a prompt provided by the server.
type GetPromptRequest struct {
	jsonrpc.Request
	Params struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments,omitempty"`
	} `json:"params"`
}

// The server's response to a prompts/get request from the client.
type GetPromptResult struct {
	jsonrpc.Result
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

/* Resources */

// Sent from the client to request a list of resources the server has.
type ListResourcesRequest struct {
	PaginatedRequest
}

// A known resource that the server is capable of reading.
type Resource struct {
	BaseMetadata
	// A description of what this resource represents.
	//
	// This can be used by clients to improve the LLM's understanding of available resources. It can be thought of like a "hint" to the model.
	Description string `json:"description,omitempty"`
	// The URI of this resource.
	Uri string `json:"uri"`
	// The MIME type of this resource, if known.
	MimeType string `json:"mimeType,omitempty"`
	// The size of the raw resource content, in bytes (i.e., before base64 encoding or any tokenization), if known.
	//
	// This can be used by Hosts to display file sizes and estimate context window usage.
	Size *int64 `json:"size,omitempty"`
	// Optional annotations for the client.
	Annotations *Annotations `json:"annotations,omitempty"`
	// See [General fields: `_meta`](/specification/2025-11-25/basic/index#_meta) for notes on `_meta` usage.
	Metadata map[string]any `json:"_meta,omitempty"`
}

// Optional annotations for the client. The client can use annotations to inform how objects are used or displayed
type Annotations struct {
	// Describes who the intended audience of this object or data is.
	//
	// It can include multiple entries to indicate content useful for multiple audiences (e.g., `["user", "assistant"]`).
	Audience []Role `json:"audience,omitempty"`
	// Describes how important this data is for operating the server.
	//
	// A value of 1 means "most important," and indicates that the data is
	// effectively required, while 0 means "least important," and indicates that
	// the data is entirely optional.
	Priority *float64 `json:"priority,omitempty"`
	// The moment the resource was last modified, as an ISO 8601 formatted string.
	//
	// Should be an ISO 8601 formatted string (e.g., "2025-01-12T15:00:58Z").
	//
	// Examples: last activity timestamp in an open file, timestamp when the resource
	// was attached, etc.
	LastModified string `json:"lastModified,omitempty"`
}

// The server's response to a resources/list request from the client.
type ListResourcesResult struct {
	PaginatedResult
	Resources []Resource `json:"resources"`
}

// Sent from the client to request a list of resource templates the server has.
type ListResourceTemplatesRequest struct {
	PaginatedRequest
}

// A template description for resources available on the server.
type ResourceTemplate struct {
	BaseMetadata
	// A URI template (according to RFC 6570) that can be used to construct resource URIs.
	UriTemplate string `json:"uriTemplate"`
	// A description of what this template is for.
	//
	// This can be used by clients to improve the LLM's understanding of available resources. It can be thought of like a "hint" to the model.
	Description string `json:"description,omitempty"`
	// The MIME type for all resources that match this template. This should only be included if all resources matching this template have the same type.
	MimeType string `json:"mimeType,omitempty"`
	// Optional annotations for the client.
	Annotations *Annotations `json:"annotations,omitempty"`
	// See [General fields: `_meta`](/specification/2025-11-25/basic/index#_meta) for notes on `_meta` usage.
	Metadata map[string]any `json:"_meta,omitempty"`
}

// The server's response to a resources/templates/list request from the client.
type ListResourceTemplatesResult struct {
	PaginatedResult
	ResourceTemplates []ResourceTemplate `json:"resourceTemplates"`
}

// Sent from the client to the server, to read a specific resource URI.
type ReadResourceRequest struct {
	jsonrpc.Request
	Params ReadResourceRequestParams `json:"params"`
}

// Parameters for a `resources/read` request.
type ReadResourceRequestParams struct {
	// The URI of the resource to read.
	Uri string `json:"uri"`
	// See [General fields: `_meta`](/specification/2025-11-25/basic/index#_meta) for notes on `_meta` usage.
	Metadata map[string]any `json:"_meta,omitempty"`
}

// The server's response to a resources/read request from the client.
type ReadResourceResult struct {
	jsonrpc.Result
	// Could be either TextResourceContents or BlobResourceContents.
	// For Toolbox, we will only be sending TextResourceContents.
	Contents []TextResourceContents `json:"contents"`
}

// The contents of a specific resource or sub-resource.
type ResourceContents struct {
	// The URI of this resource.
	Uri string `json:"uri"`
	// The MIME type of this resource, if known.
	MimeType string `json:"mimeType,omitempty"`
	// See [General fields: `_meta`](/specification/2025-11-25/basic/index#_meta) for notes on `_meta` usage.
	Metadata map[string]any `json:"_meta,omitempty"`
}

// TextResourceContents represents text-based resource content.
type TextResourceContents struct {
	ResourceContents
	// The text of the item. This must only be set if the item can actually be represented as text (not binary data).
	Text string `json:"text"`
}
