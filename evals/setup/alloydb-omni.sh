#!/bin/bash
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

# Dispatched by evals/setup/dispatch.sh when TOOLBOX_PREBUILT=alloydb-omni.
# Waits for the AlloyDB Omni instance to be ready, ensures the evaluation
# database exists, and installs the columnar engine extension.

set -euo pipefail

PYTHON_BIN="/evalbench/.venv/bin/python"
if [ ! -x "$PYTHON_BIN" ]; then
  PYTHON_BIN="$(command -v python3 || command -v python)"
fi

echo "Running AlloyDB Omni eval setup via ${PYTHON_BIN}..."

"$PYTHON_BIN" - << 'PYEOF'
import os
import sys
import time
import psycopg2

host = os.environ.get("ALLOYDB_OMNI_HOST", "alloydb-omni")
port = int(os.environ.get("ALLOYDB_OMNI_PORT", "5432"))
user = os.environ.get("ALLOYDB_OMNI_USER", "postgres")
password = os.environ.get("ALLOYDB_OMNI_PASSWORD", "")
database = os.environ.get("ALLOYDB_OMNI_DATABASE", "test_database")

print(f"Waiting for AlloyDB Omni instance at {host}:{port} (user={user})...")
start_time = time.time()
conn = None
max_wait_seconds = 90

while time.time() - start_time < max_wait_seconds:
    try:
        conn = psycopg2.connect(
            host=host,
            port=port,
            user=user,
            password=password,
            dbname="postgres",
            connect_timeout=3,
        )
        print("Connected to AlloyDB Omni PostgreSQL instance.")
        break
    except Exception as e:
        time.sleep(2)

if not conn:
    print(f"ERROR: Timed out waiting for AlloyDB Omni at {host}:{port} after {max_wait_seconds}s", file=sys.stderr)
    sys.exit(1)

conn.autocommit = True
cur = conn.cursor()

# Ensure target evaluation database exists
cur.execute("SELECT 1 FROM pg_database WHERE datname = %s;", (database,))
if not cur.fetchone():
    print(f"Creating database {database}...")
    cur.execute(f'CREATE DATABASE "{database}";')
conn.close()

# Connect to target database to initialize columnar engine extension
print(f"Connecting to {database} to initialize extensions...")
db_conn = psycopg2.connect(
    host=host,
    port=port,
    user=user,
    password=password,
    dbname=database,
)
db_conn.autocommit = True
db_cur = db_conn.cursor()

try:
    db_cur.execute("CREATE EXTENSION IF NOT EXISTS google_columnar_engine;")
    print("Extension google_columnar_engine installed/verified successfully.")
except Exception as e:
    print(f"Note on google_columnar_engine extension: {e}")

db_cur.execute("SELECT version();")
ver = db_cur.fetchone()[0]
print(f"Database ready: {ver}")

db_conn.close()
print("AlloyDB Omni setup completed successfully.")
PYEOF
