# Copyright 2026 Google LLC
# 
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
# 
#      http://www.apache.org/licenses/LICENSE-2.0
# 
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -e

# --- Configuration (from Environment Variables) ---
# TARGET_ROOT: The directory to search for tests (e.g., docs/en/getting-started/quickstart/js)
# TARGET_LANG: python, js, go
# TABLE_NAME: Database table name to use
# SQL_FILE: Path to the SQL setup file
# AGENT_FILE_PATTERN: Filename to look for (e.g., quickstart.js or agent.py)

VERSION=$(cat ./cmd/version.txt)

# Process IDs & Logs
PROXY_PID=""
TOOLBOX_PID=""
PROXY_LOG="cloud_sql_proxy.log"
TOOLBOX_LOG="toolbox_server.log"

install_system_packages() {
  echo "Installing system packages..."
  apt-get update && apt-get install -y \
    postgresql-client \
    wget \
    gettext-base  \
    netcat-openbsd
    
  if [[ "$TARGET_LANG" == "python" ]]; then
    apt-get install -y python3-venv
  fi
}

start_cloud_sql_proxy() {
  echo "Starting Cloud SQL Proxy..."
  wget -q "https://storage.googleapis.com/cloud-sql-connectors/cloud-sql-proxy/v2.10.0/cloud-sql-proxy.linux.amd64" -O /usr/local/bin/cloud-sql-proxy
  chmod +x /usr/local/bin/cloud-sql-proxy
  cloud-sql-proxy "${CLOUD_SQL_INSTANCE}" > "$PROXY_LOG" 2>&1 &
  PROXY_PID=$!

  # Health Check
  for i in {1..30}; do
    if nc -z 127.0.0.1 5432; then
      echo "Cloud SQL Proxy is up and running."
      return
    fi
    sleep 1
  done
  echo "ERROR: Cloud SQL Proxy failed to start. Logs:"
  cat "$PROXY_LOG"
  exit 1
}

setup_toolbox() {
  echo "Setting up Toolbox server..."
  TOOLBOX_YAML="/tools.yaml"
  echo "${TOOLS_YAML_CONTENT}" > "$TOOLBOX_YAML"
  wget -q "https://storage.googleapis.com/mcp-toolbox-for-databases/v${VERSION}/linux/amd64/toolbox" -O "/toolbox"
  chmod +x "/toolbox"
  /toolbox --tools-file "$TOOLBOX_YAML" > "$TOOLBOX_LOG" 2>&1 &
  TOOLBOX_PID=$!
  
  # Health Check
  for i in {1..15}; do
    if nc -z 127.0.0.1 5000; then
      echo "Toolbox server is up and running."
      return
    fi
    sleep 1
  done
  echo "ERROR: Toolbox server failed to start. Logs:"
  cat "$TOOLBOX_LOG"
  exit 1
}

setup_db_table() {
  echo "Setting up database table $TABLE_NAME using $SQL_FILE..."
  export TABLE_NAME
  envsubst < "$SQL_FILE" | psql -h 127.0.0.1 -p 5432 -U "$DB_USER" -d "$DATABASE_NAME"
}

run_python_test() {
  local dir=$1
  local name=$(basename "$dir")
  echo "--- Running Python Test: $name ---"
  (
    cd "$dir" || exit 1
    rm -rf .venv
    python3 -m venv .venv || exit 1
    source .venv/bin/activate || exit 1
    pip install -q -r requirements.txt pytest || exit 1
    
    cd .. || exit 1
    local test_file=$(find . -maxdepth 1 -name "*test.py" | head -n 1)
    local rc=0
    if [ -n "$test_file" ]; then
        echo "Found native test: $test_file. Running pytest..."
        export ORCH_NAME="$name"
        export PYTHONPATH="../"
        pytest "$test_file" || rc=$?
    else
        echo "No native test found. running agent directly..."
        export PYTHONPATH="../"
        python3 "${name}/${AGENT_FILE_PATTERN}" || rc=$?
    fi
    rm -rf "${name}/.venv"
    exit "$rc"
  )
}

run_js_test() {
  local dir=$1
  local name=$(basename "$dir")
  echo "--- Running JS Test: $name ---"
  (
    cd "$dir" || exit 1
    if [ -f "package-lock.json" ]; then npm ci -q || exit 1; else npm install -q || exit 1; fi
    
    cd .. || exit 1
    # Looking for a JS test file in the parent directory
    local test_file=$(find . -maxdepth 1 -name "*test.js" | head -n 1)
    local rc=0
    if [ -n "$test_file" ]; then
        echo "Found native test: $test_file. Running node --test..."
        export ORCH_NAME="$name"
        node --test "$test_file" || rc=$?
    else
        echo "No native test found. running agent directly..."
        node "${name}/${AGENT_FILE_PATTERN}" || rc=$?
    fi
    rm -rf "${name}/node_modules"
    exit "$rc"
  )
}

run_go_test() {
  local dir=$1
  local name=$(basename "$dir")

  # TODO: Re-enable 'langchain' Go test once tmc/langchaingo supports thought_signature on Gemini 3+ (#1464)
  if [ "$name" == "openAI" ] || [ "$name" == "langchain" ]; then
      echo -e "\nSkipping framework '${name}': Temporarily excluded."
      return 0
  fi

  echo "--- Running Go Test: $name ---"
  (
    cd "$dir" || exit 1
    if [ -f "go.mod" ]; then
      go mod tidy || exit 1
    fi
    
    cd .. || exit 1
    local test_file=$(find . -maxdepth 1 -name "*test.go" | head -n 1)
    local rc=0
    if [ -n "$test_file" ]; then
        echo "Found native test: $test_file. Running go test..."
        export ORCH_NAME="$name"
        export SAMPLE_TEST_MAX_ATTEMPTS=1
        go test -v ./... || rc=$?
    else
        echo "No native test found. running agent directly..."
        cd "$name" || exit 1
        go run "." || rc=$?
    fi
    exit "$rc"
  )
}

is_transient_llm_error() {
  local log_file=$1
  grep -qiE '(Error 503|503 Service Unavailable|Status: UNAVAILABLE|"status":[[:space:]]*"UNAVAILABLE"|experiencing high demand|Error 429|429 Too Many Requests|RESOURCE_EXHAUSTED|overloaded)' "$log_file"
}

run_with_retry() {
  local max_attempts=3
  local delay=10
  local attempt=1
  local log_file
  log_file=$(mktemp)

  while [ "$attempt" -le "$max_attempts" ]; do
    local rc=0
    set +e
    "$@" 2>&1 | tee "$log_file"
    rc=${PIPESTATUS[0]}
    set -e

    if [ "$rc" -eq 0 ]; then
      rm -f "$log_file"
      return 0
    fi

    if [ "$attempt" -lt "$max_attempts" ] && is_transient_llm_error "$log_file"; then
      echo "WARN: Transient upstream LLM error detected (attempt ${attempt}/${max_attempts}). Resetting DB table and retrying in ${delay}s..."
      setup_db_table
      sleep "$delay"
      delay=$((delay * 2))
      attempt=$((attempt + 1))
      : > "$log_file"
      continue
    fi

    rm -f "$log_file"
    return "$rc"
  done
}

cleanup() {
  echo "Cleaning up background processes..."
  [ -n "$TOOLBOX_PID" ] && kill "$TOOLBOX_PID" || true
  [ -n "$PROXY_PID" ] && kill "$PROXY_PID" || true
}
trap cleanup EXIT

# --- Execution ---
install_system_packages
start_cloud_sql_proxy

export PGHOST=127.0.0.1
export PGPORT=5432
export PGPASSWORD="$DB_PASSWORD"
export GOOGLE_API_KEY="$GOOGLE_API_KEY"

setup_toolbox
setup_db_table

echo "Scanning $TARGET_ROOT for tests with pattern $AGENT_FILE_PATTERN..."

while read -r agent_file; do
    sample_dir=$(dirname "$agent_file")
    if [[ "$TARGET_LANG" == "python" ]]; then
        run_with_retry run_python_test "$sample_dir"
    elif [[ "$TARGET_LANG" == "js" ]]; then
        run_with_retry run_js_test "$sample_dir"
    elif [[ "$TARGET_LANG" == "go" ]]; then
        run_with_retry run_go_test "$sample_dir"
    fi
done < <(find "$TARGET_ROOT" -name "$AGENT_FILE_PATTERN" | sort)
