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

package cloudhealthcare

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/healthcare/v1"
	"google.golang.org/api/option"
)

const SourceType string = "cloud-healthcare"

var allowedFHIRHosts = map[string]struct{}{
	"healthcare.googleapis.com":      {},
	"healthcare.mtls.googleapis.com": {},
}

// validate interface
var _ sources.SourceConfig = Config{}

type HealthcareServiceCreator func(tokenString string) (*healthcare.Service, error)

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

type Config struct {
	// Healthcare configs
	Name               string   `yaml:"name" validate:"required"`
	Type               string   `yaml:"type" validate:"required"`
	Project            string   `yaml:"project" validate:"required"`
	Region             string   `yaml:"region" validate:"required"`
	Dataset            string   `yaml:"dataset" validate:"required"`
	AllowedFHIRStores  []string `yaml:"allowedFhirStores"`
	AllowedDICOMStores []string `yaml:"allowedDicomStores"`
	UseClientOAuth     bool     `yaml:"useClientOAuth"`
}

func (c Config) SourceConfigType() string {
	return SourceType
}

func (c Config) Initialize(ctx context.Context, tracer trace.Tracer) (sources.Source, error) {
	var service *healthcare.Service
	var serviceCreator HealthcareServiceCreator
	var tokenSource oauth2.TokenSource

	svc, tok, err := initHealthcareConnection(ctx, tracer, c.Name)
	if err != nil {
		return nil, fmt.Errorf("error creating service from ADC: %w", err)
	}
	if c.UseClientOAuth {
		serviceCreator, err = newHealthcareServiceCreator(ctx, tracer, c.Name)
		if err != nil {
			return nil, fmt.Errorf("error constructing service creator: %w", err)
		}
	} else {
		service = svc
		tokenSource = tok
	}

	dsName := fmt.Sprintf("projects/%s/locations/%s/datasets/%s", c.Project, c.Region, c.Dataset)
	if _, err = svc.Projects.Locations.Datasets.FhirStores.Get(dsName).Do(); err != nil {
		if gerr, ok := err.(*googleapi.Error); ok && gerr.Code == http.StatusNotFound {
			return nil, fmt.Errorf("dataset '%s' not found", dsName)
		}
		return nil, fmt.Errorf("failed to verify existence of dataset '%s': %w", dsName, err)
	}

	allowedFHIRStores := make(map[string]struct{})
	for _, store := range c.AllowedFHIRStores {
		name := fmt.Sprintf("%s/fhirStores/%s", dsName, store)
		_, err := svc.Projects.Locations.Datasets.FhirStores.Get(name).Do()
		if err != nil {
			if gerr, ok := err.(*googleapi.Error); ok && gerr.Code == http.StatusNotFound {
				return nil, fmt.Errorf("allowedFhirStore '%s' not found in dataset '%s'", store, dsName)
			}
			return nil, fmt.Errorf("failed to verify allowedFhirStore '%s' in datasest '%s': %w", store, dsName, err)
		}
		allowedFHIRStores[store] = struct{}{}
	}
	allowedDICOMStores := make(map[string]struct{})
	for _, store := range c.AllowedDICOMStores {
		name := fmt.Sprintf("%s/dicomStores/%s", dsName, store)
		_, err := svc.Projects.Locations.Datasets.DicomStores.Get(name).Do()
		if err != nil {
			if gerr, ok := err.(*googleapi.Error); ok && gerr.Code == http.StatusNotFound {
				return nil, fmt.Errorf("allowedDicomStore '%s' not found in dataset '%s'", store, dsName)
			}
			return nil, fmt.Errorf("failed to verify allowedDicomFhirStore '%s' in datasest '%s': %w", store, dsName, err)
		}
		allowedDICOMStores[store] = struct{}{}
	}
	s := &Source{
		Config:             c,
		service:            service,
		serviceCreator:     serviceCreator,
		tokenSource:        tokenSource,
		allowedFHIRStores:  allowedFHIRStores,
		allowedDICOMStores: allowedDICOMStores,
	}
	return s, nil
}

func newHealthcareServiceCreator(ctx context.Context, tracer trace.Tracer, name string) (func(string) (*healthcare.Service, error), error) {
	userAgent, err := util.UserAgentFromContext(ctx)
	if err != nil {
		return nil, err
	}
	return func(tokenString string) (*healthcare.Service, error) {
		return initHealthcareConnectionWithOAuthToken(ctx, tracer, name, userAgent, tokenString)
	}, nil
}

func initHealthcareConnectionWithOAuthToken(ctx context.Context, tracer trace.Tracer, name string, userAgent string, tokenString string) (*healthcare.Service, error) {
	ctx, span := sources.InitConnectionSpan(ctx, tracer, SourceType, name)
	defer span.End()
	// Construct token source
	token := &oauth2.Token{
		AccessToken: string(tokenString),
	}
	ts := oauth2.StaticTokenSource(token)

	// Initialize the Healthcare service with tokenSource
	service, err := healthcare.NewService(ctx, option.WithUserAgent(userAgent), option.WithTokenSource(ts))
	if err != nil {
		return nil, fmt.Errorf("failed to create Healthcare service: %w", err)
	}
	service.UserAgent = userAgent
	return service, nil
}

func initHealthcareConnection(ctx context.Context, tracer trace.Tracer, name string) (*healthcare.Service, oauth2.TokenSource, error) {
	ctx, span := sources.InitConnectionSpan(ctx, tracer, SourceType, name)
	defer span.End()

	cred, err := google.FindDefaultCredentials(ctx, healthcare.CloudHealthcareScope)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to find default Google Cloud credentials with scope %q: %w", healthcare.CloudHealthcareScope, err)
	}

	userAgent, err := util.UserAgentFromContext(ctx)
	if err != nil {
		return nil, nil, err
	}

	service, err := healthcare.NewService(ctx, option.WithUserAgent(userAgent), option.WithCredentials(cred))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create Healthcare service: %w", err)
	}
	service.UserAgent = userAgent
	return service, cred.TokenSource, nil
}

var _ sources.Source = &Source{}

type Source struct {
	Config
	service            *healthcare.Service
	serviceCreator     HealthcareServiceCreator
	tokenSource        oauth2.TokenSource
	allowedFHIRStores  map[string]struct{}
	allowedDICOMStores map[string]struct{}
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

func (s *Source) Project() string {
	return s.Config.Project
}

func (s *Source) Region() string {
	return s.Config.Region
}

func (s *Source) DatasetID() string {
	return s.Dataset
}

func (s *Source) Service() *healthcare.Service {
	return s.service
}

func (s *Source) ServiceCreator() HealthcareServiceCreator {
	return s.serviceCreator
}

func (s *Source) TokenSource() oauth2.TokenSource {
	return s.tokenSource
}

func (s *Source) AllowedFHIRStores() map[string]struct{} {
	if len(s.allowedFHIRStores) == 0 {
		return nil
	}
	return s.allowedFHIRStores
}

func (s *Source) AllowedDICOMStores() map[string]struct{} {
	if len(s.allowedDICOMStores) == 0 {
		return nil
	}
	return s.allowedDICOMStores
}

func (s *Source) IsFHIRStoreAllowed(storeID string) bool {
	if len(s.allowedFHIRStores) == 0 {
		return true
	}
	_, ok := s.allowedFHIRStores[storeID]
	return ok
}

func (s *Source) IsDICOMStoreAllowed(storeID string) bool {
	if len(s.allowedDICOMStores) == 0 {
		return true
	}
	_, ok := s.allowedDICOMStores[storeID]
	return ok
}

func (s *Source) UseClientAuthorization() bool {
	return s.UseClientOAuth
}

func parseResults(resp *http.Response) (any, error) {
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("could not read response: %w", err)
	}
	if resp.StatusCode > 299 {
		return nil, fmt.Errorf("status %d %s: %s", resp.StatusCode, resp.Status, respBytes)
	}
	var jsonMap map[string]interface{}
	if err := json.Unmarshal(respBytes, &jsonMap); err != nil {
		return nil, fmt.Errorf("could not unmarshal response as json: %w", err)
	}
	return jsonMap, nil
}

func (s *Source) getService(tokenStr string) (*healthcare.Service, error) {
	svc := s.Service()
	var err error
	// Initialize new service if using user OAuth token
	if s.UseClientAuthorization() {
		svc, err = s.ServiceCreator()(tokenStr)
		if err != nil {
			return nil, fmt.Errorf("error creating service from OAuth access token: %w", err)
		}
	}
	return svc, nil
}

func isAlphanumeric(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

func isValidAPIVersion(v string) bool {
	if len(v) < 2 || v[0] != 'v' {
		return false
	}
	// The character after 'v' must be a digit '1'-'9'
	if v[1] < '1' || v[1] > '9' {
		return false
	}
	for i := 2; i < len(v); i++ {
		if !isAlphanumeric(v[i]) {
			return false
		}
	}
	return true
}

func (s *Source) validateFHIRPageURL(pageURL string) (string, error) {
	parsed, err := url.Parse(pageURL)
	if err != nil {
		return "", fmt.Errorf("invalid page URL: %w", err)
	}

	if parsed.Scheme != "https" {
		return "", fmt.Errorf("URL scheme must be https, got %q", parsed.Scheme)
	}

	parsed.Host = strings.ToLower(parsed.Host)
	host := parsed.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	if _, ok := allowedFHIRHosts[host]; !ok {
		return "", fmt.Errorf("URL host must be an allowed FHIR host, got %q", host)
	}

	// Clean and split path
	cleanPath := path.Clean(parsed.Path)
	// Truncate leading and trailing slashes for easier splitting
	trimmed := strings.Trim(cleanPath, "/")
	parts := strings.Split(trimmed, "/")

	// Page URL format Reference: https://docs.cloud.google.com/healthcare-api/docs/how-tos/fhir-search#using_the_search_method_with_get
	if len(parts) < 10 {
		return "", fmt.Errorf("invalid FHIR URL path structure: path too short")
	}

	if !isValidAPIVersion(parts[0]) {
		return "", fmt.Errorf("invalid API version prefix: %q", parts[0])
	}

	if parts[1] != "projects" {
		return "", fmt.Errorf("invalid path: expected 'projects', got %q", parts[1])
	}
	if parts[2] != s.Project() {
		return "", fmt.Errorf("invalid project %q: must match source project %q", parts[2], s.Project())
	}
	if parts[3] != "locations" {
		return "", fmt.Errorf("invalid path: expected 'locations', got %q", parts[3])
	}
	if parts[4] != s.Region() {
		return "", fmt.Errorf("invalid location/region %q: must match source region %q", parts[4], s.Region())
	}
	if parts[5] != "datasets" {
		return "", fmt.Errorf("invalid path: expected 'datasets', got %q", parts[5])
	}
	if parts[6] != s.DatasetID() {
		return "", fmt.Errorf("invalid dataset ID %q: must match source dataset %q", parts[6], s.DatasetID())
	}
	if parts[7] != "fhirStores" {
		return "", fmt.Errorf("invalid path: expected 'fhirStores', got %q", parts[7])
	}
	if !s.IsFHIRStoreAllowed(parts[8]) {
		return "", fmt.Errorf("fhir store ID %q is not allowed by this source", parts[8])
	}
	if parts[9] != "fhir" {
		return "", fmt.Errorf("invalid path: expected 'fhir', got %q", parts[9])
	}

	parsed.Path = cleanPath
	return parsed.String(), nil
}

func (s *Source) FHIRFetchPage(ctx context.Context, pageURL, tokenStr string) (any, error) {
	cleanedURL, err := s.validateFHIRPageURL(pageURL)
	if err != nil {
		return nil, fmt.Errorf("url validation failed: %w", err)
	}

	var httpClient *http.Client
	if s.UseClientAuthorization() {
		ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: tokenStr})
		httpClient = oauth2.NewClient(ctx, ts)
	} else {
		// The source.Service() object holds a client with the default credentials.
		// However, the client is not exported, so we have to create a new one.
		var err error
		httpClient, err = google.DefaultClient(ctx, healthcare.CloudHealthcareScope)
		if err != nil {
			return nil, fmt.Errorf("failed to create default http client: %w", err)
		}
	}

	httpClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return fmt.Errorf("stopped after 10 redirects")
		}
		_, err := s.validateFHIRPageURL(req.URL.String())
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "GET", cleanedURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create http request: %w", err)
	}
	req.Header.Set("Accept", "application/fhir+json;charset=utf-8")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get fhir page from %q: %w", cleanedURL, err)
	}
	defer resp.Body.Close()
	return parseResults(resp)
}

func (s *Source) FHIRPatientEverything(storeID, patientID, tokenStr string, opts []googleapi.CallOption) (any, error) {
	svc, err := s.getService(tokenStr)
	if err != nil {
		return nil, err
	}

	name := fmt.Sprintf("projects/%s/locations/%s/datasets/%s/fhirStores/%s/fhir/Patient/%s", s.Project(), s.Region(), s.DatasetID(), storeID, patientID)
	resp, err := svc.Projects.Locations.Datasets.FhirStores.Fhir.PatientEverything(name).Do(opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to call patient everything for %q: %w", name, err)
	}
	defer resp.Body.Close()
	return parseResults(resp)
}

func (s *Source) FHIRPatientSearch(storeID, tokenStr string, opts []googleapi.CallOption) (any, error) {
	svc, err := s.getService(tokenStr)
	if err != nil {
		return nil, err
	}

	name := fmt.Sprintf("projects/%s/locations/%s/datasets/%s/fhirStores/%s", s.Project(), s.Region(), s.DatasetID(), storeID)
	body, err := json.Marshal(map[string]any{"ResourceType": "Patient"})
	if err != nil {
		return nil, fmt.Errorf("error marshaling body")
	}
	resp, err := svc.Projects.Locations.Datasets.FhirStores.Fhir.SearchType(name, "Patient", bytes.NewReader(body)).Do(opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to search patient resources: %w", err)
	}
	defer resp.Body.Close()
	return parseResults(resp)
}

func (s *Source) GetDataset(tokenStr string) (*healthcare.Dataset, error) {
	svc, err := s.getService(tokenStr)
	if err != nil {
		return nil, err
	}

	datasetName := fmt.Sprintf("projects/%s/locations/%s/datasets/%s", s.Project(), s.Region(), s.DatasetID())
	dataset, err := svc.Projects.Locations.Datasets.Get(datasetName).Do()
	if err != nil {
		return nil, fmt.Errorf("failed to get dataset %q: %w", datasetName, err)
	}
	return dataset, nil
}

func (s *Source) GetFHIRResource(storeID, resType, resID, tokenStr string) (any, error) {
	svc, err := s.getService(tokenStr)
	if err != nil {
		return nil, err
	}

	name := fmt.Sprintf("projects/%s/locations/%s/datasets/%s/fhirStores/%s/fhir/%s/%s", s.Project(), s.Region(), s.DatasetID(), storeID, resType, resID)
	call := svc.Projects.Locations.Datasets.FhirStores.Fhir.Read(name)
	call.Header().Set("Content-Type", "application/fhir+json;charset=utf-8")
	resp, err := call.Do()
	if err != nil {
		return nil, fmt.Errorf("failed to get fhir resource %q: %w", name, err)
	}
	defer resp.Body.Close()
	return parseResults(resp)
}

func (s *Source) GetDICOMStore(storeID, tokenStr string) (*healthcare.DicomStore, error) {
	svc, err := s.getService(tokenStr)
	if err != nil {
		return nil, err
	}

	storeName := fmt.Sprintf("projects/%s/locations/%s/datasets/%s/dicomStores/%s", s.Project(), s.Region(), s.DatasetID(), storeID)
	store, err := svc.Projects.Locations.Datasets.DicomStores.Get(storeName).Do()
	if err != nil {
		return nil, fmt.Errorf("failed to get DICOM store %q: %w", storeName, err)
	}
	return store, nil
}

func (s *Source) GetFHIRStore(storeID, tokenStr string) (*healthcare.FhirStore, error) {
	svc, err := s.getService(tokenStr)
	if err != nil {
		return nil, err
	}

	storeName := fmt.Sprintf("projects/%s/locations/%s/datasets/%s/fhirStores/%s", s.Project(), s.Region(), s.DatasetID(), storeID)
	store, err := svc.Projects.Locations.Datasets.FhirStores.Get(storeName).Do()
	if err != nil {
		return nil, fmt.Errorf("failed to get FHIR store %q: %w", storeName, err)
	}
	return store, nil
}

func (s *Source) GetDICOMStoreMetrics(storeID, tokenStr string) (*healthcare.DicomStoreMetrics, error) {
	svc, err := s.getService(tokenStr)
	if err != nil {
		return nil, err
	}

	storeName := fmt.Sprintf("projects/%s/locations/%s/datasets/%s/dicomStores/%s", s.Project(), s.Region(), s.DatasetID(), storeID)
	store, err := svc.Projects.Locations.Datasets.DicomStores.GetDICOMStoreMetrics(storeName).Do()
	if err != nil {
		return nil, fmt.Errorf("failed to get metrics for DICOM store %q: %w", storeName, err)
	}
	return store, nil
}

func (s *Source) GetFHIRStoreMetrics(storeID, tokenStr string) (*healthcare.FhirStoreMetrics, error) {
	svc, err := s.getService(tokenStr)
	if err != nil {
		return nil, err
	}

	storeName := fmt.Sprintf("projects/%s/locations/%s/datasets/%s/fhirStores/%s", s.Project(), s.Region(), s.DatasetID(), storeID)
	store, err := svc.Projects.Locations.Datasets.FhirStores.GetFHIRStoreMetrics(storeName).Do()
	if err != nil {
		return nil, fmt.Errorf("failed to get metrics for FHIR store %q: %w", storeName, err)
	}
	return store, nil
}

func (s *Source) ListDICOMStores(tokenStr string) ([]*healthcare.DicomStore, error) {
	svc, err := s.getService(tokenStr)
	if err != nil {
		return nil, err
	}

	datasetName := fmt.Sprintf("projects/%s/locations/%s/datasets/%s", s.Project(), s.Region(), s.DatasetID())
	stores, err := svc.Projects.Locations.Datasets.DicomStores.List(datasetName).Do()
	if err != nil {
		return nil, fmt.Errorf("failed to get dataset %q: %w", datasetName, err)
	}
	var filtered []*healthcare.DicomStore
	for _, store := range stores.DicomStores {
		if len(s.AllowedDICOMStores()) == 0 {
			filtered = append(filtered, store)
			continue
		}
		if len(store.Name) == 0 {
			continue
		}
		parts := strings.Split(store.Name, "/")
		if _, ok := s.AllowedDICOMStores()[parts[len(parts)-1]]; ok {
			filtered = append(filtered, store)
		}
	}
	return filtered, nil
}

func (s *Source) ListFHIRStores(tokenStr string) ([]*healthcare.FhirStore, error) {
	svc, err := s.getService(tokenStr)
	if err != nil {
		return nil, err
	}

	datasetName := fmt.Sprintf("projects/%s/locations/%s/datasets/%s", s.Project(), s.Region(), s.DatasetID())
	stores, err := svc.Projects.Locations.Datasets.FhirStores.List(datasetName).Do()
	if err != nil {
		return nil, fmt.Errorf("failed to get dataset %q: %w", datasetName, err)
	}
	var filtered []*healthcare.FhirStore
	for _, store := range stores.FhirStores {
		if len(s.AllowedFHIRStores()) == 0 {
			filtered = append(filtered, store)
			continue
		}
		if len(store.Name) == 0 {
			continue
		}
		parts := strings.Split(store.Name, "/")
		if _, ok := s.AllowedFHIRStores()[parts[len(parts)-1]]; ok {
			filtered = append(filtered, store)
		}
	}
	return filtered, nil
}

func (s *Source) RetrieveRenderedDICOMInstance(storeID, study, series, sop string, frame int, tokenStr string) (any, error) {
	svc, err := s.getService(tokenStr)
	if err != nil {
		return nil, err
	}

	name := fmt.Sprintf("projects/%s/locations/%s/datasets/%s/dicomStores/%s", s.Project(), s.Region(), s.DatasetID(), storeID)
	dicomWebPath := fmt.Sprintf("studies/%s/series/%s/instances/%s/frames/%d/rendered", study, series, sop, frame)
	call := svc.Projects.Locations.Datasets.DicomStores.Studies.Series.Instances.Frames.RetrieveRendered(name, dicomWebPath)
	call.Header().Set("Accept", "image/jpeg")
	resp, err := call.Do()
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve dicom instance rendered image: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("could not read response: %w", err)
	}
	if resp.StatusCode > 299 {
		return nil, fmt.Errorf("RetrieveRendered: status %d %s: %s", resp.StatusCode, resp.Status, respBytes)
	}
	base64String := base64.StdEncoding.EncodeToString(respBytes)
	return base64String, nil
}

func (s *Source) SearchDICOM(toolType, storeID, dicomWebPath, tokenStr string, opts []googleapi.CallOption) (any, error) {
	svc, err := s.getService(tokenStr)
	if err != nil {
		return nil, err
	}
	name := fmt.Sprintf("projects/%s/locations/%s/datasets/%s/dicomStores/%s", s.Project(), s.Region(), s.DatasetID(), storeID)
	var resp *http.Response
	switch toolType {
	case "cloud-healthcare-search-dicom-instances":
		resp, err = svc.Projects.Locations.Datasets.DicomStores.SearchForInstances(name, dicomWebPath).Do(opts...)
	case "cloud-healthcare-search-dicom-series":
		resp, err = svc.Projects.Locations.Datasets.DicomStores.SearchForSeries(name, dicomWebPath).Do(opts...)
	case "cloud-healthcare-search-dicom-studies":
		resp, err = svc.Projects.Locations.Datasets.DicomStores.SearchForStudies(name, dicomWebPath).Do(opts...)
	default:
		return nil, fmt.Errorf("incompatible tool type: %s", toolType)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to search dicom series: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("could not read response: %w", err)
	}
	if resp.StatusCode > 299 {
		return nil, fmt.Errorf("search: status %d %s: %s", resp.StatusCode, resp.Status, respBytes)
	}
	if len(respBytes) == 0 {
		return []interface{}{}, nil
	}
	var result []interface{}
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, fmt.Errorf("could not unmarshal response as list: %w", err)
	}
	return result, nil
}
