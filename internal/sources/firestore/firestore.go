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

package firestore

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/firebaserules/v1"
	"google.golang.org/api/option"
	"google.golang.org/genproto/googleapis/type/latlng"
)

const SourceType string = "firestore"

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
	// Firestore configs
	Name     string `yaml:"name" validate:"required"`
	Type     string `yaml:"type" validate:"required"`
	Project  string `yaml:"project" validate:"required"`
	Database string `yaml:"database"` // Optional, defaults to "(default)"
}

func (r Config) SourceConfigType() string {
	// Returns Firestore source type
	return SourceType
}

func (r Config) Initialize(ctx context.Context, tracer trace.Tracer, deferConnect bool) (sources.Source, error) {
	// Initializes a Firestore source
	s := r.newSource(ctx, tracer)
	if deferConnect {
		return s, nil
	}
	if _, err := s.clients(ctx); err != nil {
		return nil, err
	}
	return s, nil
}

func (r Config) newSource(ctx context.Context, tracer trace.Tracer) *Source {
	return &Source{
		Config: r,
		conn:   sources.NewConnectOnce[*clientSet](ctx, r.Name, SourceType, tracer),
	}
}

var _ sources.Source = &Source{}

type clientSet struct {
	client      *firestore.Client
	rulesClient *firebaserules.Service
}

type Source struct {
	Config
	conn *sources.ConnectOnce[*clientSet]
}

func (s *Source) clients(ctx context.Context) (*clientSet, error) {
	return s.conn.Do(ctx, func(ctx context.Context) (*clientSet, error) {
		client, err := initFirestoreConnection(ctx, s.Project, s.Database)
		if err != nil {
			return nil, err
		}

		// Initialize Firebase Rules client
		rulesClient, err := initFirebaseRulesConnection(ctx, s.Project)
		if err != nil {
			client.Close()
			return nil, fmt.Errorf("failed to initialize Firebase Rules client: %w", err)
		}

		return &clientSet{client: client, rulesClient: rulesClient}, nil
	})
}

func (s *Source) IsReadOnly() bool {
	return false
}

func (s *Source) SourceType() string {
	// Returns Firestore source type
	return SourceType
}

func (s *Source) ToConfig() sources.SourceConfig {
	return s.Config
}

// FirestoreClient reports the client once connected; use FirestoreClientContext for a guaranteed-live one.
func (s *Source) FirestoreClient() *firestore.Client {
	cs, ok := s.conn.Get()
	if !ok {
		return nil
	}
	return cs.client
}

// FirestoreClientContext returns the Firestore client, connecting on first use.
func (s *Source) FirestoreClientContext(ctx context.Context) (*firestore.Client, error) {
	cs, err := s.clients(ctx)
	if err != nil {
		return nil, err
	}
	return cs.client, nil
}

// FirebaseRulesClient reports the Firebase Rules client if one has been made.
func (s *Source) FirebaseRulesClient() *firebaserules.Service {
	cs, ok := s.conn.Get()
	if !ok {
		return nil
	}
	return cs.rulesClient
}

func (s *Source) GetProjectId() string {
	return s.Project
}

func (s *Source) GetDatabaseId() string {
	if s.Database == "" {
		return "(default)"
	}
	return s.Database
}

// FirestoreValueToJSON converts a Firestore value to a simplified JSON representation
// This removes type information and returns plain values
func FirestoreValueToJSON(value any) any {
	if value == nil {
		return nil
	}

	switch v := value.(type) {
	case time.Time:
		return v.Format(time.RFC3339Nano)
	case *latlng.LatLng:
		return map[string]any{
			"latitude":  v.Latitude,
			"longitude": v.Longitude,
		}
	case []byte:
		return base64.StdEncoding.EncodeToString(v)
	case []any:
		result := make([]any, len(v))
		for i, item := range v {
			result[i] = FirestoreValueToJSON(item)
		}
		return result
	case map[string]any:
		result := make(map[string]any)
		for k, val := range v {
			result[k] = FirestoreValueToJSON(val)
		}
		return result
	case *firestore.DocumentRef:
		return v.Path
	default:
		return value
	}
}

// BuildQuery constructs the Firestore query from parameters
func (s *Source) BuildQuery(collectionPath string, filter firestore.EntityFilter, selectFields []string, field string, direction firestore.Direction, limit int, analyzeQuery bool) (*firestore.Query, error) {
	cs, err := s.clients(context.Background())
	if err != nil {
		return nil, err
	}
	collection := cs.client.Collection(collectionPath)
	query := collection.Query

	// Process and apply filters if template is provided
	if filter != nil {
		query = query.WhereEntity(filter)
	}
	if len(selectFields) > 0 {
		query = query.Select(selectFields...)
	}
	if field != "" {
		query = query.OrderBy(field, direction)
	}
	query = query.Limit(limit)

	// Apply analyze options if enabled
	if analyzeQuery {
		query = query.WithRunOptions(firestore.ExplainOptions{
			Analyze: true,
		})
	}

	return &query, nil
}

// QueryResult represents a document result from the query
type QueryResult struct {
	ID         string         `json:"id"`
	Path       string         `json:"path"`
	Data       map[string]any `json:"data"`
	CreateTime any            `json:"createTime,omitempty"`
	UpdateTime any            `json:"updateTime,omitempty"`
	ReadTime   any            `json:"readTime,omitempty"`
}

// QueryResponse represents the full response including optional metrics
type QueryResponse struct {
	Documents      []QueryResult  `json:"documents"`
	ExplainMetrics map[string]any `json:"explainMetrics,omitempty"`
}

// ExecuteQuery runs the query and formats the results
func (s *Source) ExecuteQuery(ctx context.Context, query *firestore.Query, analyzeQuery bool) (any, error) {
	docIterator := query.Documents(ctx)
	docs, err := docIterator.GetAll()
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}
	// Convert results to structured format
	results := make([]QueryResult, len(docs))
	for i, doc := range docs {
		results[i] = QueryResult{
			ID:         doc.Ref.ID,
			Path:       doc.Ref.Path,
			Data:       doc.Data(),
			CreateTime: doc.CreateTime,
			UpdateTime: doc.UpdateTime,
			ReadTime:   doc.ReadTime,
		}
	}

	// Return with explain metrics if requested
	if analyzeQuery {
		explainMetrics, err := getExplainMetrics(docIterator)
		if err == nil && explainMetrics != nil {
			response := QueryResponse{
				Documents:      results,
				ExplainMetrics: explainMetrics,
			}
			return response, nil
		}
	}
	return results, nil
}

// getExplainMetrics extracts explain metrics from the query iterator
func getExplainMetrics(docIterator *firestore.DocumentIterator) (map[string]any, error) {
	explainMetrics, err := docIterator.ExplainMetrics()
	if err != nil || explainMetrics == nil {
		return nil, err
	}

	metricsData := make(map[string]any)

	// Add plan summary if available
	if explainMetrics.PlanSummary != nil {
		planSummary := make(map[string]any)
		planSummary["indexesUsed"] = explainMetrics.PlanSummary.IndexesUsed
		metricsData["planSummary"] = planSummary
	}

	// Add execution stats if available
	if explainMetrics.ExecutionStats != nil {
		executionStats := make(map[string]any)
		executionStats["resultsReturned"] = explainMetrics.ExecutionStats.ResultsReturned
		executionStats["readOperations"] = explainMetrics.ExecutionStats.ReadOperations

		if explainMetrics.ExecutionStats.ExecutionDuration != nil {
			executionStats["executionDuration"] = explainMetrics.ExecutionStats.ExecutionDuration.String()
		}

		if explainMetrics.ExecutionStats.DebugStats != nil {
			executionStats["debugStats"] = *explainMetrics.ExecutionStats.DebugStats
		}

		metricsData["executionStats"] = executionStats
	}

	return metricsData, nil
}

func (s *Source) GetDocuments(ctx context.Context, documentPaths []string) ([]any, error) {
	cs, err := s.clients(ctx)
	if err != nil {
		return nil, err
	}

	// Create document references from paths
	docRefs := make([]*firestore.DocumentRef, len(documentPaths))
	for i, path := range documentPaths {
		docRefs[i] = cs.client.Doc(path)
	}

	// Get all documents
	snapshots, err := cs.client.GetAll(ctx, docRefs)
	if err != nil {
		return nil, fmt.Errorf("failed to get documents: %w", err)
	}

	// Convert snapshots to response data
	results := make([]any, len(snapshots))
	for i, snapshot := range snapshots {
		docData := make(map[string]any)
		docData["path"] = documentPaths[i]
		docData["exists"] = snapshot.Exists()

		if snapshot.Exists() {
			docData["data"] = snapshot.Data()
			docData["createTime"] = snapshot.CreateTime
			docData["updateTime"] = snapshot.UpdateTime
			docData["readTime"] = snapshot.ReadTime
		}

		results[i] = docData
	}

	return results, nil
}

func (s *Source) AddDocuments(ctx context.Context, collectionPath string, documentData any, returnData bool) (map[string]any, error) {
	cs, err := s.clients(ctx)
	if err != nil {
		return nil, err
	}

	// Get the collection reference
	collection := cs.client.Collection(collectionPath)

	// Add the document to the collection
	docRef, writeResult, err := collection.Add(ctx, documentData)
	if err != nil {
		return nil, fmt.Errorf("failed to add document: %w", err)
	}
	// Build the response
	response := map[string]any{
		"documentPath": docRef.Path,
		"createTime":   writeResult.UpdateTime.Format("2006-01-02T15:04:05.999999999Z"),
	}
	// Add document data if requested
	if returnData {
		// Fetch the updated document to return the current state
		snapshot, err := docRef.Get(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to retrieve updated document: %w", err)
		}
		// Convert the document data back to simple JSON format
		simplifiedData := FirestoreValueToJSON(snapshot.Data())
		response["documentData"] = simplifiedData
	}
	return response, nil
}

func (s *Source) UpdateDocument(ctx context.Context, documentPath string, updates []firestore.Update, documentData any, returnData bool) (map[string]any, error) {
	cs, err := s.clients(ctx)
	if err != nil {
		return nil, err
	}

	// Get the document reference
	docRef := cs.client.Doc(documentPath)

	// Prepare update data
	var writeResult *firestore.WriteResult
	var writeErr error

	if len(updates) > 0 {
		writeResult, writeErr = docRef.Update(ctx, updates)
	} else {
		writeResult, writeErr = docRef.Set(ctx, documentData, firestore.MergeAll)
	}

	if writeErr != nil {
		return nil, fmt.Errorf("failed to update document: %w", writeErr)
	}

	// Build the response
	response := map[string]any{
		"documentPath": docRef.Path,
		"updateTime":   writeResult.UpdateTime.Format("2006-01-02T15:04:05.999999999Z"),
	}

	// Add document data if requested
	if returnData {
		// Fetch the updated document to return the current state
		snapshot, err := docRef.Get(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to retrieve updated document: %w", err)
		}
		// Convert the document data to simple JSON format
		simplifiedData := FirestoreValueToJSON(snapshot.Data())
		response["documentData"] = simplifiedData
	}

	return response, nil
}

func (s *Source) DeleteDocuments(ctx context.Context, documentPaths []string) ([]any, error) {
	cs, err := s.clients(ctx)
	if err != nil {
		return nil, err
	}

	// Create a BulkWriter to handle multiple deletions efficiently
	bulkWriter := cs.client.BulkWriter(ctx)

	// Keep track of jobs for each document
	jobs := make([]*firestore.BulkWriterJob, len(documentPaths))

	// Add all delete operations to the BulkWriter
	for i, path := range documentPaths {
		docRef := cs.client.Doc(path)
		job, err := bulkWriter.Delete(docRef)
		if err != nil {
			return nil, fmt.Errorf("failed to add delete operation for document %q: %w", path, err)
		}
		jobs[i] = job
	}

	// End the BulkWriter to execute all operations
	bulkWriter.End()

	// Collect results
	results := make([]any, len(documentPaths))
	for i, job := range jobs {
		docData := make(map[string]any)
		docData["path"] = documentPaths[i]

		// Wait for the job to complete and get the result
		_, err := job.Results()
		if err != nil {
			docData["success"] = false
			docData["error"] = err.Error()
		} else {
			docData["success"] = true
		}

		results[i] = docData
	}
	return results, nil
}

func (s *Source) ListCollections(ctx context.Context, parentPath string) ([]any, error) {
	cs, err := s.clients(ctx)
	if err != nil {
		return nil, err
	}

	var collectionRefs []*firestore.CollectionRef
	if parentPath != "" {
		// List subcollections of the specified document
		docRef := cs.client.Doc(parentPath)
		collectionRefs, err = docRef.Collections(ctx).GetAll()
		if err != nil {
			return nil, fmt.Errorf("failed to list subcollections of document %q: %w", parentPath, err)
		}
	} else {
		// List root collections
		collectionRefs, err = cs.client.Collections(ctx).GetAll()
		if err != nil {
			return nil, fmt.Errorf("failed to list root collections: %w", err)
		}
	}

	// Convert collection references to response data
	results := make([]any, len(collectionRefs))
	for i, collRef := range collectionRefs {
		collData := make(map[string]any)
		collData["id"] = collRef.ID
		collData["path"] = collRef.Path

		// If this is a subcollection, include parent information
		if collRef.Parent != nil {
			collData["parent"] = collRef.Parent.Path
		}
		results[i] = collData
	}
	return results, nil
}

func (s *Source) GetRules(ctx context.Context) (any, error) {
	cs, err := s.clients(ctx)
	if err != nil {
		return nil, err
	}

	// Get the latest release for Firestore
	releaseName := fmt.Sprintf("projects/%s/releases/cloud.firestore/%s", s.GetProjectId(), s.GetDatabaseId())
	release, err := cs.rulesClient.Projects.Releases.Get(releaseName).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("failed to get latest Firestore release: %w", err)
	}

	if release.RulesetName == "" {
		return nil, fmt.Errorf("no active Firestore rules were found in project '%s' and database '%s'", s.GetProjectId(), s.GetDatabaseId())
	}

	// Get the ruleset content
	ruleset, err := cs.rulesClient.Projects.Rulesets.Get(release.RulesetName).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("failed to get ruleset content: %w", err)
	}

	if ruleset.Source == nil || len(ruleset.Source.Files) == 0 {
		return nil, fmt.Errorf("no rules files found in ruleset")
	}

	return ruleset, nil
}

// SourcePosition represents the location of an issue in the source
type SourcePosition struct {
	FileName      string `json:"fileName,omitempty"`
	Line          int64  `json:"line"`          // 1-based
	Column        int64  `json:"column"`        // 1-based
	CurrentOffset int64  `json:"currentOffset"` // 0-based, inclusive start
	EndOffset     int64  `json:"endOffset"`     // 0-based, exclusive end
}

// Issue represents a validation issue in the rules
type Issue struct {
	SourcePosition SourcePosition `json:"sourcePosition"`
	Description    string         `json:"description"`
	Severity       string         `json:"severity"`
}

// ValidationResult represents the result of rules validation
type ValidationResult struct {
	Valid           bool    `json:"valid"`
	IssueCount      int     `json:"issueCount"`
	FormattedIssues string  `json:"formattedIssues,omitempty"`
	RawIssues       []Issue `json:"rawIssues,omitempty"`
}

func (s *Source) ValidateRules(ctx context.Context, sourceParam string) (any, error) {
	cs, err := s.clients(ctx)
	if err != nil {
		return nil, err
	}

	// Create test request
	testRequest := &firebaserules.TestRulesetRequest{
		Source: &firebaserules.Source{
			Files: []*firebaserules.File{
				{
					Name:    "firestore.rules",
					Content: sourceParam,
				},
			},
		},
		// We don't need test cases for validation only
		TestSuite: &firebaserules.TestSuite{
			TestCases: []*firebaserules.TestCase{},
		},
	}
	// Call the test API
	projectName := fmt.Sprintf("projects/%s", s.GetProjectId())
	response, err := cs.rulesClient.Projects.Test(projectName, testRequest).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("failed to validate rules: %w", err)
	}

	// Process the response
	if len(response.Issues) == 0 {
		return ValidationResult{
			Valid:           true,
			IssueCount:      0,
			FormattedIssues: "✓ No errors detected. Rules are valid.",
		}, nil
	}

	// Convert issues to our format
	issues := make([]Issue, len(response.Issues))
	for i, issue := range response.Issues {
		issues[i] = Issue{
			Description: issue.Description,
			Severity:    issue.Severity,
			SourcePosition: SourcePosition{
				FileName:      issue.SourcePosition.FileName,
				Line:          issue.SourcePosition.Line,
				Column:        issue.SourcePosition.Column,
				CurrentOffset: issue.SourcePosition.CurrentOffset,
				EndOffset:     issue.SourcePosition.EndOffset,
			},
		}
	}

	// Format issues
	sourceLines := strings.Split(sourceParam, "\n")
	var formattedOutput []string

	formattedOutput = append(formattedOutput, fmt.Sprintf("Found %d issue(s) in rules source:\n", len(issues)))

	for _, issue := range issues {
		issueString := fmt.Sprintf("%s: %s [Ln %d, Col %d]",
			issue.Severity,
			issue.Description,
			issue.SourcePosition.Line,
			issue.SourcePosition.Column)

		if issue.SourcePosition.Line > 0 {
			lineIndex := int(issue.SourcePosition.Line - 1) // 0-based index
			if lineIndex >= 0 && lineIndex < len(sourceLines) {
				errorLine := sourceLines[lineIndex]
				issueString += fmt.Sprintf("\n```\n%s", errorLine)

				// Add carets if we have column and offset information
				if issue.SourcePosition.Column > 0 &&
					issue.SourcePosition.CurrentOffset >= 0 &&
					issue.SourcePosition.EndOffset > issue.SourcePosition.CurrentOffset {

					startColumn := int(issue.SourcePosition.Column - 1) // 0-based
					errorTokenLength := int(issue.SourcePosition.EndOffset - issue.SourcePosition.CurrentOffset)

					if startColumn >= 0 && errorTokenLength > 0 && startColumn <= len(errorLine) {
						padding := strings.Repeat(" ", startColumn)
						carets := strings.Repeat("^", errorTokenLength)
						issueString += fmt.Sprintf("\n%s%s", padding, carets)
					}
				}
				issueString += "\n```"
			}
		}

		formattedOutput = append(formattedOutput, issueString)
	}

	formattedIssues := strings.Join(formattedOutput, "\n\n")

	return ValidationResult{
		Valid:           false,
		IssueCount:      len(issues),
		FormattedIssues: formattedIssues,
		RawIssues:       issues,
	}, nil
}

// FieldSchema represents metadata about a document field
type FieldSchema struct {
	Name  string   `json:"name"`
	Types []string `json:"types"`
}

// CollectionSchema represents metadata for a Firestore collection
type CollectionSchema struct {
	Collection string        `json:"collection"`
	Fields     []FieldSchema `json:"fields"`
}

// GetSchema returns schema information for the specified collection or all root collections.
func (s *Source) GetSchema(ctx context.Context, collection string) (any, error) {
	var collectionsToInspect []string
	if collection != "" {
		collectionsToInspect = []string{collection}
	} else {
		// Discover root collections
		collRefs, err := s.FirestoreClient().Collections(ctx).GetAll()
		if err != nil {
			return nil, fmt.Errorf("failed to list collections: %w", err)
		}
		for _, ref := range collRefs {
			collectionsToInspect = append(collectionsToInspect, ref.ID)
		}
	}

	result := make([]CollectionSchema, 0, len(collectionsToInspect))
	for _, collName := range collectionsToInspect {
		schema, err := s.getSchemaFromPipeline(ctx, collName)
		if err == nil && len(schema.Fields) > 0 {
			result = append(result, schema)
			continue
		}

		// Fallback: sample documents directly if get_schema pipeline stage is not available
		collRef := s.FirestoreClient().Collection(collName)
		docs, err := collRef.Limit(50).Documents(ctx).GetAll()
		if err != nil {
			return nil, fmt.Errorf("failed to sample documents from collection %q: %w", collName, err)
		}

		fieldsMap := make(map[string]map[string]bool)

		for _, doc := range docs {
			data := doc.Data()
			extractFieldTypes("", data, fieldsMap)
		}

		fields := make([]FieldSchema, 0, len(fieldsMap))
		for fieldName, typeSet := range fieldsMap {
			typesList := make([]string, 0, len(typeSet))
			for t := range typeSet {
				typesList = append(typesList, t)
			}
			fields = append(fields, FieldSchema{
				Name:  fieldName,
				Types: typesList,
			})
		}

		result = append(result, CollectionSchema{
			Collection: collName,
			Fields:     fields,
		})
	}

	return result, nil
}

func (s *Source) getSchemaFromPipeline(ctx context.Context, collection string) (CollectionSchema, error) {
	getSchemaQueryBytes, err := json.Marshal(map[string]string{
		"collection": collection,
		"semantics":  "mongodb",
	})
	if err != nil {
		return CollectionSchema{}, fmt.Errorf("failed to marshal schema query: %w", err)
	}

	payload := map[string]any{
		"structuredPipeline": map[string]any{
			"pipeline": map[string]any{
				"stages": []map[string]any{
					{
						"name": "get_schema",
						"args": []map[string]any{
							{
								"stringValue": string(getSchemaQueryBytes),
							},
						},
					},
				},
			},
		},
	}
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return CollectionSchema{}, err
	}

	userAgent, _ := util.UserAgentFromContext(ctx)
	if userAgent == "" {
		userAgent = "mcp-toolbox"
	}

	httpClient, err := google.DefaultClient(ctx, "https://www.googleapis.com/auth/datastore", "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return CollectionSchema{}, err
	}

	url := fmt.Sprintf("https://firestore.googleapis.com/v1/projects/%s/databases/%s/documents:executePipeline", s.GetProjectId(), s.GetDatabaseId())
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return CollectionSchema{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("x-goog-request-params", fmt.Sprintf("project_id=%s&database_id=%s", s.GetProjectId(), s.GetDatabaseId()))
	req.Header.Set("x-goog-firestore-api-requester", "querydata")

	resp, err := httpClient.Do(req)
	if err != nil {
		return CollectionSchema{}, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return CollectionSchema{}, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return CollectionSchema{}, fmt.Errorf("get_schema API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var rawResult any
	if err := json.Unmarshal(respBody, &rawResult); err != nil {
		return CollectionSchema{}, err
	}

	fields := parseFieldsFromPipelineResponse(rawResult)
	return CollectionSchema{
		Collection: collection,
		Fields:     fields,
	}, nil
}

func parseFieldsFromPipelineResponse(raw any) []FieldSchema {
	var fields []FieldSchema
	switch data := raw.(type) {
	case []any:
		for _, item := range data {
			if itemMap, ok := item.(map[string]any); ok {
				if docMap, ok := itemMap["result"].(map[string]any); ok {
					if fMap, ok := docMap["fields"].(map[string]any); ok {
						fields = append(fields, flattenFieldsFromMap("", fMap)...)
					}
				} else if docMap, ok := itemMap["document"].(map[string]any); ok {
					if fMap, ok := docMap["fields"].(map[string]any); ok {
						fields = append(fields, flattenFieldsFromMap("", fMap)...)
					}
				} else if fMap, ok := itemMap["fields"].(map[string]any); ok {
					fields = append(fields, flattenFieldsFromMap("", fMap)...)
				}
			}
		}
	case map[string]any:
		if results, ok := data["results"].([]any); ok {
			return parseFieldsFromPipelineResponse(results)
		}
		if fMap, ok := data["fields"].(map[string]any); ok {
			fields = append(fields, flattenFieldsFromMap("", fMap)...)
		}
	}
	return fields
}

func flattenFieldsFromMap(prefix string, fieldsMap map[string]any) []FieldSchema {
	var result []FieldSchema
	for name, val := range fieldsMap {
		fieldName := name
		if prefix != "" {
			fieldName = prefix + "." + name
		}
		if valMap, ok := val.(map[string]any); ok {
			if strVal, hasStr := valMap["stringValue"].(string); hasStr {
				result = append(result, FieldSchema{
					Name:  fieldName,
					Types: []string{strVal},
				})
			} else if mapVal, hasMap := valMap["mapValue"].(map[string]any); hasMap {
				if innerFields, ok := mapVal["fields"].(map[string]any); ok {
					result = append(result, flattenFieldsFromMap(fieldName, innerFields)...)
				}
			} else {
				for k := range valMap {
					t := strings.TrimSuffix(k, "Value")
					result = append(result, FieldSchema{
						Name:  fieldName,
						Types: []string{t},
					})
					break
				}
			}
		} else if strVal, ok := val.(string); ok {
			result = append(result, FieldSchema{
				Name:  fieldName,
				Types: []string{strVal},
			})
		}
	}
	return result
}

func extractFieldTypes(prefix string, data map[string]any, fieldsMap map[string]map[string]bool) {
	for k, v := range data {
		fullKey := k
		if prefix != "" {
			fullKey = prefix + "." + k
		}
		typeName := getTypeName(v)
		if fieldsMap[fullKey] == nil {
			fieldsMap[fullKey] = make(map[string]bool)
		}
		fieldsMap[fullKey][typeName] = true

		if nestedMap, ok := v.(map[string]any); ok {
			extractFieldTypes(fullKey, nestedMap, fieldsMap)
		}
	}
}

func getTypeName(v any) string {
	if v == nil {
		return "null"
	}
	switch v.(type) {
	case string:
		return "string"
	case bool:
		return "boolean"
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return "integer"
	case float32, float64:
		return "double"
	case time.Time:
		return "timestamp"
	case map[string]any:
		return "map"
	case []any:
		return "array"
	case *latlng.LatLng:
		return "geopoint"
	case *firestore.DocumentRef:
		return "reference"
	case []byte:
		return "bytes"
	default:
		return fmt.Sprintf("%T", v)
	}
}

// ExecuteMQL sends an MQL query to the Firestore executePipeline API via the iql stage or as a raw structured pipeline.
func (s *Source) ExecuteMQL(ctx context.Context, query string) (any, error) {
	userAgent, err := util.UserAgentFromContext(ctx)
	if err != nil {
		userAgent = "mcp-toolbox"
	}

	httpClient, err := google.DefaultClient(ctx, "https://www.googleapis.com/auth/datastore", "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return nil, fmt.Errorf("failed to create authenticated HTTP client: %w", err)
	}

	url := fmt.Sprintf("https://firestore.googleapis.com/v1/projects/%s/databases/%s/documents:executePipeline", s.GetProjectId(), s.GetDatabaseId())

	trimmed := strings.TrimSpace(query)
	var bodyBytes []byte

	// If the query is already formatted as a full structuredPipeline JSON payload
	if strings.HasPrefix(trimmed, "{") && (strings.Contains(trimmed, "structuredPipeline") || strings.Contains(trimmed, "pipeline")) {
		bodyBytes = []byte(trimmed)
	} else {
		mqlQuery := trimmed
		if !strings.HasPrefix(mqlQuery, "db.") && !strings.HasPrefix(mqlQuery, "db[") {
			mqlQuery = "db." + mqlQuery
		}

		// Construct the structuredPipeline payload with "iql" stage
		payload := map[string]any{
			"structuredPipeline": map[string]any{
				"pipeline": map[string]any{
					"stages": []map[string]any{
						{
							"name": "iql",
							"args": []map[string]any{
								{
									"stringValue": mqlQuery,
								},
							},
						},
					},
				},
			},
		}
		bodyBytes, err = json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal pipeline payload: %w", err)
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("x-goog-request-params", fmt.Sprintf("project_id=%s&database_id=%s", s.GetProjectId(), s.GetDatabaseId()))
	req.Header.Set("x-goog-firestore-api-requester", "querydata")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute pipeline request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("executePipeline API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result any
	if err := json.Unmarshal(respBody, &result); err != nil {
		return string(respBody), nil
	}

	return result, nil
}

func initFirestoreConnection(
	ctx context.Context,
	project string,
	database string,
) (*firestore.Client, error) {
	userAgent, err := util.UserAgentFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// If database is not specified, use the default database
	if database == "" {
		database = "(default)"
	}

	// Create the Firestore client
	client, err := firestore.NewClientWithDatabase(ctx, project, database, option.WithUserAgent(userAgent))
	if err != nil {
		return nil, fmt.Errorf("failed to create Firestore client for project %q and database %q: %w", project, database, err)
	}

	return client, nil
}

func initFirebaseRulesConnection(
	ctx context.Context,
	project string,
) (*firebaserules.Service, error) {
	// Create the Firebase Rules client
	rulesClient, err := firebaserules.NewService(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create Firebase Rules client for project %q: %w", project, err)
	}

	return rulesClient, nil
}
