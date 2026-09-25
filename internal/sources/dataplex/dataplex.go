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

package dataplex

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	dataplexapi "cloud.google.com/go/dataplex/apiv1"
	"cloud.google.com/go/dataplex/apiv1/dataplexpb"
	"cloud.google.com/go/longrunning/autogen/longrunningpb"
	resourcemanager "cloud.google.com/go/resourcemanager/apiv3"
	resourcemanagerpb "cloud.google.com/go/resourcemanager/apiv3/resourcemanagerpb"
	"github.com/cenkalti/backoff/v6"
	"github.com/goccy/go-yaml"
	"github.com/google/uuid"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/impersonate"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	grpcstatus "google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

const SourceType string = "dataplex"

// CloudPlatformScope is a broad scope for Google Cloud Platform services.
const CloudPlatformScope = "https://www.googleapis.com/auth/cloud-platform"

var operationNameRegex = regexp.MustCompile(`^projects/[^/]+/locations/[^/]+/operations/[^/]+$`)

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

type Config struct {
	// Dataplex configs
	Name                      string   `yaml:"name" validate:"required"`
	Type                      string   `yaml:"type" validate:"required"`
	Project                   string   `yaml:"project" validate:"required"`
	ImpersonateServiceAccount string   `yaml:"impersonateServiceAccount" validate:"omitempty,email"`
	Scopes                    []string `yaml:"scopes"`
}

func (r Config) SourceConfigType() string {
	// Returns Dataplex source type
	return SourceType
}

func (r Config) Initialize(ctx context.Context, tracer trace.Tracer) (sources.Source, error) {
	// Initializes a Dataplex source
	client, dataScanClient, dataProductClient, projectsClient, err := initDataplexConnection(ctx, tracer, r.Name, r.Project, r.ImpersonateServiceAccount, r.Scopes)
	if err != nil {
		return nil, err
	}

	// Resolve project number
	proj, err := projectsClient.GetProject(ctx, &resourcemanagerpb.GetProjectRequest{
		Name: "projects/" + r.Project,
	})
	if err != nil {
		client.Close()
		dataScanClient.Close()
		dataProductClient.Close()
		projectsClient.Close()
		return nil, fmt.Errorf("failed to get project details for project %q: %w", r.Project, err)
	}
	parts := strings.Split(proj.Name, "/")
	if len(parts) < 2 {
		client.Close()
		dataScanClient.Close()
		dataProductClient.Close()
		projectsClient.Close()
		return nil, fmt.Errorf("unexpected project resource name format: %q", proj.Name)
	}
	projectNumberStr := parts[1]
	projectNumber, err := strconv.ParseInt(projectNumberStr, 10, 64)
	if err != nil {
		client.Close()
		dataScanClient.Close()
		dataProductClient.Close()
		projectsClient.Close()
		return nil, fmt.Errorf("failed to parse project number %q as int64: %w", projectNumberStr, err)
	}

	s := &Source{
		Config:            r,
		Client:            client,
		DataScanClient:    dataScanClient,
		dataProductClient: dataProductClient,
		projectsClient:    projectsClient,
		projectNumber:     projectNumber,
	}

	return s, nil
}

var _ sources.Source = &Source{}

type Source struct {
	Config
	Client            *dataplexapi.CatalogClient
	DataScanClient    *dataplexapi.DataScanClient
	dataProductClient *dataplexapi.DataProductClient
	projectsClient    *resourcemanager.ProjectsClient
	projectNumber     int64
}

func (s *Source) IsReadOnly() bool {
	return false
}

func (s *Source) SourceType() string {
	// Returns Dataplex source type
	return SourceType
}

func (s *Source) ToConfig() sources.SourceConfig {
	return s.Config
}

func (s *Source) ProjectID() string {
	return s.Project
}

func (s *Source) ProjectNumber() int64 {
	return s.projectNumber
}

func (s *Source) ProjectsClient() *resourcemanager.ProjectsClient {
	return s.projectsClient
}

func (s *Source) CatalogClient() *dataplexapi.CatalogClient {
	return s.Client
}

func (s *Source) GetDataScanClient() *dataplexapi.DataScanClient {
	return s.DataScanClient
}

func (s *Source) GetDataProductClient() *dataplexapi.DataProductClient {
	return s.dataProductClient
}

func initDataplexConnection(
	ctx context.Context,
	tracer trace.Tracer,
	name string,
	project string,
	impersonateServiceAccount string,
	scopes []string,
) (*dataplexapi.CatalogClient, *dataplexapi.DataScanClient, *dataplexapi.DataProductClient, *resourcemanager.ProjectsClient, error) {
	ctx, span := sources.InitConnectionSpan(ctx, tracer, SourceType, name)
	defer span.End()

	userAgent, err := util.UserAgentFromContext(ctx)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	var opts []option.ClientOption

	credScopes := scopes
	if len(credScopes) == 0 {
		credScopes = []string{CloudPlatformScope}
	}

	if impersonateServiceAccount != "" {
		// Create impersonated credentials token source
		ts, err := impersonate.CredentialsTokenSource(ctx, impersonate.CredentialsConfig{
			TargetPrincipal: impersonateServiceAccount,
			Scopes:          credScopes,
		})
		if err != nil {
			return nil, nil, nil, nil, fmt.Errorf("failed to create impersonated credentials for %q for project %q: %w", impersonateServiceAccount, project, err)
		}
		opts = []option.ClientOption{
			option.WithUserAgent(userAgent),
			option.WithTokenSource(ts),
		}
	} else {
		// Use default credentials
		cred, err := google.FindDefaultCredentials(ctx, credScopes...)
		if err != nil {
			return nil, nil, nil, nil, fmt.Errorf("failed to find default Google Cloud credentials for project %q: %w", project, err)
		}
		opts = []option.ClientOption{
			option.WithUserAgent(userAgent),
			option.WithCredentials(cred),
		}
	}

	client, err := dataplexapi.NewCatalogClient(ctx, opts...)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to create Dataplex client for project %q: %w", project, err)
	}

	dataScanClient, err := dataplexapi.NewDataScanClient(ctx, opts...)
	if err != nil {
		client.Close()
		return nil, nil, nil, nil, fmt.Errorf("failed to create Dataplex DataScan client for project %q: %w", project, err)
	}

	dataProductClient, err := dataplexapi.NewDataProductClient(ctx, opts...)
	if err != nil {
		client.Close()
		dataScanClient.Close()
		return nil, nil, nil, nil, fmt.Errorf("failed to create Dataplex DataProduct client for project %q: %w", project, err)
	}

	projectsClient, err := resourcemanager.NewProjectsClient(ctx, opts...)
	if err != nil {
		client.Close()
		dataScanClient.Close()
		dataProductClient.Close()
		return nil, nil, nil, nil, fmt.Errorf("failed to create ResourceManager projects client for project %q: %w", project, err)
	}

	return client, dataScanClient, dataProductClient, projectsClient, nil
}

func (s *Source) LookupEntry(ctx context.Context, name string, view int, aspectTypes []string, entry string) (*dataplexpb.Entry, error) {
	viewMap := map[int]dataplexpb.EntryView{
		1: dataplexpb.EntryView_BASIC,
		2: dataplexpb.EntryView_FULL,
		3: dataplexpb.EntryView_CUSTOM,
		4: dataplexpb.EntryView_ALL,
	}
	req := &dataplexpb.LookupEntryRequest{
		Name:        name,
		View:        viewMap[view],
		AspectTypes: aspectTypes,
		Entry:       entry,
	}
	result, err := s.CatalogClient().LookupEntry(ctx, req)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Source) searchRequest(ctx context.Context, query string, pageSize int, orderBy string, scope string) (*dataplexapi.SearchEntriesResultIterator, error) {
	// Create SearchEntriesRequest with the provided parameters
	req := &dataplexpb.SearchEntriesRequest{
		Query:          query,
		Name:           fmt.Sprintf("projects/%s/locations/global", s.ProjectID()),
		PageSize:       int32(pageSize),
		OrderBy:        orderBy,
		SemanticSearch: true,
	}

	if scope != "" {
		req.Scope = scope
	}

	// Perform the search using the CatalogClient - this will return an iterator
	it := s.CatalogClient().SearchEntries(ctx, req)
	if it == nil {
		return nil, fmt.Errorf("failed to create search entries iterator for project %q", s.ProjectID())
	}
	return it, nil
}

func (s *Source) SearchAspectTypes(ctx context.Context, query string, pageSize int, orderBy string) ([]*dataplexpb.AspectType, error) {
	if pageSize <= 0 {
		return nil, fmt.Errorf("pageSize must be positive: %d", pageSize)
	}
	q := query + " type=projects/dataplex-types/locations/global/entryTypes/aspecttype"
	it, err := s.searchRequest(ctx, q, pageSize, orderBy, "")
	if err != nil {
		return nil, err
	}

	// Iterate through the search results and call GetAspectType for each result using the resource name
	var results []*dataplexpb.AspectType
	for len(results) < pageSize {
		entry, err := it.Next()

		if err == iterator.Done {
			break
		}
		if err != nil {
			if st, ok := grpcstatus.FromError(err); ok {
				errorCode := st.Code()
				errorMessage := st.Message()
				return nil, fmt.Errorf("failed to search aspect types with error code: %q message: %s", errorCode.String(), errorMessage)
			}
			return nil, fmt.Errorf("failed to search aspect types: %w", err)
		}

		// Create an instance of exponential backoff with default values for retrying GetAspectType calls
		// InitialInterval, RandomizationFactor, Multiplier, MaxInterval = 500 ms, 0.5, 1.5, 60 s
		getAspectBackOff := backoff.NewExponentialBackOff()

		resourceName := entry.DataplexEntry.GetEntrySource().Resource
		getAspectTypeReq := &dataplexpb.GetAspectTypeRequest{
			Name: resourceName,
		}

		operation := func() (*dataplexpb.AspectType, error) {
			aspectType, err := s.CatalogClient().GetAspectType(ctx, getAspectTypeReq)
			if err != nil {
				return nil, fmt.Errorf("failed to get aspect type for entry %q: %w", resourceName, err)
			}
			return aspectType, nil
		}

		// Retry the GetAspectType operation with exponential backoff
		aspectType, err := backoff.Retry(ctx, operation, backoff.WithBackOff(getAspectBackOff))
		if err != nil {
			return nil, fmt.Errorf("failed to get aspect type after retries for entry %q: %w", resourceName, err)
		}

		results = append(results, aspectType)
	}
	return results, nil
}

func (s *Source) SearchEntries(ctx context.Context, query string, pageSize int, orderBy string, scope string) ([]*dataplexpb.SearchEntriesResult, error) {
	if pageSize <= 0 {
		return nil, fmt.Errorf("pageSize must be positive: %d", pageSize)
	}
	it, err := s.searchRequest(ctx, query, pageSize, orderBy, scope)
	if err != nil {
		return nil, err
	}

	var results []*dataplexpb.SearchEntriesResult
	for len(results) < pageSize {
		entry, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			if st, ok := grpcstatus.FromError(err); ok {
				errorCode := st.Code()
				errorMessage := st.Message()
				return nil, fmt.Errorf("failed to search entries with error code: %q message: %s", errorCode.String(), errorMessage)
			}
			return nil, fmt.Errorf("failed to search entries: %w", err)
		}
		results = append(results, entry)
	}
	return results, nil
}

func (s *Source) LookupContext(ctx context.Context, name string, resources []string) (*dataplexpb.LookupContextResponse, error) {
	req := &dataplexpb.LookupContextRequest{
		Name:      name,
		Resources: resources,
	}
	result, err := s.CatalogClient().LookupContext(ctx, req)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Source) SearchDataQualityScans(ctx context.Context, filter string, pageSize int, orderBy string) ([]*dataplexpb.DataScan, error) {
	if pageSize <= 0 {
		return nil, fmt.Errorf("pageSize must be positive: %d", pageSize)
	}
	req := &dataplexpb.ListDataScansRequest{
		Parent:   fmt.Sprintf("projects/%s/locations/-", s.ProjectID()),
		Filter:   filter,
		PageSize: int32(pageSize),
		OrderBy:  orderBy,
	}

	it := s.GetDataScanClient().ListDataScans(ctx, req)

	var results []*dataplexpb.DataScan
	for len(results) < pageSize {
		scan, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			if st, ok := grpcstatus.FromError(err); ok {
				return nil, fmt.Errorf("failed to list data scans: code=%s message=%s", st.Code(), st.Message())
			}
			return nil, fmt.Errorf("failed to list data scans: %w", err)
		}
		results = append(results, scan)
	}
	return results, nil
}

type DataProductSummary struct {
	LocationID    string   `json:"locationId"`
	DataProductID string   `json:"dataProductId"`
	DisplayName   string   `json:"displayName"`
	OwnerEmails   []string `json:"ownerEmails"`
	AssetCount    int32    `json:"assetCount"`
}

func (s *Source) ListDataProducts(
	ctx context.Context,
	filter string,
	pageSize int,
	orderBy string,
) ([]*DataProductSummary, error) {
	if pageSize <= 0 {
		return nil, fmt.Errorf("pageSize must be positive: %d", pageSize)
	}
	parent := fmt.Sprintf("projects/%s/locations/-", s.ProjectID())
	req := &dataplexpb.ListDataProductsRequest{
		Parent:   parent,
		Filter:   filter,
		PageSize: int32(pageSize),
		OrderBy:  orderBy,
	}

	it := s.GetDataProductClient().ListDataProducts(ctx, req)
	var results []*DataProductSummary

	for len(results) < pageSize {
		dp, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			if st, ok := grpcstatus.FromError(err); ok {
				return nil, fmt.Errorf("failed to list data products: code=%s message=%s", st.Code(), st.Message())
			}
			return nil, fmt.Errorf("failed to list data products: %w", err)
		}
		parts := strings.Split(dp.GetName(), "/")
		var locID, prodID string
		if len(parts) >= 6 && parts[0] == "projects" && parts[2] == "locations" && parts[4] == "dataProducts" {
			locID = parts[3]
			prodID = parts[5]
		}
		results = append(results, &DataProductSummary{
			LocationID:    locID,
			DataProductID: prodID,
			DisplayName:   dp.GetDisplayName(),
			OwnerEmails:   dp.GetOwnerEmails(),
			AssetCount:    dp.GetAssetCount(),
		})
	}
	return results, nil
}

type AccessGroup struct {
	ID             string `json:"id"`
	DisplayName    string `json:"displayName"`
	Description    string `json:"description"`
	GoogleGroup    string `json:"googleGroup"`
	ServiceAccount string `json:"serviceAccount"`
}

type DataProduct struct {
	LocationID    string            `json:"locationId"`
	DataProductID string            `json:"dataProductId"`
	DisplayName   string            `json:"displayName"`
	Description   string            `json:"description"`
	OwnerEmails   []string          `json:"ownerEmails"`
	AssetCount    int32             `json:"assetCount"`
	Labels        map[string]string `json:"labels"`
	AccessGroups  []AccessGroup     `json:"accessGroups"`
}

func (s *Source) GetDataProduct(ctx context.Context, locationID string, dataProductID string) (*DataProduct, error) {
	name := fmt.Sprintf("projects/%s/locations/%s/dataProducts/%s", s.ProjectID(), locationID, dataProductID)
	req := &dataplexpb.GetDataProductRequest{
		Name: name,
	}
	resp, err := s.GetDataProductClient().GetDataProduct(ctx, req)
	if err != nil {
		return nil, err
	}

	accessGroups := []AccessGroup{}
	for _, ag := range resp.GetAccessGroups() {
		accessGroups = append(accessGroups, AccessGroup{
			ID:             ag.GetId(),
			DisplayName:    ag.GetDisplayName(),
			Description:    ag.GetDescription(),
			GoogleGroup:    ag.GetPrincipal().GetGoogleGroup(),
			ServiceAccount: ag.GetPrincipal().GetServiceAccount(),
		})
	}

	parts := strings.Split(resp.GetName(), "/")
	var locID, prodID string
	if len(parts) >= 6 && parts[0] == "projects" && parts[2] == "locations" && parts[4] == "dataProducts" {
		locID = parts[3]
		prodID = parts[5]
	}

	return &DataProduct{
		LocationID:    locID,
		DataProductID: prodID,
		DisplayName:   resp.GetDisplayName(),
		Description:   resp.GetDescription(),
		OwnerEmails:   resp.GetOwnerEmails(),
		AssetCount:    resp.GetAssetCount(),
		Labels:        resp.GetLabels(),
		AccessGroups:  accessGroups,
	}, nil
}

// Common between ListDataAssets and GetDataAsset.
// The only difference between the objects returned by these two methods is whether the AccessGroupConfigs field (marked omitempty) is included.
type DataAsset struct {
	LocationID         string                                             `json:"locationId"`
	DataProductID      string                                             `json:"dataProductId"`
	DataAssetID        string                                             `json:"dataAssetId"`
	ResourceURI        string                                             `json:"resourceUri"`
	Labels             map[string]string                                  `json:"labels"`
	AccessGroupConfigs map[string]*dataplexpb.DataAsset_AccessGroupConfig `json:"accessGroupConfigs,omitempty"`
}

func (s *Source) ListDataAssets(
	ctx context.Context,
	locationID string,
	dataProductID string,
	filter string,
	pageSize int,
	orderBy string,
) ([]*DataAsset, error) {
	if pageSize <= 0 {
		return nil, fmt.Errorf("pageSize must be positive: %d", pageSize)
	}
	parent := fmt.Sprintf("projects/%s/locations/%s/dataProducts/%s", s.ProjectID(), locationID, dataProductID)
	req := &dataplexpb.ListDataAssetsRequest{
		Parent:   parent,
		Filter:   filter,
		PageSize: int32(pageSize),
		OrderBy:  orderBy,
	}

	it := s.GetDataProductClient().ListDataAssets(ctx, req)
	var results []*DataAsset

	for len(results) < pageSize {
		asset, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			if st, ok := grpcstatus.FromError(err); ok {
				return nil, fmt.Errorf("failed to list data assets: code=%s message=%s", st.Code(), st.Message())
			}
			return nil, fmt.Errorf("failed to list data assets: %w", err)
		}
		parts := strings.Split(asset.GetName(), "/")
		var locID, prodID, assetID string
		if len(parts) >= 8 && parts[0] == "projects" && parts[2] == "locations" && parts[4] == "dataProducts" && parts[6] == "dataAssets" {
			locID = parts[3]
			prodID = parts[5]
			assetID = parts[7]
		}
		results = append(results, &DataAsset{
			LocationID:    locID,
			DataProductID: prodID,
			DataAssetID:   assetID,
			ResourceURI:   asset.GetResource(),
			Labels:        asset.GetLabels(),
		})
	}
	return results, nil
}

func (s *Source) GetDataAsset(ctx context.Context, locationID string, dataProductID string, dataAssetID string) (*DataAsset, error) {
	name := fmt.Sprintf("projects/%s/locations/%s/dataProducts/%s/dataAssets/%s", s.ProjectID(), locationID, dataProductID, dataAssetID)
	req := &dataplexpb.GetDataAssetRequest{
		Name: name,
	}
	resp, err := s.GetDataProductClient().GetDataAsset(ctx, req)
	if err != nil {
		return nil, err
	}

	parts := strings.Split(resp.GetName(), "/")
	var locID, prodID, assetID string
	if len(parts) >= 8 && parts[0] == "projects" && parts[2] == "locations" && parts[4] == "dataProducts" && parts[6] == "dataAssets" {
		locID = parts[3]
		prodID = parts[5]
		assetID = parts[7]
	}

	return &DataAsset{
		LocationID:         locID,
		DataProductID:      prodID,
		DataAssetID:        assetID,
		ResourceURI:        resp.GetResource(),
		Labels:             resp.GetLabels(),
		AccessGroupConfigs: resp.GetAccessGroupConfigs(),
	}, nil
}

// dataProductId is optional. If empty, the Dataplex backend will automatically generate a unique ID.
func (s *Source) CreateDataProduct(
	ctx context.Context,
	locationID string,
	dataProductID string,
	displayName string,
	description string,
	ownerEmails []string,
	accessGroups []AccessGroup,
) (map[string]string, error) {
	parent := fmt.Sprintf("projects/%s/locations/%s", s.ProjectID(), locationID)

	agMap := make(map[string]*dataplexpb.DataProduct_AccessGroup)
	for _, ag := range accessGroups {
		principal := &dataplexpb.DataProduct_Principal{}
		if ag.GoogleGroup != "" {
			principal.Type = &dataplexpb.DataProduct_Principal_GoogleGroup{
				GoogleGroup: ag.GoogleGroup,
			}
		}
		if ag.ServiceAccount != "" {
			principal.ServiceAccount = &ag.ServiceAccount
		}
		agMap[ag.ID] = &dataplexpb.DataProduct_AccessGroup{
			Id:          ag.ID,
			DisplayName: ag.DisplayName,
			Description: ag.Description,
			Principal:   principal,
		}
	}

	req := &dataplexpb.CreateDataProductRequest{
		Parent:        parent,
		DataProductId: dataProductID,
		DataProduct: &dataplexpb.DataProduct{
			DisplayName:  displayName,
			Description:  description,
			OwnerEmails:  ownerEmails,
			AccessGroups: agMap,
		},
	}

	op, err := s.GetDataProductClient().CreateDataProduct(ctx, req)
	if err != nil {
		return nil, err
	}

	opName := op.Name()
	parts := strings.Split(opName, "/")
	if len(parts) < 6 || parts[0] != "projects" || parts[2] != "locations" || parts[4] != "operations" {
		return nil, fmt.Errorf("invalid operation name: %q", opName)
	}
	return map[string]string{
		"locationId":  parts[3],
		"operationId": parts[5],
	}, nil
}

func (s *Source) UpdateDataProduct(
	ctx context.Context,
	locationID string,
	dataProductID string,
	description string,
	displayName string,
	ownerEmails []string,
	accessGroups []AccessGroup,
	updateMask []string,
) (map[string]string, error) {
	name := fmt.Sprintf("projects/%s/locations/%s/dataProducts/%s", s.ProjectID(), locationID, dataProductID)

	agMap := make(map[string]*dataplexpb.DataProduct_AccessGroup)
	if len(accessGroups) > 0 {
		for _, ag := range accessGroups {
			principal := &dataplexpb.DataProduct_Principal{}
			if ag.GoogleGroup != "" {
				principal.Type = &dataplexpb.DataProduct_Principal_GoogleGroup{
					GoogleGroup: ag.GoogleGroup,
				}
			}
			if ag.ServiceAccount != "" {
				principal.ServiceAccount = &ag.ServiceAccount
			}
			agMap[ag.ID] = &dataplexpb.DataProduct_AccessGroup{
				Id:          ag.ID,
				DisplayName: ag.DisplayName,
				Description: ag.Description,
				Principal:   principal,
			}
		}
	}

	req := &dataplexpb.UpdateDataProductRequest{
		DataProduct: &dataplexpb.DataProduct{
			Name:         name,
			DisplayName:  displayName,
			Description:  description,
			OwnerEmails:  ownerEmails,
			AccessGroups: agMap,
		},
	}

	if len(updateMask) > 0 {
		var snakeMask []string
		for _, path := range updateMask {
			snakeMask = append(snakeMask, util.SnakeFromCamelCase(path))
		}
		req.UpdateMask = &fieldmaskpb.FieldMask{
			Paths: snakeMask,
		}
	}

	op, err := s.GetDataProductClient().UpdateDataProduct(ctx, req)
	if err != nil {
		return nil, err
	}

	opName := op.Name()
	parts := strings.Split(opName, "/")
	if len(parts) < 6 || parts[0] != "projects" || parts[2] != "locations" || parts[4] != "operations" {
		return nil, fmt.Errorf("invalid operation name: %q", opName)
	}
	return map[string]string{
		"locationId":  parts[3],
		"operationId": parts[5],
	}, nil
}

func (s *Source) CreateDataAsset(
	ctx context.Context,
	locationID string,
	dataProductID string,
	dataAssetID string,
	resourceURI string,
	labels map[string]string,
	accessGroupConfigs map[string][]string,
) (map[string]string, error) {
	parent := fmt.Sprintf("projects/%s/locations/%s/dataProducts/%s", s.ProjectID(), locationID, dataProductID)

	agcMap := make(map[string]*dataplexpb.DataAsset_AccessGroupConfig)
	for k, v := range accessGroupConfigs {
		agcMap[k] = &dataplexpb.DataAsset_AccessGroupConfig{
			IamRoles: v,
		}
	}

	req := &dataplexpb.CreateDataAssetRequest{
		Parent:      parent,
		DataAssetId: dataAssetID,
		DataAsset: &dataplexpb.DataAsset{
			Resource:           resourceURI,
			Labels:             labels,
			AccessGroupConfigs: agcMap,
		},
	}

	op, err := s.GetDataProductClient().CreateDataAsset(ctx, req)
	if err != nil {
		return nil, err
	}

	opName := op.Name()
	parts := strings.Split(opName, "/")
	if len(parts) < 6 || parts[0] != "projects" || parts[2] != "locations" || parts[4] != "operations" {
		return nil, fmt.Errorf("invalid operation name: %q", opName)
	}
	return map[string]string{
		"locationId":  parts[3],
		"operationId": parts[5],
	}, nil
}

func (s *Source) UpdateDataAsset(
	ctx context.Context,
	locationID string,
	dataProductID string,
	dataAssetID string,
	labels map[string]string,
	accessGroupConfigs map[string][]string,
	updateMask []string,
) (map[string]string, error) {
	name := fmt.Sprintf("projects/%s/locations/%s/dataProducts/%s/dataAssets/%s", s.ProjectID(), locationID, dataProductID, dataAssetID)

	agcMap := make(map[string]*dataplexpb.DataAsset_AccessGroupConfig)
	for k, v := range accessGroupConfigs {
		agcMap[k] = &dataplexpb.DataAsset_AccessGroupConfig{
			IamRoles: v,
		}
	}

	req := &dataplexpb.UpdateDataAssetRequest{
		DataAsset: &dataplexpb.DataAsset{
			Name:               name,
			Labels:             labels,
			AccessGroupConfigs: agcMap,
		},
	}

	if len(updateMask) > 0 {
		var snakeMask []string
		for _, path := range updateMask {
			snakeMask = append(snakeMask, util.SnakeFromCamelCase(path))
		}
		req.UpdateMask = &fieldmaskpb.FieldMask{
			Paths: snakeMask,
		}
	}

	op, err := s.GetDataProductClient().UpdateDataAsset(ctx, req)
	if err != nil {
		return nil, err
	}

	opName := op.Name()
	parts := strings.Split(opName, "/")
	if len(parts) < 6 || parts[0] != "projects" || parts[2] != "locations" || parts[4] != "operations" {
		return nil, fmt.Errorf("invalid operation name: %q", opName)
	}
	return map[string]string{
		"locationId":  parts[3],
		"operationId": parts[5],
	}, nil
}

func (s *Source) UpdateEntry(ctx context.Context, entry *dataplexpb.Entry, updateMask *fieldmaskpb.FieldMask) (*dataplexpb.Entry, error) {
	req := &dataplexpb.UpdateEntryRequest{
		Entry:      entry,
		UpdateMask: updateMask,
	}
	return s.CatalogClient().UpdateEntry(ctx, req)
}

func (s *Source) GenerateDataInsights(ctx context.Context, location, resourcePath string, publish bool) (string, error) {
	parent := fmt.Sprintf("projects/%s/locations/%s", s.ProjectID(), location)
	dataScanID := fmt.Sprintf("nq-doc-%s", uuid.New().String())

	req := &dataplexpb.CreateDataScanRequest{
		Parent:     parent,
		DataScanId: dataScanID,
		DataScan: &dataplexpb.DataScan{
			Data: &dataplexpb.DataSource{
				Source: &dataplexpb.DataSource_Resource{
					Resource: resourcePath,
				},
			},
			Spec: &dataplexpb.DataScan_DataDocumentationSpec{
				DataDocumentationSpec: &dataplexpb.DataDocumentationSpec{
					CatalogPublishingEnabled: publish,
				},
			},
			ExecutionSpec: &dataplexpb.DataScan_ExecutionSpec{
				Trigger: &dataplexpb.Trigger{
					Mode: &dataplexpb.Trigger_OneTime_{
						OneTime: &dataplexpb.Trigger_OneTime{},
					},
				},
			},
			Type: dataplexpb.DataScanType_DATA_DOCUMENTATION,
			Labels: map[string]string{
				"onemcp-server": "true",
			},
		},
	}

	op, err := s.DataScanClient.CreateDataScan(ctx, req)
	if err != nil {
		return "", err
	}
	return op.Name(), nil
}

func (s *Source) GetDataScan(ctx context.Context, location, scanID string) (*dataplexpb.DataScan, error) {
	name := fmt.Sprintf("projects/%s/locations/%s/dataScans/%s", s.ProjectID(), location, scanID)
	req := &dataplexpb.GetDataScanRequest{
		Name: name,
		View: dataplexpb.GetDataScanRequest_FULL,
	}
	return s.DataScanClient.GetDataScan(ctx, req)
}

func (s *Source) GetOperation(ctx context.Context, opName string) (map[string]any, error) {
	if !operationNameRegex.MatchString(opName) {
		return nil, fmt.Errorf("invalid operation name format: %q (expected projects/*/locations/*/operations/*)", opName)
	}

	req := &longrunningpb.GetOperationRequest{
		Name: opName,
	}
	op, err := s.DataScanClient.LROClient.GetOperation(ctx, req)
	if err != nil {
		return nil, err
	}

	bytes, err := protojson.Marshal(op)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal operation to JSON: %w", err)
	}

	var opData map[string]any
	if err := json.Unmarshal(bytes, &opData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal operation JSON to map: %w", err)
	}

	return opData, nil
}

func (s *Source) GetJobStatus(ctx context.Context, location, scanID, jobID string) (*dataplexpb.DataScanJob, error) {
	// If jobID is provided, fetch that specific job directly!
	if jobID != "" {
		name := fmt.Sprintf("projects/%s/locations/%s/dataScans/%s/jobs/%s", s.ProjectID(), location, scanID, jobID)
		req := &dataplexpb.GetDataScanJobRequest{
			Name: name,
		}
		return s.DataScanClient.GetDataScanJob(ctx, req)
	}

	// Fallback to listing and returning the latest job (PageSize: 1)
	parent := fmt.Sprintf("projects/%s/locations/%s/dataScans/%s", s.ProjectID(), location, scanID)
	req := &dataplexpb.ListDataScanJobsRequest{
		Parent:   parent,
		PageSize: 1,
	}

	it := s.DataScanClient.ListDataScanJobs(ctx, req)
	if it == nil {
		return nil, fmt.Errorf("failed to list data scan jobs for scan %q", scanID)
	}

	job, err := it.Next()
	if err == iterator.Done {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return job, nil
}

func (s *Source) GenerateDataProfile(ctx context.Context, location, resourcePath string, publish bool) (string, error) {
	parent := fmt.Sprintf("projects/%s/locations/%s", s.ProjectID(), location)
	dataScanID := fmt.Sprintf("nq-prof-%s", uuid.New().String())

	req := &dataplexpb.CreateDataScanRequest{
		Parent:     parent,
		DataScanId: dataScanID,
		DataScan: &dataplexpb.DataScan{
			Data: &dataplexpb.DataSource{
				Source: &dataplexpb.DataSource_Resource{
					Resource: resourcePath,
				},
			},
			Spec: &dataplexpb.DataScan_DataProfileSpec{
				DataProfileSpec: &dataplexpb.DataProfileSpec{
					CatalogPublishingEnabled: publish,
				},
			},
			ExecutionSpec: &dataplexpb.DataScan_ExecutionSpec{
				Trigger: &dataplexpb.Trigger{
					Mode: &dataplexpb.Trigger_OneTime_{
						OneTime: &dataplexpb.Trigger_OneTime{},
					},
				},
			},
			Type: dataplexpb.DataScanType_DATA_PROFILE,
			Labels: map[string]string{
				"onemcp-server": "true",
			},
		},
	}

	op, err := s.DataScanClient.CreateDataScan(ctx, req)
	if err != nil {
		return "", err
	}
	return op.Name(), nil
}

func (s *Source) GenerateDataDiscovery(ctx context.Context, location, resourcePath string) (string, error) {
	parent := fmt.Sprintf("projects/%s/locations/%s", s.ProjectID(), location)
	dataScanID := fmt.Sprintf("nq-disc-%s", uuid.New().String())

	req := &dataplexpb.CreateDataScanRequest{
		Parent:     parent,
		DataScanId: dataScanID,
		DataScan: &dataplexpb.DataScan{
			Data: &dataplexpb.DataSource{
				Source: &dataplexpb.DataSource_Resource{
					Resource: resourcePath,
				},
			},
			Spec: &dataplexpb.DataScan_DataDiscoverySpec{
				DataDiscoverySpec: &dataplexpb.DataDiscoverySpec{},
			},
			ExecutionSpec: &dataplexpb.DataScan_ExecutionSpec{
				Trigger: &dataplexpb.Trigger{
					Mode: &dataplexpb.Trigger_OneTime_{
						OneTime: &dataplexpb.Trigger_OneTime{},
					},
				},
			},
			Type: dataplexpb.DataScanType_DATA_DISCOVERY,
			Labels: map[string]string{
				"onemcp-server": "true",
			},
		},
	}

	op, err := s.DataScanClient.CreateDataScan(ctx, req)
	if err != nil {
		return "", err
	}
	return op.Name(), nil
}

func (s *Source) GenerateDataQuality(ctx context.Context, location, resourcePath string, specJSON string, publish bool) (string, error) {
	parent := fmt.Sprintf("projects/%s/locations/%s", s.ProjectID(), location)
	dataScanID := fmt.Sprintf("nq-dq-%s", uuid.New().String())

	var dqSpec dataplexpb.DataQualitySpec
	if err := protojson.Unmarshal([]byte(specJSON), &dqSpec); err != nil {
		return "", fmt.Errorf("failed to parse data quality spec JSON: %w", err)
	}
	dqSpec.CatalogPublishingEnabled = publish

	req := &dataplexpb.CreateDataScanRequest{
		Parent:     parent,
		DataScanId: dataScanID,
		DataScan: &dataplexpb.DataScan{
			Data: &dataplexpb.DataSource{
				Source: &dataplexpb.DataSource_Resource{
					Resource: resourcePath,
				},
			},
			Spec: &dataplexpb.DataScan_DataQualitySpec{
				DataQualitySpec: &dqSpec,
			},
			ExecutionSpec: &dataplexpb.DataScan_ExecutionSpec{
				Trigger: &dataplexpb.Trigger{
					Mode: &dataplexpb.Trigger_OneTime_{
						OneTime: &dataplexpb.Trigger_OneTime{},
					},
				},
			},
			Type: dataplexpb.DataScanType_DATA_QUALITY,
			Labels: map[string]string{
				"onemcp-server": "true",
			},
		},
	}

	op, err := s.DataScanClient.CreateDataScan(ctx, req)
	if err != nil {
		return "", err
	}
	return op.Name(), nil
}
