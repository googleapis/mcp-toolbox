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

package prompts

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

// Argument is a wrapper around a parameters.Parameter that provides prompt-specific functionality.
// If the 'type' field is not specified in a YAML definition, it defaults to 'string'.
type Argument struct {
	parameters.Parameter
}

// Parse converts MCP string arguments before validating their declared parameter type.
func (arg Argument) Parse(value any) (any, error) {
	if text, ok := value.(string); ok && arg.GetType() != parameters.TypeString {
		decoder := json.NewDecoder(strings.NewReader(text))
		decoder.UseNumber()
		if err := decoder.Decode(&value); err != nil {
			return nil, fmt.Errorf("unable to decode argument %q: %w", arg.GetName(), err)
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			return nil, fmt.Errorf("argument %q must contain a single JSON value", arg.GetName())
		}
	}
	return arg.Parameter.Parse(value)
}

// Arguments is a slice of Argument.
type Arguments []Argument

// UnmarshalYAML provides custom unmarshaling logic for Arguments.
func (args *Arguments) UnmarshalYAML(ctx context.Context, unmarshal func(interface{}) error) error {
	*args = make(Arguments, 0)
	var rawList []util.DelayedUnmarshaler
	if err := unmarshal(&rawList); err != nil {
		return err
	}

	for _, u := range rawList {
		var p map[string]any
		if err := u.Unmarshal(&p); err != nil {
			return fmt.Errorf("error parsing argument: %w", err)
		}

		// If 'type' is missing, default it to string.
		paramType, ok := p["type"]
		if !ok {
			p["type"] = parameters.TypeString
			paramType = parameters.TypeString
		}

		// Call the clean, exported parser from the tools package. No more duplicated logic!
		param, err := parameters.ParseParameter(ctx, p, paramType.(string))
		if err != nil {
			return err
		}

		*args = append(*args, Argument{Parameter: param})
	}
	return nil
}

// ParseArguments validates and processes the user-provided arguments against the prompt's requirements.
func ParseArguments(arguments Arguments, args map[string]any, data map[string]map[string]any) (parameters.ParamValues, error) {
	var params parameters.Parameters
	for _, arg := range arguments {
		params = append(params, arg)
	}
	return parameters.ParseParams(params, args, data)
}
