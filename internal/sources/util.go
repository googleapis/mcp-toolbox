// Copyright 2025 Google LLC
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

package sources

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"reflect"
	"sort"
	"strings"
	"sync"

	"cloud.google.com/go/cloudsqlconn"
	"github.com/google/uuid"
	"github.com/googleapis/mcp-toolbox/internal/log"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// NormalizeValue converts specific database types to friendly representations (e.g. UUID to string).
func NormalizeValue(val any, oid uint32) any {
	// 2950 is the standard OID for UUID in PostgreSQL
	if oid == 2950 {
		if b, ok := val.([16]byte); ok {
			return uuid.UUID(b).String()
		}
	}
	// 2951 is the standard OID for _uuid (UUID array) in PostgreSQL
	if oid == 2951 {
		if arr, ok := val.([][16]byte); ok {
			strs := make([]string, len(arr))
			for i, b := range arr {
				strs[i] = uuid.UUID(b).String()
			}
			return strs
		}
		if arr, ok := val.([]any); ok {
			strs := make([]any, len(arr))
			for i, item := range arr {
				if b, ok := item.([16]byte); ok {
					strs[i] = uuid.UUID(b).String()
				} else {
					strs[i] = item
				}
			}
			return strs
		}
	}
	return val
}

// GetCloudSQLDialOpts retrieve dial options with the right ip type and user agent for cloud sql
// databases.
func GetCloudSQLOpts(ipType, userAgent string, useIAM bool) ([]cloudsqlconn.Option, error) {
	opts := []cloudsqlconn.Option{cloudsqlconn.WithUserAgent(userAgent)}
	switch strings.ToLower(ipType) {
	case "private":
		opts = append(opts, cloudsqlconn.WithDefaultDialOptions(cloudsqlconn.WithPrivateIP()))
	case "public":
		opts = append(opts, cloudsqlconn.WithDefaultDialOptions(cloudsqlconn.WithPublicIP()))
	case "psc":
		opts = append(opts, cloudsqlconn.WithDefaultDialOptions(cloudsqlconn.WithPSC()))
	default:
		return nil, fmt.Errorf("invalid ipType %s. Must be one of `public`, `private`, or `psc`", ipType)
	}

	if useIAM {
		opts = append(opts, cloudsqlconn.WithIAMAuthN())
	}
	return opts, nil
}

// GetIAMPrincipalEmailFromADC finds the email associated with ADC
func GetIAMPrincipalEmailFromADC(ctx context.Context, dbType string) (string, error) {
	// Finds ADC and returns an HTTP client associated with it
	client, err := google.DefaultClient(ctx,
		"https://www.googleapis.com/auth/userinfo.email")
	if err != nil {
		return "", fmt.Errorf("failed to call userinfo endpoint: %w", err)
	}

	// Retrieve the email associated with the token
	resp, err := client.Get("https://oauth2.googleapis.com/tokeninfo")
	if err != nil {
		return "", fmt.Errorf("failed to call tokeninfo endpoint: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading response body %d: %s", resp.StatusCode, string(bodyBytes))
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("tokeninfo endpoint returned non-OK status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	// Unmarshal response body and get `email`
	var responseJSON map[string]any
	err = json.Unmarshal(bodyBytes, &responseJSON)
	if err != nil {

		return "", fmt.Errorf("error parsing JSON: %v", err)
	}

	emailValue, ok := responseJSON["email"]
	if !ok {
		return "", fmt.Errorf("email not found in response: %v", err)
	}

	fullEmail, ok := emailValue.(string)
	if !ok {
		return "", fmt.Errorf("email field is not a string")
	}

	var username string
	// Format the username based on Database Type
	switch strings.ToLower(dbType) {
	case "mysql":
		username, _, _ = strings.Cut(fullEmail, "@")

	case "postgres":
		// service account email used for IAM should trim the suffix
		username = strings.TrimSuffix(fullEmail, ".gserviceaccount.com")

	default:
		return "", fmt.Errorf("unsupported dbType: %s. Use 'mysql' or 'postgres'", dbType)
	}

	if username == "" {
		return "", fmt.Errorf("username from ADC cannot be an empty string")
	}

	return username, nil
}

func GetIAMAccessToken(ctx context.Context) (string, error) {
	creds, err := google.FindDefaultCredentials(ctx, "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return "", fmt.Errorf("failed to find default credentials (run 'gcloud auth application-default login'?): %w", err)
	}

	token, err := creds.TokenSource.Token() // This gets an oauth2.Token
	if err != nil {
		return "", fmt.Errorf("failed to get token from token source: %w", err)
	}

	if !token.Valid() {
		return "", fmt.Errorf("retrieved token is invalid or expired")
	}
	return token.AccessToken, nil
}

// reloadingTokenSource wraps an ADC token source and reloads credentials from
// disk when a token mint fails and the on-disk ADC JSON has changed.
type reloadingTokenSource struct {
	mu                 sync.Mutex
	gen                uint64
	cur                oauth2.TokenSource
	curJSON            []byte
	baseJSON           []byte
	discover           func() (*google.Credentials, error)
	name               string
	logger             log.Logger
	pinnedQuotaProject string
	skipQuotaGuard     bool
	baseQuotaProject   string
}

func (r *reloadingTokenSource) Token() (*oauth2.Token, error) {
	r.mu.Lock()
	cur, gen := r.cur, r.gen
	r.mu.Unlock()
	var tok *oauth2.Token
	var mintErr error
	if cur != nil {
		tok, mintErr = cur.Token()
		if mintErr == nil {
			return tok, nil
		}
	} else {
		mintErr = errors.New("current token source is nil")
	}

	nextTS, err := func() (oauth2.TokenSource, error) {
		r.mu.Lock()
		defer r.mu.Unlock()
		if r.gen != gen && r.cur != nil {
			return r.cur, nil
		}
		newCred, err := r.discover()
		if err != nil {
			return nil, fmt.Errorf("failed to reload default credentials after a mint failure (%v): %w", mintErr, err)
		}
		if newCred == nil || newCred.TokenSource == nil {
			return nil, fmt.Errorf("failed to reload default credentials after a mint failure (%v): discovered credentials have no token source", mintErr)
		}
		if bytes.Equal(newCred.JSON, r.curJSON) {
			return nil, mintErr
		}
		if err := r.checkADCDrift(newCred.JSON); err != nil {
			r.warn("refused to adopt reloaded application default credentials", "source", r.name, "error", err.Error())
			return nil, fmt.Errorf("%w (after mint failure: %v)", err, mintErr)
		}
		r.cur, r.curJSON = newCred.TokenSource, newCred.JSON
		r.gen++
		r.warn("adopted reloaded application default credentials from disk after a token mint failure", "source", r.name, "credentialType", adcFieldString(newCred.JSON, "type"))
		return r.cur, nil
	}()
	if err != nil {
		return nil, err
	}
	tok, err = nextTS.Token()
	if err != nil {
		return nil, fmt.Errorf("failed to mint a token from the reloaded default credentials: %w", err)
	}
	return tok, nil
}

func (r *reloadingTokenSource) warn(msg string, keysAndValues ...any) {
	if r.logger != nil {
		r.logger.WarnContext(context.Background(), msg, keysAndValues...)
		return
	}
	slog.Warn(msg, keysAndValues...)
}

// adcRotationFields lists top-level ADC JSON fields allowed to change across routine re-auth or key rotation.
var adcRotationFields = map[string]bool{
	"refresh_token":    true,
	"private_key":      true,
	"private_key_id":   true,
	"expiry":           true,
	"quota_project_id": true,
}

// checkADCDrift rejects reloaded credentials whose identity/endpoint fields or effective quota project changed.
func (r *reloadingTokenSource) checkADCDrift(newJSON []byte) error {
	if field := adcFieldDrift(r.baseJSON, newJSON); field != "" {
		return fmt.Errorf("failed to reload default credentials: the %s field on disk changed from what this server started with, which a routine re-auth would not do; restart the server to adopt the new credentials", field)
	}
	if !r.skipQuotaGuard && r.pinnedQuotaProject == "" && r.baseQuotaProject != "" {
		if qp := adcQuotaProjectFromJSON(newJSON); qp != r.baseQuotaProject {
			return fmt.Errorf("failed to reload default credentials: their quota project changed from %q to %q and clients built from this source still send the original value; restart the server to pick up the change", r.baseQuotaProject, qp)
		}
	}
	return nil
}

// adcFieldDrift returns the first non-rotation field that differs between baseJSON and newJSON, or "".
func adcFieldDrift(baseJSON, newJSON []byte) string {
	base, ok := adcFields(baseJSON)
	if !ok {
		return "unparseable baseline"
	}
	next, ok := adcFields(newJSON)
	if !ok {
		return "unparseable reload"
	}
	fields := make([]string, 0, len(base)+len(next))
	for f := range base {
		fields = append(fields, f)
	}
	for f := range next {
		if _, seen := base[f]; !seen {
			fields = append(fields, f)
		}
	}
	sort.Strings(fields)
	for _, f := range fields {
		if adcRotationFields[f] {
			continue
		}
		if f == "source_credentials" {
			if len(base[f]) == 0 || len(next[f]) == 0 {
				return f
			}
			if _, ok := adcFields(base[f]); !ok {
				return f
			}
			if _, ok := adcFields(next[f]); !ok {
				return f
			}
			if sub := adcFieldDrift(base[f], next[f]); sub != "" {
				return f + "." + sub
			}
			continue
		}
		if !jsonRawEqual(base[f], next[f]) {
			return f
		}
	}
	return ""
}

func jsonRawEqual(a, b json.RawMessage) bool {
	if bytes.Equal(a, b) {
		return true
	}
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	var va, vb any
	if err := json.Unmarshal(a, &va); err != nil {
		return false
	}
	if err := json.Unmarshal(b, &vb); err != nil {
		return false
	}
	return reflect.DeepEqual(va, vb)
}

func adcFields(credJSON []byte) (map[string]json.RawMessage, bool) {
	fields := map[string]json.RawMessage{}
	if len(credJSON) == 0 {
		return fields, true
	}
	if err := json.Unmarshal(credJSON, &fields); err != nil || fields == nil {
		return nil, false
	}
	if _, ok := fields["universe_domain"]; !ok {
		fields["universe_domain"] = json.RawMessage(`"googleapis.com"`)
	}
	return fields, true
}

func adcFieldString(credJSON []byte, field string) string {
	fields, ok := adcFields(credJSON)
	if !ok {
		return ""
	}
	var v string
	if err := json.Unmarshal(fields[field], &v); err != nil {
		return ""
	}
	return v
}

const quotaProjectEnvVar = "GOOGLE_CLOUD_QUOTA_PROJECT"

// ADCReloadParams configures NewReloadingDefaultCredentials.
type ADCReloadParams struct {
	Name                  string
	Scopes                []string
	QuotaProjectPin       string
	SkipQuotaProjectGuard bool
}

// NewReloadingDefaultCredentials finds Application Default Credentials and wraps
// their TokenSource so rotated on-disk credentials are reloaded automatically on mint failure.
func NewReloadingDefaultCredentials(ctx context.Context, p ADCReloadParams) (*google.Credentials, error) {
	cred, err := google.FindDefaultCredentials(ctx, p.Scopes...)
	if err != nil {
		return nil, fmt.Errorf("failed to find default Google Cloud credentials with scopes %v: %w", p.Scopes, err)
	}
	cred.TokenSource = newReloadingTokenSource(ctx, cred, p)
	return cred, nil
}

func newReloadingTokenSource(ctx context.Context, cred *google.Credentials, p ADCReloadParams) oauth2.TokenSource {
	return oauth2.ReuseTokenSource(nil, newReloader(ctx, cred, p))
}

func newReloader(ctx context.Context, cred *google.Credentials, p ADCReloadParams) *reloadingTokenSource {
	pinned := p.QuotaProjectPin
	if pinned == "" {
		pinned = os.Getenv(quotaProjectEnvVar)
	}
	logger, err := util.LoggerFromContext(ctx)
	if err != nil {
		logger = nil
	}
	scopes := p.Scopes
	return &reloadingTokenSource{
		cur:      cred.TokenSource,
		curJSON:  cred.JSON,
		baseJSON: cred.JSON,
		discover: func() (*google.Credentials, error) {
			return google.FindDefaultCredentials(context.Background(), scopes...)
		},
		name:               p.Name,
		logger:             logger,
		pinnedQuotaProject: pinned,
		skipQuotaGuard:     p.SkipQuotaProjectGuard,
		baseQuotaProject:   adcQuotaProjectFromJSON(cred.JSON),
	}
}

func adcQuotaProjectFromJSON(credJSON []byte) string {
	if len(credJSON) == 0 {
		return ""
	}
	var f struct {
		QuotaProjectID string `json:"quota_project_id"`
	}
	if err := json.Unmarshal(credJSON, &f); err != nil {
		return ""
	}
	return f.QuotaProjectID
}
