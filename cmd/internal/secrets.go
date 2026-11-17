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

package internal

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"regexp"
	"strings"
	"unicode/utf8"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/lexer"
	"github.com/goccy/go-yaml/token"
	"github.com/google/safetext/yamltemplate"
	"github.com/google/uuid"
	"github.com/googleapis/gax-go/v2"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"google.golang.org/api/option"
)

const refKey = "__REF__"

var (
	refOpenRegex           = regexp.MustCompile(`__REF__\s*\(`)
	rawRefTokenRegex       = regexp.MustCompile(`__REF__\s*\([^)]*\)`)
	secretResourceURIRegex = regexp.MustCompile(`^//secretmanager\.googleapis\.com/projects/([^/]+)/secrets/([^/]+)/versions/([^/]+)$`)
)

// SecretManagerClient abstracts the Google Cloud Secret Manager client for testing.
type SecretManagerClient interface {
	AccessSecretVersion(ctx context.Context, req *secretmanagerpb.AccessSecretVersionRequest, opts ...gax.CallOption) (*secretmanagerpb.AccessSecretVersionResponse, error)
	Close() error
}

// newSecretManagerClient creates a Google Cloud Secret Manager client using ADC.
var newSecretManagerClient = func(ctx context.Context, opts ...option.ClientOption) (SecretManagerClient, error) {
	return secretmanager.NewClient(ctx, opts...)
}

// SetSecretManagerClientFactoryForTest overrides the Secret Manager client factory in tests
// and returns a restore function.
func SetSecretManagerClientFactoryForTest(fn func(ctx context.Context, opts ...option.ClientOption) (SecretManagerClient, error)) func() {
	orig := newSecretManagerClient
	newSecretManagerClient = fn
	return func() {
		newSecretManagerClient = orig
	}
}

// SecretRef represents a parsed inline __REF__(...) Secret Manager reference.
type SecretRef struct {
	Project  string
	SecretID string
	Version  string
}

// ResourceName returns the Secret Manager secret version resource path.
func (r SecretRef) ResourceName() string {
	return fmt.Sprintf("projects/%s/secrets/%s/versions/%s", r.Project, r.SecretID, r.Version)
}

// randomTemplateVarID returns a valid Go template identifier for safetext/yamltemplate substitution.
func randomTemplateVarID() string {
	return "var_" + strings.ReplaceAll(uuid.NewString(), "-", "_")
}

// ParseInlineSecretRef parses the inner expression inside __REF__(...).
// Supports matching double quotes, matching single quotes, or unquoted URI strings of the form:
// //secretmanager.googleapis.com/projects/<project>/secrets/<secret>/versions/<version>
func ParseInlineSecretRef(rawInner string) (SecretRef, error) {
	trimmed := strings.TrimSpace(rawInner)
	if trimmed == "" {
		return SecretRef{}, fmt.Errorf("empty secret reference in __REF__()")
	}

	// Unescape outer escaped quotes from raw YAML double-quoted (\"...\") or single-quoted (''...'') scalars.
	if len(trimmed) >= 4 && strings.HasPrefix(trimmed, `\"`) && strings.HasSuffix(trimmed, `\"`) {
		trimmed = `"` + trimmed[2:len(trimmed)-2] + `"`
	} else if len(trimmed) >= 4 && strings.HasPrefix(trimmed, `''`) && strings.HasSuffix(trimmed, `''`) {
		trimmed = `'` + trimmed[2:len(trimmed)-2] + `'`
	}

	uri := trimmed
	first, last := trimmed[0], trimmed[len(trimmed)-1]
	if first == '"' || first == '\'' || last == '"' || last == '\'' {
		if len(trimmed) < 2 || first != last || (first != '"' && first != '\'') {
			return SecretRef{}, fmt.Errorf("mismatched quotes in secret reference __REF__(%s)", rawInner)
		}
		uri = strings.TrimSpace(trimmed[1 : len(trimmed)-1])
		if uri == "" {
			return SecretRef{}, fmt.Errorf("empty secret reference in __REF__(%s)", rawInner)
		}
	}

	m := secretResourceURIRegex.FindStringSubmatch(uri)
	if m == nil {
		return SecretRef{}, fmt.Errorf(
			"invalid secret reference %q in __REF__(...): expected \"//secretmanager.googleapis.com/projects/<project>/secrets/<secret>/versions/<version>\"",
			uri,
		)
	}

	return SecretRef{
		Project:  m[1],
		SecretID: m[2],
		Version:  m[3],
	}, nil
}

// findInlineSecretRefs locates and validates all __REF__(...) placeholders within a decoded YAML string value.
func findInlineSecretRefs(s string) ([]SecretRef, error) {
	if !strings.Contains(s, refKey) {
		return nil, nil
	}

	var refs []SecretRef
	pos := 0
	for pos < len(s) {
		loc := refOpenRegex.FindStringIndex(s[pos:])
		if loc == nil {
			break
		}
		innerStart := pos + loc[1]
		closeRel := strings.IndexByte(s[innerStart:], ')')
		if closeRel == -1 {
			return nil, fmt.Errorf("unclosed __REF__(...) secret reference")
		}
		ref, err := ParseInlineSecretRef(s[innerStart : innerStart+closeRel])
		if err != nil {
			return nil, err
		}
		refs = append(refs, ref)
		pos = innerStart + closeRel + 1
	}
	return refs, nil
}

// FetchSecretRefs fetches payloads from Google Cloud Secret Manager for each unique
// secret version reference and verifies UTF-8 validity and CRC32C checksum integrity.
func FetchSecretRefs(ctx context.Context, refs []SecretRef) (map[string]string, error) {
	if len(refs) == 0 {
		return nil, nil
	}

	var uniqueRefs []SecretRef
	seen := make(map[string]bool, len(refs))
	for _, r := range refs {
		resName := r.ResourceName()
		if !seen[resName] {
			seen[resName] = true
			uniqueRefs = append(uniqueRefs, r)
		}
	}

	var clientOpts []option.ClientOption
	if ua, err := util.UserAgentFromContext(ctx); err == nil && ua != "" {
		clientOpts = append(clientOpts, option.WithUserAgent(ua))
	}

	client, err := newSecretManagerClient(ctx, clientOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Secret Manager client: %w", err)
	}
	defer func() {
		_ = client.Close()
	}()

	crc32cTable := crc32.MakeTable(crc32.Castagnoli)
	resolved := make(map[string]string, len(uniqueRefs))

	for _, ref := range uniqueRefs {
		resName := ref.ResourceName()
		resp, err := client.AccessSecretVersion(ctx, &secretmanagerpb.AccessSecretVersionRequest{
			Name: resName,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to access secret %q: %w", resName, err)
		}

		payload := resp.GetPayload()
		if payload == nil {
			return nil, fmt.Errorf("secret %q returned an empty payload", resName)
		}

		data := payload.GetData()
		if payload.DataCrc32C != nil {
			checksum := int64(crc32.Checksum(data, crc32cTable))
			if checksum != payload.GetDataCrc32C() {
				return nil, fmt.Errorf("secret %q failed CRC32C integrity check", resName)
			}
		}
		if !utf8.Valid(data) {
			return nil, fmt.Errorf("secret %q payload is not valid UTF-8", resName)
		}

		resolved[resName] = string(data)
	}

	return resolved, nil
}

// extractRefsFromYAMLValues decodes YAML documents and collects __REF__(...) references
// strictly from scalar string values (ignoring comments and YAML map keys).
func extractRefsFromYAMLValues(raw []byte) ([]SecretRef, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(raw), yaml.UseOrderedMap())
	var refs []SecretRef
	for {
		var doc any
		if err := decoder.Decode(&doc); err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		if err := collectSecretRefsFromValue(doc, &refs); err != nil {
			return nil, err
		}
	}
	return refs, nil
}

func collectSecretRefsFromValue(v any, out *[]SecretRef) error {
	switch val := v.(type) {
	case string:
		found, err := findInlineSecretRefs(val)
		if err != nil {
			return err
		}
		*out = append(*out, found...)
	case yaml.MapSlice:
		for _, item := range val {
			// Only inspect map values (map keys are ignored, matching Parameter Manager renderer).
			if err := collectSecretRefsFromValue(item.Value, out); err != nil {
				return err
			}
		}
	case []any:
		for _, elem := range val {
			if err := collectSecretRefsFromValue(elem, out); err != nil {
				return err
			}
		}
	}
	return nil
}

type colSpan struct {
	startCol int
	endCol   int
}

// templatizeYAMLPayload replaces active __REF__(...) value tokens in raw YAML with
// unique template variables ({{.var_<uuid>}} using custom delimiters) for safetext/yamltemplate.
func templatizeYAMLPayload(raw []byte, refs []SecretRef, leftDelim, rightDelim string) (string, map[string]SecretRef, error) {
	input := strings.ReplaceAll(string(raw), "\r\n", "\n")
	input = strings.ReplaceAll(input, "\r", "\n")

	uidByResource := make(map[string]string, len(refs))
	refByUID := make(map[string]SecretRef, len(refs))
	for _, ref := range refs {
		resName := ref.ResourceName()
		if _, exists := uidByResource[resName]; !exists {
			uid := randomTemplateVarID()
			uidByResource[resName] = uid
			refByUID[uid] = ref
		}
	}

	commentColByLine := make(map[int]int)
	keySpansByLine := make(map[int][]colSpan)
	for _, t := range lexer.Tokenize(input) {
		if t.Position == nil {
			continue
		}
		if t.Type == token.CommentType {
			if existingCol, exists := commentColByLine[t.Position.Line]; !exists || t.Position.Column < existingCol {
				commentColByLine[t.Position.Line] = t.Position.Column
			}
		} else if t.NextType() == token.MappingValueType {
			keyLen := utf8.RuneCountInString(t.Value)
			if keyLen < len(t.Origin) {
				keyLen = utf8.RuneCountInString(strings.TrimSpace(t.Origin))
			}
			keySpansByLine[t.Position.Line] = append(keySpansByLine[t.Position.Line], colSpan{
				startCol: t.Position.Column,
				endCol:   t.Position.Column + keyLen,
			})
		}
	}

	matches := rawRefTokenRegex.FindAllStringIndex(input, -1)
	var out strings.Builder
	lastIndex := 0
	line, column := 1, 1
	scannedBytes := 0

	for _, match := range matches {
		start, end := match[0], match[1]
		for _, r := range input[scannedBytes:start] {
			if r == '\n' {
				line++
				column = 1
			} else {
				column++
			}
		}
		scannedBytes = start

		if commentCol, ok := commentColByLine[line]; ok && column >= commentCol {
			out.WriteString(input[lastIndex:end])
			lastIndex = end
			continue
		}

		inKey := false
		for _, ks := range keySpansByLine[line] {
			if column >= ks.startCol && column <= ks.endCol {
				inKey = true
				break
			}
		}
		if inKey {
			out.WriteString(input[lastIndex:end])
			lastIndex = end
			continue
		}

		tokenStr := input[start:end]
		openLoc := refOpenRegex.FindStringIndex(tokenStr)
		if openLoc == nil {
			out.WriteString(input[lastIndex:end])
			lastIndex = end
			continue
		}
		rawInner := tokenStr[openLoc[1] : len(tokenStr)-1]
		ref, err := ParseInlineSecretRef(rawInner)
		if err != nil {
			return "", nil, err
		}
		uid, ok := uidByResource[ref.ResourceName()]
		if !ok {
			out.WriteString(input[lastIndex:end])
			lastIndex = end
			continue
		}

		out.WriteString(input[lastIndex:start])
		out.WriteString(fmt.Sprintf("%s .%s %s", leftDelim, uid, rightDelim))
		lastIndex = end
	}
	out.WriteString(input[lastIndex:])

	return out.String(), refByUID, nil
}

// ResolveSecretRefsInYAML locates inline __REF__(...) placeholders in YAML scalar values,
// fetches each unique secret from Google Cloud Secret Manager, and renders the YAML using
// github.com/google/safetext/yamltemplate to prevent YAML injection.
// When skipFetch is true (used by offline commands like skills-generate), syntax and template
// structure are validated without making Secret Manager network calls.
func ResolveSecretRefsInYAML(ctx context.Context, raw []byte, skipFetch bool) ([]byte, error) {
	if !bytes.Contains(raw, []byte(refKey)) {
		return raw, nil
	}

	refs, err := extractRefsFromYAMLValues(raw)
	if err != nil {
		return nil, err
	}
	if len(refs) == 0 {
		return raw, nil
	}

	// Custom per-invocation delimiters avoid colliding with existing {{.param}} Go templates
	// inside MCP Toolbox prompts and HTTP/SQL tools.
	delimToken := strings.ReplaceAll(uuid.NewString(), "-", "_")
	leftDelim := fmt.Sprintf("{%%__SM_REF_%s__", delimToken)
	rightDelim := fmt.Sprintf("__%s%%}", delimToken)

	templatizedPayload, refByUID, err := templatizeYAMLPayload(raw, refs, leftDelim, rightDelim)
	if err != nil {
		return nil, err
	}

	tmpl, err := yamltemplate.New(uuid.NewString()).Delims(leftDelim, rightDelim).Parse(templatizedPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to parse YAML template for secret references: %w", err)
	}

	if skipFetch {
		return raw, nil
	}

	resolvedByResource, err := FetchSecretRefs(ctx, refs)
	if err != nil {
		return nil, err
	}

	dataMap := make(map[string]string, len(refByUID))
	for uid, ref := range refByUID {
		dataMap[uid] = resolvedByResource[ref.ResourceName()]
	}

	var out bytes.Buffer
	if err := tmpl.Execute(&out, dataMap); err != nil {
		if errors.Is(err, yamltemplate.ErrYAMLInjection) {
			return nil, fmt.Errorf("YAML injection detected while resolving secret references: %w", err)
		}
		return nil, fmt.Errorf("failed to substitute secret references in YAML: %w", err)
	}

	return out.Bytes(), nil
}
