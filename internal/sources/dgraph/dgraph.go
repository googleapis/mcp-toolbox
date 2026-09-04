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

package dgraph

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	"go.opentelemetry.io/otel/trace"
)

const SourceType string = "dgraph"

// validate interface
var _ sources.SourceConfig = Config{}

func init() {
	if !sources.Register(SourceType, newConfig) {
		panic(fmt.Sprintf("source type %q already registered", SourceType))
	}
}

func newConfig(ctx context.Context, name string, decoder *yaml.Decoder) (sources.SourceConfig, error) {
	actual := Config{Name: name}
	if err := decoder.DecodeContext(ctx, &actual); err != nil {
		return nil, err
	}
	return actual, nil
}

// HttpToken stores credentials for making HTTP request
type HttpToken struct {
	UserId       string
	Password     string
	AccessJwt    string
	RefreshToken string
	Namespace    uint64
}

type DgraphClient struct {
	httpClient *http.Client
	*HttpToken
	baseUrl string
	apiKey  string
}

type Config struct {
	Name      string `yaml:"name" validate:"required"`
	Type      string `yaml:"type" validate:"required"`
	DgraphUrl string `yaml:"dgraphUrl" validate:"required"`
	User      string `yaml:"user"`
	Password  string `yaml:"password"`
	Namespace uint64 `yaml:"namespace"`
	ApiKey    string `yaml:"apiKey"`
}

func (r Config) SourceConfigType() string {
	return SourceType
}

func (r Config) Initialize(ctx context.Context, tracer trace.Tracer, deferConnect bool) (sources.Source, error) {
	s := r.newSource(ctx, tracer)
	if deferConnect {
		return s, nil
	}
	if _, err := s.client(ctx); err != nil {
		return nil, err
	}
	return s, nil
}

func (r Config) newSource(ctx context.Context, tracer trace.Tracer) *Source {
	return &Source{
		Config: r,
		conn:   sources.NewConnectOnce[*DgraphClient](ctx, r.Name, SourceType, tracer),
	}
}

var _ sources.Source = &Source{}

type Source struct {
	Config
	conn *sources.ConnectOnce[*DgraphClient]
}

func (s *Source) client(ctx context.Context) (*DgraphClient, error) {
	return s.conn.Do(ctx, func(ctx context.Context) (*DgraphClient, error) {
		hc, err := initDgraphHttpClient(ctx, s.Config)
		if err != nil {
			return nil, err
		}

		if err := hc.healthCheck(); err != nil {
			return nil, err
		}
		return hc, nil
	})
}

func (s *Source) IsReadOnly() bool {
	return false
}

func (s *Source) SourceType() string {
	return SourceType
}

func (s *Source) ToConfig() sources.SourceConfig {
	return s.Config
}

// DgraphClient reports the client once connected.
func (s *Source) DgraphClient() *DgraphClient {
	client, _ := s.conn.Get()
	return client
}

func (s *Source) RunSQL(ctx context.Context, statement string, params parameters.ParamValues, isQuery bool, timeout string) (any, error) {
	client, err := s.client(ctx)
	if err != nil {
		return nil, err
	}

	paramsMap := params.AsMapWithDollarPrefix()
	resp, err := client.ExecuteQuery(statement, paramsMap, isQuery, timeout)
	if err != nil {
		return nil, err
	}

	if err := checkError(resp); err != nil {
		return nil, err
	}

	var result struct {
		Data map[string]interface{} `json:"data"`
	}

	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("error parsing JSON: %v", err)
	}

	return result.Data, nil
}

func initDgraphHttpClient(ctx context.Context, r Config) (*DgraphClient, error) {
	if r.DgraphUrl == "" {
		return nil, fmt.Errorf("dgraph url should not be empty")
	}

	hc := &DgraphClient{
		httpClient: &http.Client{},
		baseUrl:    r.DgraphUrl,
		HttpToken: &HttpToken{
			UserId:    r.User,
			Namespace: r.Namespace,
			Password:  r.Password,
		},
		apiKey: r.ApiKey,
	}

	if r.User != "" || r.Password != "" {
		if err := hc.loginWithCredentials(); err != nil {
			return nil, err
		}
	}

	return hc, nil
}

func (hc *DgraphClient) ExecuteQuery(query string, paramsMap map[string]interface{},
	isQuery bool, timeout string) ([]byte, error) {
	if isQuery {
		return hc.postDqlQuery(query, paramsMap, timeout)
	} else {
		return hc.mutate(query, paramsMap)
	}
}

// postDqlQuery sends a DQL query to the Dgraph server with query, parameters, and optional timeout.
// Returns the response body ([]byte) and an error, if any.
func (hc *DgraphClient) postDqlQuery(query string, paramsMap map[string]interface{}, timeout string) ([]byte, error) {
	urlParams := url.Values{}
	urlParams.Add("timeout", timeout)
	url, err := getUrl(hc.baseUrl, "/query", urlParams)
	if err != nil {
		return nil, err
	}
	p := struct {
		Query     string                 `json:"query"`
		Variables map[string]interface{} `json:"variables"`
	}{
		Query:     query,
		Variables: paramsMap,
	}
	body, err := json.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("error marshlling json: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("error building req for endpoint [%v] :%v", url, err)
	}

	req.Header.Add("Content-Type", "application/json")

	return hc.doReq(req)
}

// mutate sends an RDF mutation to the Dgraph server with "commitNow: true", embedding parameters.
// Returns the server's response as a byte slice or an error if the mutation fails.
func (hc *DgraphClient) mutate(mutation string, paramsMap map[string]interface{}) ([]byte, error) {
	mu := embedParamsIntoMutation(mutation, paramsMap)
	params := url.Values{}
	params.Add("commitNow", "true")
	url, err := getUrl(hc.baseUrl, "/mutate", params)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBufferString(mu))
	if err != nil {
		return nil, fmt.Errorf("error building req for endpoint [%v] :%v", url, err)
	}

	req.Header.Add("Content-Type", "application/rdf")

	return hc.doReq(req)
}

func (hc *DgraphClient) doReq(req *http.Request) ([]byte, error) {
	if hc.HttpToken != nil {
		req.Header.Add("X-Dgraph-AccessToken", hc.AccessJwt)
	}
	if hc.apiKey != "" {
		req.Header.Set("Dg-Auth", hc.apiKey)
	}

	resp, err := hc.httpClient.Do(req)

	if err != nil && !strings.Contains(err.Error(), "Token is expired") {
		return nil, fmt.Errorf("error performing HTTP request: %w", err)
	} else if err != nil && strings.Contains(err.Error(), "Token is expired") {
		if errLogin := hc.loginWithToken(); errLogin != nil {
			return nil, errLogin
		}
		if hc.HttpToken != nil {
			req.Header.Add("X-Dgraph-AccessToken", hc.AccessJwt)
		}
		resp, err = hc.httpClient.Do(req)
		if err != nil {
			return nil, err
		}
	}

	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: url: [%v], err: [%v]", req.URL, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("got non 200 resp: %v", string(respBody))
	}

	return respBody, nil
}

func (hc *DgraphClient) loginWithCredentials() error {
	credentials := map[string]interface{}{
		"userid":    hc.UserId,
		"password":  hc.Password,
		"namespace": hc.Namespace,
	}
	return hc.doLogin(credentials)
}

func (hc *DgraphClient) loginWithToken() error {
	credentials := map[string]interface{}{
		"refreshJWT": hc.RefreshToken,
		"namespace":  hc.Namespace,
	}
	return hc.doLogin(credentials)
}

func (hc *DgraphClient) doLogin(creds map[string]interface{}) error {
	url, err := getUrl(hc.baseUrl, "/login", nil)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(creds)
	if err != nil {
		return fmt.Errorf("failed to marshal credentials: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(payload))
	if err != nil {
		return fmt.Errorf("error building req for endpoint [%v] : %v", url, err)
	}
	req.Header.Add("Content-Type", "application/json")
	if hc.apiKey != "" {
		req.Header.Set("Dg-Auth", hc.apiKey)
	}

	resp, err := hc.doReq(req)
	if err != nil {
		if strings.Contains(err.Error(), "Token is expired") &&
			!strings.Contains(err.Error(), "unable to authenticate the refresh token") {
			return hc.loginWithToken()
		}
		return err
	}

	if err := checkError(resp); err != nil {
		return err
	}

	var r struct {
		Data struct {
			AccessJWT  string `json:"accessJWT"`
			RefreshJWT string `json:"refreshJWT"`
		} `json:"data"`
	}

	if err := json.Unmarshal(resp, &r); err != nil {
		return fmt.Errorf("failed to unmarshal response: %v", err)
	}

	if r.Data.AccessJWT == "" {
		return fmt.Errorf("no access JWT found in the response")
	}
	if r.Data.RefreshJWT == "" {
		return fmt.Errorf("no refresh JWT found in the response")
	}

	hc.AccessJwt = r.Data.AccessJWT
	hc.RefreshToken = r.Data.RefreshJWT
	return nil
}

func (hc *DgraphClient) healthCheck() error {
	url, err := getUrl(hc.baseUrl, "/health", nil)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}

	resp, err := hc.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("error performing request: %w", err)
	}

	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	var result []struct {
		Instance string `json:"instance"`
		Address  string `json:"address"`
		Status   string `json:"status"`
	}

	// Unmarshal response into the struct
	if err := json.Unmarshal(data, &result); err != nil {
		return fmt.Errorf("failed to unmarshal json: %v", err)
	}

	if len(result) == 0 {
		return fmt.Errorf("health info should not empty for: %v", url)
	}

	var unhealthyErr error
	for _, info := range result {
		if info.Status != "healthy" {
			unhealthyErr = fmt.Errorf("dgraph instance [%v] is not in healthy state, address is %v",
				info.Instance, info.Address)
		} else {
			return nil
		}
	}

	return unhealthyErr
}

func getUrl(baseUrl, resource string, params url.Values) (string, error) {
	u, err := url.ParseRequestURI(baseUrl)
	if err != nil {
		return "", fmt.Errorf("failed to get url %v", err)
	}
	u.Path = resource
	u.RawQuery = params.Encode()
	return u.String(), nil
}

func checkError(resp []byte) error {
	var errResp struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}

	if err := json.Unmarshal(resp, &errResp); err != nil {
		return fmt.Errorf("failed to unmarshal json: %v", err)
	}

	if len(errResp.Errors) > 0 {
		return fmt.Errorf("error : %v", errResp.Errors)
	}

	return nil
}

func embedParamsIntoMutation(mutation string, paramsMap map[string]interface{}) string {
	for key, value := range paramsMap {
		mutation = strings.ReplaceAll(mutation, key, fmt.Sprintf(`"%v"`, value))
	}
	return mutation
}
