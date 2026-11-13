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

package conversationalanalytics

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	cloudgdads "github.com/googleapis/mcp-toolbox/internal/sources/cloudgda"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"golang.org/x/oauth2"
)

// CompatibleSource defines the source interface required by conversational analytics tools.
type CompatibleSource interface {
	GoogleCloudTokenSourceWithScope(ctx context.Context, scope string) (oauth2.TokenSource, error)
	GetProjectID() string
	UseClientAuthorization() bool
}

// validate compatible sources are still compatible
var _ CompatibleSource = &cloudgdads.Source{}

// GetTokenSource resolves the OAuth2 token source from either the client-supplied
// Bearer access token (when client authorization is enabled on the source) or the
// source's configured Google Cloud credentials.
func GetTokenSource(ctx context.Context, source CompatibleSource, accessToken tools.AccessToken) (oauth2.TokenSource, util.ToolboxError) {
	if source.UseClientAuthorization() {
		if accessToken == "" {
			return nil, util.NewClientServerError("tool is configured for client OAuth but no token was provided in the request header", http.StatusUnauthorized, nil)
		}
		tokenStr, err := accessToken.ParseBearerToken()
		if err != nil {
			return nil, util.NewClientServerError("error parsing access token", http.StatusUnauthorized, err)
		}
		return oauth2.StaticTokenSource(&oauth2.Token{AccessToken: tokenStr}), nil
	}
	tokenSource, err := source.GoogleCloudTokenSourceWithScope(ctx, "")
	if err != nil {
		return nil, util.NewClientServerError("failed to get token source", http.StatusInternalServerError, err)
	}
	if tokenSource == nil {
		return nil, util.NewClientServerError("cloud-platform token source is missing", http.StatusInternalServerError, nil)
	}
	return tokenSource, nil
}

// DoGDARequest builds and executes an HTTP request against the Gemini Data Analytics API,
// sets standard headers, checks for a 2xx status code, and decodes the JSON response body
// (treating an empty body such as 204 No Content as {"done": true}).
func DoGDARequest(ctx context.Context, client *http.Client, method, caURL string, body []byte) (map[string]any, util.ToolboxError) {
	var bodyReader io.Reader
	if len(body) > 0 {
		bodyReader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, caURL, bodyReader)
	if err != nil {
		return nil, util.NewClientServerError("failed to create request", http.StatusInternalServerError, err)
	}
	req.Header.Set("X-Goog-API-Client", util.GDAClientID)
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, util.NewClientServerError("failed to send request", http.StatusInternalServerError, err)
	}
	respBody, readErr := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, util.NewAgentError(fmt.Sprintf("API returned error status: %d %s", resp.StatusCode, string(respBody)), nil)
	}
	if readErr != nil {
		return nil, util.NewClientServerError("failed to read response body", http.StatusInternalServerError, readErr)
	}

	var result map[string]any
	if len(bytes.TrimSpace(respBody)) == 0 {
		result = map[string]any{"done": true}
	} else if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, util.NewClientServerError("failed to decode response", http.StatusInternalServerError, err)
	} else if result == nil {
		return nil, util.NewClientServerError("failed to decode response: expected JSON object", http.StatusInternalServerError, nil)
	}
	return result, nil
}
