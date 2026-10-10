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

package cloudstoragecommon

import (
	"os"
	"path/filepath"
	"testing"
)

// symlinkFixture builds a tree with an allowed directory containing a variety
// of links, plus an "outside" directory the links escape to:
//
//	<base>/allowed/real.txt          regular file
//	<base>/allowed/link_in.txt    -> <base>/allowed/real.txt
//	<base>/allowed/link_out.txt   -> <base>/outside/secret.txt
//	<base>/allowed/dir_out        -> <base>/outside
//	<base>/allowed/dangling.txt   -> <base>/outside/missing.txt
//	<base>/outside/secret.txt        regular file
//
// base is symlink-resolved so callers can compare against it directly on
// platforms where the temp dir sits behind a link.
func symlinkFixture(t *testing.T) (base, allowed, outside string) {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("resolving temp dir: %v", err)
	}
	allowed = filepath.Join(base, "allowed")
	outside = filepath.Join(base, "outside")
	for _, dir := range []string{allowed, outside} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll(%q): %v", dir, err)
		}
	}
	for _, f := range []struct{ path, content string }{
		{filepath.Join(allowed, "real.txt"), "INSIDE"},
		{filepath.Join(outside, "secret.txt"), "OUTSIDE_SECRET"},
	} {
		if err := os.WriteFile(f.path, []byte(f.content), 0o600); err != nil {
			t.Fatalf("WriteFile(%q): %v", f.path, err)
		}
	}
	for _, l := range []struct{ target, link string }{
		{filepath.Join(allowed, "real.txt"), filepath.Join(allowed, "link_in.txt")},
		{filepath.Join(outside, "secret.txt"), filepath.Join(allowed, "link_out.txt")},
		{outside, filepath.Join(allowed, "dir_out")},
		{filepath.Join(outside, "missing.txt"), filepath.Join(allowed, "dangling.txt")},
	} {
		if err := os.Symlink(l.target, l.link); err != nil {
			t.Fatalf("Symlink(%q -> %q): %v", l.link, l.target, err)
		}
	}
	return base, allowed, outside
}

func TestValidateLocalPath(t *testing.T) {
	base := t.TempDir()
	dotPath := base + string(filepath.Separator) + "." + string(filepath.Separator) + "a" + string(filepath.Separator) + "b" + string(filepath.Separator) + "c"
	containsDotsPath := filepath.Join(base, "foo..bar", "baz")

	tcs := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{in: filepath.Join(base, "out.bin"), want: filepath.Join(base, "out.bin")},
		{in: dotPath, want: filepath.Join(base, "a", "b", "c")},
		{in: containsDotsPath, want: containsDotsPath},

		{in: "", wantErr: true},
		{in: "relative/path", wantErr: true},
		{in: "../escape", wantErr: true},
		{in: "/legit/../../etc/passwd", wantErr: true},
		{in: `C:\legit\..\secret.txt`, wantErr: true},
		{in: `C:/legit/../secret.txt`, wantErr: true},
		{in: "..", wantErr: true},
	}
	for _, tc := range tcs {
		t.Run(tc.in, func(t *testing.T) {
			got, err := ValidateLocalPath(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestResolveSymlinks(t *testing.T) {
	_, allowed, outside := symlinkFixture(t)

	tcs := []struct {
		desc    string
		in      string
		want    string
		wantErr bool
	}{
		{
			desc: "regular file resolves to itself",
			in:   filepath.Join(allowed, "real.txt"),
			want: filepath.Join(allowed, "real.txt"),
		},
		{
			desc: "link staying inside resolves to its target",
			in:   filepath.Join(allowed, "link_in.txt"),
			want: filepath.Join(allowed, "real.txt"),
		},
		{
			desc: "link escaping resolves outside",
			in:   filepath.Join(allowed, "link_out.txt"),
			want: filepath.Join(outside, "secret.txt"),
		},
		{
			desc: "link as intermediate directory resolves outside",
			in:   filepath.Join(allowed, "dir_out", "secret.txt"),
			want: filepath.Join(outside, "secret.txt"),
		},
		{
			desc: "missing leaf keeps its literal name",
			in:   filepath.Join(allowed, "new.txt"),
			want: filepath.Join(allowed, "new.txt"),
		},
		{
			desc: "several missing components keep their literal names",
			in:   filepath.Join(allowed, "a", "b", "c.txt"),
			want: filepath.Join(allowed, "a", "b", "c.txt"),
		},
		{
			desc: "missing leaf under an escaping directory link resolves outside",
			in:   filepath.Join(allowed, "dir_out", "new.txt"),
			want: filepath.Join(outside, "new.txt"),
		},
		{
			desc:    "dangling link is rejected, not treated as a missing name",
			in:      filepath.Join(allowed, "dangling.txt"),
			wantErr: true,
		},
		{
			desc:    "path through a dangling directory link is rejected",
			in:      filepath.Join(allowed, "dangling.txt", "child.txt"),
			wantErr: true,
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			got, err := ResolveSymlinks(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ResolveSymlinks(%q) = %q, want error", tc.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveSymlinks(%q) unexpected error: %v", tc.in, err)
			}
			if got != tc.want {
				t.Fatalf("ResolveSymlinks(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestResolveWithinDirSymlinks covers the destination_dir boundary against
// links planted inside it, which a name-only check cannot see.
func TestResolveWithinDirSymlinks(t *testing.T) {
	base, allowed, _ := symlinkFixture(t)
	linkedDir := filepath.Join(base, "linked_allowed")
	if err := os.Symlink(allowed, linkedDir); err != nil {
		t.Fatalf("Symlink(%q): %v", linkedDir, err)
	}

	tcs := []struct {
		desc    string
		dir     string
		rel     string
		want    string
		wantErr bool
	}{
		{
			desc: "regular file inside dir",
			dir:  allowed,
			rel:  "real.txt",
			want: filepath.Join(allowed, "real.txt"),
		},
		{
			desc: "link staying inside dir",
			dir:  allowed,
			rel:  "link_in.txt",
			want: filepath.Join(allowed, "link_in.txt"),
		},
		{
			desc: "new file inside dir",
			dir:  allowed,
			rel:  filepath.Join("nested", "out.bin"),
			want: filepath.Join(allowed, "nested", "out.bin"),
		},
		{
			desc: "dir reached through a link still matches",
			dir:  linkedDir,
			rel:  "real.txt",
			want: filepath.Join(linkedDir, "real.txt"),
		},
		{
			desc:    "link escaping dir is rejected",
			dir:     allowed,
			rel:     "link_out.txt",
			wantErr: true,
		},
		{
			desc:    "intermediate directory link escaping dir is rejected",
			dir:     allowed,
			rel:     filepath.Join("dir_out", "secret.txt"),
			wantErr: true,
		},
		{
			desc:    "new file under an escaping directory link is rejected",
			dir:     allowed,
			rel:     filepath.Join("dir_out", "new.bin"),
			wantErr: true,
		},
		{
			desc:    "dangling link escaping dir is rejected",
			dir:     allowed,
			rel:     "dangling.txt",
			wantErr: true,
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			got, err := ResolveWithinDir(tc.dir, tc.rel)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ResolveWithinDir(%q, %q) = %q, want error", tc.dir, tc.rel, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveWithinDir(%q, %q) unexpected error: %v", tc.dir, tc.rel, err)
			}
			if got != tc.want {
				t.Fatalf("ResolveWithinDir(%q, %q) = %q, want %q", tc.dir, tc.rel, got, tc.want)
			}
		})
	}
}

func TestResolveWithinDir(t *testing.T) {
	base := t.TempDir()

	tcs := []struct {
		desc    string
		dir     string
		rel     string
		want    string
		wantErr bool
	}{
		{desc: "valid relative path", dir: base, rel: filepath.Join("nested", "out.bin"), want: filepath.Join(base, "nested", "out.bin")},
		{desc: "cleans in-dir path", dir: base, rel: filepath.Join("nested", ".", "out.bin"), want: filepath.Join(base, "nested", "out.bin")},
		{desc: "cleans parent segment that stays in dir", dir: base, rel: filepath.Join("nested", "..", "out.bin"), want: filepath.Join(base, "out.bin")},
		{desc: "empty dir", dir: "", rel: "out.bin", wantErr: true},
		{desc: "relative dir", dir: "relative/base", rel: "out.bin", wantErr: true},
		{desc: "empty relative path", dir: base, rel: "", wantErr: true},
		{desc: "absolute relative path rejected", dir: base, rel: filepath.Join(base, "out.bin"), wantErr: true},
		{desc: "parent escape rejected", dir: base, rel: filepath.Join("..", "escape.bin"), wantErr: true},
		{desc: "nested parent escape rejected", dir: base, rel: filepath.Join("nested", "..", "..", "escape.bin"), wantErr: true},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			got, err := ResolveWithinDir(tc.dir, tc.rel)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
