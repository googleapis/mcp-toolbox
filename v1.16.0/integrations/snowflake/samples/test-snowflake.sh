#!/bin/bash

# Test script to demonstrate Snowflake configuration with environment variables
# This script shows how to set up and test the Snowflake toolbox configuration

echo "=== Testing Snowflake Configuration ==="
echo ""

# Set up test environment variables (replace with your actual values)
echo "Setting up test environment variables..."
export SNOWFLAKE_ACCOUNT="test-account"
export SNOWFLAKE_USER="test-user"
export SNOWFLAKE_PASSWORD="test-password"
export SNOWFLAKE_DATABASE="test-database"
export SNOWFLAKE_SCHEMA="test-schema"
export SNOWFLAKE_WAREHOUSE="COMPUTE_WH"
export SNOWFLAKE_ROLE="ACCOUNTADMIN"

echo "Environment variables set:"
echo "  SNOWFLAKE_ACCOUNT: $SNOWFLAKE_ACCOUNT"
echo "  SNOWFLAKE_USER: $SNOWFLAKE_USER"
echo "  SNOWFLAKE_DATABASE: $SNOWFLAKE_DATABASE"
echo "  SNOWFLAKE_SCHEMA: $SNOWFLAKE_SCHEMA"
echo "  SNOWFLAKE_WAREHOUSE: $SNOWFLAKE_WAREHOUSE"
echo "  SNOWFLAKE_ROLE: $SNOWFLAKE_ROLE"
echo ""

echo "=== Testing Prebuilt Configuration ==="
echo "This will attempt to initialize with the prebuilt Snowflake configuration:"
echo "Command: ./toolbox --prebuilt snowflake --stdio"
echo ""
echo "Expected result: Connection failure due to test credentials (this is normal)"
echo ""

# Test the prebuilt configuration (this will fail with test credentials, which is expected)
timeout 5s ./toolbox --prebuilt snowflake --stdio 2>&1 | head -5

echo ""
echo "=== Testing Custom Configuration ==="
echo "This will attempt to initialize with the custom Snowflake configuration:"
echo "Command: ./toolbox --config docs/en/integrations/snowflake/samples/snowflake-config.yaml --stdio"
echo ""
echo "Expected result: Connection failure due to test credentials (this is normal)"
echo ""

# Test the custom configuration (this will fail with test credentials, which is expected)
timeout 5s ./toolbox --config docs/en/integrations/snowflake/samples/snowflake-config.yaml --stdio 2>&1 | head -5

echo ""
echo "=== Instructions for Real Usage ==="
echo "1. Copy docs/en/integrations/snowflake/samples/snowflake-env.sh to your own file"
echo "2. Edit it with your actual Snowflake credentials"
echo "3. Source the file: source your-snowflake-env.sh"
echo "4. Run: ./toolbox --prebuilt snowflake"
echo ""
echo "For more information, see docs/en/integrations/snowflake/samples"
