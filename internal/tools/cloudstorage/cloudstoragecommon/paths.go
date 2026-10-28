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
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ValidateLocalPath enforces the local-filesystem path contract used by
// download_object and upload_object: non-empty, absolute after filepath.Clean,
// and free of ".." components. It returns the cleaned path. OS permissions
// remain the real isolation boundary; this check just prevents obvious
// traversal mistakes and forces callers to be explicit about where they want
// bytes to land. Confining a path to a configured directory is a separate
// concern; see ResolveWithinDir and ResolveSymlinks.
func ValidateLocalPath(p string) (string, error) {
	if p == "" {
		return "", fmt.Errorf("path is empty")
	}
	// Reject any ".." segment in the raw input. We check the raw input
	// (not just the cleaned output) so that escapes like
	// "/legit/../../etc/passwd" — which filepath.Clean collapses to an
	// innocuous-looking absolute path — are still rejected. Legitimate
	// names that happen to *contain* two dots (e.g. "foo..bar") are fine;
	// only a standalone ".." segment is disallowed.
	for _, seg := range strings.FieldsFunc(p, func(r rune) bool {
		return r == '/' || r == '\\'
	}) {
		if seg == ".." {
			return "", fmt.Errorf("path %q contains '..'", p)
		}
	}
	clean := filepath.Clean(p)
	if !filepath.IsAbs(clean) {
		return "", fmt.Errorf("path %q must be absolute", p)
	}
	return clean, nil
}

// ResolveSymlinks returns the final filesystem target of path, following every
// symbolic link along the way. Comparing *this* against a configured boundary —
// rather than the caller-supplied name — is what stops a path that merely looks
// like it sits inside the boundary from opening a file outside it.
//
// Paths whose trailing components do not exist yet (the normal case for a
// download destination) are resolved as deeply as the filesystem allows, and
// the missing components are appended literally.
//
// A component that exists as a symbolic link but does not resolve — a dangling
// link — is rejected rather than treated as a missing name. Creating a file at
// such a path follows the link, so accepting it on the strength of its literal
// name would reopen the very escape this function exists to close.
//
// The returned path reflects the filesystem as it was during the walk, so a
// caller that opens the path afterwards is still racing anyone able to write
// into the directories it traverses. Hard links are not detectable here at all.
// Both remain the operator's to contain with OS permissions.
func ResolveSymlinks(path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		return resolved, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("cannot resolve path %q: %w", path, err)
	}

	// Walk up to the deepest ancestor that does resolve, remembering the
	// components we stepped over so they can be reattached to it.
	var missing []string
	cur := path
	for {
		if fi, lerr := os.Lstat(cur); lerr == nil && fi.Mode()&fs.ModeSymlink != 0 {
			return "", fmt.Errorf("path %q traverses unresolvable symbolic link %q", path, cur)
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", fmt.Errorf("cannot resolve path %q: no existing ancestor", path)
		}
		missing = append([]string{filepath.Base(cur)}, missing...)
		cur = parent

		resolvedParent, perr := filepath.EvalSymlinks(cur)
		if perr == nil {
			return filepath.Join(append([]string{resolvedParent}, missing...)...), nil
		}
		if !errors.Is(perr, fs.ErrNotExist) {
			return "", fmt.Errorf("cannot resolve path %q: %w", path, perr)
		}
	}
}

// escapes reports whether target lies outside dir. Both are expected to be
// cleaned absolute paths.
func escapes(dir, target string) (bool, error) {
	within, err := filepath.Rel(dir, target)
	if err != nil {
		return true, err
	}
	return within == ".." || strings.HasPrefix(within, ".."+string(filepath.Separator)) || filepath.IsAbs(within), nil
}

// ResolveWithinDir joins rel onto dir and returns the result only if it stays
// inside dir, both as written and after symlinks are resolved. The returned
// path is the cleaned join, not the symlink-resolved target, so callers keep
// reporting the location the user asked for.
func ResolveWithinDir(dir, rel string) (string, error) {
	cleanDir, err := ValidateLocalPath(dir)
	if err != nil {
		return "", fmt.Errorf("directory %q is invalid: %w", dir, err)
	}
	if rel == "" {
		return "", fmt.Errorf("relative path is empty")
	}
	if filepath.IsAbs(rel) {
		return "", fmt.Errorf("path %q must be relative", rel)
	}

	cleanDest := filepath.Clean(filepath.Join(cleanDir, rel))
	out, err := escapes(cleanDir, cleanDest)
	if err != nil {
		return "", fmt.Errorf("path %q cannot be resolved within %q: %w", rel, cleanDir, err)
	}
	if out {
		return "", fmt.Errorf("path %q escapes configured directory %q", rel, cleanDir)
	}

	// Repeat the check against the real targets. A symlink under cleanDir can
	// point anywhere, so the name-level check above proves nothing on its own.
	resolvedDir, err := ResolveSymlinks(cleanDir)
	if err != nil {
		return "", fmt.Errorf("directory %q cannot be resolved: %w", cleanDir, err)
	}
	resolvedDest, err := ResolveSymlinks(cleanDest)
	if err != nil {
		return "", fmt.Errorf("path %q cannot be resolved: %w", rel, err)
	}
	out, err = escapes(resolvedDir, resolvedDest)
	if err != nil {
		return "", fmt.Errorf("path %q cannot be resolved within %q: %w", rel, cleanDir, err)
	}
	if out {
		return "", fmt.Errorf("path %q resolves through a symbolic link to a target outside configured directory %q", rel, cleanDir)
	}
	return cleanDest, nil
}
