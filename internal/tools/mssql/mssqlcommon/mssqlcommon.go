// Copyright 2026 Google LLC
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

package mssqlcommon

import (
	mssql "github.com/microsoft/go-mssqldb"
)

// ConvertToType converts a value scanned from a SQL Server result column into
// one that encodes to JSON as the value it holds. databaseTypeName is the
// column's sql.ColumnType.DatabaseTypeName().
//
// go-mssqldb returns DECIMAL, NUMERIC, MONEY and SMALLMONEY values as their
// decimal text in a []byte, and UNIQUEIDENTIFIER values as their 16 wire bytes.
// encoding/json writes a []byte as base64, so these are converted to the exact
// decimal text (as the MySQL source returns DECIMAL) and to the canonical GUID
// string. All other values, including BINARY, VARBINARY and IMAGE, are returned
// unchanged.
//
// go-mssqldb reports DECIMAL for NUMERIC columns as well, so NUMERIC is listed
// only so that the set of type names is explicit.
func ConvertToType(databaseTypeName string, v any) any {
	b, ok := v.([]byte)
	if !ok {
		return v
	}
	switch databaseTypeName {
	case "DECIMAL", "NUMERIC", "MONEY", "SMALLMONEY":
		return string(b)
	case "UNIQUEIDENTIFIER":
		var id mssql.UniqueIdentifier
		if err := id.Scan(b); err == nil {
			return id.String()
		}
	}
	return v
}
