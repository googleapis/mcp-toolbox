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

package resources_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/resources"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/util"
)

func mockFailingFactory(ctx context.Context, name string, decoder *yaml.Decoder) (resources.ResourceConfig, error) {
	return nil, errors.New("factory error")
}

func mockNilReturningFactory(ctx context.Context, name string, decoder *yaml.Decoder) (resources.ResourceConfig, error) {
	return nil, nil
}

func TestRegister(t *testing.T) {
	if resources.Register("nilFactory", nil) {
		t.Errorf("Expected Register to return false for nil factory")
	}

	if !resources.Register("mockNew", mockFactory) {
		t.Errorf("Expected Register to return true for new type")
	}

	if resources.Register("mockNew", mockFactory) {
		t.Errorf("Expected Register to return false for duplicate type")
	}
}

func mockFactory(ctx context.Context, name string, decoder *yaml.Decoder) (resources.ResourceConfig, error) {
	var cfg testutils.MockResourceConfig
	cfg.Name = name
	cfg.Type = "mock"
	if err := decoder.DecodeContext(ctx, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func TestDecodeConfig(t *testing.T) {
	resources.Register("mock_success", mockFactory)
	resources.Register("failing", mockFailingFactory)
	resources.Register("nilReturn", mockNilReturningFactory)

	t.Run("NilDecoder", func(t *testing.T) {
		ctx := context.Background()
		_, err := resources.DecodeConfig(ctx, "mock_success", "testMock", nil)
		if err == nil {
			t.Fatalf("Expected error when decoder is nil, got nil")
		}
	})

	t.Run("NilReturningFactory", func(t *testing.T) {
		yamlBytes := []byte("uri: mock://test")
		decoder := yaml.NewDecoder(bytes.NewReader(yamlBytes))
		ctx := context.Background()
		_, err := resources.DecodeConfig(ctx, "nilReturn", "testMock", decoder)
		if err == nil {
			t.Fatalf("Expected error when factory returns nil config, got nil")
		}
	})

	t.Run("Success", func(t *testing.T) {
		yamlBytes := []byte("uri: mock://test")
		decoder := yaml.NewDecoder(bytes.NewReader(yamlBytes))
		ctx := context.Background()
		cfg, err := resources.DecodeConfig(ctx, "mock_success", "testMock", decoder)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		mockCfg, ok := cfg.(*testutils.MockResourceConfig)
		if !ok {
			t.Fatalf("Expected *testutils.MockResourceConfig, got %T", cfg)
		}

		if mockCfg.Name != "testMock" {
			t.Errorf("Expected Name 'testMock', got %q", mockCfg.Name)
		}
		if mockCfg.Type != "mock" {
			t.Errorf("Expected Type 'mock', got %q", mockCfg.Type)
		}
		if mockCfg.URI != "mock://test" {
			t.Errorf("Expected URI 'mock://test', got %q", mockCfg.URI)
		}

	})

	t.Run("UnknownType", func(t *testing.T) {
		yamlBytes := []byte("uri: mock://test")
		decoder := yaml.NewDecoder(bytes.NewReader(yamlBytes))
		ctx := context.Background()
		_, err := resources.DecodeConfig(ctx, "unknown", "test", decoder)
		if err == nil {
			t.Fatalf("Expected error for unknown type, got nil")
		}
	})

	t.Run("FactoryError", func(t *testing.T) {
		yamlBytes := []byte("uri: mock://test")
		decoder := yaml.NewDecoder(bytes.NewReader(yamlBytes))
		ctx := context.Background()
		_, err := resources.DecodeConfig(ctx, "failing", "test", decoder)
		if err == nil {
			t.Fatalf("Expected error from failing factory, got nil")
		}
	})
}

func TestGetBaseDirFromContext(t *testing.T) {
	ctx := context.Background()

	t.Run("EmptyContext", func(t *testing.T) {
		if resources.GetBaseDirFromContext(ctx) != "" {
			t.Errorf("Expected empty string for empty context")
		}
	})

	t.Run("NilContext", func(t *testing.T) {
		var nilCtx context.Context
		if resources.GetBaseDirFromContext(nilCtx) != "" {
			t.Errorf("Expected empty string for nil context")
		}
	})

	t.Run("ValidString", func(t *testing.T) {
		ctxWithDir := context.WithValue(ctx, resources.BaseDirKey, "/test/dir")
		if resources.GetBaseDirFromContext(ctxWithDir) != "/test/dir" {
			t.Errorf("Expected '/test/dir', got %q", resources.GetBaseDirFromContext(ctxWithDir))
		}
	})

	t.Run("InvalidType", func(t *testing.T) {
		ctxWithInt := context.WithValue(ctx, resources.BaseDirKey, 12345)
		if resources.GetBaseDirFromContext(ctxWithInt) != "" {
			t.Errorf("Expected empty string when base dir is not a string type")
		}
	})
}

func TestResourceConfigBase_YAML(t *testing.T) {
	yamlStr := `
name: testName
type: testType
uri: file:///test
description: A test description
title: A Test Title
mimeType: text/plain
annotations:
  priority: 0.5
`
	var cfg resources.ResourceConfigBase
	if err := yaml.Unmarshal([]byte(yamlStr), &cfg); err != nil {
		t.Fatalf("Failed to unmarshal ResourceConfigBase: %v", err)
	}

	if cfg.Name != "testName" {
		t.Errorf("Expected Name 'testName', got %q", cfg.Name)
	}
	if cfg.Type != "testType" {
		t.Errorf("Expected Type 'testType', got %q", cfg.Type)
	}
	if cfg.URI != "file:///test" {
		t.Errorf("Expected URI 'file:///test', got %q", cfg.URI)
	}
	if cfg.Description != "A test description" {
		t.Errorf("Expected Description 'A test description', got %q", cfg.Description)
	}
	if cfg.Title != "A Test Title" {
		t.Errorf("Expected Title 'A Test Title', got %q", cfg.Title)
	}
	if cfg.MimeType != "text/plain" {
		t.Errorf("Expected MimeType 'text/plain', got %q", cfg.MimeType)
	}
	if cfg.Annotations == nil || cfg.Annotations.Priority == nil || *cfg.Annotations.Priority != 0.5 {
		t.Errorf("Expected annotation priority=0.5, got %v", cfg.Annotations)
	}
}

func TestStrictDecoding_Error(t *testing.T) {
	raw := map[string]any{
		"name":               "testResource",
		"type":               "mock_strict",
		"invalidRandomField": true, // This should trigger the strict decoding error
	}

	decoder, err := util.NewStrictDecoder(raw)
	if err != nil {
		t.Fatalf("Failed to create strict decoder: %v", err)
	}

	ctx := context.Background()
	resources.Register("mock_strict", mockFactory)

	_, err = resources.DecodeConfig(ctx, "mock_strict", "testResource", decoder)
	if err == nil {
		t.Fatalf("Expected DecodeConfig to return an error for an unknown field 'invalidRandomField', but got nil")
	}
}
