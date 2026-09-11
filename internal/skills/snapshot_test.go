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

package skills_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/googleapis/mcp-toolbox/internal/resources"
	"github.com/googleapis/mcp-toolbox/internal/resources/file"
	"github.com/googleapis/mcp-toolbox/internal/skills"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
)

// fileResource writes content to dir/filename and returns a file resource for it
// at uri. Unlike the text resources the rest of the suite uses, this one has a
// backing store the test can edit after discovery.
func fileResource(t *testing.T, ctx context.Context, dir, filename, uri, content string) (resources.Resource, string) {
	t.Helper()
	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("unable to write %q: %s", path, err)
	}
	cfg := &file.Config{
		ResourceConfigBase: resources.ResourceConfigBase{
			ConfigBase: resources.ConfigBase{Name: filename, Type: "file", MimeType: "text/markdown"},
			URI:        uri,
		},
		Path: path,
	}
	res, err := cfg.Initialize(ctx)
	if err != nil {
		t.Fatalf("unable to initialize %q: %s", uri, err)
	}
	return res, path
}

func readString(t *testing.T, ctx context.Context, res resources.Resource) string {
	t.Helper()
	got, err := res.Read(ctx, nil)
	if err != nil {
		t.Fatalf("Read() = %v, want nil", err)
	}
	content, ok := got.(string)
	if !ok {
		t.Fatalf("Read() returned %T, want string", got)
	}
	return content
}

// TestSnapshotPinsBytes is the guarantee the whole PR exists for: a file edited
// after discovery must not change what the skill serves, because the digest
// already published to hosts covers the bytes read at discovery.
func TestSnapshotPinsBytes(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()

	const original = "# Common queries\n"
	skillDoc := skillMD("analytics-guide", "Query and summarize the warehouse")
	skillRes, _ := fileResource(t, ctx, dir, "SKILL.md", "skill://analytics-guide/SKILL.md", skillDoc)
	queriesRes, queriesPath := fileResource(t, ctx, dir, "queries.md",
		"skill://analytics-guide/references/queries.md", original)

	resourcesMap := map[string]resources.Resource{"guide": skillRes, "queries": queriesRes}

	entries, snapshots, err := skills.Discover(ctx, resourcesMap)
	if err != nil {
		t.Fatalf("Discover() = %v, want nil", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}

	snap, ok := snapshots["skill://analytics-guide/references/queries.md"]
	if !ok {
		t.Fatalf("no snapshot for the supporting file, got %v", snapshots)
	}

	// The unwrapped resource still reads live, so this edit is observable.
	const edited = "# Common queries, revised and rather longer\n"
	if err := os.WriteFile(queriesPath, []byte(edited), 0600); err != nil {
		t.Fatal(err)
	}
	if got := readString(t, ctx, queriesRes); got != edited {
		t.Fatalf("the backing resource did not pick up the edit, got %q; the test proves nothing", got)
	}

	if got := readString(t, ctx, snap); got != original {
		t.Errorf("snapshot Read() = %q, want the bytes read at discovery %q", got, original)
	}

	// The digest is what a host verifies against, so it must still describe
	// what the snapshot serves.
	var ref skills.ResourceRef
	for _, r := range entries[0].Resources.Refs {
		if r.URI == "skill://analytics-guide/references/queries.md" {
			ref = r
		}
	}
	if want := digestOf(original); ref.Digest != want {
		t.Errorf("digest = %q, want %q", ref.Digest, want)
	}
	if got := digestOf(readString(t, ctx, snap)); got != ref.Digest {
		t.Errorf("digest of the served bytes = %q, want the published %q", got, ref.Digest)
	}

	// A live GetSize would advertise the edited length next to the pinned bytes.
	size := snap.GetSize()
	if size == nil {
		t.Fatal("GetSize() = nil, want the pinned length")
	}
	if *size != int64(len(original)) {
		t.Errorf("GetSize() = %d, want %d", *size, len(original))
	}
	if ref.Size != int64(len(original)) {
		t.Errorf("manifest size = %d, want %d", ref.Size, len(original))
	}
}

// TestSnapshotForwardsMetadata pins that wrapping is transparent to everything
// except Read and GetSize.
func TestSnapshotForwardsMetadata(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	skillDoc := skillMD("analytics-guide", "Query and summarize the warehouse")
	skillRes, _ := fileResource(t, ctx, dir, "SKILL.md", "skill://analytics-guide/SKILL.md", skillDoc)

	_, snapshots, err := skills.Discover(ctx, map[string]resources.Resource{"guide": skillRes})
	if err != nil {
		t.Fatalf("Discover() = %v, want nil", err)
	}
	snap := snapshots["skill://analytics-guide/SKILL.md"]
	if snap == nil {
		t.Fatal("no snapshot for SKILL.md")
	}

	if got, want := snap.GetURI(), skillRes.GetURI(); got != want {
		t.Errorf("GetURI() = %q, want %q", got, want)
	}
	if got, want := snap.GetName(), skillRes.GetName(); got != want {
		t.Errorf("GetName() = %q, want %q", got, want)
	}
	if got, want := snap.GetMimeType(), skillRes.GetMimeType(); got != want {
		t.Errorf("GetMimeType() = %q, want %q", got, want)
	}
	if snap.ToConfig() == nil {
		t.Error("ToConfig() = nil, want the wrapped resource's config")
	}
}

// TestSnapshotSkipsNonSkillResources keeps the freeze scoped: an ordinary
// resource must keep reading from its backing store.
func TestSnapshotSkipsNonSkillResources(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	skillDoc := skillMD("analytics-guide", "Query and summarize the warehouse")
	skillRes, _ := fileResource(t, ctx, dir, "SKILL.md", "skill://analytics-guide/SKILL.md", skillDoc)
	plainRes, plainPath := fileResource(t, ctx, dir, "notes.md", "file://notes", "original notes\n")

	_, snapshots, err := skills.Discover(ctx, map[string]resources.Resource{
		"guide": skillRes,
		"notes": plainRes,
	})
	if err != nil {
		t.Fatalf("Discover() = %v, want nil", err)
	}
	if _, ok := snapshots["file://notes"]; ok {
		t.Error("the plain resource was snapshotted, want it left alone")
	}

	const edited = "edited notes\n"
	if err := os.WriteFile(plainPath, []byte(edited), 0600); err != nil {
		t.Fatal(err)
	}
	if got := readString(t, ctx, plainRes); got != edited {
		t.Errorf("plain resource Read() = %q, want the live %q", got, edited)
	}
}

// TestDiscoverRejectsOversizeSkillWhileReading pins that the total-size limit is
// enforced as files are read, not after they are all in memory, so a single
// skill cannot allocate past the limit before being rejected.
func TestDiscoverRejectsOversizeSkillWhileReading(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatal(err)
	}

	const chunk = 4 << 20 // 4 MiB per file, five files clears the 16 MiB limit
	resourcesMap := map[string]resources.Resource{
		"guide/SKILL.md": textResource(t, ctx, "guide/SKILL.md",
			"skill://analytics-guide/SKILL.md",
			skillMD("analytics-guide", "Query and summarize the warehouse")),
	}
	for i := range 5 {
		name := string(rune('a' + i))
		resourcesMap[name] = textResource(t, ctx, name,
			"skill://analytics-guide/refs/"+name+".md", strings.Repeat("x", chunk))
	}

	_, _, err = skills.Discover(ctx, resourcesMap)
	if err == nil {
		t.Fatal("Discover() = nil, want a total-size error")
	}
	if !strings.Contains(err.Error(), "total size exceeds the limit") {
		t.Errorf("Discover() = %v, want a total-size error", err)
	}
	if !strings.Contains(err.Error(), "skill://analytics-guide/SKILL.md") {
		t.Errorf("Discover() = %v, want the error to name the skill", err)
	}
}
