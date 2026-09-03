/**
 * Extension Identifier: com.google.cloud/toolbox.v1
 * Protocol Version: 2026-07-28
 *
 * This schema defines the TypeScript interfaces for the Groups feature
 * under the com.google.cloud/toolbox.v1 MCP extension.
 */

/**
 * Extension identifier constant.
 */
export const TOOLBOX_EXTENSION_ID = "com.google.cloud/toolbox.v1";

/**
 * Client capabilities metadata structure advertising support for com.google.cloud/toolbox.v1.
 */
export interface GroupsClientCapabilities {
  extensions?: {
    [TOOLBOX_EXTENSION_ID]?: Record<string, unknown>;
    [key: string]: unknown;
  };
}

/**
 * Request metadata required on every groups request. Both the protocol version
 * and the client capabilities declaring the extension MUST be present.
 */
export interface GroupsRequestMeta {
  "io.modelcontextprotocol/protocolVersion": "2026-07-28";
  "io.modelcontextprotocol/clientInfo": {
    name: string;
    version: string;
    [key: string]: unknown;
  };
  "io.modelcontextprotocol/clientCapabilities": GroupsClientCapabilities;
  [key: string]: unknown;
}

/**
 * A single entry in a `groups/list` response.
 */
export interface Group {
  /**
   * The unique name of the group. Doubles as the group's endpoint path
   * segment (`/mcp/{name}`).
   */
  name: string;

  /**
   * Human-readable description of what the group contains. Configured on the
   * group and exposed only through `groups/list`.
   */
  description?: string;
}

/**
 * Request parameters for `groups/list`.
 *
 * The method is not paginated: a server configures a bounded set of groups, so
 * `groups/list` always returns all of them in one response. There is no
 * `cursor` field.
 */
export interface ListGroupsParams {
  _meta: GroupsRequestMeta;
}

/**
 * The server's response to a `groups/list` request.
 *
 * Carries no `ttlMs` / `cacheScope` hint: the response spans groups that may
 * each configure a different TTL, so no single hint applies.
 */
export interface ListGroupsResult {
  /**
   * Always `"complete"` for this method.
   */
  resultType: "complete";

  /**
   * Every named group on the server, sorted alphabetically by name. The
   * default (nameless) group is omitted.
   */
  groups: Group[];

  _meta?: {
    "io.modelcontextprotocol/serverInfo"?: {
      name: string;
      version: string;
      [key: string]: unknown;
    };
    [key: string]: unknown;
  };
}

/**
 * Request parameters for `groups/get`.
 */
export interface GetGroupParams {
  /**
   * The name of the group to fetch. An omitted or empty string resolves to the
   * default (nameless) group, which holds every tool and prompt on the server.
   */
  name?: string;

  _meta: GroupsRequestMeta;
}

/**
 * The server's response to a `groups/get` request: the group's tools and
 * prompts, plus the group's cache hints.
 *
 * The group's `description` is intentionally omitted; it is exposed only
 * through `groups/list`.
 */
export interface GetGroupResult {
  /**
   * Always `"complete"` for this method.
   */
  resultType: "complete";

  /**
   * The name of the group that was resolved. Empty for the default group.
   */
  name: string;

  /**
   * The tools scoped to this group, in the same shape `tools/list` returns.
   * Tools defining secure parameters carry a `secureInputSchema`; see the
   * Secure Parameters specification.
   */
  tools: Tool[];

  /**
   * The prompts scoped to this group, in the same shape `prompts/list`
   * returns.
   */
  prompts: Prompt[];

  /**
   * How long (in milliseconds) the client MAY cache this response. Taken from
   * the group's configured `ttlMs`, defaulting to 300000 (5 minutes).
   */
  ttlMs: number;

  /**
   * Intended scope of the cached response. Taken from the group's configured
   * `cacheScope`, defaulting to `"public"`.
   */
  cacheScope: "public" | "private";

  _meta?: {
    "io.modelcontextprotocol/serverInfo"?: {
      name: string;
      version: string;
      [key: string]: unknown;
    };
    [key: string]: unknown;
  };
}

/**
 * A tool as returned inside `GetGroupResult.tools`. Matches the base MCP `Tool`
 * shape, optionally augmented by the Secure Parameters feature of this same
 * extension.
 */
export interface Tool {
  name: string;
  title?: string;
  description?: string;
  inputSchema: Record<string, unknown>;
  secureInputSchema?: Record<string, unknown>;
  annotations?: Record<string, unknown>;
  _meta?: Record<string, unknown>;
}

/**
 * A prompt as returned inside `GetGroupResult.prompts`. Matches the base MCP
 * `Prompt` shape.
 */
export interface Prompt {
  name: string;
  title?: string;
  description?: string;
  arguments?: Array<{
    name: string;
    title?: string;
    description?: string;
    required?: boolean;
  }>;
  _meta?: Record<string, unknown>;
}

/**
 * JSON-RPC Error codes related to Groups.
 */
export enum GroupsErrorCode {
  /**
   * Returned when the client's negotiated protocol version predates
   * `2026-07-28`. `groups/list` and `groups/get` do not exist on those
   * versions.
   */
  MethodNotFound = -32601,

  /**
   * Returned when calling `groups/list` or `groups/get` on protocol version
   * `2026-07-28` without declaring `com.google.cloud/toolbox.v1` in client
   * capabilities — including when the server was started with
   * `--disable-ext com.google.cloud/toolbox.v1`.
   */
  MissingRequiredClientCapability = -32021,

  /**
   * Returned by `groups/get` when the requested group does not exist, and by
   * either method when required request metadata is missing.
   */
  InvalidParams = -32602,
}
