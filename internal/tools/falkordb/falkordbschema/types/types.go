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

// Package types defines the data structures used to represent a FalkorDB
// graph schema, as returned by the falkordb-schema tool.
package types

// SchemaInfo is the complete schema description of a FalkorDB graph.
type SchemaInfo struct {
	GraphInfo     GraphInfo        `json:"graphInfo"`
	NodeLabels    []NodeLabel      `json:"nodeLabels"`
	Relationships []Relationship   `json:"relationships"`
	Indexes       []map[string]any `json:"indexes"`
	Constraints   []map[string]any `json:"constraints"`
	Statistics    Statistics       `json:"statistics"`
	Errors        []string         `json:"errors,omitempty"`
}

// GraphInfo describes the graph that was inspected.
type GraphInfo struct {
	Name      string `json:"name"`
	NodeCount int64  `json:"nodeCount"`
	EdgeCount int64  `json:"edgeCount"`
}

// NodeLabel describes a node label, its frequency, and the properties
// observed on sampled nodes carrying the label.
type NodeLabel struct {
	Name       string         `json:"name"`
	Count      int64          `json:"count"`
	Properties []PropertyInfo `json:"properties"`
}

// Relationship describes a relationship type, its frequency, the label
// patterns observed on sampled relationships, and their properties.
type Relationship struct {
	Type       string         `json:"type"`
	Count      int64          `json:"count"`
	StartNode  string         `json:"startNode,omitempty"`
	EndNode    string         `json:"endNode,omitempty"`
	Properties []PropertyInfo `json:"properties"`
}

// PropertyInfo describes a property and the value types observed for it.
type PropertyInfo struct {
	Name  string   `json:"name"`
	Types []string `json:"types"`
}

// RelConnectivityInfo tracks the most common start and end labels observed
// for a relationship type while sampling.
type RelConnectivityInfo struct {
	StartNode string
	EndNode   string
	Count     int64
}

// Statistics aggregates counts across the graph.
type Statistics struct {
	TotalNodes          int64            `json:"totalNodes"`
	TotalRelationships  int64            `json:"totalRelationships"`
	NodesByLabel        map[string]int64 `json:"nodesByLabel,omitempty"`
	RelationshipsByType map[string]int64 `json:"relationshipsByType,omitempty"`
}
