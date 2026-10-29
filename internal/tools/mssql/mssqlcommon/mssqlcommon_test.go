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

package mssqlcommon_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/googleapis/mcp-toolbox/internal/tools/mssql/mssqlcommon"
)

func TestConvertToType(t *testing.T) {
	// 6F9619FF-8B86-D011-B42D-00C04FC964FF as SQL Server sends it: the first
	// three groups are little-endian.
	guidWire := []byte{0xFF, 0x19, 0x96, 0x6F, 0x86, 0x8B, 0x11, 0xD0, 0xB4, 0x2D, 0x00, 0xC0, 0x4F, 0xC9, 0x64, 0xFF}
	tcs := []struct {
		desc             string
		databaseTypeName string
		in               any
		want             string // JSON encoding of the converted value
	}{
		{
			desc:             "decimal",
			databaseTypeName: "DECIMAL",
			in:               []byte("12345678901234567890.0123456789"),
			want:             `"12345678901234567890.0123456789"`,
		},
		{
			desc:             "negative decimal",
			databaseTypeName: "DECIMAL",
			in:               []byte("-0.0000000001"),
			want:             `"-0.0000000001"`,
		},
		{
			desc:             "numeric type name",
			databaseTypeName: "NUMERIC",
			in:               []byte("1.5"),
			want:             `"1.5"`,
		},
		{
			desc:             "money",
			databaseTypeName: "MONEY",
			in:               []byte("4400592.8004"),
			want:             `"4400592.8004"`,
		},
		{
			desc:             "smallmoney",
			databaseTypeName: "SMALLMONEY",
			in:               []byte("-214748.3648"),
			want:             `"-214748.3648"`,
		},
		{
			desc:             "uniqueidentifier",
			databaseTypeName: "UNIQUEIDENTIFIER",
			in:               guidWire,
			want:             `"6F9619FF-8B86-D011-B42D-00C04FC964FF"`,
		},
		{
			desc:             "uniqueidentifier with an unexpected length is unchanged",
			databaseTypeName: "UNIQUEIDENTIFIER",
			in:               []byte{0x01, 0x02},
			want:             `"AQI="`,
		},
		{
			desc:             "varbinary is unchanged",
			databaseTypeName: "VARBINARY",
			in:               []byte{0x01, 0x02, 0xFF},
			want:             `"AQL/"`,
		},
		{
			desc:             "null decimal",
			databaseTypeName: "DECIMAL",
			in:               nil,
			want:             `null`,
		},
		{
			desc:             "bigint is unchanged",
			databaseTypeName: "BIGINT",
			in:               int64(9007199254740993),
			want:             `9007199254740993`,
		},
		{
			desc:             "datetime is unchanged",
			databaseTypeName: "DATETIME",
			in:               time.Date(2013, 5, 30, 13, 14, 15, 0, time.UTC),
			want:             `"2013-05-30T13:14:15Z"`,
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			got, err := json.Marshal(mssqlcommon.ConvertToType(tc.databaseTypeName, tc.in))
			if err != nil {
				t.Fatalf("unable to marshal result: %s", err)
			}
			if string(got) != tc.want {
				t.Errorf("incorrect result: got %s, want %s", got, tc.want)
			}
		})
	}
}
