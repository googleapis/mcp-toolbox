---
title: "Knowledge Catalog (formerly known as Dataplex)"
type: docs
description: "Details of the Knowledge Catalog prebuilt configuration."
aliases:
  - /integrations/dataplex/prebuilt-configs/dataplex/
---

## Knowledge Catalog

*   `--prebuilt` value: `dataplex`
*   **Environment Variables:**
    *   `DATAPLEX_PROJECT`: The GCP project ID.
*   **Permissions:**
    *   **Dataplex Reader** (`roles/dataplex.viewer`) to search and look up
        entries.
    *   **Dataplex Editor** (`roles/dataplex.editor`) to modify entries.
*   **Tools:**
    *   `search_entries`: Searches for entries in Knowledge Catalog.
    *   `lookup_entry`: Retrieves a specific entry from Knowledge Catalog.
    *   `lookup_context`: Retrieves rich metadata regarding one or more data assets along with their relationships.
    *   `search_dq_scans`: Searches for data quality scans in Dataplex.
    *   `list_data_products`: Lists Data Products across all locations.
    *   `get_data_product`: Retrieves a specific Data Product.
    *   `list_data_assets`: Lists Data Assets under a Data Product.
    *   `get_data_asset`: Retrieves specific metadata regarding a Data Asset.
    *   `create_data_product`: Creates a new Data Product.
    *   `update_data_product`: Updates an existing Data Product.
    *   `create_data_asset`: Creates a new Data Asset under a Data Product.
    *   `update_data_asset`: Updates an existing Data Asset under a Data Product.
    *   `update_data_product_aspects`: Updates aspects (like documentation or contracts) on a Data Product Entry.
    *   `generate_data_insights`: Creates a new Dataplex Data Documentation scan template and triggers the run.
    *   `get_data_insights`: Retrieves the final generated data insights for a completed scan.
    *   `generate_data_profile`: Creates a new Dataplex Data Profile scan template and triggers the run.
    *   `get_data_profile`: Retrieves the final generated data profile results.
    *   `discover_metadata`: Creates a new Dataplex Data Discovery scan template and triggers the run.
    *   `get_discovery_results`: Retrieves the final generated data discovery results.
    *   `check_data_quality`: Creates a new Dataplex Data Quality scan template and triggers the run.
    *   `get_data_quality_results`: Retrieves the final generated data quality results.
    *   `get_operation`: Retrieves the status of a Dataplex long-running operation.
    *   `get_run_status`: Retrieves the execution status of the latest background job run.
*   **Toolsets:**
    *   `discovery`: Metadata discovery and search toolset (`search_entries`, `lookup_entry`, `lookup_context`, `search_dq_scans`).
    *   `data-products`: Data Products and Data Assets curation and management toolset (`search_entries`, `lookup_entry`, `lookup_context`, `list_data_products`, `get_data_product`, `list_data_assets`, `get_data_asset`, `create_data_product`, `update_data_product`, `create_data_asset`, `update_data_asset`, `update_data_product_aspects`, `get_operation`).
    *   `enrich`: Metadata enrichment pipeline orchestration and execution toolset (`search_entries`, `lookup_entry`, `lookup_context`, `generate_data_insights`, `get_data_insights`, `generate_data_profile`, `get_data_profile`, `discover_metadata`, `get_discovery_results`, `check_data_quality`, `get_data_quality_results`, `get_operation`, `get_run_status`).
