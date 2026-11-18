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
	"testing"

	"github.com/googleapis/gax-go/v2/apierror"
	"google.golang.org/api/googleapi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestProcessGcpErrorNil(t *testing.T) {
	if got := ProcessGcpError(nil); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestProcessGcpErrorTreatsNonAuthErrorsAsAgentError(t *testing.T) {
	tcs := []struct {
		name string
		err  error
	}{
		{
			name: "http bad request",
			err: &googleapi.Error{
				Code:    http.StatusBadRequest,
				Message: "Bad Request",
				Errors: []googleapi.ErrorItem{{
					Reason:  "invalidQuery",
					Message: "No matching signature for operator >=",
				}},
			},
		},
		{
			name: "http not found",
			err: &googleapi.Error{
				Code:    http.StatusNotFound,
				Message: "Not Found",
			},
		},
		{
			name: "grpc invalid argument",
			err:  status.Error(codes.InvalidArgument, "invalid query syntax"),
		},
		{
			name: "grpc not found",
			err:  status.Error(codes.NotFound, "table not found"),
		},
		{
			name: "wrapped grpc invalid argument",
			err:  fmt.Errorf("query failed: %w", status.Error(codes.InvalidArgument, "bad filter")),
		},
		{
			name: "apierror wrapping http bad request",
			err: func() error {
				apiErr, _ := apierror.FromError(&googleapi.Error{
					Code:    http.StatusBadRequest,
					Message: "Bad Request",
				})
				return apiErr
			}(),
		},
		{
			name: "plain error",
			err:  errors.New("unexpected transport error"),
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			got := ProcessGcpError(tc.err)

			var agentErr *AgentError
			if !errors.As(got, &agentErr) {
				t.Fatalf("expected AgentError, got %T (%v)", got, got)
			}
			if got.Category() != CategoryAgent {
				t.Fatalf("expected category %q, got %q", CategoryAgent, got.Category())
			}
			if !errors.Is(got, tc.err) {
				t.Fatalf("expected AgentError to wrap original error")
			}
		})
	}
}

func TestProcessGcpErrorPreservesAuthFailuresAsScrubbedServerErrors(t *testing.T) {
	mustAPIError := func(err error) error {
		t.Helper()
		apiErr, ok := apierror.FromError(err)
		if !ok {
			t.Fatalf("failed to construct apierror.APIError from %v", err)
		}
		return apiErr
	}

	tcs := []struct {
		name     string
		err      error
		wantCode int
	}{
		{
			name: "http unauthorized",
			err: &googleapi.Error{
				Code:    http.StatusUnauthorized,
				Message: "Request had invalid authentication credentials for projects/123456789",
			},
			wantCode: http.StatusUnauthorized,
		},
		{
			name: "http forbidden",
			err: &googleapi.Error{
				Code:    http.StatusForbidden,
				Message: "Caller service-account@example.iam.gserviceaccount.com lacks permission on projects/123456789",
			},
			wantCode: http.StatusForbidden,
		},
		{
			name: "wrapped http unauthorized",
			err: fmt.Errorf("request failed: %w", &googleapi.Error{
				Code:    http.StatusUnauthorized,
				Message: "Invalid token for projects/123456789",
			}),
			wantCode: http.StatusUnauthorized,
		},
		{
			name: "wrapped http forbidden",
			err: fmt.Errorf("request failed: %w", &googleapi.Error{
				Code:    http.StatusForbidden,
				Message: "Permission denied on projects/123456789",
			}),
			wantCode: http.StatusForbidden,
		},
		{
			name:     "grpc unauthenticated",
			err:      status.Error(codes.Unauthenticated, "invalid credentials for projects/123456789"),
			wantCode: http.StatusUnauthorized,
		},
		{
			name:     "grpc permission denied",
			err:      status.Error(codes.PermissionDenied, "permission denied for service-account@example.iam.gserviceaccount.com on projects/123456789"),
			wantCode: http.StatusForbidden,
		},
		{
			name:     "wrapped grpc unauthenticated",
			err:      fmt.Errorf("rpc failed: %w", status.Error(codes.Unauthenticated, "expired token on projects/123456789")),
			wantCode: http.StatusUnauthorized,
		},
		{
			name:     "wrapped grpc permission denied",
			err:      fmt.Errorf("rpc failed: %w", status.Error(codes.PermissionDenied, "caller lacks iam permission on projects/123456789")),
			wantCode: http.StatusForbidden,
		},
		{
			name:     "apierror wrapping grpc unauthenticated",
			err:      mustAPIError(status.Error(codes.Unauthenticated, "unauthenticated access to projects/123456789")),
			wantCode: http.StatusUnauthorized,
		},
		{
			name:     "apierror wrapping grpc permission denied",
			err:      mustAPIError(status.Error(codes.PermissionDenied, "denied access to projects/123456789")),
			wantCode: http.StatusForbidden,
		},
		{
			name:     "joined grpc unauthenticated",
			err:      errors.Join(errors.New("batch failure"), status.Error(codes.Unauthenticated, "invalid token on projects/123456789")),
			wantCode: http.StatusUnauthorized,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			got := ProcessGcpError(tc.err)

			var clientServerErr *ClientServerError
			if !errors.As(got, &clientServerErr) {
				t.Fatalf("expected ClientServerError, got %T (%v)", got, got)
			}
			if got.Category() != CategoryServer {
				t.Fatalf("expected category %q, got %q", CategoryServer, got.Category())
			}
			if clientServerErr.Code != tc.wantCode {
				t.Fatalf("expected code %d, got %d", tc.wantCode, clientServerErr.Code)
			}
			if got.Unwrap() != nil {
				t.Fatalf("expected nil unwrapped cause for scrubbed auth error, got %v", got.Unwrap())
			}
			if got.Error() != "failed to access GCP resource" {
				t.Fatalf("expected scrubbed message %q, got %q", "failed to access GCP resource", got.Error())
			}
			if strings.Contains(got.Error(), "projects/123456789") || strings.Contains(got.Error(), "service-account@") {
				t.Fatalf("upstream resource details leaked in error string: %q", got.Error())
			}
		})
	}
}
