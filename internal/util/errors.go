// Copyright 2026 Google LLC
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
	"errors"
	"fmt"
	"net/http"
	"strings"

	"google.golang.org/api/googleapi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type ErrorCategory string

const (
	CategoryAgent  ErrorCategory = "AGENT_ERROR"
	CategoryServer ErrorCategory = "SERVER_ERROR"
)

// ToolboxError is the interface all custom errors must satisfy
type ToolboxError interface {
	error
	Category() ErrorCategory
	Error() string
	Unwrap() error
}

// Agent Errors return 200 to the sender
type AgentError struct {
	Msg   string
	Cause error
}

var _ ToolboxError = &AgentError{}

func (e *AgentError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Msg, e.Cause)
	}
	return e.Msg
}

func (e *AgentError) Category() ErrorCategory { return CategoryAgent }

func (e *AgentError) Unwrap() error { return e.Cause }

func NewAgentError(msg string, cause error) *AgentError {
	return &AgentError{Msg: msg, Cause: cause}
}

var _ ToolboxError = &AgentError{}

// ClientServerError returns 4XX/5XX error code
type ClientServerError struct {
	Msg   string
	Code  int
	Cause error
}

var _ ToolboxError = &ClientServerError{}

func (e *ClientServerError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Msg, e.Cause)
	}
	return e.Msg
}

func (e *ClientServerError) Category() ErrorCategory { return CategoryServer }

func (e *ClientServerError) Unwrap() error { return e.Cause }

func NewClientServerError(msg string, code int, cause error) *ClientServerError {
	return &ClientServerError{Msg: msg, Code: code, Cause: cause}
}

// ProcessGcpError catches auth related errors in GCP requests results and return 401/403 error codes
// Returns AgentError for all other errors
func ProcessGcpError(err error) ToolboxError {
	if err == nil {
		return nil
	}

	var gErr *googleapi.Error
	if errors.As(err, &gErr) {
		switch gErr.Code {
		case http.StatusUnauthorized:
			return NewClientServerError(
				"failed to access GCP resource",
				http.StatusUnauthorized,
				nil,
			)
		case http.StatusForbidden:
			return NewClientServerError(
				"failed to access GCP resource",
				http.StatusForbidden,
				nil,
			)
		}
	}

	var statusErr interface{ GRPCStatus() *status.Status }
	if errors.As(err, &statusErr) {
		if st := statusErr.GRPCStatus(); st != nil {
			switch st.Code() {
			case codes.Unauthenticated:
				return NewClientServerError(
					"failed to access GCP resource",
					http.StatusUnauthorized,
					nil,
				)
			case codes.PermissionDenied:
				return NewClientServerError(
					"failed to access GCP resource",
					http.StatusForbidden,
					nil,
				)
			}
		}
	}

	return NewAgentError("error processing GCP request", err)
}

// ProcessGeneralError handles generic errors by inspecting the error string
// for common status code patterns.
func ProcessGeneralError(err error) ToolboxError {
	if err == nil {
		return nil
	}

	errStr := err.Error()

	// Check for Unauthorized
	if strings.Contains(errStr, "Error 401") || strings.Contains(errStr, "status 401") || strings.Contains(errStr, "status=401") {
		return NewClientServerError(
			"failed to access resource",
			http.StatusUnauthorized,
			err,
		)
	}

	// Check for Forbidden
	if strings.Contains(errStr, "Error 403") || strings.Contains(errStr, "status 403") || strings.Contains(errStr, "status=403") {
		return NewClientServerError(
			"failed to access resource",
			http.StatusForbidden,
			err,
		)
	}

	// Check for StatusTooManyRequests
	if strings.Contains(errStr, "Error 429") || strings.Contains(errStr, "status 429") || strings.Contains(errStr, "status=429") {
		return NewClientServerError(
			"rate limited - too many requests",
			http.StatusTooManyRequests,
			err,
		)
	}

	// Default to AgentError for logical failures (task execution failed)
	return NewAgentError("error processing request", err)
}
