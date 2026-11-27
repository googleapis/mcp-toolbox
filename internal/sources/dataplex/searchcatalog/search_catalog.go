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

package searchcatalog

import (
	"context"
	"fmt"
	"strings"
	"sync"

	dataplexapi "cloud.google.com/go/dataplex/apiv1"
	dataplexpb "cloud.google.com/go/dataplex/apiv1/dataplexpb"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	"golang.org/x/oauth2"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

// DataplexSearchResponse represents a simplified search result.
type DataplexSearchResponse struct {
	DisplayName   string
	Description   string
	Type          string
	Resource      string
	DataplexEntry string
}

// ConstructSearchQueryHelper builds a query clause for a specific predicate.
func ConstructSearchQueryHelper(predicate string, operator string, items []string) string {
	if len(items) == 0 {
		return ""
	}

	if len(items) == 1 {
		return predicate + operator + items[0]
	}

	var builder strings.Builder
	builder.WriteString("(")
	for i, item := range items {
		if i > 0 {
			builder.WriteString(" OR ")
		}
		builder.WriteString(predicate)
		builder.WriteString(operator)
		builder.WriteString(item)
	}
	builder.WriteString(")")
	return builder.String()
}

// ConstructSearchQuery builds the full Dataplex search query.
func ConstructSearchQuery(prompt string, projectIds []string, parentIds []string, types []string, system string) string {
	queryParts := []string{}

	if clause := ConstructSearchQueryHelper("projectid", "=", projectIds); clause != "" {
		queryParts = append(queryParts, clause)
	}

	if clause := ConstructSearchQueryHelper("parent", "=", parentIds); clause != "" {
		queryParts = append(queryParts, clause)
	}

	if clause := ConstructSearchQueryHelper("type", "=", types); clause != "" {
		queryParts = append(queryParts, clause)
	}

	if system != "" {
		queryParts = append(queryParts, "system="+system)
	}

	return fmt.Sprintf("%s %s", prompt, strings.Join(queryParts, " AND "))
}

// ExtractType extracts the mapped type from a resource string based on a type map.
func ExtractType(resourceString string, typeMap map[string]string) string {
	lastIndex := strings.LastIndex(resourceString, "/")
	if lastIndex == -1 {
		return resourceString
	}
	return typeMap[resourceString[lastIndex+1:]]
}

// ExecuteSearch performs the search and processes results.
func ExecuteSearch(ctx context.Context, client *dataplexapi.CatalogClient, req *dataplexpb.SearchEntriesRequest, typeMap map[string]string) ([]DataplexSearchResponse, error) {
	if client == nil {
		return nil, fmt.Errorf("dataplex catalog client is nil")
	}
	it := client.SearchEntries(ctx, req)
	if it == nil {
		return nil, fmt.Errorf("failed to create search entries iterator")
	}

	results := []DataplexSearchResponse{}
	for req.PageSize <= 0 || len(results) < int(req.PageSize) {
		entry, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		entrySource := entry.DataplexEntry.GetEntrySource()

		resp := DataplexSearchResponse{
			DisplayName:   entrySource.GetDisplayName(),
			Description:   entrySource.GetDescription(),
			Type:          ExtractType(entry.DataplexEntry.GetEntryType(), typeMap),
			Resource:      entrySource.GetResource(),
			DataplexEntry: entry.DataplexEntry.GetName(),
		}
		results = append(results, resp)
	}
	return results, nil
}

// InvokeSearchCatalog performs the common search catalog logic.
func InvokeSearchCatalog(
	ctx context.Context,
	paramsMap map[string]any,
	tokenStr string,
	systemName string,
	parentParamName string,
	typeMap map[string]string,
	projectID string,
	getCatalogClient func(ctx context.Context, token string) (*dataplexapi.CatalogClient, error),
) ([]DataplexSearchResponse, error) {
	pageSize := int32(paramsMap["pageSize"].(int))
	prompt, _ := paramsMap["prompt"].(string)

	projectIdSlice, err := parameters.ConvertAnySliceToTyped(paramsMap["projectIds"].([]any), "string")
	if err != nil {
		return nil, fmt.Errorf("can't convert projectIds to array of strings: %w", err)
	}
	projectIds := projectIdSlice.([]string)

	parentIdSlice, err := parameters.ConvertAnySliceToTyped(paramsMap[parentParamName].([]any), "string")
	if err != nil {
		return nil, fmt.Errorf("can't convert %s to array of strings: %w", parentParamName, err)
	}
	parentIds := parentIdSlice.([]string)

	typesSlice, err := parameters.ConvertAnySliceToTyped(paramsMap["types"].([]any), "string")
	if err != nil {
		return nil, fmt.Errorf("can't convert types to array of strings: %w", err)
	}
	types := typesSlice.([]string)

	query := ConstructSearchQuery(prompt, projectIds, parentIds, types, systemName)

	req := &dataplexpb.SearchEntriesRequest{
		Query:          query,
		Name:           fmt.Sprintf("projects/%s/locations/global", projectID),
		PageSize:       pageSize,
		SemanticSearch: true,
	}

	catalogClient, err := getCatalogClient(ctx, tokenStr)
	if err != nil {
		return nil, fmt.Errorf("failed to get catalog client: %w", err)
	}

	return ExecuteSearch(ctx, catalogClient, req, typeMap)
}

// Cache is an interface for a thread-safe, expiring key-value store.
type Cache interface {
	Get(key string) (any, bool)
	Set(key string, value any)
}

// DataplexClientManager manages Dataplex Catalog clients, including caching and lazy initialization.
type DataplexClientManager struct {
	UseClientOAuth       bool
	Cache                Cache
	DefaultClientCreator func(ctx context.Context) (*dataplexapi.CatalogClient, error)
	OAuthClientCreator   func(ctx context.Context, token string) (*dataplexapi.CatalogClient, error)

	catalogClient    *dataplexapi.CatalogClient
	catalogClientErr error
	once             sync.Once
	mu               sync.Mutex
}

// GetCatalogClient returns a Dataplex Catalog client.
func (m *DataplexClientManager) GetCatalogClient(ctx context.Context, tokenString string) (*dataplexapi.CatalogClient, error) {
	if m.UseClientOAuth && tokenString != "" {
		m.mu.Lock()
		defer m.mu.Unlock()

		// Check cache
		if m.Cache != nil {
			if val, found := m.Cache.Get(tokenString); found {
				return val.(*dataplexapi.CatalogClient), nil
			}
		}

		// Cache miss - create new client
		var client *dataplexapi.CatalogClient
		var initErr error
		if m.OAuthClientCreator != nil {
			client, initErr = m.OAuthClientCreator(ctx, tokenString)
		} else {
			userAgent, err := util.UserAgentFromContext(ctx)
			if err != nil {
				userAgent = "genai-toolbox"
			}
			ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: tokenString})
			client, initErr = dataplexapi.NewCatalogClient(ctx, option.WithUserAgent(userAgent), option.WithTokenSource(ts))
		}
		if initErr != nil {
			return nil, initErr
		}

		// Set in cache
		if m.Cache != nil {
			m.Cache.Set(tokenString, client)
		}
		return client, nil
	}

	// Fallback to default client (lazy initialized)
	m.once.Do(func() {
		if m.DefaultClientCreator != nil {
			m.catalogClient, m.catalogClientErr = m.DefaultClientCreator(context.Background())
		} else {
			userAgent, err := util.UserAgentFromContext(ctx)
			if err != nil {
				userAgent = "genai-toolbox"
			}
			// Uses Application Default Credentials (ADC) if no token is provided.
			m.catalogClient, m.catalogClientErr = dataplexapi.NewCatalogClient(context.Background(), option.WithUserAgent(userAgent))
		}
	})
	if m.catalogClientErr != nil {
		return nil, m.catalogClientErr
	}
	return m.catalogClient, nil
}
