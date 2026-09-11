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
 * Extension Identifier: com.google.cloud/toolbox.v1
 * Protocol Version: 2026-07-28
 *
 * This schema defines the TypeScript interfaces for the Groups feature
 * under the com.google.cloud/toolbox.v1 MCP extension.
 */

import type {
  CacheableResult,
  Prompt,
  RequestParams,
  Result,
} from "../../secureParams/schema/spec.types.js";
import type { ToolWithSecureParams } from "../../secureParams/schema/schema.js";

/**
 * A single entry in a `groups/list` response.
 *
 * @category `groups`
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
 * `groups/list` always returns all of them in one response. These params
 * therefore extend {@link RequestParams} rather than `PaginatedRequestParams`
 * — there is no `cursor` field.
 *
 * @category `groups`
 */
export type ListGroupsParams = RequestParams;

/**
 * The server's response to a `groups/list` request.
 *
 * Extends {@link Result} rather than `CacheableResult`: the response spans
 * groups that may each configure a different TTL, so no single `ttlMs` /
 * `cacheScope` hint applies.
 *
 * @category `groups`
 */
export interface ListGroupsResult extends Result {
  /**
   * Every named group on the server, sorted alphabetically by name. The
   * default (nameless) group is omitted.
   */
  groups: Group[];
}

/**
 * Request parameters for `groups/get`.
 *
 * @category `groups`
 */
export interface GetGroupParams extends RequestParams {
  /**
   * The name of the group to fetch. An omitted or empty string resolves to the
   * default (nameless) group, which holds every tool and prompt on the server.
   */
  name?: string;
}

/**
 * The server's response to a `groups/get` request: the group's tools and
 * prompts, plus the group's cache hints.
 *
 * Extends {@link CacheableResult}, whose `ttlMs` and `cacheScope` carry the
 * group's own configured values — the same hints `tools/list` and
 * `prompts/list` return when called on that group's endpoint. They default to
 * 300000 (5 minutes) and `"public"`.
 *
 * The group's `description` is intentionally omitted; it is exposed only
 * through `groups/list`.
 *
 * @category `groups`
 */
export interface GetGroupResult extends CacheableResult {
  /**
   * The name of the group that was resolved. Empty for the default group.
   */
  name: string;

  /**
   * The tools scoped to this group, in the same shape `tools/list` returns.
   * Typed as {@link ToolWithSecureParams} because reaching `groups/get` at all
   * requires declaring com.google.cloud/toolbox.v1, so tools defining secure
   * parameters always carry a `secureInputSchema` here.
   */
  tools: ToolWithSecureParams[];

  /**
   * The prompts scoped to this group, in the same shape `prompts/list`
   * returns.
   */
  prompts: Prompt[];
}
