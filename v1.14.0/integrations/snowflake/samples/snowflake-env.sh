#!/bin/bash

# Snowflake Connection Configuration
# Copy this file to snowflake-env.sh and update with your actual values
# Then source it before running the toolbox: source snowflake-env.sh

# Required environment variables
export SNOWFLAKE_ACCOUNT="your-account-identifier"     # e.g., "xy12345.snowflakecomputing.com"
export SNOWFLAKE_USER="your-username"                  # Your Snowflake username
export SNOWFLAKE_PASSWORD="your-password"              # Your Snowflake password
export SNOWFLAKE_DATABASE="your-database"              # Database name
export SNOWFLAKE_SCHEMA="your-schema"                  # Schema name (usually "PUBLIC")

# Optional environment variables (will use defaults if not set)
export SNOWFLAKE_WAREHOUSE="COMPUTE_WH"                # Warehouse name (default: COMPUTE_WH)
export SNOWFLAKE_ROLE="ACCOUNTADMIN"                   # Role name (default: ACCOUNTADMIN)

echo "Snowflake environment variables have been set!"
echo "Account: $SNOWFLAKE_ACCOUNT"
echo "User: $SNOWFLAKE_USER"
echo "Database: $SNOWFLAKE_DATABASE"
echo "Schema: $SNOWFLAKE_SCHEMA"
echo "Warehouse: $SNOWFLAKE_WAREHOUSE"
echo "Role: $SNOWFLAKE_ROLE"
echo ""
echo "You can now run the toolbox with:"
echo "  ./toolbox --prebuilt snowflake                    # Use prebuilt configuration"
echo "  ./toolbox --config docs/en/integrations/snowflake/samples/snowflake-config.yaml  # Use custom configuration"
