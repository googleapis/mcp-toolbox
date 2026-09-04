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

package datalineage

import (
	"context"
	"fmt"
	"io"

	lineage "cloud.google.com/go/datacatalog/lineage/apiv1"
	lineagepb "cloud.google.com/go/datacatalog/lineage/apiv1/lineagepb"
	"github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"google.golang.org/grpc/metadata"
)

const SourceType string = "datalineage"

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
	Name    string `yaml:"name" validate:"required"`
	Type    string `yaml:"type" validate:"required"`
	Project string `yaml:"project" validate:"required"`
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
		conn:   sources.NewConnectOnce[*lineage.Client](ctx, r.Name, SourceType, tracer),
	}
}

var _ sources.Source = &Source{}

type Source struct {
	Config
	conn *sources.ConnectOnce[*lineage.Client]
}

func (s *Source) client(ctx context.Context) (*lineage.Client, error) {
	return s.conn.Do(ctx, func(ctx context.Context) (*lineage.Client, error) {
		return initLineageConnection(ctx, s.Project)
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

func (s *Source) ProjectID() string {
	return s.Project
}

func initLineageConnection(
	ctx context.Context,
	project string,
) (*lineage.Client, error) {
	cred, err := google.FindDefaultCredentials(ctx, sources.CloudPlatformScope)
	if err != nil {
		return nil, fmt.Errorf("failed to find default Google Cloud credentials for project %q: %w", project, err)
	}

	userAgent, err := util.UserAgentFromContext(ctx)
	if err != nil {
		return nil, err
	}

	client, err := lineage.NewClient(ctx, option.WithUserAgent(userAgent), option.WithCredentials(cred))
	if err != nil {
		return nil, fmt.Errorf("failed to create Lineage client for project %q: %w", project, err)
	}

	return client, nil
}

func (s *Source) SearchLineageStreaming(
	ctx context.Context,
	parentLocation string,
	locations []string,
	rootEntities []*lineagepb.EntityReference,
	direction lineagepb.SearchLineageStreamingRequest_SearchDirection,
	maxDepth int32,
	maxResults int32,
	maxProcessPerLink int32,
	requestProcessDetails bool,
) ([]*lineagepb.LineageLink, []string, error) {
	client, err := s.client(ctx)
	if err != nil {
		return nil, nil, err
	}

	parent := fmt.Sprintf("projects/%s/locations/%s", s.ProjectID(), parentLocation)

	req := &lineagepb.SearchLineageStreamingRequest{
		Parent:    parent,
		Locations: locations,
		RootCriteria: &lineagepb.SearchLineageStreamingRequest_RootCriteria{
			Criteria: &lineagepb.SearchLineageStreamingRequest_RootCriteria_Entities{
				Entities: &lineagepb.MultipleEntityReference{
					Entities: rootEntities,
				},
			},
		},
		Direction: direction,
	}

	if maxDepth > 0 || maxResults > 0 || maxProcessPerLink > 0 {
		req.Limits = &lineagepb.SearchLineageStreamingRequest_SearchLimits{
			MaxDepth:          maxDepth,
			MaxResults:        maxResults,
			MaxProcessPerLink: maxProcessPerLink,
		}
	}

	if requestProcessDetails {
		ctx = metadata.AppendToOutgoingContext(ctx, "x-goog-fieldmask", "links,links.processes.process,unreachable")
	}

	stream, err := client.SearchLineageStreaming(ctx, req)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to start search lineage streaming: %w", err)
	}

	var links []*lineagepb.LineageLink
	unreachableMap := make(map[string]bool)
	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, fmt.Errorf("error receiving from search lineage stream: %w", err)
		}
		links = append(links, resp.GetLinks()...)
		for _, loc := range resp.GetUnreachable() {
			unreachableMap[loc] = true
		}
	}

	var unreachable []string
	if len(unreachableMap) > 0 {
		unreachable = make([]string, 0, len(unreachableMap))
		for loc := range unreachableMap {
			unreachable = append(unreachable, loc)
		}
	}

	return links, unreachable, nil
}
