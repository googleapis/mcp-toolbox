---
title: "Looker Dev"
type: docs
description: "Details of the Looker Dev prebuilt configuration."
---

## Looker Dev

*   `--prebuilt` value: `looker-dev`
*   Almost always used in combination with Looker, `--prebuilt looker,looker-dev`
*   **Environment Variables:**
    *   `LOOKER_BASE_URL`: The URL of your Looker instance.
    *   `LOOKER_CLIENT_ID`: The client ID for the Looker API.
    *   `LOOKER_CLIENT_SECRET`: The client secret for the Looker API.
    *   `LOOKER_VERIFY_SSL`: Whether to verify SSL certificates.
    *   `LOOKER_USE_CLIENT_OAUTH`: Whether to use OAuth for authentication.
    *   `LOOKER_SHOW_HIDDEN_MODELS`: Whether to show hidden models.
    *   `LOOKER_SHOW_HIDDEN_EXPLORES`: Whether to show hidden explores.
    *   `LOOKER_SHOW_HIDDEN_FIELDS`: Whether to show hidden fields.
*   **Permissions:**
    *   A Looker account with permissions to access the desired projects
        and LookML is required.
*   **Tools:**
    *   `health_pulse`: Test the health of a Looker instance.
    *   `health_analyze`: Analyze the LookML usage of a Looker instance.
    *   `health_vacuum`: Suggest LookML elements that can be removed.
    *   `dev_mode`: Activate developer mode.
    *   `get_projects`: Get the LookML projects in a Looker instance.
    *   `get_project_files`: List the project files in a project.
    *   `get_project_file`: Get the content of a LookML file.
    *   `create_project_file`: Create a new LookML file.
    *   `update_project_file`: Update an existing LookML file.
    *   `delete_project_file`: Delete a LookML file.
    *   `get_project_directories`: Retrieves a list of project directories for a given LookML project.
    *   `create_project_directory`: Creates a new directory within a specified LookML project.
    *   `delete_project_directory`: Deletes a directory from a specified LookML project.
    *   `validate_project`: Check the syntax of a LookML project.
    *   `get_connections`: Get the available connections in a Looker instance.
    *   `get_connection_schemas`: Get the available schemas in a connection.
    *   `get_connection_databases`: Get the available databases in a connection.
    *   `get_connection_tables`: Get the available tables in a connection.
    *   `get_connection_table_columns`: Get the available columns for a table.
    *   `get_lookml_tests`: Retrieves a list of available LookML tests for a project.
    *   `run_lookml_tests`: Executes specific LookML tests within a project.
    *   `create_view_from_table`: Generates boilerplate LookML views directly from the database schema.
    *   `list_git_branches`: List the available git branches of a LookML project.
    *   `get_git_branch`: Get the current git branch of a LookML project.
    *   `create_git_branch`: Create a new git branch for a LookML project.
    *   `switch_git_branch`: Switch the git branch of a LookML project.
    *   `delete_git_branch`: Delete a git branch of a LookML project.
    *   `project_commit`: Commit changes in a LookML project.
    *   `push_branch`: Push the current dev branch to the remote git repository for a LookML project.
