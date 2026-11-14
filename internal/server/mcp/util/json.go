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

package util

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"

	"github.com/googleapis/mcp-toolbox/internal/util"
)

// UnmarshalJSON decodes a single JSON value without converting numbers to float64.
func UnmarshalJSON(data []byte, v any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(v); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return errors.New("unexpected data after JSON value")
	}
	return nil
}

// MarshalArguments keeps integer literals exact while retaining decimal and exponent normalization.
func MarshalArguments(arguments map[string]any) ([]byte, error) {
	normalized, err := util.ConvertNumbers(arguments)
	if err != nil {
		return nil, err
	}
	return json.Marshal(normalized)
}
