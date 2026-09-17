// Copyright 2025 Google LLC
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

package bigquerycommon_test

import (
	"sort"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/tools/bigquery/bigquerycommon"
)

func TestTableParser(t *testing.T) {
	testCases := []struct {
		name             string
		sql              string
		defaultProjectID string
		want             []string
		wantErr          bool
		wantErrMsg       string
	}{
		{
			name:             "single fully qualified table",
			sql:              "SELECT * FROM `my-project.my_dataset.my_table`",
			defaultProjectID: "default-proj",
			want:             []string{"my-project.my_dataset.my_table"},
			wantErr:          false,
		},
		{
			name:             "multiple statements with same table",
			sql:              "select * from proj1.data1.tbl1 limit 1; select A.b from proj1.data1.tbl1 as A limit 1;",
			defaultProjectID: "default-proj",
			want:             []string{"proj1.data1.tbl1"},
			wantErr:          false,
		},
		{
			name:             "multiple fully qualified tables",
			sql:              "SELECT * FROM `proj1.data1`.`tbl1` JOIN proj2.`data2.tbl2` ON id",
			defaultProjectID: "default-proj",
			want:             []string{"proj1.data1.tbl1", "proj2.data2.tbl2"},
			wantErr:          false,
		},
		{
			name:             "duplicate tables",
			sql:              "SELECT * FROM `proj1.data1.tbl1` JOIN proj1.data1.tbl1 ON id",
			defaultProjectID: "default-proj",
			want:             []string{"proj1.data1.tbl1"},
			wantErr:          false,
		},
		{
			name:             "partial table with default project",
			sql:              "SELECT * FROM `my_dataset`.my_table",
			defaultProjectID: "default-proj",
			want:             []string{"default-proj.my_dataset.my_table"},
			wantErr:          false,
		},
		{
			name:             "partial table without default project",
			sql:              "SELECT * FROM `my_dataset.my_table`",
			defaultProjectID: "",
			want:             nil,
			wantErr:          true,
		},
		{
			name:             "mixed fully qualified and partial tables",
			sql:              "SELECT t1.*, t2.* FROM `proj1.data1.tbl1` AS t1 JOIN `data2.tbl2` AS t2 ON t1.id = t2.id",
			defaultProjectID: "default-proj",
			want:             []string{"proj1.data1.tbl1", "default-proj.data2.tbl2"},
			wantErr:          false,
		},
		{
			name:             "no tables",
			sql:              "SELECT 1+1",
			defaultProjectID: "default-proj",
			want:             []string{},
			wantErr:          false,
		},
		{
			name:             "ignore single part identifiers (like CTEs)",
			sql:              "WITH my_cte AS (SELECT 1) SELECT * FROM `my_cte`",
			defaultProjectID: "default-proj",
			want:             []string{},
			wantErr:          false,
		},
		{
			name:             "complex CTE",
			sql:              "WITH cte1 AS (SELECT * FROM `real.table.one`), cte2 AS (SELECT * FROM cte1) SELECT * FROM cte2 JOIN `real.table.two` ON true",
			defaultProjectID: "default-proj",
			want:             []string{"real.table.one", "real.table.two"},
			wantErr:          false,
		},
		{
			name:             "nested subquery should be parsed",
			sql:              "SELECT * FROM (SELECT a FROM (SELECT A.b FROM `real.table.nested` AS A))",
			defaultProjectID: "default-proj",
			want:             []string{"real.table.nested"},
			wantErr:          false,
		},
		{
			name:             "from clause with unnest",
			sql:              "SELECT event.name FROM `my-project.my_dataset.my_table` AS A, UNNEST(A.events) AS event",
			defaultProjectID: "default-proj",
			want:             []string{"my-project.my_dataset.my_table"},
			wantErr:          false,
		},
		{
			name:             "ignore more than 3 parts",
			sql:              "SELECT * FROM `proj.data.tbl.col`",
			defaultProjectID: "default-proj",
			want:             []string{},
			wantErr:          false,
		},
		{
			name:             "complex query",
			sql:              "SELECT name FROM (SELECT name FROM `proj1.data1.tbl1`) UNION ALL SELECT name FROM `data2.tbl2`",
			defaultProjectID: "default-proj",
			want:             []string{"proj1.data1.tbl1", "default-proj.data2.tbl2"},
			wantErr:          false,
		},
		{
			name:             "empty sql",
			sql:              "",
			defaultProjectID: "default-proj",
			want:             []string{},
			wantErr:          false,
		},
		{
			name:             "with comments",
			sql:              "SELECT * FROM `proj1.data1.tbl1`; -- comment `fake.table.one` \n SELECT * FROM `proj2.data2.tbl2`; # comment `fake.table.two`",
			defaultProjectID: "default-proj",
			want:             []string{"proj1.data1.tbl1", "proj2.data2.tbl2"},
			wantErr:          false,
		},
		{
			name:             "model as column name in where clause",
			sql:              "SELECT * FROM `proj.data.tbl` WHERE model = 'v1' AND status = 'active'",
			defaultProjectID: "default-proj",
			want:             []string{"proj.data.tbl"},
			wantErr:          false,
		},
		{
			name:             "AI.FORECAST function call",
			sql:              "SELECT * FROM AI.FORECAST(TABLE `project.dataset.table`, data_col => 'val')",
			defaultProjectID: "my-project",
			want:             []string{"project.dataset.table"},
			wantErr:          false,
		},
		{
			name:             "ML.GET_INSIGHTS function call",
			sql:              "SELECT * FROM ML.GET_INSIGHTS(MODEL `project.dataset.model`)",
			defaultProjectID: "my-project",
			want:             []string{"project.dataset.model"},
			wantErr:          false,
		},
		{
			name:             "multi-statement with semicolon",
			sql:              "SELECT * FROM `proj1.data1.tbl1`; SELECT * FROM `proj2.data2.tbl2`",
			defaultProjectID: "default-proj",
			want:             []string{"proj1.data1.tbl1", "proj2.data2.tbl2"},
			wantErr:          false,
		},
		{
			name:             "simple execute immediate",
			sql:              "EXECUTE IMMEDIATE 'SELECT * FROM `exec.proj.tbl`'",
			defaultProjectID: "default-proj",
			want:             nil,
			wantErr:          true,
			wantErrMsg:       "EXECUTE IMMEDIATE is not allowed when dataset restrictions are in place",
		},
		{
			name:             "execute immediate with multiple spaces",
			sql:              "EXECUTE  IMMEDIATE 'SELECT 1'",
			defaultProjectID: "default-proj",
			want:             nil,
			wantErr:          true,
			wantErrMsg:       "EXECUTE IMMEDIATE is not allowed when dataset restrictions are in place",
		},
		{
			name:             "execute immediate with newline",
			sql:              "EXECUTE\nIMMEDIATE 'SELECT 1'",
			defaultProjectID: "default-proj",
			want:             nil,
			wantErr:          true,
			wantErrMsg:       "EXECUTE IMMEDIATE is not allowed when dataset restrictions are in place",
		},
		{
			name:             "execute immediate with comment",
			sql:              "EXECUTE -- some comment\n IMMEDIATE 'SELECT * FROM `exec.proj.tbl`'",
			defaultProjectID: "default-proj",
			want:             nil,
			wantErr:          true,
			wantErrMsg:       "EXECUTE IMMEDIATE is not allowed when dataset restrictions are in place",
		},
		{
			name:             "nested execute immediate",
			sql:              "EXECUTE IMMEDIATE \"EXECUTE IMMEDIATE '''SELECT * FROM `nested.exec.tbl`'''\"",
			defaultProjectID: "default-proj",
			want:             nil,
			wantErr:          true,
			wantErrMsg:       "EXECUTE IMMEDIATE is not allowed when dataset restrictions are in place",
		},
		{
			name:             "begin execute immediate",
			sql:              "BEGIN EXECUTE IMMEDIATE 'SELECT * FROM `exec.proj.tbl`'; END;",
			defaultProjectID: "default-proj",
			want:             nil,
			wantErr:          true,
			wantErrMsg:       "EXECUTE IMMEDIATE is not allowed when dataset restrictions are in place",
		},
		{
			name:             "table inside string literal should be ignored",
			sql:              "SELECT * FROM `real.table.one` WHERE name = 'select * from `fake.table.two`'",
			defaultProjectID: "default-proj",
			want:             []string{"real.table.one"},
			wantErr:          false,
		},
		{
			name:             "string with escaped single quote",
			sql:              "SELECT 'this is a string with an escaped quote \\' and a fake table `fake.table.one`' FROM `real.table.two`",
			defaultProjectID: "default-proj",
			want:             []string{"real.table.two"},
			wantErr:          false,
		},
		{
			name:             "string with escaped double quote",
			sql:              `SELECT "this is a string with an escaped quote \" and a fake table ` + "`fake.table.one`" + `" FROM ` + "`real.table.two`",
			defaultProjectID: "default-proj",
			want:             []string{"real.table.two"},
			wantErr:          false,
		},
		{
			name:             "multi-line comment",
			sql:              "/* `fake.table.1` */ SELECT * FROM `real.table.2`",
			defaultProjectID: "default-proj",
			want:             []string{"real.table.2"},
			wantErr:          false,
		},
		{
			name:             "raw string with backslash should be ignored",
			sql:              "SELECT * FROM `real.table.one` WHERE name = r'a raw string with a \\ and a fake table `fake.table.two`'",
			defaultProjectID: "default-proj",
			want:             []string{"real.table.one"},
			wantErr:          false,
		},
		{
			name:             "capital R raw string with quotes inside should be ignored",
			sql:              `SELECT * FROM ` + "`real.table.one`" + ` WHERE name = R"""a raw string with a ' and a " and a \ and a fake table ` + "`fake.table.two`" + `"""`,
			defaultProjectID: "default-proj",
			want:             []string{"real.table.one"},
			wantErr:          false,
		},
		{
			name:             "triple quoted raw string should be ignored",
			sql:              "SELECT * FROM `real.table.one` WHERE name = r'''a raw string with a ' and a \" and a \\ and a fake table `fake.table.two`'''",
			defaultProjectID: "default-proj",
			want:             []string{"real.table.one"},
			wantErr:          false,
		},
		{
			name:             "triple quoted capital R raw string should be ignored",
			sql:              `SELECT * FROM ` + "`real.table.one`" + ` WHERE name = R"""a raw string with a ' and a " and a \ and a fake table ` + "`fake.table.two`" + `"""`,
			defaultProjectID: "default-proj",
			want:             []string{"real.table.one"},
			wantErr:          false,
		},
		{
			name:             "unquoted fully qualified table",
			sql:              "SELECT * FROM my-project.my_dataset.my_table",
			defaultProjectID: "default-proj",
			want:             []string{"my-project.my_dataset.my_table"},
			wantErr:          false,
		},
		{
			name:             "unquoted partial table with default project",
			sql:              "SELECT * FROM my_dataset.my_table",
			defaultProjectID: "default-proj",
			want:             []string{"default-proj.my_dataset.my_table"},
			wantErr:          false,
		},
		{
			name:             "unquoted partial table without default project",
			sql:              "SELECT * FROM my_dataset.my_table",
			defaultProjectID: "",
			want:             nil,
			wantErr:          true,
		},
		{
			name:             "mixed quoting style 1",
			sql:              "SELECT * FROM `my-project`.my_dataset.my_table",
			defaultProjectID: "default-proj",
			want:             []string{"my-project.my_dataset.my_table"},
			wantErr:          false,
		},
		{
			name:             "mixed quoting style 2",
			sql:              "SELECT * FROM `my-project`.`my_dataset`.my_table",
			defaultProjectID: "default-proj",
			want:             []string{"my-project.my_dataset.my_table"},
			wantErr:          false,
		},
		{
			name:             "mixed quoting style 3",
			sql:              "SELECT * FROM `my-project`.`my_dataset`.`my_table`",
			defaultProjectID: "default-proj",
			want:             []string{"my-project.my_dataset.my_table"},
			wantErr:          false,
		},
		{
			name:             "mixed quoted and unquoted tables",
			sql:              "SELECT * FROM `proj1.data1.tbl1` JOIN proj2.data2.tbl2 ON id",
			defaultProjectID: "default-proj",
			want:             []string{"proj1.data1.tbl1", "proj2.data2.tbl2"},
			wantErr:          false,
		},
		{
			name:             "create table statement",
			sql:              "CREATE TABLE `my-project.my_dataset.my_table` (x INT64)",
			defaultProjectID: "default-proj",
			want:             []string{"my-project.my_dataset.my_table"},
			wantErr:          false,
		},
		{
			name:             "insert into statement",
			sql:              "INSERT INTO `my-project.my_dataset.my_table` (x) VALUES (1)",
			defaultProjectID: "default-proj",
			want:             []string{"my-project.my_dataset.my_table"},
			wantErr:          false,
		},
		{
			name:             "update statement",
			sql:              "UPDATE `my-project.my_dataset.my_table` SET x = 2 WHERE true",
			defaultProjectID: "default-proj",
			want:             []string{"my-project.my_dataset.my_table"},
			wantErr:          false,
		},
		{
			name:             "delete from statement",
			sql:              "DELETE FROM `my-project.my_dataset.my_table` WHERE true",
			defaultProjectID: "default-proj",
			want:             []string{"my-project.my_dataset.my_table"},
			wantErr:          false,
		},
		{
			name:             "merge into statement",
			sql:              "MERGE `proj.data.target` T USING `proj.data.source` S ON T.id = S.id WHEN NOT MATCHED THEN INSERT ROW",
			defaultProjectID: "default-proj",
			want:             []string{"proj.data.source", "proj.data.target"},
			wantErr:          false,
		},
		{
			name:             "create schema statement",
			sql:              "CREATE SCHEMA `my-project.my_dataset`",
			defaultProjectID: "default-proj",
			want:             nil,
			wantErr:          true,
			wantErrMsg:       "dataset-level operations like 'CREATE SCHEMA' are not allowed",
		},
		{
			name:             "create dataset statement",
			sql:              "CREATE DATASET `my-project.my_dataset`",
			defaultProjectID: "default-proj",
			want:             nil,
			wantErr:          true,
			wantErrMsg:       "dataset-level operations like 'CREATE DATASET' are not allowed",
		},
		{
			name:             "drop schema statement",
			sql:              "DROP SCHEMA `my-project.my_dataset`",
			defaultProjectID: "default-proj",
			want:             nil,
			wantErr:          true,
			wantErrMsg:       "dataset-level operations like 'DROP SCHEMA' are not allowed",
		},
		{
			name:             "drop dataset statement",
			sql:              "DROP DATASET `my-project.my_dataset`",
			defaultProjectID: "default-proj",
			want:             nil,
			wantErr:          true,
			wantErrMsg:       "dataset-level operations like 'DROP DATASET' are not allowed",
		},
		{
			name:             "alter schema statement",
			sql:              "ALTER SCHEMA my_dataset SET OPTIONS(description='new description')",
			defaultProjectID: "default-proj",
			want:             nil,
			wantErr:          true,
			wantErrMsg:       "dataset-level operations like 'ALTER SCHEMA' are not allowed",
		},
		{
			name:             "alter dataset statement",
			sql:              "ALTER DATASET my_dataset SET OPTIONS(description='new description')",
			defaultProjectID: "default-proj",
			want:             nil,
			wantErr:          true,
			wantErrMsg:       "dataset-level operations like 'ALTER DATASET' are not allowed",
		},
		{
			name:             "begin...end block",
			sql:              "BEGIN CREATE TABLE `proj.data.tbl1` (x INT64); INSERT `proj.data.tbl2` (y) VALUES (1); END;",
			defaultProjectID: "default-proj",
			want:             []string{"proj.data.tbl1", "proj.data.tbl2"},
			wantErr:          false,
		},
		{
			name: "complex begin...end block with comments and different quoting",
			sql: `
				BEGIN
					-- Create a new table
					CREATE TABLE proj.data.tbl1 (x INT64);
					/* Insert some data from another table */
					INSERT INTO ` + "`proj.data.tbl2`" + ` (y) SELECT y FROM proj.data.source;
				END;`,
			defaultProjectID: "default-proj",
			want:             []string{"proj.data.source", "proj.data.tbl1", "proj.data.tbl2"},
			wantErr:          false,
		},
		{
			name:             "call fully qualified procedure",
			sql:              "CALL my-project.my_dataset.my_procedure()",
			defaultProjectID: "default-proj",
			want:             nil,
			wantErr:          true,
			wantErrMsg:       "CALL is not allowed when dataset restrictions are in place",
		},
		{
			name:             "call partially qualified procedure",
			sql:              "CALL my_dataset.my_procedure()",
			defaultProjectID: "default-proj",
			want:             nil,
			wantErr:          true,
			wantErrMsg:       "CALL is not allowed when dataset restrictions are in place",
		},
		{
			name:             "call procedure in begin...end block",
			sql:              "BEGIN CALL proj.data.proc1(); SELECT * FROM proj.data.tbl1; END;",
			defaultProjectID: "default-proj",
			want:             nil,
			wantErr:          true,
			wantErrMsg:       "CALL is not allowed when dataset restrictions are in place",
		},
		{
			name:             "call inside if-then is rejected",
			sql:              "IF TRUE THEN CALL proj.data.proc1(); END IF",
			defaultProjectID: "default-proj",
			want:             nil,
			wantErr:          true,
			wantErrMsg:       "CALL is not allowed when dataset restrictions are in place",
		},
		{
			name:             "call inside while-do is rejected",
			sql:              "WHILE x < 1 DO CALL proj.data.proc1(); END WHILE",
			defaultProjectID: "default-proj",
			want:             nil,
			wantErr:          true,
			wantErrMsg:       "CALL is not allowed when dataset restrictions are in place",
		},
		{
			name:             "column named call is not a procedure call",
			sql:              "SELECT call FROM proj.data.tbl1",
			defaultProjectID: "default-proj",
			want:             []string{"proj.data.tbl1"},
			wantErr:          false,
		},
		{
			name:             "call procedure with newline",
			sql:              "CALL\nmy_dataset.my_procedure()",
			defaultProjectID: "default-proj",
			want:             nil,
			wantErr:          true,
			wantErrMsg:       "CALL is not allowed when dataset restrictions are in place",
		},
		{
			name:             "call procedure without default project should fail",
			sql:              "CALL my_dataset.my_procedure()",
			defaultProjectID: "",
			want:             nil,
			wantErr:          true,
			wantErrMsg:       "CALL is not allowed when dataset restrictions are in place",
		},
		{
			name:             "create procedure statement",
			sql:              "CREATE PROCEDURE my_dataset.my_procedure() BEGIN SELECT 1; END;",
			defaultProjectID: "default-proj",
			want:             nil,
			wantErr:          true,
			wantErrMsg:       "unanalyzable statements like 'CREATE PROCEDURE' are not allowed",
		},
		{
			name:             "create or replace procedure statement",
			sql:              "CREATE\n OR \nREPLACE \nPROCEDURE my_dataset.my_procedure() BEGIN SELECT 1; END;",
			defaultProjectID: "default-proj",
			want:             nil,
			wantErr:          true,
			wantErrMsg:       "unanalyzable statements like 'CREATE PROCEDURE' are not allowed",
		},
		{
			name:             "create function statement",
			sql:              "CREATE FUNCTION my_dataset.my_function() RETURNS INT64 AS (1);",
			defaultProjectID: "default-proj",
			want:             nil,
			wantErr:          true,
			wantErrMsg:       "unanalyzable statements like 'CREATE FUNCTION' are not allowed",
		},
		{
			name:             "alias filtering simple",
			sql:              "SELECT t1.col FROM proj.data.table AS t1",
			defaultProjectID: "default-proj",
			want:             []string{"proj.data.table"},
			wantErr:          false,
		},
		{
			name:             "alias filtering complex",
			sql:              "SELECT t1.col1, t2.col2 FROM proj.data.tbl1 t1 JOIN proj.data.tbl2 AS t2 ON t1.id = t2.id",
			defaultProjectID: "default-proj",
			want:             []string{"proj.data.tbl1", "proj.data.tbl2"},
			wantErr:          false,
		},
		{
			name:             "alias filtering in where clause",
			sql:              "SELECT * FROM proj.data.tbl1 AS t1 WHERE t1.id > 10",
			defaultProjectID: "default-proj",
			want:             []string{"proj.data.tbl1"},
			wantErr:          false,
		},
		{
			name:             "unnest column reference",
			sql:              "SELECT x FROM `proj.ds.tbl` AS t, UNNEST(t.arr) AS x",
			defaultProjectID: "default-proj",
			want:             []string{"proj.ds.tbl"},
			wantErr:          false,
		},
		{
			name:             "CTE with dots",
			sql:              "WITH `my.cte` AS (SELECT 1) SELECT * FROM `my.cte`",
			defaultProjectID: "default-proj",
			want:             []string{},
			wantErr:          false,
		},
		{
			name: "nested CTEs with dot-containing aliases",
			sql: `
				WITH raw_metrics AS (
					SELECT id, score FROM production-data.analytics.events
				),
				derived.results AS (
					SELECT id, score * 2 as double_score FROM raw_metrics
				)
				SELECT * FROM derived.results WHERE double_score > 100
			`,
			defaultProjectID: "default-proj",
			want:             []string{"production-data.analytics.events"},
			wantErr:          false,
		},
		{
			name:             "implicit join with comma",
			sql:              "SELECT * FROM proj.data.tbl1, proj.data.tbl2",
			defaultProjectID: "default-proj",
			want:             []string{"proj.data.tbl1", "proj.data.tbl2"},
			wantErr:          false,
		},
		{
			name:             "implicit alias",
			sql:              "SELECT t.col FROM proj.data.tbl t",
			defaultProjectID: "default-proj",
			want:             []string{"proj.data.tbl"},
			wantErr:          false,
		},
		{
			name:             "unnest column reference complex",
			sql:              "SELECT x FROM `proj.ds.tbl` AS t, UNNEST(t.arr) AS x JOIN `other.ds.tbl2` as o ON t.id = o.id",
			defaultProjectID: "default-proj",
			want:             []string{"proj.ds.tbl", "other.ds.tbl2"},
			wantErr:          false,
		},
		{
			name:             "create schema statement",
			sql:              "CREATE SCHEMA proj.data",
			defaultProjectID: "default-proj",
			want:             nil,
			wantErr:          true,
			wantErrMsg:       "dataset-level operations like 'CREATE SCHEMA' are not allowed",
		},
		{
			name:             "create dataset statement",
			sql:              "CREATE DATASET proj.data",
			defaultProjectID: "default-proj",
			want:             nil,
			wantErr:          true,
			wantErrMsg:       "dataset-level operations like 'CREATE DATASET' are not allowed",
		},
		{
			name:             "drop schema statement",
			sql:              "DROP SCHEMA proj.data",
			defaultProjectID: "default-proj",
			want:             nil,
			wantErr:          true,
			wantErrMsg:       "dataset-level operations like 'DROP SCHEMA' are not allowed",
		},
		{
			name:             "drop dataset statement",
			sql:              "DROP DATASET proj.data",
			defaultProjectID: "default-proj",
			want:             nil,
			wantErr:          true,
			wantErrMsg:       "dataset-level operations like 'DROP DATASET' are not allowed",
		},
		{
			name:             "alter schema statement",
			sql:              "ALTER SCHEMA proj.data SET OPTIONS(description='new one')",
			defaultProjectID: "default-proj",
			want:             nil,
			wantErr:          true,
			wantErrMsg:       "dataset-level operations like 'ALTER SCHEMA' are not allowed",
		},
		{
			name:             "alter dataset statement",
			sql:              "ALTER DATASET proj.data SET OPTIONS(description='new one')",
			defaultProjectID: "default-proj",
			want:             nil,
			wantErr:          true,
			wantErrMsg:       "dataset-level operations like 'ALTER DATASET' are not allowed",
		},
		{
			name:             "call fully qualified procedure",
			sql:              "CALL proj.data.proc()",
			defaultProjectID: "default-proj",
			want:             nil,
			wantErr:          true,
			wantErrMsg:       "CALL is not allowed when dataset restrictions are in place",
		},
		{
			name:             "create procedure statement",
			sql:              "CREATE PROCEDURE proj.data.proc() BEGIN SELECT 1; END",
			defaultProjectID: "default-proj",
			want:             nil,
			wantErr:          true,
			wantErrMsg:       "unanalyzable statements like 'CREATE PROCEDURE' are not allowed",
		},
		{
			name:             "create or replace procedure statement",
			sql:              "CREATE OR REPLACE PROCEDURE proj.data.proc() BEGIN SELECT 1; END",
			defaultProjectID: "default-proj",
			want:             nil,
			wantErr:          true,
			wantErrMsg:       "unanalyzable statements like 'CREATE PROCEDURE' are not allowed",
		},
		{
			name:             "create function statement",
			sql:              "CREATE FUNCTION proj.data.func() RETURNS INT64 AS (1)",
			defaultProjectID: "default-proj",
			want:             nil,
			wantErr:          true,
			wantErrMsg:       "unanalyzable statements like 'CREATE FUNCTION' are not allowed",
		},
		{
			name:             "simple execute immediate",
			sql:              "EXECUTE IMMEDIATE 'SELECT 1'",
			defaultProjectID: "default-proj",
			want:             nil,
			wantErr:          true,
			wantErrMsg:       "EXECUTE IMMEDIATE is not allowed",
		},
		{
			name:             "EXTERNAL_QUERY query",
			sql:              "SELECT * FROM EXTERNAL_QUERY('my-conn', 'SELECT 1')",
			defaultProjectID: "default-proj",
			want:             nil,
			wantErr:          true,
			wantErrMsg:       "EXTERNAL_QUERY is not allowed when dataset restrictions are in place",
		},
		{
			name:             "regional INFORMATION_SCHEMA query",
			sql:              "SELECT * FROM region-us.INFORMATION_SCHEMA.SCHEMATA",
			defaultProjectID: "default-proj",
			want:             nil,
			wantErr:          true,
			wantErrMsg:       "querying non-dataset-level INFORMATION_SCHEMA view \"SCHEMATA\" is not allowed when dataset restrictions are in place",
		},
		{
			name:             "project-level INFORMATION_SCHEMA query",
			sql:              "SELECT * FROM INFORMATION_SCHEMA.SCHEMATA",
			defaultProjectID: "default-proj",
			want:             nil,
			wantErr:          true,
			wantErrMsg:       "querying non-dataset-level INFORMATION_SCHEMA view \"SCHEMATA\" is not allowed when dataset restrictions are in place",
		},
		{
			name:             "INFORMATION_SCHEMA query without dataset prefix",
			sql:              "SELECT * FROM INFORMATION_SCHEMA.TABLES",
			defaultProjectID: "default-proj",
			want:             nil,
			wantErr:          true,
			wantErrMsg:       "querying INFORMATION_SCHEMA views without a dataset prefix is not allowed when dataset restrictions are in place",
		},
		{
			name:             "dataset-level INFORMATION_SCHEMA query with 3 parts",
			sql:              "SELECT * FROM my_dataset.INFORMATION_SCHEMA.TABLES",
			defaultProjectID: "default-proj",
			want:             []string{"default-proj.my_dataset.INFORMATION_SCHEMA"},
			wantErr:          false,
		},
		{
			name:             "dataset-level INFORMATION_SCHEMA query with 4 parts",
			sql:              "SELECT * FROM my-project.my_dataset.INFORMATION_SCHEMA.TABLES",
			defaultProjectID: "default-proj",
			want:             []string{"my-project.my_dataset.INFORMATION_SCHEMA"},
			wantErr:          false,
		},
		{
			name:             "invalid INFORMATION_SCHEMA query path with too many parts",
			sql:              "SELECT * FROM foo.bar.baz.INFORMATION_SCHEMA.TABLES",
			defaultProjectID: "default-proj",
			want:             nil,
			wantErr:          true,
			wantErrMsg:       "invalid INFORMATION_SCHEMA query path \"foo.bar.baz.INFORMATION_SCHEMA.TABLES\"",
		},
		{
			name:       "create table function is rejected",
			sql:        "CREATE TABLE FUNCTION ds.tvf() AS (SELECT 1)",
			wantErr:    true,
			wantErrMsg: "unanalyzable statements like 'CREATE TABLE FUNCTION' are not allowed",
		},
		{
			name:       "create or replace table function is rejected",
			sql:        "CREATE OR REPLACE TABLE FUNCTION ds.tvf() AS (SELECT 1)",
			wantErr:    true,
			wantErrMsg: "unanalyzable statements like 'CREATE TABLE FUNCTION' are not allowed",
		},
		{
			name:             "CTE name does not shadow a fully qualified table",
			sql:              "WITH t AS (SELECT 1) SELECT * FROM p.forbidden_ds.t",
			defaultProjectID: "myproj",
			want:             []string{"p.forbidden_ds.t"},
		},
		{
			name:             "CTE multi-part prefix does not shadow restricted dataset",
			sql:              "WITH forbidden_dataset.dummy AS (SELECT 1) SELECT * FROM forbidden_dataset.highly_sensitive_table",
			defaultProjectID: "myproj",
			want:             []string{"myproj.forbidden_dataset.highly_sensitive_table"},
		},
		{
			name:             "CREATE TABLE with column named schema",
			sql:              "CREATE TABLE allowed.t (schema STRING, x INT64)",
			defaultProjectID: "default-proj",
			want:             []string{"default-proj.allowed.t"},
			wantErr:          false,
		},
		{
			name:             "ALTER TABLE with column named dataset",
			sql:              "ALTER TABLE allowed.t ADD COLUMN dataset STRING",
			defaultProjectID: "default-proj",
			want:             []string{"default-proj.allowed.t"},
			wantErr:          false,
		},
		{
			name:             "CREATE SCHEMA is rejected",
			sql:              "CREATE SCHEMA foo",
			defaultProjectID: "default-proj",
			wantErr:          true,
			wantErrMsg:       "dataset-level operations like 'CREATE SCHEMA' are not allowed",
		},
		{
			name:             "CREATE OR REPLACE SCHEMA is rejected",
			sql:              "CREATE OR REPLACE SCHEMA foo",
			defaultProjectID: "default-proj",
			wantErr:          true,
			wantErrMsg:       "dataset-level operations like 'CREATE SCHEMA' are not allowed",
		},
		{
			name:             "DROP SCHEMA is rejected",
			sql:              "DROP SCHEMA foo",
			defaultProjectID: "default-proj",
			wantErr:          true,
			wantErrMsg:       "dataset-level operations like 'DROP SCHEMA' are not allowed",
		},
		{
			name:             "ALTER SCHEMA is rejected",
			sql:              "ALTER SCHEMA foo SET OPTIONS(description='x')",
			defaultProjectID: "default-proj",
			wantErr:          true,
			wantErrMsg:       "dataset-level operations like 'ALTER SCHEMA' are not allowed",
		},
		{
			name:             "CASE statement with THEN call",
			sql:              "SELECT CASE WHEN a THEN call ELSE b END FROM allowed.t",
			defaultProjectID: "default-proj",
			want:             []string{"default-proj.allowed.t"},
			wantErr:          false,
		},
		{
			name:             "CASE statement with AS call alias",
			sql:              "SELECT CASE WHEN a THEN 1 ELSE 2 END AS call FROM allowed.t",
			defaultProjectID: "default-proj",
			want:             []string{"default-proj.allowed.t"},
			wantErr:          false,
		},
		{
			name:             "CALL statement is rejected",
			sql:              "CALL proj.ds.my_proc()",
			defaultProjectID: "default-proj",
			wantErr:          true,
			wantErrMsg:       "CALL is not allowed when dataset restrictions are in place",
		},
		{
			name:             "BEGIN CALL statement is rejected",
			sql:              "BEGIN CALL proj.ds.p(); END",
			defaultProjectID: "default-proj",
			wantErr:          true,
			wantErrMsg:       "CALL is not allowed when dataset restrictions are in place",
		},
		{
			name:             "BEGIN with CASE statement with call",
			sql:              "BEGIN SELECT CASE WHEN x THEN call END FROM allowed.t; END",
			defaultProjectID: "default-proj",
			want:             []string{"default-proj.allowed.t"},
			wantErr:          false,
		},
		{
			name:             "semicolon resets table keyword state",
			sql:              "SELECT * FROM allowed.t; SELECT STRUCT(1, forbidden.col)",
			defaultProjectID: "default-proj",
			want:             []string{"default-proj.allowed.t"},
			wantErr:          false,
		},
		{
			name:             "semicolon multiple statements",
			sql:              "SELECT * FROM allowed.t; SELECT * FROM allowed2.t2",
			defaultProjectID: "default-proj",
			want:             []string{"default-proj.allowed.t", "default-proj.allowed2.t2"},
			wantErr:          false,
		},
		{
			name:             "comma separated tables in single statement",
			sql:              "SELECT * FROM a.t1, a.t2;",
			defaultProjectID: "default-proj",
			want:             []string{"default-proj.a.t1", "default-proj.a.t2"},
			wantErr:          false,
		},
		{
			name:             "CAST type does not shadow table name",
			sql:              "SELECT CAST(x AS int64) FROM int64.mytable",
			defaultProjectID: "default-proj",
			want:             []string{"default-proj.int64.mytable"},
			wantErr:          false,
		},
		{
			name:             "LEFT JOIN preserves both tables",
			sql:              "SELECT * FROM allowed.t LEFT JOIN forbidden.u ON true",
			defaultProjectID: "default-proj",
			want:             []string{"default-proj.allowed.t", "default-proj.forbidden.u"},
			wantErr:          false,
		},
		{
			name:             "CTE masking table name",
			sql:              "WITH c AS (SELECT * FROM ds.base) SELECT * FROM c",
			defaultProjectID: "default-proj",
			want:             []string{"default-proj.ds.base"},
			wantErr:          false,
		},
		{
			name:             "table alias in select list",
			sql:              "SELECT a.x FROM ds.t AS a",
			defaultProjectID: "default-proj",
			want:             []string{"default-proj.ds.t"},
			wantErr:          false,
		},
		{
			name:             "join with table aliases",
			sql:              "SELECT * FROM ds.t a JOIN ds.u b ON a.id = b.id",
			defaultProjectID: "default-proj",
			want:             []string{"default-proj.ds.t", "default-proj.ds.u"},
			wantErr:          false,
		},
		{
			name:             "SET session variable is rejected",
			sql:              "SET @@dataset_id = 'disallowed_ds'",
			defaultProjectID: "default-proj",
			wantErr:          true,
			wantErrMsg:       "session variable assignment ('SET @@') is not allowed",
		},
		{
			name:             "exceeding max parse depth fails gracefully",
			sql:              strings.Repeat("SELECT * FROM (", 100) + "SELECT 1" + strings.Repeat(")", 100),
			defaultProjectID: "default-proj",
			wantErr:          true,
			wantErrMsg:       "query nesting is too deep",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := bigquerycommon.TableParser(tc.sql, tc.defaultProjectID)
			if (err != nil) != tc.wantErr {
				t.Errorf("TableParser() error = %v, wantErr %v", err, tc.wantErr)
				return
			}
			if tc.wantErr && tc.wantErrMsg != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErrMsg) {
					t.Errorf("TableParser() error = %v, want err containing %q", err, tc.wantErrMsg)
				}
			}
			// Sort slices to ensure comparison is order-independent.
			sort.Strings(got)
			sort.Strings(tc.want)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("TableParser() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestIsAnyTableExplicitlyReferenced(t *testing.T) {
	testCases := []struct {
		name             string
		sql              string
		defaultProjectID string
		targetTableIDs   []string
		want             bool
	}{
		{
			name:             "simple match",
			sql:              "SELECT * FROM `proj.ds.tbl`",
			defaultProjectID: "def-proj",
			targetTableIDs:   []string{"proj.ds.tbl"},
			want:             true,
		},
		{
			name:             "match without project id in sql",
			sql:              "SELECT * FROM `ds.tbl`",
			defaultProjectID: "def-proj",
			targetTableIDs:   []string{"def-proj.ds.tbl"},
			want:             true,
		},
		{
			name:             "no match",
			sql:              "SELECT * FROM `ds.view`",
			defaultProjectID: "def-proj",
			targetTableIDs:   []string{"def-proj.ds.tbl"},
			want:             false,
		},
		{
			name:             "ignore in strings",
			sql:              "SELECT 'proj.ds.tbl' FROM `ds.view` ",
			defaultProjectID: "def-proj",
			targetTableIDs:   []string{"proj.ds.tbl"},
			want:             false,
		},
		{
			name:             "ignore in comments",
			sql:              "SELECT * FROM `ds.view` -- referencing proj.ds.tbl here",
			defaultProjectID: "def-proj",
			targetTableIDs:   []string{"proj.ds.tbl"},
			want:             false,
		},
		{
			name:             "match in join",
			sql:              "SELECT * FROM `ds.view` JOIN `proj.ds.tbl` ON 1=1",
			defaultProjectID: "def-proj",
			targetTableIDs:   []string{"proj.ds.tbl"},
			want:             true,
		},
		{
			name:             "match as column reference",
			sql:              "SELECT proj.ds.tbl.col FROM `ds.view`",
			defaultProjectID: "def-proj",
			targetTableIDs:   []string{"proj.ds.tbl"},
			want:             true,
		},
		{
			name:             "match with different casing",
			sql:              "SELECT * FROM `PROJ.ds.TBL`",
			defaultProjectID: "def-proj",
			targetTableIDs:   []string{"proj.ds.tbl"},
			want:             true,
		},
		{
			name:             "raw string ignore",
			sql:              "SELECT r'proj.ds.tbl' FROM `ds.view` ",
			defaultProjectID: "def-proj",
			targetTableIDs:   []string{"proj.ds.tbl"},
			want:             false,
		},
		{
			name:             "mixed quoting style",
			sql:              "SELECT * FROM `proj`.ds.`tbl` ",
			defaultProjectID: "def-proj",
			targetTableIDs:   []string{"proj.ds.tbl"},
			want:             true,
		},
		{
			name:             "all parts quoted separately",
			sql:              "SELECT * FROM `proj`.`ds`.`tbl` ",
			defaultProjectID: "def-proj",
			targetTableIDs:   []string{"proj.ds.tbl"},
			want:             true,
		},
		{
			name:             "middle part quoted",
			sql:              "SELECT * FROM proj.`ds`.tbl ",
			defaultProjectID: "def-proj",
			targetTableIDs:   []string{"proj.ds.tbl"},
			want:             true,
		},
		{
			name:             "fully qualified column reference",
			sql:              "SELECT proj.ds.tbl.col FROM `something` ",
			defaultProjectID: "def-proj",
			targetTableIDs:   []string{"proj.ds.tbl"},
			want:             true,
		},
		{
			name:             "domain scoped project ID match",
			sql:              "SELECT * FROM `google.com:project.dataset.table` ",
			defaultProjectID: "def-proj",
			targetTableIDs:   []string{"google.com:project.dataset.table"},
			want:             true,
		},
		{
			name:             "domain scoped project ID match with column",
			sql:              "SELECT `google.com:project.dataset.table`.col FROM `something_else` ",
			defaultProjectID: "def-proj",
			targetTableIDs:   []string{"google.com:project.dataset.table"},
			want:             true,
		},
		{
			name:           "raw string containing a backslash does not desync the lexer",
			sql:            `SELECT r'\' AS c FROM p.forbidden_ds.t`,
			targetTableIDs: []string{"p.forbidden_ds.t"},
			want:           true,
		},
		{
			name:             "wildcard table reference does not match literal table ID",
			sql:              "SELECT * FROM `proj.forbidden_ds.events_*`",
			defaultProjectID: "proj",
			targetTableIDs:   []string{"proj.forbidden_ds.events_20240101"},
			want:             false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := bigquerycommon.IsAnyTableExplicitlyReferenced(tc.sql, tc.defaultProjectID, tc.targetTableIDs)
			if err != nil {
				t.Fatalf("IsAnyTableExplicitlyReferenced() error = %v", err)
			}
			if got != tc.want {
				t.Errorf("IsAnyTableExplicitlyReferenced() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestTableParserDetailed(t *testing.T) {
	testCases := []struct {
		name                string
		sql                 string
		defaultProjectID    string
		wantTableIDs        []string
		wantUnqualifiedRefs []string
		wantErr             bool
	}{
		{
			name:                "unqualified table reference",
			sql:                 "SELECT * FROM secret_table",
			defaultProjectID:    "proj",
			wantTableIDs:        []string{},
			wantUnqualifiedRefs: []string{"secret_table"},
		},
		{
			name:                "qualified table reference",
			sql:                 "SELECT * FROM proj.allowed_ds.v",
			defaultProjectID:    "proj",
			wantTableIDs:        []string{"proj.allowed_ds.v"},
			wantUnqualifiedRefs: []string{},
		},
		{
			name:                "CTE is not unqualified reference",
			sql:                 "WITH c AS (SELECT 1) SELECT * FROM c",
			defaultProjectID:    "proj",
			wantTableIDs:        []string{},
			wantUnqualifiedRefs: []string{},
		},
		{
			name:                "multiple unqualified tables",
			sql:                 "SELECT * FROM t1, t2",
			defaultProjectID:    "proj",
			wantTableIDs:        []string{},
			wantUnqualifiedRefs: []string{"t1", "t2"},
		},
		{
			name:                "mixed qualified and unqualified",
			sql:                 "SELECT * FROM allowed_ds.v JOIN secret_table ON true",
			defaultProjectID:    "proj",
			wantTableIDs:        []string{"proj.allowed_ds.v"},
			wantUnqualifiedRefs: []string{"secret_table"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := bigquerycommon.TableParserDetailed(tc.sql, tc.defaultProjectID)
			if (err != nil) != tc.wantErr {
				t.Fatalf("TableParserDetailed() error = %v, wantErr %v", err, tc.wantErr)
			}
			if !tc.wantErr {
				if diff := cmp.Diff(tc.wantTableIDs, res.TableIDs); diff != "" {
					t.Errorf("TableIDs mismatch (-want +got):\n%s", diff)
				}
				if diff := cmp.Diff(tc.wantUnqualifiedRefs, res.UnqualifiedRefs); diff != "" {
					t.Errorf("UnqualifiedRefs mismatch (-want +got):\n%s", diff)
				}
			}
		})
	}
}
