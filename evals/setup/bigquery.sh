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

# Seeds the dataset the bigquery evalset queries: the project has no stable
# BigQuery fixture, so the scenarios would otherwise have nothing deterministic
# to read. The name comes from EVAL_BQ_DATASET (.ci/run_evals.sh), which the
# evalset references too.
#
# Idempotent: run_evals.sh invokes EvalBench once per harness, and EvalBench
# runs this once per invocation.

set -euo pipefail

: "${BIGQUERY_PROJECT:?}" "${BIGQUERY_LOCATION:?}" "${EVAL_BQ_DATASET:?}"

echo "seeding ${BIGQUERY_PROJECT}.${EVAL_BQ_DATASET} in ${BIGQUERY_LOCATION}"

# Python rather than bq: bq is a separate gcloud component the EvalBench image
# need not carry, while google-cloud-bigquery is already present.
uv run --no-sync python - <<'PY'
import os

from google.cloud import bigquery

project = os.environ["BIGQUERY_PROJECT"]
location = os.environ["BIGQUERY_LOCATION"]
dataset_id = f"{project}.{os.environ['EVAL_BQ_DATASET']}"

# location on the client too: a query job that does not carry one defaults to
# US, so the CREATE below would miss a dataset seeded anywhere else.
client = bigquery.Client(project=project, location=location)
dataset = bigquery.Dataset(dataset_id)
dataset.location = location
# Bounds the storage a build killed before teardown leaves behind, until the
# sweep collects the dataset itself.
dataset.default_table_expiration_ms = 24 * 60 * 60 * 1000
client.create_dataset(dataset, exists_ok=True)

# globex is the top customer by total amount whether or not the agent filters
# on status, so the expected answer does not turn on how it reads the prompt.
client.query(
    f"""
    CREATE OR REPLACE TABLE `{dataset_id}.orders` AS
    SELECT * FROM UNNEST([
      STRUCT(1 AS order_id, 'acme' AS customer, NUMERIC '120.50' AS amount,
             'shipped' AS status, TIMESTAMP '2026-01-05 09:00:00' AS ordered_at),
      STRUCT(2, 'globex', NUMERIC '400.00', 'shipped',   TIMESTAMP '2026-01-06 11:30:00'),
      STRUCT(3, 'acme',   NUMERIC '45.25',  'pending',   TIMESTAMP '2026-01-07 14:15:00'),
      STRUCT(4, 'initech',NUMERIC '300.00', 'shipped',   TIMESTAMP '2026-01-08 08:45:00'),
      STRUCT(5, 'globex', NUMERIC '210.75', 'cancelled', TIMESTAMP '2026-01-09 16:20:00'),
      STRUCT(6, 'acme',   NUMERIC '15.00',  'shipped',   TIMESTAMP '2026-01-10 10:05:00'),
      STRUCT(7, 'initech',NUMERIC '60.00',  'pending',   TIMESTAMP '2026-01-11 13:40:00'),
      STRUCT(8, 'globex', NUMERIC '95.00',  'shipped',   TIMESTAMP '2026-01-12 17:55:00')
    ])
    """
).result()

# 90 daily points so AI.FORECAST has real history to fit; the sine term keeps it
# from being a straight line.
client.query(
    f"""
    CREATE OR REPLACE TABLE `{dataset_id}.daily_revenue` AS
    SELECT
      TIMESTAMP(day) AS day,
      ROUND(1000 + 12 * off + 150 * SIN(off / 7 * 2 * ACOS(-1)), 2) AS revenue
    FROM UNNEST(GENERATE_DATE_ARRAY(DATE '2026-01-01', DATE '2026-03-31')) AS day
    WITH OFFSET AS off
    """
).result()

# FARM_FINGERPRINT rather than RAND so the noise is identical every build. The
# planted drop is paid_search in US; nothing else moves.
#
# day is not projected: CONTRIBUTION_ANALYSIS rejects any column that is not the
# metric, a dimension, or the test flag.
client.query(
    f"""
    CREATE OR REPLACE TABLE `{dataset_id}.signups` AS
    SELECT
      channel,
      country,
      day >= DATE '2026-02-01' AS is_test,
      20
        + MOD(ABS(FARM_FINGERPRINT(FORMAT('%t|%s|%s', day, channel, country))), 5)
        - IF(day >= DATE '2026-02-01' AND channel = 'paid_search' AND country = 'US', 15, 0)
        AS signups
    FROM UNNEST(GENERATE_DATE_ARRAY(DATE '2026-01-01', DATE '2026-02-28')) AS day
    CROSS JOIN UNNEST(['paid_search', 'organic', 'referral']) AS channel
    CROSS JOIN UNNEST(['US', 'CA', 'DE']) AS country
    """
).result()
PY
