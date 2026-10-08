// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package util

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"google.golang.org/api/option"
	htransport "google.golang.org/api/transport/http"
)

const (
	gdaDefaultEndpoint = "https://geminidataanalytics.googleapis.com"
	gdaMTLSEndpoint    = "https://geminidataanalytics.mtls.googleapis.com"

	defaultGDAPollInterval = 2 * time.Second
	defaultGDAPollTimeout  = 60 * time.Second
)

var gdaPathSegmentRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

// ValidateGDAPathSegment validates that a single resource path segment
// (such as location or data_agent_id) is non-empty and contains only allowed
// characters matching ADK's _SEGMENT pattern. It is not meant for project IDs,
// which may be domain-scoped (e.g. "google.com:my-project").
func ValidateGDAPathSegment(value, fieldName string) ToolboxError {
	if value == "" {
		return NewAgentError(fmt.Sprintf("%s is required and must be a non-empty string", fieldName), nil)
	}
	if !gdaPathSegmentRegex.MatchString(value) {
		return NewAgentError(fmt.Sprintf("%s %q contains disallowed characters", fieldName, value), nil)
	}
	return nil
}

// GetGDAEndpoint returns the Gemini Data Analytics API endpoint,
// choosing the mTLS endpoint if mTLS is enabled.
func GetGDAEndpoint() string {
	mtlsMode := getMTLSMode()
	if mtlsMode == "always" {
		return gdaMTLSEndpoint
	}
	if mtlsMode == "never" {
		return gdaDefaultEndpoint
	}
	// Default mode is "auto"
	if isClientCertificateEnabled() {
		return gdaMTLSEndpoint
	}
	return gdaDefaultEndpoint
}

// NewGDAClient returns an HTTP client configured for Gemini Data Analytics.
// It handles mTLS and authentication if a token source is provided.
func NewGDAClient(ctx context.Context, opts ...option.ClientOption) (*http.Client, error) {
	// Default options for GDA
	defaultOpts := []option.ClientOption{
		option.WithEndpoint(GetGDAEndpoint()),
		option.WithScopes("https://www.googleapis.com/auth/cloud-platform"),
	}

	allOpts := append(defaultOpts, opts...)

	client, _, err := htransport.NewClient(ctx, allOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create GDA HTTP client: %w", err)
	}
	return client, nil
}

// AwaitGDAOperation waits for a Gemini Data Analytics long-running operation (LRO)
// to finish, polling until done or timeout. It follows the ADK _await_lro behavior:
//  1. Checks `done` on the initial response before requiring an operation name.
//  2. Validates that any unfinished operation name contains "/operations/" (returning
//     completed non-operation resources directly if `done` was omitted).
//  3. Polls immediately on the first check and waits `pollInterval` between subsequent
//     checks, retrying only transient HTTP status codes (429, 500, 502, 503, 504).
func AwaitGDAOperation(
	ctx context.Context,
	client *http.Client,
	endpoint string,
	initialOp map[string]any,
	actionDesc string,
	pollInterval time.Duration,
	pollTimeout time.Duration,
) (any, ToolboxError) {
	if pollInterval <= 0 {
		pollInterval = defaultGDAPollInterval
	}
	if pollTimeout <= 0 {
		pollTimeout = defaultGDAPollTimeout
	}

	if val, done, tbErr := extractGDAOperationResult(initialOp, actionDesc); done {
		if tbErr != nil {
			return nil, tbErr
		}
		return val, nil
	}

	opName, _ := initialOp["name"].(string)
	if opName == "" || !strings.Contains(opName, "/operations/") {
		if _, hasDone := initialOp["done"]; !hasDone && len(initialOp) > 0 {
			return initialOp, nil
		}
		return nil, NewClientServerError(
			fmt.Sprintf("operation is not complete and has no pollable '/operations/' name: %v", initialOp),
			http.StatusInternalServerError,
			nil,
		)
	}

	deadline := time.Now().Add(pollTimeout)
	var lastStatus string
	firstIteration := true

	for {
		if !firstIteration {
			remaining := time.Until(deadline)
			if remaining <= 0 {
				return nil, newGDAPollTimeoutError(actionDesc, opName, lastStatus)
			}
			sleepDur := pollInterval
			if remaining < sleepDur {
				sleepDur = remaining
			}
			timer := time.NewTimer(sleepDur)
			select {
			case <-ctx.Done():
				timer.Stop()
				errMsg := fmt.Sprintf("context cancelled while waiting for data agent %s (op: %s): %v", actionDesc, opName, ctx.Err())
				if lastStatus != "" {
					errMsg += fmt.Sprintf(". Last status: %s", lastStatus)
				}
				return nil, NewClientServerError(errMsg, http.StatusInternalServerError, nil)
			case <-timer.C:
			}
		}
		firstIteration = false

		if err := ctx.Err(); err != nil {
			errMsg := fmt.Sprintf("context cancelled while waiting for data agent %s (op: %s): %v", actionDesc, opName, err)
			if lastStatus != "" {
				errMsg += fmt.Sprintf(". Last status: %s", lastStatus)
			}
			return nil, NewClientServerError(errMsg, http.StatusInternalServerError, nil)
		}
		if time.Now().After(deadline) {
			return nil, newGDAPollTimeoutError(actionDesc, opName, lastStatus)
		}

		opURL := fmt.Sprintf("%s/v1/%s", endpoint, opName)
		opReq, err := http.NewRequestWithContext(ctx, http.MethodGet, opURL, nil)
		if err != nil {
			lastStatus = fmt.Sprintf("request creation error: %v", err)
			continue
		}
		opReq.Header.Set("X-Goog-API-Client", GDAClientID)

		opResp, err := client.Do(opReq)
		if err != nil {
			lastStatus = fmt.Sprintf("network error: %v", err)
			continue
		}

		opRespBody, _ := io.ReadAll(opResp.Body)
		opResp.Body.Close()

		if opResp.StatusCode != http.StatusOK {
			if isRetryableGDAPollStatus(opResp.StatusCode) {
				lastStatus = fmt.Sprintf("HTTP status %d", opResp.StatusCode)
				continue
			}
			if opResp.StatusCode == http.StatusUnauthorized || opResp.StatusCode == http.StatusForbidden {
				return nil, NewClientServerError(
					fmt.Sprintf("polling failed with %d: %s", opResp.StatusCode, string(opRespBody)),
					opResp.StatusCode,
					nil,
				)
			}
			return nil, NewAgentError(
				fmt.Sprintf("polling failed with %d: %s", opResp.StatusCode, string(opRespBody)),
				nil,
			)
		}

		var pollOp map[string]any
		if err := json.Unmarshal(opRespBody, &pollOp); err != nil {
			return nil, NewClientServerError(
				fmt.Sprintf("error decoding operation poll response: %v (body: %s)", err, string(opRespBody)),
				http.StatusInternalServerError,
				err,
			)
		}

		if val, done, tbErr := extractGDAOperationResult(pollOp, actionDesc); done {
			if tbErr != nil {
				return nil, tbErr
			}
			return val, nil
		}
	}
}

func newGDAPollTimeoutError(actionDesc, opName, lastStatus string) ToolboxError {
	errMsg := fmt.Sprintf("timed out waiting for data agent %s (op: %s)", actionDesc, opName)
	if lastStatus != "" {
		errMsg += fmt.Sprintf(". Last status: %s", lastStatus)
	}
	errMsg += ". The operation may still be executing asynchronously in the background. Do not retry the operation."
	return NewAgentError(errMsg, nil)
}

func isRetryableGDAPollStatus(statusCode int) bool {
	switch statusCode {
	case http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func extractGDAOperationResult(result map[string]any, actionDesc string) (any, bool, ToolboxError) {
	if d, ok := result["done"].(bool); ok && d {
		if errVal, ok := result["error"]; ok && errVal != nil {
			errBytes, err := json.Marshal(errVal)
			if err != nil {
				return nil, true, NewAgentError(fmt.Sprintf("data agent %s failed: %v", actionDesc, errVal), nil)
			}
			return nil, true, NewAgentError(fmt.Sprintf("data agent %s failed: %s", actionDesc, string(errBytes)), nil)
		}
		if responseVal, ok := result["response"].(map[string]any); ok {
			return responseVal, true, nil
		}
		return result, true, nil
	}
	return nil, false, nil
}

func isClientCertificateEnabled() bool {
	return strings.ToLower(os.Getenv("GOOGLE_API_USE_CLIENT_CERTIFICATE")) == "true"
}

func getMTLSMode() string {
	mode := os.Getenv("GOOGLE_API_USE_MTLS_ENDPOINT")
	if mode == "" {
		mode = os.Getenv("GOOGLE_API_USE_MTLS") // Deprecated
	}
	if mode == "" {
		return "auto"
	}
	return strings.ToLower(mode)
}
