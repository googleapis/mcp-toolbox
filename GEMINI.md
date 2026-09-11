# MCP Toolbox Context & Style Guide

This file (symlinked as `CLAUDE.md`, `AGENTS.md`, and `.gemini/styleguide.md`) provides context and guidelines for AI agents working on the MCP Toolbox for Databases project. It summarizes key information from `CONTRIBUTING.md` and `DEVELOPER.md`.

## Project Overview

**MCP Toolbox for Databases** is a Go-based project designed to provide Model Context Protocol (MCP) tools for various data sources and services. It allows Large Language Models (LLMs) to interact with databases and other tools safely and efficiently.


## Tech Stack

-   **Language:** Go (1.23+)
-   **Documentation:** Hugo (Extended Edition v0.146.0+)
-   **Containerization:** Docker
-   **CI/CD:** GitHub Actions, Google Cloud Build
-   **Linting:** `golangci-lint`

## Key Directories

-   `cmd/`: Application entry points.
-   `internal/sources/`: Implementations of database sources (e.g., Postgres, BigQuery).
-   `internal/tools/`: Implementations of specific tools for each source.
-   `internal/prompts/`: Implementations of MCP prompts.
-   `internal/resources/`: Implementations of MCP resources and resource templates (e.g., text, file).
-   `internal/group/`: Implementations of groups scoping tools, prompts, resources, and resource templates.
-   `tests/`: Integration tests.
-   `evals/`: Evalsets and harness configs for evaluating the prebuilt tool configs with [EvalBench](https://github.com/googlecloudplatform/evalbench); run by `.ci/evals.cloudbuild.yaml`.
-   `docs/en`: Project documentation. Separated logically into:
    - `documentation/`: Documentation and concepts (Section I).
    - `integrations/`: Reference architectures for DB connectivity and tools (Section II).
    - `samples/`: Tutorials and code samples (Section III).
    - `reference/`: CLI info and FAQs (Section IV).

## Development Workflow

### Prerequisites

-   Go 1.23 or later.
-   Docker (for building container images and running some tests).
-   Access to necessary Google Cloud resources for integration testing (if applicable).

### Building and Running

1.  **Build Binary:** `go build -o toolbox`
2.  **Run Server:** `go run .` (Listens on port 5000 by default)
3.  **Run with Help:** `go run . --help`
4.  **Test Endpoint:** `curl http://127.0.0.1:5000`

### Testing

-   **Unit Tests:** `go test -race -v ./cmd/... ./internal/...`
-   **Integration Tests:**
    -   Run specific source tests: `go test -race -v ./tests/<source_dir>`
    -   Example: `go test -race -v ./tests/alloydbpg`
    -   Add new sources to `.ci/integration.cloudbuild.yaml`
-   **Linting:** `golangci-lint run --fix`


## Developing Documentation

### Prerequisites

-   Hugo (Extended Edition v0.146.0+)
-   Node.js (for `npm ci`)

### Running Local Server

1.  Navigate to `.hugo` directory: `cd .hugo`
2.  Install dependencies: `npm ci`
3.  **Generate Search Index:** Because Pagefind requires physical files, `hugo server` alone will not populate the search bar. Build the local index first (using the development environment to block analytics) by running:
    `hugo --environment development && npx pagefind --site public --output-path static/pagefind`
4.  Start server: `hugo server`

### Versioning Workflows

Documentation builds automatically generate standard HTML alongside AI-friendly text files (`llms.txt` and `llms-full.txt`).

There are 6 workflows in total, handling parallel deployments to both GitHub Pages and Cloudflare Pages. **All deployment workflows automatically execute `npx pagefind --site public` to generate version-scoped search indexes.**

1.  **Deploy In-development docs**: Commits merged to `main` deploy to the `/dev/` path. Automatically defaults to version `Dev`.
2.  **Deploy Versioned Docs**: New GitHub releases deploy to `/<version>/` and the root path. The release tag is automatically injected into the build as the documentation version. *(Note: Developers must manually add the new version to the `[[params.versions]]` dropdown array in `hugo.toml` prior to merging a release PR).*
3.  **Deploy Previous Version Docs**: A manual workflow to rebuild older versions by explicitly passing the target tag via the GitHub Actions UI.

## Coding Conventions

### Tool Naming

-   **Tool Name:** `snake_case` (e.g., `list_collections`, `run_query`).
    -   Do *not* include the product name (e.g., avoid `firestore_list_collections`).
-   **Tool Type:** `kebab-case` (e.g., `firestore-list-collections`).
    -   *Must* include the product name.

### Branching and Commits

-   **Branch Naming:** `feat/`, `fix/`, `docs/`, `chore/` (e.g., `feat/add-gemini-md`).
-   **Commit Messages:** [Conventional Commits](https://www.conventionalcommits.org/) format.
    -   Format: `<type>(<scope>): <description>`
    -   Example: `feat(source/postgres): add new connection option`
    -   Types: `feat`, `fix`, `docs`, `chore`, `test`, `ci`, `refactor`, `revert`, `style`.

 ### PR Title Format

Format: `<type>[optional scope]: <description>`

- **Example:** `feat(source/postgres): add support for "new-field" field`
- **Example (Breaking Change):** `fix(tool/sql)!: change default parameter value`

#### Types

| Type | Description | Version change affected |
| :--- | :--- | :--- |
| **BREAKING CHANGE** | Anything with this type or a `!` after the type/scope introduces a breaking API change. E.g. `fix!: description` or `feat!: description`. | major |
| **feat** | Adding a new feature to the codebase. | minor |
| **fix** | Fixing a bug or typo in the codebase. | patch |
| **ci** | Changes made to the continuous integration configuration files or scripts (usually the yml and other configuration files). | n/a |
| **docs** | Documentations-related PRs, including fixes on docs. | n/a |
| **chore** | Other small tasks or updates that don't fall into any of the types above. | n/a |
| **perf** | changed src code, with improvement of performance metrics. | n/a |
| **refactor** | Change src code but unlike feat, there are no tests broken and no lines lost coverage. | n/a |
| **revert** | Revert changes made in another commit. | n/a |
| **style** | updated src code, with only formatting and whitespace updates. In other words, this includes anything a code formatter or linter changes. | n/a |
| **test** | Changes made to test files. | n/a |
| **build** | Changes related to build of the projects and dependency. | n/a |

#### Scopes

PRs addressing a specific source or tool should **always** add the source or tool name as scope.

The scope is formatted as `<type>/<kind>`. Common scopes include:
- `source/postgres`, `source/cloudsql-mysql`
- `tool/mssql-sql`, `tool/list-tables`
- `auth/google`

**Multiple Scopes:**
- If the PR covers multiple scopes of the same kind, separate them with a comma: `feat(source/postgres,source/alloydbpg): ...`.
- If the PR covers multiple scope types (e.g., adding a new database source and tool), disregard the scope type prefix: `feat(new-db): adding support for new-db source and tool`.

#### PR Description

Every PR must include a description that follows the repository's template:

**1. Description**
A concise description of the changes (bug or feature), its impact, and a summary of the solution.

**2. PR Checklist**
- [ ] Make sure to open an issue as a bug/issue before writing your code!
- [ ] Ensure you have manually reviewed the entire diff before requesting a review
- [ ] Ensure the tests and linter pass
- [ ] Code coverage does not decrease (if any source code was changed)
- [ ] Appropriate docs were updated (if necessary)
- [ ] Make sure to add `!` if this involves a breaking change

**3. Issue Reference**
Use the format: `Fixes #<issue_number> 🦕`

## Adding New Features

### Adding a New Data Source

1.  Create a new directory: `internal/sources/<newdb>`.
2.  Define `Config` and `Source` structs in `internal/sources/<newdb>/<newdb>.go`.
3.  Implement `SourceConfig` interface (`SourceConfigType`, `Initialize`).
4.  Implement `Source` interface (`SourceType`, `ToConfig`).
5.  Implement `init()` to register the source.
6.  Add unit tests in `internal/sources/<newdb>/<newdb>_test.go`.

### Adding a New Tool

1.  Create a new directory: `internal/tools/<newdb>/<toolname>`.
2.  Define a `Config` struct that **embeds `tools.ConfigBase`** (with `yaml:",inline"`). This supplies the shared `name`, `description`, `authRequired`, and `scopesRequired` fields and their getters — add only tool-specific fields and do not redeclare the shared ones.
3.  Define a `Tool` struct that **embeds `tools.BaseTool[Config]`**. Do *not* re-declare the boilerplate `Tool` methods (`GetName`, `GetDescription`, `Manifest`, `GetParameters`, `Authorized`, `RequiresClientAuthorization`, `GetAuthTokenHeaderName`, `EmbedParams`, etc.) — they are inherited from `BaseTool`.
4.  Implement `ToolConfig` interface (`ToolConfigType`, `Initialize`). In `Initialize`, construct the tool via `tools.NewBaseTool(cfg, annotations, manifest, staticParameters)`.
5.  Implement only the methods `BaseTool` does not provide: `Invoke` and `ToConfig`. Override an inherited method (e.g. `EmbedParams`, `RequiresClientAuthorization`, `GetAuthTokenHeaderName`) **only** when the tool's behavior differs from the default.
6.  Implement `init()` to register the tool.
7.  Add unit tests.

Refer to `internal/tools/postgres/postgressql/postgressql.go` for the canonical pattern.

### Adding Documentation

-   **For a new source:** Add source documentation to `docs/en/integrations/<source_name>/source.md`. Ensure the root `_index.md` file contains **strictly only frontmatter** and no markdown body text.
-   **For a new native tool:** Add tool documentation to `docs/en/integrations/<source_name>/tools/<tool_name>.md`. Ensure the `tools/_index.md` file contains **strictly only frontmatter**.
-   **For a new resource:** Add resource documentation to `docs/en/documentation/configuration/resources/<resource_type>.md`.
-   **Adding Integration Samples:** Add integration-specific samples to `docs/en/integrations/<source_name>/samples/`. Ensure the `samples/_index.md` file contains **strictly only frontmatter**.
-   **Tool Inheritance (Shared Tools):** Managed databases (e.g., Cloud SQL Postgres) that use the tools of their underlying engine (e.g., Postgres) map their inherited tools by utilizing the `shared_tools` frontmatter parameter inside their `tools/_index.md` file. This file must contain only frontmatter.
-   **New Top-Level Directories:** If adding a completely new top-level section to the documentation site, you must update the "Diátaxis Narrative Framework" section inside both `.hugo/layouts/index.llms.txt` and `.hugo/layouts/index.llms-full.txt` to keep the AI context synced with the site structure.


#### Integration Documentation Rules

When generating or editing documentation for this repository, you must strictly adhere to the following CI-enforced rules. Failure to do so will break the build.

##### Source Page Constraints (`integrations/**/source.md`)

1.  **File Naming:** The primary connection guide for a source must be named `source.md`. Use `_index.md` solely as an empty structural folder wrapper containing **only YAML frontmatter**.
2.  **LinkTitle:** The linkTitle has to be set to the string `Source` always.
3.  **Title Convention:** The YAML frontmatter `title` must always end with "Source" (e.g., `title: "Postgres Source"`).
4.  **No H1 Tags:** Never generate H1 (`#`) headings in the markdown body.
5.  **Strict H2 Ordering:** You must use the following H2 (`##`) headings in this exact sequence.
    *   `## About` (Required)
    *   `## Available Tools` (Optional)
    *   `## Requirements` (Optional)
    *   `## Example` (Required)
    *   `## Reference` (Required)
    *   `## Advanced Usage` (Optional)
    *   `## Troubleshooting` (Optional)
    *   `## Additional Resources` (Optional)
6.  **Shortcode Placement:** If you generate the `## Available Tools` section, you must include the `{{< list-tools >}}` shortcode beneath it.

##### Tool Page Constraints (`integrations/**/tools/*.md`)

1.  **Location:** All native tools must reside inside a nested `tools/` subdirectory. The `tools/` directory must contain an `_index.md` file consisting **strictly of frontmatter**.
2.  **No H1 Tags:** Never generate H1 (`#`) headings in the markdown body.
3.  **Strict H2 Ordering:** You must use the following H2 (`##`) headings in this exact sequence.
    *   `## About` (Required)
    *   `## Compatible Sources` (Optional)
    *   `## Requirements` (Optional)
    *   `## Parameters` (Optional)
    *   `## Example` (Required)
    *   `## Output Format` (Optional)
    *   `## Reference` (Optional)
    *   `## Advanced Usage` (Optional)
    *   `## Troubleshooting` (Optional)
    *   `## Additional Resources` (Optional)
4.  **Shortcode Placement:** If you generate the `## Compatible Sources` section, you must include the `{{< compatible-sources >}}` shortcode beneath it.
5.  **Title Convention:** The YAML frontmatter `title` must always be exactly the kebab-case name of the tool (e.g., `title: "arcadedb-execute-sql"`). Do **NOT** append the word "Tool" to the title (unlike source pages, which end with "Source").

##### Samples Architecture Constraints
Sample code is aggregated visually in the UI via the Samples section, but the physical markdown files are distributed logically based on their scope.
1.  **Quickstarts:** `docs/en/documentation/getting-started/`
2.  **Integration-Specific Samples:** `docs/en/integrations/<source_name>/samples/`. (The `samples/_index.md` wrapper must contain **strictly only frontmatter**).
3.  **General/Cross-Category Samples:** `docs/en/samples/`

##### Samples Maintenance Rules

1. **Filtering (`sample_filters`):** Always include `sample_filters` in the frontmatter. You MUST use strict enums for filtering.
   * **Source of Truth:** Always refer to `.hugo/data/filters.yaml` for the permitted list of Data Sources, Languages, Frameworks, and Categories.
   * **Adding New Filters:** If your sample requires the addition of a new filter, add it to `.hugo/data/filters.yaml` using **Title Case** (capitalize the first letter of every word, with words separated by spaces). Do not use snake_case or lowercase.
2. **Metadata:** Ensure `is_sample: true` is present to prevent the sample from being excluded from the Samples Gallery.

##### Prebuilt Config Constraints (`integrations/**/prebuilt-configs/*.md`)

1. **Naming & Path:** All prebuilt config docs must reside in `prebuilt-configs/`.
2. **Shortcode Requirement:** The main `documentation/configuration/prebuilt-configs/_index.md` page uses the `{{< list-prebuilt-configs >}}` shortcode, which only detects directories named exactly `prebuilt-configs`.
3. **YAML Mapping:** Always verify the `kind` of the data source in `internal/prebuiltconfigs/tools/` before choosing the integration folder.


##### Asset Constraints (`docs/`)

1.  **File Size Limits:** Never add files larger than 24MB to the `docs/` directory.

