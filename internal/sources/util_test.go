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

package sources

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

func TestNormalizeValue(t *testing.T) {
	// Test case 1: UUID OID (2950) with [16]byte
	uuidBytes := [16]byte{1, 158, 103, 138, 45, 22, 120, 229, 146, 32, 191, 118, 245, 114, 247, 50}
	expectedUUIDStr := "019e678a-2d16-78e5-9220-bf76f572f732"

	result := NormalizeValue(uuidBytes, 2950)
	if str, ok := result.(string); !ok || str != expectedUUIDStr {
		t.Errorf("Expected UUID string %s, got %v", expectedUUIDStr, result)
	}

	// Test case 2: UUID OID (2950) with other type
	result = NormalizeValue("not-bytes", 2950)
	if str, ok := result.(string); !ok || str != "not-bytes" {
		t.Errorf("Expected original string 'not-bytes', got %v", result)
	}

	// Test case 3: Other OID
	result = NormalizeValue(uuidBytes, 1000)
	if bytes, ok := result.([16]byte); !ok || bytes != uuidBytes {
		t.Errorf("Expected original [16]byte array, got %v", result)
	}

	// Test case 4: UUID Array OID (2951) with [][16]byte
	uuidArray := [][16]byte{uuidBytes}
	result = NormalizeValue(uuidArray, 2951)
	if arr, ok := result.([]string); !ok || len(arr) != 1 || arr[0] != expectedUUIDStr {
		t.Errorf("Expected []string with %s, got %v", expectedUUIDStr, result)
	}

	// Test case 5: UUID Array OID (2951) with []any
	uuidAnyArray := []any{uuidBytes, "not-a-uuid"}
	result = NormalizeValue(uuidAnyArray, 2951)
	if arr, ok := result.([]any); !ok || len(arr) != 2 || arr[0] != expectedUUIDStr || arr[1] != "not-a-uuid" {
		t.Errorf("Expected []any with normalized UUID, got %v", result)
	}
}

type stubTokenSource struct {
	tok   *oauth2.Token
	err   error
	onTok func()
}

func (s stubTokenSource) Token() (*oauth2.Token, error) {
	if s.onTok != nil {
		s.onTok()
	}
	return s.tok, s.err
}

func revokedGrantError() error {
	return &oauth2.RetrieveError{
		Response:  &http.Response{StatusCode: http.StatusBadRequest},
		ErrorCode: "invalid_grant",
	}
}

const (
	adcOldJSON     = `{"type":"authorized_user","client_id":"id","refresh_token":"old","quota_project_id":"proj-a"}`
	adcRotatedJSON = `{"type":"authorized_user","client_id":"id","refresh_token":"new","quota_project_id":"proj-a"}`
)

func newTestReloader(cur oauth2.TokenSource, curJSON string, discover func() (*google.Credentials, error)) *reloadingTokenSource {
	var j []byte
	if curJSON != "" {
		j = []byte(curJSON)
	}
	return &reloadingTokenSource{
		cur:              cur,
		curJSON:          j,
		baseJSON:         j,
		discover:         discover,
		baseQuotaProject: adcQuotaProjectFromJSON(j),
	}
}

func TestReloadingTokenSource_HappyPathNoReload(t *testing.T) {
	r := newTestReloader(
		stubTokenSource{tok: &oauth2.Token{AccessToken: "cached"}},
		adcOldJSON,
		func() (*google.Credentials, error) {
			t.Error("discover must not be called on the happy path")
			return nil, errors.New("unreachable")
		},
	)
	tok, err := r.Token()
	if err != nil {
		t.Fatalf("Token() returned error, want success: %v", err)
	}
	if tok.AccessToken != "cached" {
		t.Errorf("AccessToken = %q, want %q", tok.AccessToken, "cached")
	}
}

func TestReloadingTokenSource_ReloadsWhenDiskChanged(t *testing.T) {
	tcs := []struct {
		desc    string
		mintErr error
	}{
		{"revoked grant", revokedGrantError()},
		{"opaque wrapper error", errors.New("oauth2/google: unable to generate access token: invalid_grant")},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			fresh := stubTokenSource{tok: &oauth2.Token{AccessToken: "fresh"}}
			var discoveries int
			r := newTestReloader(
				stubTokenSource{err: tc.mintErr},
				adcOldJSON,
				func() (*google.Credentials, error) {
					discoveries++
					return &google.Credentials{TokenSource: fresh, JSON: []byte(adcRotatedJSON)}, nil
				},
			)
			tok, err := r.Token()
			if err != nil {
				t.Fatalf("Token() returned error, want successful reload: %v", err)
			}
			if tok.AccessToken != "fresh" {
				t.Errorf("AccessToken = %q, want %q", tok.AccessToken, "fresh")
			}
			if discoveries != 1 {
				t.Errorf("discover called %d times, want 1", discoveries)
			}
			if !strings.Contains(string(r.curJSON), `"refresh_token":"new"`) {
				t.Error("expected curJSON to be updated to the reloaded ADC JSON")
			}
			if r.gen != 1 {
				t.Errorf("gen = %d, want 1 after one reload", r.gen)
			}
		})
	}
}

func TestReloadingTokenSource_NoReloadWhenDiskUnchanged(t *testing.T) {
	tcs := []struct {
		desc    string
		curJSON string
		mintErr error
	}{
		{"server error", adcOldJSON, &oauth2.RetrieveError{Response: &http.Response{StatusCode: http.StatusInternalServerError}}},
		{"rate limited", adcOldJSON, &oauth2.RetrieveError{Response: &http.Response{StatusCode: http.StatusTooManyRequests}}},
		{"network drop", adcOldJSON, errors.New("net/http: TLS handshake timeout")},
		{"revoked grant, no re-auth yet", adcOldJSON, revokedGrantError()},
		{"JSON-less metadata-server credentials", "", errors.New("metadata: connection error")},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			cur := stubTokenSource{err: tc.mintErr}
			r := newTestReloader(cur, tc.curJSON, func() (*google.Credentials, error) {
				var j []byte
				if tc.curJSON != "" {
					j = []byte(tc.curJSON)
				}
				return &google.Credentials{TokenSource: stubTokenSource{tok: &oauth2.Token{AccessToken: "must-not-serve"}}, JSON: j}, nil
			})
			_, err := r.Token()
			if !errors.Is(err, tc.mintErr) {
				t.Errorf("Token() error = %v, want the original mint error %v", err, tc.mintErr)
			}
			if r.gen != 0 {
				t.Error("token source was swapped although the ADC on disk did not change")
			}
		})
	}
}

func TestReloadingTokenSource_DiscoverFailurePropagates(t *testing.T) {
	mintErr := revokedGrantError()
	discoverErr := errors.New("ADC gone from disk")
	r := newTestReloader(stubTokenSource{err: mintErr}, adcOldJSON, func() (*google.Credentials, error) {
		return nil, discoverErr
	})
	tok, err := r.Token()
	if err == nil {
		t.Fatalf("Token() returned nil error, want discovery failure; token=%v", tok)
	}
	if !strings.Contains(err.Error(), "invalid_grant") {
		t.Errorf("Token() error %q does not mention the mint failure", err)
	}
	if !errors.Is(err, discoverErr) {
		t.Errorf("Token() error %q does not wrap the discovery error", err)
	}
}

func TestReloadingTokenSource_ReloadedSourceAlsoFails(t *testing.T) {
	stillBroken := errors.New("still broken after reload")
	r := newTestReloader(stubTokenSource{err: revokedGrantError()}, adcOldJSON, func() (*google.Credentials, error) {
		return &google.Credentials{TokenSource: stubTokenSource{err: stillBroken}, JSON: []byte(adcRotatedJSON)}, nil
	})
	_, err := r.Token()
	if !errors.Is(err, stillBroken) {
		t.Fatalf("Token() error = %v, want the reloaded source's mint error", err)
	}
	if !strings.Contains(err.Error(), "reloaded") {
		t.Errorf("Token() error %q does not say a reload already happened", err)
	}
}

func TestReloadingTokenSource_ConcurrentReload(t *testing.T) {
	stillBroken := errors.New("still broken after reload")
	tcs := []struct {
		desc    string
		tok     *oauth2.Token
		wantErr error
	}{
		{
			desc: "reloaded source succeeds",
			tok:  &oauth2.Token{AccessToken: "fresh"},
		},
		{
			desc:    "reloaded source fails",
			wantErr: stillBroken,
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			const n = 32
			var entered sync.WaitGroup
			entered.Add(n)
			release := make(chan struct{})
			var discoveries atomic.Int32
			initial := stubTokenSource{
				err: revokedGrantError(),
				onTok: func() {
					entered.Done()
					<-release
				},
			}
			var r *reloadingTokenSource
			reloaded := stubTokenSource{
				tok: tc.tok,
				err: tc.wantErr,
				onTok: func() {
					r.mu.Lock()
					gen := r.gen
					r.mu.Unlock()
					if gen != 1 {
						t.Errorf("reloaded Token() saw gen = %d, want 1", gen)
					}
				},
			}
			r = newTestReloader(initial, adcOldJSON, func() (*google.Credentials, error) {
				discoveries.Add(1)
				return &google.Credentials{TokenSource: reloaded, JSON: []byte(adcRotatedJSON)}, nil
			})
			var wg sync.WaitGroup
			wg.Add(n)
			for range n {
				go func() {
					defer wg.Done()
					tok, err := r.Token()
					if tc.wantErr != nil {
						if !errors.Is(err, tc.wantErr) {
							t.Errorf("concurrent Token() error = %v, want %v", err, tc.wantErr)
						}
						return
					}
					if err != nil || tok.AccessToken != "fresh" {
						t.Errorf("concurrent Token() = (%v, %v), want (fresh, nil)", tok, err)
					}
				}()
			}
			entered.Wait()
			close(release)
			wg.Wait()
			if got := discoveries.Load(); got != 1 {
				t.Errorf("discover ran %d times, want 1", got)
			}
		})
	}
}

func TestCheckADCDrift(t *testing.T) {
	tcs := []struct {
		desc      string
		baseJSON  string
		pinned    string
		skipQuota bool
		newJSON   string
		wantErr   string
	}{
		{
			desc:     "routine re-auth, only the refresh token changed",
			baseJSON: adcOldJSON,
			newJSON:  adcRotatedJSON,
		},
		{
			desc:     "service account key rotation keeps the principal",
			baseJSON: `{"type":"service_account","client_email":"sa@p.iam.gserviceaccount.com","private_key_id":"k1","private_key":"pem1"}`,
			newJSON:  `{"type":"service_account","client_email":"sa@p.iam.gserviceaccount.com","private_key_id":"k2","private_key":"pem2"}`,
		},
		{
			desc:     "impersonated_service_account routine re-auth rotates nested source_credentials.refresh_token",
			baseJSON: `{"type":"impersonated_service_account","service_account_impersonation_url":"https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/sa@p.iam.gserviceaccount.com:generateAccessToken","source_credentials":{"type":"authorized_user","client_id":"id","client_secret":"s","refresh_token":"old"}}`,
			newJSON:  `{"type":"impersonated_service_account","service_account_impersonation_url":"https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/sa@p.iam.gserviceaccount.com:generateAccessToken","source_credentials":{"type":"authorized_user","client_id":"id","client_secret":"s","refresh_token":"new"}}`,
		},
		{
			desc:     "impersonated_service_account nested source_credentials.client_id drift is refused",
			baseJSON: `{"type":"impersonated_service_account","service_account_impersonation_url":"https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/sa@p.iam.gserviceaccount.com:generateAccessToken","source_credentials":{"type":"authorized_user","client_id":"id","client_secret":"s","refresh_token":"old"}}`,
			newJSON:  `{"type":"impersonated_service_account","service_account_impersonation_url":"https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/sa@p.iam.gserviceaccount.com:generateAccessToken","source_credentials":{"type":"authorized_user","client_id":"other-id","client_secret":"s","refresh_token":"new"}}`,
			wantErr:  "the source_credentials.client_id field on disk changed",
		},
		{
			desc:     "impersonated_service_account malformed source_credentials is refused",
			baseJSON: `{"type":"impersonated_service_account","source_credentials":{"type":"authorized_user","client_id":"id","refresh_token":"old"}}`,
			newJSON:  `{"type":"impersonated_service_account","source_credentials":null}`,
			wantErr:  "the source_credentials field on disk changed",
		},
		{
			desc:     "reformatted whitespace and key order in nested object is not drift",
			baseJSON: `{"type":"external_account","audience":"//iam.googleapis.com/x","credential_source":{"file":"/var/run/token","format":{"type":"text"}}}`,
			newJSON:  "{\n  \"type\": \"external_account\",\n  \"audience\": \"//iam.googleapis.com/x\",\n  \"credential_source\": {\n    \"format\": { \"type\": \"text\" },\n    \"file\": \"/var/run/token\"\n  }\n}",
		},
		{
			desc:     "universe_domain spelled out as the default is not drift",
			baseJSON: `{"type":"service_account","client_email":"sa@p.iam.gserviceaccount.com"}`,
			newJSON:  `{"type":"service_account","client_email":"sa@p.iam.gserviceaccount.com","universe_domain":"googleapis.com"}`,
		},
		{
			desc:     "credential type changed",
			baseJSON: adcOldJSON,
			newJSON:  `{"type":"service_account","client_id":"id","refresh_token":"x","quota_project_id":"proj-a"}`,
			wantErr:  "the type field on disk changed",
		},
		{
			desc:     "service account principal changed",
			baseJSON: `{"type":"service_account","client_email":"a@p.iam.gserviceaccount.com"}`,
			newJSON:  `{"type":"service_account","client_email":"b@p.iam.gserviceaccount.com"}`,
			wantErr:  "the client_email field on disk changed",
		},
		{
			desc:     "token endpoint changed",
			baseJSON: `{"type":"service_account","client_email":"sa@p.iam.gserviceaccount.com","token_uri":"https://oauth2.googleapis.com/token"}`,
			newJSON:  `{"type":"service_account","client_email":"sa@p.iam.gserviceaccount.com","token_uri":"https://attacker.example/token"}`,
			wantErr:  "the token_uri field on disk changed",
		},
		{
			desc:     "workload-identity credential_source changed",
			baseJSON: `{"type":"external_account","audience":"//iam.googleapis.com/x","credential_source":{"file":"/var/run/token"}}`,
			newJSON:  `{"type":"external_account","audience":"//iam.googleapis.com/x","credential_source":{"file":"/etc/shadow"}}`,
			wantErr:  "the credential_source field on disk changed",
		},
		{
			desc:     "universe domain changed to a sovereign cloud",
			baseJSON: `{"type":"service_account","client_email":"sa@p.iam.gserviceaccount.com"}`,
			newJSON:  `{"type":"service_account","client_email":"sa@p.iam.gserviceaccount.com","universe_domain":"example.sovereign.cloud"}`,
			wantErr:  "the universe_domain field on disk changed",
		},
		{
			desc:     "quota project changed and clients derive it from ADC",
			baseJSON: adcOldJSON,
			newJSON:  `{"type":"authorized_user","client_id":"id","refresh_token":"new","quota_project_id":"proj-b"}`,
			wantErr:  `"proj-a" to "proj-b"`,
		},
		{
			desc:     "quota project disappeared and clients derive it from ADC",
			baseJSON: adcOldJSON,
			newJSON:  `{"type":"authorized_user","client_id":"id","refresh_token":"new"}`,
			wantErr:  `"proj-a" to ""`,
		},
		{
			desc:     "quota project appeared where none was baked in",
			baseJSON: `{"type":"authorized_user","client_id":"id","refresh_token":"old"}`,
			newJSON:  `{"type":"authorized_user","client_id":"id","refresh_token":"new","quota_project_id":"proj-b"}`,
		},
		{
			desc:     "quota project changed but explicitly pinned on the clients",
			baseJSON: adcOldJSON,
			pinned:   "pinned-proj",
			newJSON:  `{"type":"authorized_user","client_id":"id","refresh_token":"new","quota_project_id":"proj-b"}`,
		},
		{
			desc:      "quota project changed but no client reads it from these credentials",
			baseJSON:  adcOldJSON,
			skipQuota: true,
			newJSON:   `{"type":"authorized_user","client_id":"id","refresh_token":"new","quota_project_id":"proj-b"}`,
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			r := newTestReloader(stubTokenSource{}, tc.baseJSON, nil)
			r.pinnedQuotaProject = tc.pinned
			r.skipQuotaGuard = tc.skipQuota
			err := r.checkADCDrift([]byte(tc.newJSON))
			if tc.wantErr == "" {
				if err != nil {
					t.Errorf("checkADCDrift() = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("checkADCDrift() = %v, want error containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestReloadingTokenSource_NilSafety(t *testing.T) {
	r := newTestReloader(nil, adcOldJSON, func() (*google.Credentials, error) {
		return &google.Credentials{TokenSource: nil, JSON: []byte(adcRotatedJSON)}, nil
	})
	if _, err := r.Token(); err == nil || !strings.Contains(err.Error(), "no token source") {
		t.Errorf("Token() = %v, want error about missing token source", err)
	}
}

func TestReloadingTokenSource_RefusalIsTerminal(t *testing.T) {
	drifted := `{"type":"authorized_user","client_id":"other-client","refresh_token":"new","quota_project_id":"proj-a"}`
	r := newTestReloader(stubTokenSource{err: revokedGrantError()}, adcOldJSON, func() (*google.Credentials, error) {
		return &google.Credentials{TokenSource: stubTokenSource{tok: &oauth2.Token{AccessToken: "must-not-serve"}}, JSON: []byte(drifted)}, nil
	})
	for i := range 2 {
		_, err := r.Token()
		if err == nil || !strings.Contains(err.Error(), "client_id") {
			t.Fatalf("call %d: Token() = %v, want the client_id drift refusal", i+1, err)
		}
	}
	if r.gen != 0 {
		t.Error("a refused reload must not swap the token source")
	}
}

type countingTokenSource struct {
	tok   *oauth2.Token
	calls atomic.Int32
}

func (c *countingTokenSource) Token() (*oauth2.Token, error) {
	c.calls.Add(1)
	return c.tok, nil
}

func TestNewReloadingTokenSource_CachesValidToken(t *testing.T) {
	initial := &countingTokenSource{tok: &oauth2.Token{AccessToken: "cached", Expiry: time.Now().Add(time.Hour)}}
	ts := newReloadingTokenSource(context.Background(), &google.Credentials{TokenSource: initial}, ADCReloadParams{})
	for range 3 {
		tok, err := ts.Token()
		if err != nil || tok.AccessToken != "cached" {
			t.Fatalf("Token() = (%v, %v), want (cached, nil)", tok, err)
		}
	}
	if got := initial.calls.Load(); got != 1 {
		t.Errorf("initial source called %d times, want 1", got)
	}
}

func TestNewReloader_QuotaProjectPinning(t *testing.T) {
	t.Setenv(quotaProjectEnvVar, "env-proj")
	tcs := []struct {
		desc       string
		pin        string
		wantPinned string
	}{
		{"env var pins when no explicit pin", "", "env-proj"},
		{"explicit pin wins over the env var", "arg-proj", "arg-proj"},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			r := newReloader(context.Background(), &google.Credentials{
				TokenSource: stubTokenSource{},
				JSON:        []byte(adcOldJSON),
			}, ADCReloadParams{QuotaProjectPin: tc.pin})
			if r.pinnedQuotaProject != tc.wantPinned {
				t.Errorf("pinnedQuotaProject = %q, want %q", r.pinnedQuotaProject, tc.wantPinned)
			}
		})
	}
}

func TestNewReloader_ParamsWiring(t *testing.T) {
	r := newReloader(context.Background(), &google.Credentials{
		TokenSource: stubTokenSource{},
		JSON:        []byte(adcOldJSON),
	}, ADCReloadParams{Name: "my-source", SkipQuotaProjectGuard: true})
	if r.name != "my-source" {
		t.Errorf("name = %q, want %q", r.name, "my-source")
	}
	if !r.skipQuotaGuard {
		t.Error("skipQuotaGuard = false, want true")
	}
}

func TestNewReloader_EnvQuotaProjectPinAllowsDrift(t *testing.T) {
	t.Setenv(quotaProjectEnvVar, "env-proj")
	fresh := stubTokenSource{tok: &oauth2.Token{AccessToken: "fresh"}}
	r := newReloader(context.Background(), &google.Credentials{
		TokenSource: stubTokenSource{err: revokedGrantError()},
		JSON:        []byte(adcOldJSON),
	}, ADCReloadParams{})
	r.discover = func() (*google.Credentials, error) {
		return &google.Credentials{TokenSource: fresh, JSON: []byte(`{"type":"authorized_user","client_id":"id","refresh_token":"new","quota_project_id":"proj-b"}`)}, nil
	}
	tok, err := r.Token()
	if err != nil {
		t.Fatalf("Token() = %v, want reload to proceed", err)
	}
	if tok.AccessToken != "fresh" {
		t.Errorf("AccessToken = %q, want %q", tok.AccessToken, "fresh")
	}
}

func TestNewReloadingDefaultCredentials_DiscoveryError(t *testing.T) {
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", filepath.Join(t.TempDir(), "missing.json"))
	_, err := NewReloadingDefaultCredentials(context.Background(), ADCReloadParams{Name: "test"})
	if err == nil || !strings.Contains(err.Error(), "failed to find default Google Cloud credentials") {
		t.Errorf("NewReloadingDefaultCredentials() error = %v, want discovery failure", err)
	}
}

func TestNewReloadingTokenSource_DiskIntegration(t *testing.T) {
	tcs := []struct {
		desc     string
		diskJSON string
		wantErr  string
	}{
		{
			desc:     "unchanged file returns the mint error untouched",
			diskJSON: `{"type":"authorized_user","client_id":"id","client_secret":"s","refresh_token":"old","quota_project_id":"proj-a"}`,
			wantErr:  "invalid_grant",
		},
		{
			desc:     "quota project drift is refused with both projects named",
			diskJSON: `{"type":"authorized_user","client_id":"id","client_secret":"s","refresh_token":"t","quota_project_id":"proj-b"}`,
			wantErr:  `"proj-a" to "proj-b"`,
		},
		{
			desc:     "identity drift is refused naming the field",
			diskJSON: `{"type":"authorized_user","client_id":"other-client","client_secret":"s","refresh_token":"t","quota_project_id":"proj-a"}`,
			wantErr:  "client_id field on disk changed",
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			adcPath := filepath.Join(t.TempDir(), "adc.json")
			if err := os.WriteFile(adcPath, []byte(tc.diskJSON), 0o600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", adcPath)
			t.Setenv(quotaProjectEnvVar, "")
			ts := newReloadingTokenSource(context.Background(), &google.Credentials{
				TokenSource: stubTokenSource{err: revokedGrantError()},
				JSON:        []byte(`{"type":"authorized_user","client_id":"id","client_secret":"s","refresh_token":"old","quota_project_id":"proj-a"}`),
			}, ADCReloadParams{Name: "disk-integration"})
			_, err := ts.Token()
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("Token() = %v, want error containing %q", err, tc.wantErr)
			}
		})
	}

	t.Run("rotated refresh token on disk reloads and mints a fresh token", func(t *testing.T) {
		var tokenCalls atomic.Int32
		tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenCalls.Add(1)
			if err := r.ParseForm(); err != nil || r.FormValue("refresh_token") != "new" {
				http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"fresh-from-disk","token_type":"Bearer","expires_in":3600}`))
		}))
		defer tokenSrv.Close()

		baseJSON := fmt.Sprintf(
			`{"type":"authorized_user","client_id":"id","client_secret":"s","refresh_token":"old","quota_project_id":"proj-a","token_uri":%q}`,
			tokenSrv.URL,
		)
		diskJSON := fmt.Sprintf(
			`{"type":"authorized_user","client_id":"id","client_secret":"s","refresh_token":"new","quota_project_id":"proj-a","token_uri":%q}`,
			tokenSrv.URL,
		)

		adcPath := filepath.Join(t.TempDir(), "adc.json")
		if err := os.WriteFile(adcPath, []byte(diskJSON), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", adcPath)
		t.Setenv(quotaProjectEnvVar, "")

		ts := newReloadingTokenSource(context.Background(), &google.Credentials{
			TokenSource: stubTokenSource{err: revokedGrantError()},
			JSON:        []byte(baseJSON),
		}, ADCReloadParams{Name: "disk-integration"})

		for range 2 {
			tok, err := ts.Token()
			if err != nil {
				t.Fatalf("Token() returned error, want success: %v", err)
			}
			if tok.AccessToken != "fresh-from-disk" {
				t.Errorf("AccessToken = %q, want %q", tok.AccessToken, "fresh-from-disk")
			}
		}
		if got := tokenCalls.Load(); got != 1 {
			t.Errorf("token endpoint called %d times, want 1 (cached by ReuseTokenSource)", got)
		}
	})
}
