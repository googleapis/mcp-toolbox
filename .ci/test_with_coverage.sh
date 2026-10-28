#!/bin/bash

# Arguments:
# $1: Display name for logs (e.g., "Cloud SQL Postgres")
# $2: Integration test's package name (e.g., cloudsqlpg)
# $3, $4, ...: Tool package names for grep (e.g., postgressql), if the
# integration test specifically check a separate package inside a folder, please
# specify the full path instead (e.g., postgressql/postgresexecutesql)
#
# Environment:
# COVERAGE_THRESHOLD: minimum coverage percentage (default 50)
# EXTRA_TEST_ARGS: extra flags passed to the test binary

DISPLAY_NAME="$1"
SOURCE_PACKAGE_NAME="$2"

# Construct the test binary name
TEST_BINARY="${SOURCE_PACKAGE_NAME}.test"

# Construct the full source path
SOURCE_PATH="sources/${SOURCE_PACKAGE_NAME}/"

# Shift arguments so that $3 and onwards become the list of tool package names
shift 2
TOOL_PACKAGE_NAMES=("$@")

COVERAGE_FILE="${TEST_BINARY%.test}_coverage.out"
FILTERED_COVERAGE_FILE="${TEST_BINARY%.test}_filtered_coverage.out"

COVERAGE_THRESHOLD="${COVERAGE_THRESHOLD:-50}"

export path="github.com/googleapis/mcp-toolbox/internal/"

GREP_PATTERN="^mode:|${path}${SOURCE_PATH}"
# Add each tool package path to the grep pattern
for tool_name in "${TOOL_PACKAGE_NAMES[@]}"; do
  if [ -n "$tool_name" ]; then
    full_tool_path="tools/${tool_name}/"
    GREP_PATTERN="${GREP_PATTERN}|${path}${full_tool_path}"
  fi
done

# Run integration test
if ! ./"${TEST_BINARY}" -test.v ${EXTRA_TEST_ARGS:-} -test.coverprofile="${COVERAGE_FILE}"; then
  echo "Error: Tests for ${DISPLAY_NAME} failed. Exiting."
  exit 1
fi

# Filter source/tool packages
if ! grep -E "${GREP_PATTERN}" "${COVERAGE_FILE}" > "${FILTERED_COVERAGE_FILE}"; then
  echo "Warning: Could not filter coverage for ${DISPLAY_NAME}. Filtered file might be empty or invalid."
fi

# Calculate coverage
echo "Calculating coverage for ${DISPLAY_NAME}..."
total_coverage=$(go tool cover -func="${FILTERED_COVERAGE_FILE}" 2>/dev/null | grep "total:" | awk '{print $3}')


echo "${DISPLAY_NAME} total coverage: $total_coverage"
coverage_numeric=$(echo "$total_coverage" | sed 's/%//')

# Check coverage threshold
if awk -v coverage="$coverage_numeric" -v threshold="$COVERAGE_THRESHOLD" 'BEGIN {exit !(coverage < threshold)}'; then
    echo "Coverage failure: ${DISPLAY_NAME} total coverage($total_coverage) is below ${COVERAGE_THRESHOLD}%."
    exit 1
else
    echo "Coverage for ${DISPLAY_NAME} is sufficient."
fi
