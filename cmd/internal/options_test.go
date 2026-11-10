// Copyright 2024 Google LLC
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

package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/googleapis/mcp-toolbox/internal/log"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
)

func TestToolboxOptions(t *testing.T) {
	w := io.Discard
	tcs := []struct {
		desc    string
		isValid func(*ToolboxOptions) error
		option  Option
	}{
		{
			desc: "with logger",
			isValid: func(o *ToolboxOptions) error {
				if o.IOStreams.Out != w || o.IOStreams.ErrOut != w {
					return errors.New("loggers do not match")
				}
				return nil
			},
			option: WithIOStreams(w, w),
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			got := NewToolboxOptions(tc.option)
			if err := tc.isValid(got); err != nil {
				t.Errorf("option did not initialize command correctly: %v", err)
			}
		})
	}
}

func TestLoadConfig(t *testing.T) {
	t.Setenv("CLOUD_HEALTHCARE_PROJECT", "mock")
	t.Setenv("CLOUD_HEALTHCARE_REGION", "mock")
	t.Setenv("CLOUD_HEALTHCARE_DATASET", "mock")
	t.Setenv("BIGQUERY_PROJECT", "mock")
	t.Setenv("POSTGRES_HOST", "localhost")
	t.Setenv("POSTGRES_PORT", "5432")
	t.Setenv("POSTGRES_DATABASE", "mock")
	t.Setenv("POSTGRES_USER", "mock")
	t.Setenv("POSTGRES_PASSWORD", "mock")

	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	tcs := []struct {
		desc            string
		prebuiltConfigs []string
		initialVersion  string
		wantVersion     string
		wantErr         string
		matchPrefix     bool
	}{
		{
			desc:            "version sanitization with multiple configs",
			prebuiltConfigs: []string{"cloud-healthcare/cloud_healthcare_fhir_tools", "bigquery"},
			initialVersion:  "v1.0.0",
			wantVersion:     "v1.0.0+prebuilt.bigquery+prebuilt.cloud-healthcare.cloud_healthcare_fhir_tools",
		},
		{
			desc:            "toolset not found in prebuilt config",
			prebuiltConfigs: []string{"postgres/invalid-toolset"},
			wantErr:         "toolset 'invalid-toolset' not found in prebuilt configuration 'postgres'. Available toolsets: data, health, monitor, replication, view-config",
		},
		{
			desc:            "invalid separator - dot",
			prebuiltConfigs: []string{"postgres.sql"},
			wantErr:         "invalid prebuilt config format 'postgres.sql'. Did you mean 'postgres/sql'? Use '/' to specify a toolset",
		},
		{
			desc:            "invalid separator - colon",
			prebuiltConfigs: []string{"postgres:sql"},
			wantErr:         "invalid prebuilt config format 'postgres:sql'. Did you mean 'postgres/sql'? Use '/' to specify a toolset",
		},
		{
			desc:            "invalid separator - at",
			prebuiltConfigs: []string{"postgres@sql"},
			wantErr:         "invalid prebuilt config format 'postgres@sql'. Did you mean 'postgres/sql'? Use '/' to specify a toolset",
		},
		{
			desc:            "no warning on unrelated dots",
			prebuiltConfigs: []string{"invalid-source.sql"},
			wantErr:         "prebuilt source tool for 'invalid-source.sql' not found",
			matchPrefix:     true,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			opts := &ToolboxOptions{
				PrebuiltConfigs: tc.prebuiltConfigs,
				Cfg: server.ServerConfig{
					Version: tc.initialVersion,
				},
			}

			parser := &ConfigParser{}
			_, err = opts.LoadConfig(ctx, parser)

			if tc.wantVersion != "" {
				// Success case
				if err != nil {
					t.Fatalf("unexpected error loading config: %v", err)
				}
				if opts.Cfg.Version != tc.wantVersion {
					t.Errorf("unexpected version: got %q, want %q", opts.Cfg.Version, tc.wantVersion)
				}
			} else {
				// Failure case
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if tc.matchPrefix {
					if !strings.HasPrefix(err.Error(), tc.wantErr) {
						t.Errorf("unexpected error message:\ngot:  %q\nwant prefix: %q", err.Error(), tc.wantErr)
					}
				} else {
					if err.Error() != tc.wantErr {
						t.Errorf("unexpected error message:\ngot:  %q\nwant: %q", err.Error(), tc.wantErr)
					}
				}
			}
		})
	}
}

func TestCheckVersion(t *testing.T) {
	tcs := []struct {
		desc                string
		currentVersion      string
		remoteVersion       string
		httpStatus          int
		disableVersionCheck bool
		wantInfoLog         string
	}{
		{
			desc:           "outdated version logs warning",
			currentVersion: "1.7.0",
			remoteVersion:  "v1.8.0",
			httpStatus:     http.StatusOK,
			wantInfoLog:    "A newer version of MCP Toolbox is available: (v1.7.0 -> v1.8.0)",
		},
		{
			desc:           "up to date version logs nothing",
			currentVersion: "1.8.0",
			remoteVersion:  "v1.8.0",
			httpStatus:     http.StatusOK,
		},
		{
			desc:           "ahead of release logs nothing",
			currentVersion: "1.9.0",
			remoteVersion:  "v1.8.0",
			httpStatus:     http.StatusOK,
		},
		{
			desc:                "flag disabled skips check",
			currentVersion:      "1.7.0",
			remoteVersion:       "v1.8.0",
			httpStatus:          http.StatusOK,
			disableVersionCheck: true,
		},
		{
			desc:           "empty version string skips check",
			currentVersion: "",
			remoteVersion:  "v1.8.0",
			httpStatus:     http.StatusOK,
		},
		{
			desc:           "github rate limited or 500 error fails gracefully",
			currentVersion: "1.7.0",
			httpStatus:     http.StatusForbidden,
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {

			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.httpStatus)
				if tc.httpStatus == http.StatusOK {
					_ = json.NewEncoder(w).Encode(map[string]string{"tag_name": tc.remoteVersion})
				}
			}))
			defer ts.Close()

			origURL := githubReleasesURL
			githubReleasesURL = ts.URL
			defer func() { githubReleasesURL = origURL }()

			buf := new(bytes.Buffer)
			logger, err := log.NewLogger("standard", "DEBUG", buf, buf)
			if err != nil {
				t.Fatalf("failed to create test logger: %v", err)
			}
			opts := &ToolboxOptions{
				Logger:     logger,
				VersionNum: tc.currentVersion,
				Cfg: server.ServerConfig{
					DisableVersionCheck: tc.disableVersionCheck,
				},
				IOStreams: IOStreams{Out: buf, ErrOut: buf},
			}

			opts.checkVersion(context.Background())
			output := buf.String()

			if tc.wantInfoLog == "" {
				if strings.Contains(output, "INFO") {
					t.Errorf("expected no INFO log, but got: %s", output)
				}
				return
			}

			if !strings.Contains(output, "INFO") {
				t.Errorf("expected log level to be INFO, but got: %s", output)
			}
			if !strings.Contains(output, tc.wantInfoLog) {
				t.Errorf("expected info log containing %q, but got: %s", tc.wantInfoLog, output)
			}
		})
	}
}
