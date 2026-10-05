---
title: "Quickstart (MCP with FalkorDB)"
type: docs
weight: 1
description: >
  How to get started running Toolbox with MCP Inspector and FalkorDB as the source.
sample_filters: ["FalkorDB", "MCP Inspector"]
is_sample: true
---

## Overview

[Model Context Protocol](https://modelcontextprotocol.io) is an open protocol
that standardizes how applications provide context to LLMs. Check out this page
on how to [connect to Toolbox via
MCP](../../../documentation/connect-to/mcp-client/_index.md).

## Step 1: Set up your FalkorDB graph and data

In this section, you'll start a FalkorDB instance and populate it with sample
data for a movies-related agent.

1. **Start FalkorDB.** The quickest way is Docker:

   ```bash
   docker run -d --name falkordb -p 6379:6379 falkordb/falkordb
   ```

1. **Populate the graph with data.** Using `redis-cli` (or the FalkorDB
   browser at `http://localhost:3000` if you expose it), create a small movie
   graph:

   ```bash
   docker exec falkordb redis-cli GRAPH.QUERY movies "CREATE
     (hanks:Person {name: 'Tom Hanks'}),
     (ryan:Person {name: 'Meg Ryan'}),
     (sleepless:Movie {title: 'Sleepless in Seattle', year: 1993}),
     (gump:Movie {title: 'Forrest Gump', year: 1994}),
     (hanks)-[:ACTED_IN]->(sleepless),
     (hanks)-[:ACTED_IN]->(gump),
     (ryan)-[:ACTED_IN]->(sleepless)"
   ```

## Step 2: Install and configure Toolbox

In this section, we will download Toolbox and run the Toolbox server with a
FalkorDB configuration.

1. Download the latest version of Toolbox as a binary. Select the [correct
   binary](https://github.com/googleapis/mcp-toolbox/releases) corresponding
   to your OS and CPU architecture.

1. Make the binary executable:

   ```bash
   chmod +x toolbox
   ```

1. Write the following into a `tools.yaml` file:

   ```yaml
   kind: source
   name: falkordb-movies
   type: falkordb
   host: 127.0.0.1
   port: "6379"
   graph: movies
   ---
   kind: tool
   name: search_movies_by_actor
   type: falkordb-cypher
   source: falkordb-movies
   statement: |
     MATCH (m:Movie)<-[:ACTED_IN]-(p:Person)
     WHERE p.name = $name
     RETURN m.title, m.year
     LIMIT 10
   description: Use this tool to list the movies a given actor acted in.
   parameters:
     - name: name
       type: string
       description: Full name of the actor.
   ---
   kind: tool
   name: execute_cypher
   type: falkordb-execute-cypher
   source: falkordb-movies
   description: Use this tool to execute a Cypher query against the movie graph.
   ---
   kind: tool
   name: get_schema
   type: falkordb-schema
   source: falkordb-movies
   description: Use this tool to get the schema of the movie graph.
   ```

1. Start the Toolbox server:

   ```bash
   ./toolbox --tools-file "tools.yaml"
   ```

## Step 3: Connect to MCP Inspector

1. Run the MCP Inspector:

   ```bash
   npx @modelcontextprotocol/inspector
   ```

1. Type `y` when it asks to install the inspector package.

1. It should show the following when the MCP Inspector is up and running:

   ```bash
   MCP Inspector is up and running at http://127.0.0.1:6274
   ```

1. Open the above link in your browser.

1. For `Transport Type`, select `Streamable HTTP`.

1. For `URL`, type in `http://127.0.0.1:5000/mcp`.

1. Click the `Connect` button.

1. Click `List Tools` — you should see the `search_movies_by_actor`,
   `execute_cypher`, and `get_schema` tools. Select one and try it out: ask
   for Tom Hanks' movies, run an ad-hoc Cypher query, or fetch the graph
   schema.
