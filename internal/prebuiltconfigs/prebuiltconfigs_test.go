// Copyright 2024 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package prebuiltconfigs

import (
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"
)

var expectedToolSources = []string{
	"alloydb-omni",
	"alloydb-postgres-admin",
	"alloydb-postgres-observability",
	"alloydb-postgres",
	"conversational-analytics-with-data-agent",
	"bigquery",
	"clickhouse",
	"cloud-healthcare",
	"cloud-storage",
	"cloud-sql-mssql-admin",
	"cloud-sql-mssql-observability",
	"cloud-sql-mssql",
	"cloud-sql-mysql-admin",
	"cloud-sql-mysql-observability",
	"cloud-sql-mysql",
	"cloud-sql-postgres-admin",
	"cloud-sql-postgres-observability",
	"cloud-sql-postgres",
	"dataplex",
	"dataproc",
	"falkordb",
	"firestore",
	"elasticsearch",
	"looker-conversational-analytics",
	"looker-dev",
	"looker",
	"mindsdb",
	"mssql",
	"mysql",
	"neo4j",
	"oceanbase",
	"oracledb",
	"postgres",
	"serverless-spark",
	"singlestore",
	"snowflake",
	"spanner-postgres",
	"spanner",
	"sqlite",
}

func TestGetPrebuiltSources(t *testing.T) {
	t.Run("Test Get Prebuilt Sources", func(t *testing.T) {
		sources := GetPrebuiltSources()
		slices.Sort(expectedToolSources)
		slices.Sort(sources)
		if diff := cmp.Diff(expectedToolSources, sources); diff != "" {
			t.Fatalf("incorrect sources parse: diff %v", diff)
		}

	})
}

func TestLoadPrebuiltToolYAMLs(t *testing.T) {
	test_name := "test load prebuilt configs"
	expectedKeys := expectedToolSources
	t.Run(test_name, func(t *testing.T) {
		configsMap, keys, err := loadPrebuiltToolYAMLs()
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		foundExpectedKeys := make(map[string]bool)

		if len(expectedKeys) != len(configsMap) {
			t.Fatalf("Failed to load all prebuilt tools.")
		}

		for _, expectedKey := range expectedKeys {
			_, ok := configsMap[expectedKey]
			if !ok {
				t.Fatalf("Prebuilt tools for '%s' was NOT FOUND in the loaded map.", expectedKey)
			} else {
				foundExpectedKeys[expectedKey] = true // Mark as found
			}
		}

		t.Log(expectedKeys)
		t.Log(keys)

		slices.Sort(expectedKeys)
		slices.Sort(keys)
		if diff := cmp.Diff(expectedKeys, keys); diff != "" {
			t.Fatalf("incorrect sources parse: diff %v", diff)
		}

	})
}

func TestGetPrebuiltTool(t *testing.T) {
	alloydb_omni_config := getOrFatal(t, "alloydb-omni")
	alloydb_admin_config := getOrFatal(t, "alloydb-postgres-admin")
	alloydb_observability_config := getOrFatal(t, "alloydb-postgres-observability")
	alloydb_config := getOrFatal(t, "alloydb-postgres")
	bigquery_config := getOrFatal(t, "bigquery")
	conversational_analytics_config := getOrFatal(t, "conversational-analytics-with-data-agent")
	clickhouse_config := getOrFatal(t, "clickhouse")
	cloudsqlpg_observability_config := getOrFatal(t, "cloud-sql-postgres-observability")
	cloudsqlpg_config := getOrFatal(t, "cloud-sql-postgres")
	cloudsqlpg_admin_config := getOrFatal(t, "cloud-sql-postgres-admin")
	cloudsqlmysql_admin_config := getOrFatal(t, "cloud-sql-mysql-admin")
	cloudsqlmssql_admin_config := getOrFatal(t, "cloud-sql-mssql-admin")
	cloudsqlmysql_observability_config := getOrFatal(t, "cloud-sql-mysql-observability")
	cloudsqlmysql_config := getOrFatal(t, "cloud-sql-mysql")
	cloudsqlmssql_observability_config := getOrFatal(t, "cloud-sql-mssql-observability")
	cloudsqlmssql_config := getOrFatal(t, "cloud-sql-mssql")
	dataplex_config := getOrFatal(t, "dataplex")
	falkordb_config := getOrFatal(t, "falkordb")
	firestoreconfig := getOrFatal(t, "firestore")
	looker_config := getOrFatal(t, "looker")
	lookerca_config := getOrFatal(t, "looker-conversational-analytics")
	mysql_config := getOrFatal(t, "mysql")
	mssql_config := getOrFatal(t, "mssql")
	oceanbase_config := getOrFatal(t, "oceanbase")
	postgresconfig := getOrFatal(t, "postgres")
	singlestore_config := getOrFatal(t, "singlestore")
	serverlessspark_config := getOrFatal(t, "serverless-spark")
	spanner_config := getOrFatal(t, "spanner")
	spannerpg_config := getOrFatal(t, "spanner-postgres")
	mindsdb_config := getOrFatal(t, "mindsdb")
	sqlite_config := getOrFatal(t, "sqlite")
	neo4jconfig := getOrFatal(t, "neo4j")
	oracle_config := getOrFatal(t, "oracledb")
	healthcare_config := getOrFatal(t, "cloud-healthcare")
	cloudstorage_config := getOrFatal(t, "cloud-storage")
	snowflake_config := getOrFatal(t, "snowflake")
	if len(alloydb_omni_config) <= 0 {
		t.Fatalf("unexpected error: could not fetch alloydb omni prebuilt tools yaml")
	}
	if len(alloydb_admin_config) <= 0 {
		t.Fatalf("unexpected error: could not fetch alloydb admin prebuilt tools yaml")
	}
	if len(alloydb_config) <= 0 {
		t.Fatalf("unexpected error: could not fetch alloydb prebuilt tools yaml")
	}
	if len(alloydb_observability_config) <= 0 {
		t.Fatalf("unexpected error: could not fetch alloydb-observability prebuilt tools yaml")
	}
	if len(bigquery_config) <= 0 {
		t.Fatalf("unexpected error: could not fetch bigquery prebuilt tools yaml")
	}
	if len(conversational_analytics_config) <= 0 {
		t.Fatalf("unexpected error: could not fetch bigquery conversational analytics prebuilt tools yaml")
	}
	if len(clickhouse_config) <= 0 {
		t.Fatalf("unexpected error: could not fetch clickhouse prebuilt tools yaml")
	}
	if len(cloudsqlpg_observability_config) <= 0 {
		t.Fatalf("unexpected error: could not fetch cloud sql pg observability prebuilt tools yaml")
	}
	if len(cloudsqlpg_config) <= 0 {
		t.Fatalf("unexpected error: could not fetch cloud sql pg prebuilt tools yaml")
	}
	if len(cloudsqlpg_admin_config) <= 0 {
		t.Fatalf("unexpected error: could not fetch cloud sql pg admin prebuilt tools yaml")
	}
	if len(cloudsqlmysql_admin_config) <= 0 {
		t.Fatalf("unexpected error: could not fetch cloud sql mysql admin prebuilt tools yaml")
	}
	if len(cloudsqlmysql_observability_config) <= 0 {
		t.Fatalf("unexpected error: could not fetch cloud sql mysql observability prebuilt tools yaml")
	}
	if len(cloudsqlmysql_config) <= 0 {
		t.Fatalf("unexpected error: could not fetch cloud sql mysql prebuilt tools yaml")
	}
	if len(cloudsqlmssql_observability_config) <= 0 {
		t.Fatalf("unexpected error: could not fetch cloud sql mssql observability prebuilt tools yaml")
	}
	if len(cloudsqlmssql_admin_config) <= 0 {
		t.Fatalf("unexpected error: could not fetch cloud sql mssql admin prebuilt tools yaml")
	}
	if len(cloudsqlmssql_config) <= 0 {
		t.Fatalf("unexpected error: could not fetch cloud sql mssql prebuilt tools yaml")
	}
	if len(dataplex_config) <= 0 {
		t.Fatalf("unexpected error: could not fetch dataplex prebuilt tools yaml")
	}
	if len(falkordb_config) <= 0 {
		t.Fatalf("unexpected error: could not fetch falkordb prebuilt tools yaml")
	}
	if len(firestoreconfig) <= 0 {
		t.Fatalf("unexpected error: could not fetch firestore prebuilt tools yaml")
	}
	if len(looker_config) <= 0 {
		t.Fatalf("unexpected error: could not fetch looker prebuilt tools yaml")
	}
	if len(lookerca_config) <= 0 {
		t.Fatalf("unexpected error: could not fetch looker-conversational-analytics prebuilt tools yaml")
	}
	if len(mysql_config) <= 0 {
		t.Fatalf("unexpected error: could not fetch mysql prebuilt tools yaml")
	}
	if len(mssql_config) <= 0 {
		t.Fatalf("unexpected error: could not fetch mssql prebuilt tools yaml")
	}
	if len(oceanbase_config) <= 0 {
		t.Fatalf("unexpected error: could not fetch oceanbase prebuilt tools yaml")
	}
	if len(postgresconfig) <= 0 {
		t.Fatalf("unexpected error: could not fetch postgres prebuilt tools yaml")
	}
	if len(singlestore_config) <= 0 {
		t.Fatalf("unexpected error: could not fetch singlestore prebuilt tools yaml")
	}
	if len(serverlessspark_config) <= 0 {
		t.Fatalf("unexpected error: could not fetch serverless spark prebuilt tools yaml")
	}
	if len(snowflake_config) <= 0 {
		t.Fatalf("unexpected error: could not fetch snowflake prebuilt tools yaml")
	}
	if len(spanner_config) <= 0 {
		t.Fatalf("unexpected error: could not fetch spanner prebuilt tools yaml")
	}
	if len(spannerpg_config) <= 0 {
		t.Fatalf("unexpected error: could not fetch spanner pg prebuilt tools yaml")
	}

	if len(mindsdb_config) <= 0 {
		t.Fatalf("unexpected error: could not fetch spanner pg prebuilt tools yaml")
	}

	if len(sqlite_config) <= 0 {
		t.Fatalf("unexpected error: could not fetch sqlite prebuilt tools yaml")
	}

	if len(neo4jconfig) <= 0 {
		t.Fatalf("unexpected error: could not fetch neo4j prebuilt tools yaml")
	}
	if len(healthcare_config) <= 0 {
		t.Fatalf("unexpected error: could not fetch healthcare prebuilt tools yaml")
	}
	if len(cloudstorage_config) <= 0 {
		t.Fatalf("unexpected error: could not fetch cloud-storage prebuilt tools yaml")
	}
	if len(snowflake_config) <= 0 {
		t.Fatalf("unexpected error: could not fetch snowflake prebuilt tools yaml")
	}
	if len(oracle_config) <= 0 {
		t.Fatalf("unexpected error: could not fetch oracle prebuilt tools yaml")
	}
}

func TestFailGetPrebuiltTool(t *testing.T) {
	_, err := Get("sql")
	if err == nil {
		t.Fatalf("unexpected an error but got nil.")
	}
}

func getOrFatal(t *testing.T, prebuiltSourceConfig string) []byte {
	bytes, err := Get(prebuiltSourceConfig)
	if err != nil {
		t.Fatalf("Cannot get prebuilt config for %q, error %v", prebuiltSourceConfig, err)
	}
	return bytes
}
