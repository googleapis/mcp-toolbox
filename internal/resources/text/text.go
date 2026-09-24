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

package text

import (
	"context"
	"fmt"

	"github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/resources"
)

const resourceType = "text"

func init() {
	if !resources.Register(resourceType, newConfig) {
		panic(fmt.Sprintf("resource type %q already registered", resourceType))
	}
}

func newConfig(ctx context.Context, name string, decoder *yaml.Decoder) (resources.ResourceConfig, error) {
	cfg := &Config{
		ResourceConfigBase: resources.ResourceConfigBase{
			ConfigBase: resources.ConfigBase{
				Name: name,
				Type: resourceType,
			},
		},
	}
	if err := decoder.DecodeContext(ctx, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Config represents the uninitialized textual resource configuration from YAML.
type Config struct {
	resources.ResourceConfigBase `yaml:",inline"`
	Text                         string `yaml:"text" validate:"required"`
}

var _ resources.ResourceConfig = &Config{}

// SetDefaults applies system defaults for unspecified optional fields.
func (c *Config) SetDefaults() {
	c.ResourceConfigBase.SetDefaults()
	if c.MimeType == "" {
		c.MimeType = "text/plain"
	}
}

// ResourceConfigType returns the resource type identifier.
func (c *Config) ResourceConfigType() string {
	return resourceType
}

// Initialize computes the size of the text and returns an initialized textual resource.
func (c *Config) Initialize(ctx context.Context) (resources.Resource, error) {
	size := int64(len(c.Text))

	return &Resource{Config: *c, Size: size}, nil
}

// Resource represents the initialized textual resource that returns plain text payloads.
type Resource struct {
	Config
	Size int64
}

var _ resources.Resource = &Resource{}

// GetSize returns the pre-computed size of the text content.
func (r *Resource) GetSize() *int64 {
	size := r.Size
	return &size
}

// Read retrieves the static text content.
func (r *Resource) Read(ctx context.Context, params map[string]any) (any, error) {
	return r.Text, nil
}

// ToConfig returns the underlying uninitialized configuration.
func (r *Resource) ToConfig() resources.ResourceConfig {
	return &r.Config
}
