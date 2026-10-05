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

package cloudstorage

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
)

func TestParseFromYamlCloudStorage(t *testing.T) {
	tcs := []struct {
		desc string
		in   string
		want server.SourceConfigs
	}{
		{
			desc: "basic example",
			in: `
			kind: source
			name: my-gcs
			type: cloud-storage
			project: my-project
			`,
			want: map[string]sources.SourceConfig{
				"my-gcs": Config{
					Name:    "my-gcs",
					Type:    SourceType,
					Project: "my-project",
				},
			},
		},
		{
			desc: "with allowed buckets and local roots",
			in: `
			kind: source
			name: my-gcs
			type: cloud-storage
			project: my-project
			allowedBuckets:
				- bucket1
				- bucket2
			allowedLocalRoots:
				- /var/tmp
				- /workspace
			`,
			want: map[string]sources.SourceConfig{
				"my-gcs": Config{
					Name:              "my-gcs",
					Type:              SourceType,
					Project:           "my-project",
					AllowedBuckets:    []string{"bucket1", "bucket2"},
					AllowedLocalRoots: []string{"/var/tmp", "/workspace"},
				},
			},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			got, _, _, _, _, _, _, _, err := server.UnmarshalPrimitiveConfig(context.Background(), testutils.FormatYaml(tc.in))
			if err != nil {
				t.Fatalf("unable to unmarshal: %s", err)
			}
			if !cmp.Equal(tc.want, got) {
				t.Fatalf("incorrect parse: %v", cmp.Diff(tc.want, got))
			}
		})
	}
}

func TestFailParseFromYaml(t *testing.T) {
	tcs := []struct {
		desc string
		in   string
		err  string
	}{
		{
			desc: "extra field",
			in: `
			kind: source
			name: my-gcs
			type: cloud-storage
			project: my-project
			foo: bar
			`,
			err: "error unmarshaling source: unable to parse source \"my-gcs\" as \"cloud-storage\": [1:1] unknown field \"foo\"\n>  1 | foo: bar\n       ^\n   2 | name: my-gcs\n   3 | project: my-project\n   4 | type: cloud-storage",
		},
		{
			desc: "missing required field",
			in: `
			kind: source
			name: my-gcs
			type: cloud-storage
			`,
			err: "error unmarshaling source: unable to parse source \"my-gcs\" as \"cloud-storage\": Key: 'Config.Project' Error:Field validation for 'Project' failed on the 'required' tag",
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			_, _, _, _, _, _, _, _, err := server.UnmarshalPrimitiveConfig(context.Background(), testutils.FormatYaml(tc.in))
			if err == nil {
				t.Fatalf("expect parsing to fail")
			}
			errStr := err.Error()
			if errStr != tc.err {
				t.Fatalf("unexpected error: got %q, want %q", errStr, tc.err)
			}
		})
	}
}

func TestValidateBucket(t *testing.T) {
	tcs := []struct {
		desc           string
		allowedBuckets []string
		bucket         string
		wantErr        bool
	}{
		{
			desc:           "empty allowedBuckets succeeds",
			allowedBuckets: nil,
			bucket:         "my-bucket",
			wantErr:        false,
		},
		{
			desc:           "allowed bucket succeeds",
			allowedBuckets: []string{"my-bucket", "other-bucket"},
			bucket:         "my-bucket",
			wantErr:        false,
		},
		{
			desc:           "disallowed bucket fails",
			allowedBuckets: []string{"my-bucket", "other-bucket"},
			bucket:         "attacker-bucket",
			wantErr:        true,
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			s := &Source{
				Config: Config{
					Name:           "my-gcs",
					AllowedBuckets: tc.allowedBuckets,
				},
			}
			err := s.validateBucket(tc.bucket)
			if (err != nil) != tc.wantErr {
				t.Fatalf("validateBucket(%q) got error: %v, wantErr: %v", tc.bucket, err, tc.wantErr)
			}
		})
	}
}

func TestValidateLocalPath(t *testing.T) {
	base := t.TempDir()

	tcs := []struct {
		desc         string
		allowedRoots []string
		path         string
		wantErr      bool
	}{
		{
			desc:         "empty allowedLocalRoots succeeds",
			allowedRoots: nil,
			path:         filepath.Join(base, "var", "tmp", "file.txt"),
			wantErr:      false,
		},
		{
			desc:         "under allowed root succeeds",
			allowedRoots: []string{filepath.Join(base, "var", "tmp"), filepath.Join(base, "workspace")},
			path:         filepath.Join(base, "workspace", "file.txt"),
			wantErr:      false,
		},
		{
			desc:         "exact match allowed root succeeds",
			allowedRoots: []string{filepath.Join(base, "var", "tmp"), filepath.Join(base, "workspace")},
			path:         filepath.Join(base, "workspace"),
			wantErr:      false,
		},
		{
			desc:         "outside allowed root fails",
			allowedRoots: []string{filepath.Join(base, "var", "tmp"), filepath.Join(base, "workspace")},
			path:         filepath.Join(base, "etc", "passwd"),
			wantErr:      true,
		},
		{
			desc:         "prefix check bypass fails (partial matching directory)",
			allowedRoots: []string{filepath.Join(base, "workspace")},
			path:         filepath.Join(base, "workspace-malicious", "file.txt"),
			wantErr:      true,
		},
		{
			desc:         "relative path is cleaned and validated",
			allowedRoots: []string{filepath.Join(base, "workspace")},
			path:         filepath.Join(base, "workspace", "..", "etc", "passwd"),
			wantErr:      true,
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			s := &Source{
				Config: Config{
					Name:              "my-gcs",
					AllowedLocalRoots: tc.allowedRoots,
				},
			}
			err := s.validateLocalPath(tc.path)
			if (err != nil) != tc.wantErr {
				t.Fatalf("validateLocalPath(%q) got error: %v, wantErr: %v", tc.path, err, tc.wantErr)
			}
		})
	}
}

// TestValidateLocalPathSymlinks covers paths that sit under an allowed root by
// name but reach a different file once symlinks are followed. Accepting those
// lets upload_object read a file outside the root and download_object write
// through a link to one, which is exactly what allowedLocalRoots exists to stop.
func TestValidateLocalPathSymlinks(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("resolving temp dir: %v", err)
	}
	allowed := filepath.Join(base, "allowed")
	outside := filepath.Join(base, "outside")
	for _, dir := range []string{allowed, outside} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll(%q): %v", dir, err)
		}
	}
	if err := os.WriteFile(filepath.Join(allowed, "real.txt"), []byte("INSIDE"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("OUTSIDE_SECRET"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	for _, l := range []struct{ target, link string }{
		{filepath.Join(allowed, "real.txt"), filepath.Join(allowed, "link_in.txt")},
		{filepath.Join(outside, "secret.txt"), filepath.Join(allowed, "link_out.txt")},
		{outside, filepath.Join(allowed, "dir_out")},
		{filepath.Join(outside, "missing.txt"), filepath.Join(allowed, "dangling.txt")},
		{allowed, filepath.Join(base, "linked_allowed")},
	} {
		if err := os.Symlink(l.target, l.link); err != nil {
			t.Fatalf("Symlink(%q -> %q): %v", l.link, l.target, err)
		}
	}

	tcs := []struct {
		desc         string
		allowedRoots []string
		path         string
		wantErr      bool
	}{
		{
			desc:         "regular file under root succeeds",
			allowedRoots: []string{allowed},
			path:         filepath.Join(allowed, "real.txt"),
		},
		{
			desc:         "new file under root succeeds",
			allowedRoots: []string{allowed},
			path:         filepath.Join(allowed, "new.txt"),
		},
		{
			desc:         "link resolving inside root succeeds",
			allowedRoots: []string{allowed},
			path:         filepath.Join(allowed, "link_in.txt"),
		},
		{
			desc:         "root reached through a link succeeds",
			allowedRoots: []string{filepath.Join(base, "linked_allowed")},
			path:         filepath.Join(base, "linked_allowed", "real.txt"),
		},
		{
			desc:         "root and path spelled through different links succeed",
			allowedRoots: []string{allowed},
			path:         filepath.Join(base, "linked_allowed", "real.txt"),
			// Name-level check rejects it: the caller's spelling is not under
			// the configured root even though it resolves there.
			wantErr: true,
		},
		{
			desc:         "link escaping root fails",
			allowedRoots: []string{allowed},
			path:         filepath.Join(allowed, "link_out.txt"),
			wantErr:      true,
		},
		{
			desc:         "intermediate directory link escaping root fails",
			allowedRoots: []string{allowed},
			path:         filepath.Join(allowed, "dir_out", "secret.txt"),
			wantErr:      true,
		},
		{
			desc:         "new file under an escaping directory link fails",
			allowedRoots: []string{allowed},
			path:         filepath.Join(allowed, "dir_out", "new.txt"),
			wantErr:      true,
		},
		{
			desc:         "dangling link escaping root fails",
			allowedRoots: []string{allowed},
			path:         filepath.Join(allowed, "dangling.txt"),
			wantErr:      true,
		},
		{
			desc:         "unset allowedLocalRoots leaves links unrestricted",
			allowedRoots: nil,
			path:         filepath.Join(allowed, "link_out.txt"),
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			s := &Source{
				Config: Config{
					Name:              "my-gcs",
					AllowedLocalRoots: tc.allowedRoots,
				},
			}
			err := s.validateLocalPath(tc.path)
			if (err != nil) != tc.wantErr {
				t.Fatalf("validateLocalPath(%q) got error: %v, wantErr: %v", tc.path, err, tc.wantErr)
			}
		})
	}
}

// TestAllowedLocalRootSymlinkEscape is the regression test for the reported
// bypass: a symlink planted under an allowed root was accepted because only its
// name was checked, so reading it returned content from outside the root.
func TestAllowedLocalRootSymlinkEscape(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("resolving temp dir: %v", err)
	}
	allowed := filepath.Join(base, "allowed")
	outside := filepath.Join(base, "outside")
	for _, dir := range []string{allowed, outside} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll(%q): %v", dir, err)
		}
	}
	secret := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(secret, []byte("OUTSIDE_SECRET"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	escape := filepath.Join(allowed, "innocent.txt")
	if err := os.Symlink(secret, escape); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	s := &Source{Config: Config{Name: "my-gcs", AllowedLocalRoots: []string{allowed}}}
	if err := s.validateLocalPath(escape); err == nil {
		// Demonstrate what acceptance would have cost, for whoever reads a
		// future failure of this test.
		content, readErr := os.ReadFile(escape)
		if readErr != nil {
			t.Fatalf("validateLocalPath(%q) accepted a path escaping %q", escape, allowed)
		}
		t.Fatalf("validateLocalPath(%q) accepted a path escaping %q; it reads %q from %q",
			escape, allowed, content, secret)
	}
}
