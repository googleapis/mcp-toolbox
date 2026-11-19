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

// Package orcarouter implements an embedding model backed by the OrcaRouter
// gateway. OrcaRouter exposes OpenAI-compatible embedding models (OpenAI,
// Google, ...) behind a single API key and endpoint, so the provider simply
// forwards the standard POST /embeddings request to the gateway.
package orcarouter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/googleapis/mcp-toolbox/internal/embeddingmodels"
	"github.com/googleapis/mcp-toolbox/internal/util"
)

const EmbeddingModelType string = "orcarouter"

// DefaultBaseURL is the default OrcaRouter API endpoint.
const DefaultBaseURL string = "https://api.orcarouter.ai/v1"

// validate interface
var _ embeddingmodels.EmbeddingModelConfig = Config{}

type Config struct {
	Name      string `yaml:"name" validate:"required"`
	Type      string `yaml:"type" validate:"required"`
	Model     string `yaml:"model" validate:"required"`
	ApiKey    string `yaml:"apiKey"`
	BaseURL   string `yaml:"baseUrl"`
	Dimension int32  `yaml:"dimension"`
}

// Returns the embedding model type
func (cfg Config) EmbeddingModelConfigType() string {
	return EmbeddingModelType
}

// Initialize an OrcaRouter embedding model
func (cfg Config) Initialize(ctx context.Context) (embeddingmodels.EmbeddingModel, error) {
	// Get API Key
	apiKey := cfg.ApiKey
	if apiKey == "" {
		apiKey = os.Getenv("ORCAROUTER_API_KEY")
	}
	if apiKey == "" {
		return nil, fmt.Errorf("missing credentials for OrcaRouter embedding: " +
			"Provide 'apiKey' in YAML or set ORCAROUTER_API_KEY env var. " +
			"See documentation for details: https://mcp-toolbox.dev/documentation/configuration/embedding-models/orcarouter/")
	}

	// Retrieve logger from context
	l, err := util.LoggerFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve logger: %w", err)
	}

	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}

	// Set user agent
	ua, err := util.UserAgentFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get user agent from context: %w", err)
	}

	l.InfoContext(ctx, "Using OrcaRouter gateway for embedding", "model", cfg.Model)

	return &EmbeddingModel{
		Config:  cfg,
		ApiKey:  apiKey,
		BaseURL: baseURL,
		Client:  &http.Client{Timeout: 60 * time.Second},
		UA:      ua,
	}, nil
}

var _ embeddingmodels.EmbeddingModel = EmbeddingModel{}

type EmbeddingModel struct {
	Config
	ApiKey  string
	BaseURL string
	Client  *http.Client
	UA      string
}

// Returns the embedding model type
func (m EmbeddingModel) EmbeddingModelType() string {
	return EmbeddingModelType
}

func (m EmbeddingModel) ToConfig() embeddingmodels.EmbeddingModelConfig {
	return m.Config
}

func (m EmbeddingModel) EmbedParameters(ctx context.Context, parameters []string) ([][]float32, error) {
	logger, err := util.LoggerFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to get logger from ctx: %s", err)
	}

	reqBody := embeddingsRequest{
		Model: m.Model,
		Input: parameters,
	}
	if m.Dimension > 0 {
		reqBody.Dimensions = &m.Dimension
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("unable to marshal embeddings request: %w", err)
	}

	url := strings.TrimSuffix(m.BaseURL, "/") + "/embeddings"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("unable to create embeddings request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+m.ApiKey)
	req.Header.Set("User-Agent", m.UA)

	resp, err := m.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("unable to call OrcaRouter embeddings API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("OrcaRouter embeddings API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var result embeddingsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("unable to decode embeddings response: %w", err)
	}

	embeddings := make([][]float32, 0, len(result.Data))
	for _, item := range result.Data {
		embeddings = append(embeddings, item.Embedding)
	}

	logger.InfoContext(ctx, "Successfully embedded %d text parameters using model %s", len(parameters), m.Model)

	return embeddings, nil
}

type embeddingsRequest struct {
	Model      string   `json:"model"`
	Input      []string `json:"input"`
	Dimensions *int32   `json:"dimensions,omitempty"`
}

type embeddingsResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
}
