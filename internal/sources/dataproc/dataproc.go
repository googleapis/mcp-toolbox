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

package dataproc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	dataproc "cloud.google.com/go/dataproc/v2/apiv1"
	"cloud.google.com/go/dataproc/v2/apiv1/dataprocpb"
	longrunning "cloud.google.com/go/longrunning/autogen"
	"github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/protobuf/encoding/protojson"
)

const SourceType string = "dataproc"

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
	Region  string `yaml:"region" validate:"required"`
}

func (r Config) SourceConfigType() string {
	return SourceType
}

func (r Config) Initialize(ctx context.Context, tracer trace.Tracer) (sources.Source, error) {
	ua, err := util.UserAgentFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("error in User Agent retrieval: %s", err)
	}
	endpoint := fmt.Sprintf("%s-dataproc.googleapis.com:443", r.Region)
	client, err := dataproc.NewClusterControllerClient(ctx, option.WithEndpoint(endpoint), option.WithUserAgent(ua))
	if err != nil {
		return nil, fmt.Errorf("failed to create dataproc client: %w", err)
	}
	opsClient, err := longrunning.NewOperationsClient(ctx, option.WithEndpoint(endpoint), option.WithUserAgent(ua))
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("failed to create longrunning client: %w", err)
	}
	jobClient, err := dataproc.NewJobControllerClient(ctx, option.WithEndpoint(endpoint), option.WithUserAgent(ua))
	if err != nil {
		client.Close()
		opsClient.Close()
		return nil, fmt.Errorf("failed to create dataproc job client: %w", err)
	}

	s := &Source{
		Config:    r,
		Client:    client,
		OpsClient: opsClient,
		JobClient: jobClient,
	}
	return s, nil
}

var _ sources.Source = &Source{}

type Source struct {
	Config
	Client    *dataproc.ClusterControllerClient
	OpsClient *longrunning.OperationsClient
	JobClient *dataproc.JobControllerClient
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

func (s *Source) GetClusterControllerClient() *dataproc.ClusterControllerClient {
	return s.Client
}

func (s *Source) GetOperationsClient(ctx context.Context) (*longrunning.OperationsClient, error) {
	return s.OpsClient, nil
}

func (s *Source) GetJobControllerClient() *dataproc.JobControllerClient {
	return s.JobClient
}

func (s *Source) Close() error {
	return errors.Join(s.Client.Close(), s.OpsClient.Close(), s.JobClient.Close())
}

// ListClustersResponse is the response from the list clusters API.
type ListClustersResponse struct {
	Clusters      []Cluster `json:"clusters"`
	NextPageToken string    `json:"nextPageToken"`
}

// Cluster represents a single Dataproc cluster.
type Cluster struct {
	Name       string `json:"name"` // Full resource name
	UUID       string `json:"uuid"`
	State      string `json:"state"`
	CreateTime string `json:"createTime"`
	ConsoleURL string `json:"consoleUrl"`
	LogsURL    string `json:"logsUrl"`
}

// ListClusters executes the list clusters operation.
func (s *Source) ListClusters(ctx context.Context, pageSize *int, pageToken, filter string) (any, error) {
	client := s.GetClusterControllerClient()

	req := &dataprocpb.ListClustersRequest{
		ProjectId: s.Project,
		Region:    s.Region,
	}

	if pageSize != nil {
		req.PageSize = int32(*pageSize)
	}
	if pageToken != "" {
		req.PageToken = pageToken
	}
	if filter != "" {
		req.Filter = filter
	}

	it := client.ListClusters(ctx, req)
	ps := 0
	if pageSize != nil {
		ps = *pageSize
	}
	pager := iterator.NewPager(it, ps, req.PageToken)

	clusterPbs := []*dataprocpb.Cluster{}
	nextPageToken, err := pager.NextPage(&clusterPbs)
	if err != nil {
		return nil, fmt.Errorf("failed to list clusters: %w", err)
	}

	clusters, err := ToClusters(clusterPbs, s.Region)
	if err != nil {
		return nil, err
	}

	return ListClustersResponse{Clusters: clusters, NextPageToken: nextPageToken}, nil
}

// ToClusters converts a slice of protobuf Cluster messages to a slice of Cluster structs.
func ToClusters(clusterPbs []*dataprocpb.Cluster, region string) ([]Cluster, error) {
	clusters := make([]Cluster, 0, len(clusterPbs))
	for _, clusterPb := range clusterPbs {
		consoleUrl := ClusterConsoleURLFromProto(clusterPb, region)
		logsUrl := ClusterLogsURLFromProto(clusterPb, region)

		state := "STATE_UNSPECIFIED"
		// Extract create time from status history.
		var createTime string
		if clusterPb.Status != nil {
			state = clusterPb.Status.State.Enum().String()
			if clusterPb.Status.StateStartTime != nil {
				createTime = clusterPb.Status.StateStartTime.AsTime().Format(time.RFC3339)
			}
		}

		fullName := fmt.Sprintf("projects/%s/regions/%s/clusters/%s", clusterPb.ProjectId, region, clusterPb.ClusterName)

		cluster := Cluster{
			Name:       fullName,
			UUID:       clusterPb.ClusterUuid,
			State:      state,
			CreateTime: createTime,
			ConsoleURL: consoleUrl,
			LogsURL:    logsUrl,
		}
		clusters = append(clusters, cluster)
	}
	return clusters, nil
}

// GetCluster gets a single cluster.
func (s *Source) GetCluster(ctx context.Context, clusterName string) (any, error) {
	client := s.GetClusterControllerClient()

	req := &dataprocpb.GetClusterRequest{
		ProjectId:   s.Project,
		Region:      s.Region,
		ClusterName: clusterName,
	}

	clusterPb, err := client.GetCluster(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster: %w", err)
	}

	jsonBytes, err := protojson.Marshal(clusterPb)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal cluster to JSON: %w", err)
	}

	var result map[string]any
	if err := json.Unmarshal(jsonBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cluster JSON: %w", err)
	}

	consoleUrl := ClusterConsoleURLFromProto(clusterPb, s.Region)
	logsUrl := ClusterLogsURLFromProto(clusterPb, s.Region)

	wrappedResult := map[string]any{
		"consoleUrl": consoleUrl,
		"logsUrl":    logsUrl,
		"cluster":    result,
	}

	return wrappedResult, nil
}

// ListJobsResponse is the response from the list jobs API.
type ListJobsResponse struct {
	Jobs          []Job  `json:"jobs"`
	NextPageToken string `json:"nextPageToken"`
}

// Job represents a single Dataproc job.
type Job struct {
	ID          string `json:"id"`
	Status      string `json:"status"`
	SubStatus   string `json:"subStatus,omitempty"`
	StartTime   string `json:"startTime"`
	EndTime     string `json:"endTime,omitempty"`
	ClusterName string `json:"clusterName"`
	ConsoleURL  string `json:"consoleUrl"`
	LogsURL     string `json:"logsUrl"`
}

// ListJobs executes the list jobs operation.
func (s *Source) ListJobs(ctx context.Context, pageSize *int, pageToken, filter, jobStateMatcher string) (any, error) {
	client := s.GetJobControllerClient()

	req := &dataprocpb.ListJobsRequest{
		ProjectId: s.Project,
		Region:    s.Region,
	}

	if pageSize != nil {
		req.PageSize = int32(*pageSize)
	}
	if pageToken != "" {
		req.PageToken = pageToken
	}
	if filter != "" {
		req.Filter = filter
	}
	if jobStateMatcher != "" {
		if v, ok := dataprocpb.ListJobsRequest_JobStateMatcher_value[jobStateMatcher]; ok {
			req.JobStateMatcher = dataprocpb.ListJobsRequest_JobStateMatcher(v)
		} else {
			return nil, fmt.Errorf("invalid jobStateMatcher: %s. Supported values: ALL, ACTIVE, NON_ACTIVE", jobStateMatcher)
		}
	}

	it := client.ListJobs(ctx, req)
	ps := 0
	if pageSize != nil {
		ps = *pageSize
	}
	pager := iterator.NewPager(it, ps, req.PageToken)

	var jobPbs []*dataprocpb.Job
	nextPageToken, err := pager.NextPage(&jobPbs)
	if err != nil {
		return nil, fmt.Errorf("failed to list jobs: %w", err)
	}

	jobs, err := ToJobs(jobPbs, s.Region)
	if err != nil {
		return nil, err
	}

	return ListJobsResponse{Jobs: jobs, NextPageToken: nextPageToken}, nil
}

// ToJobs converts a slice of protobuf Job messages to a slice of Job structs.
func ToJobs(jobPbs []*dataprocpb.Job, region string) ([]Job, error) {
	jobs := make([]Job, 0, len(jobPbs))
	for _, jobPb := range jobPbs {
		consoleUrl := JobConsoleURLFromProto(jobPb, region)
		logsUrl, err := JobLogsURLFromProto(jobPb, region)
		if err != nil {
			return nil, fmt.Errorf("error generating logs url: %v", err)
		}

		status := "STATE_UNSPECIFIED"
		subStatus := ""
		var startTime, endTime string

		if jobPb.Status != nil {
			status = jobPb.Status.State.Enum().String()
			subStatus = jobPb.Status.Substate.Enum().String()
		}

		var sTime, eTime time.Time
		for _, s := range jobPb.StatusHistory {
			t := s.StateStartTime.AsTime()
			if sTime.IsZero() || t.Before(sTime) {
				sTime = t
			}
		}
		if jobPb.Status != nil {
			t := jobPb.Status.StateStartTime.AsTime()
			if sTime.IsZero() || t.Before(sTime) {
				sTime = t
			}
			switch jobPb.Status.State {
			case dataprocpb.JobStatus_DONE, dataprocpb.JobStatus_CANCELLED, dataprocpb.JobStatus_ERROR:
				eTime = t
			}
		}

		if !sTime.IsZero() {
			startTime = sTime.Format(time.RFC3339)
		}
		if !eTime.IsZero() {
			endTime = eTime.Format(time.RFC3339)
		}

		clusterName := ""
		if jobPb.Placement != nil {
			clusterName = jobPb.Placement.ClusterName
		}

		job := Job{
			ID:          jobPb.Reference.JobId,
			Status:      status,
			SubStatus:   subStatus,
			StartTime:   startTime,
			EndTime:     endTime,
			ClusterName: clusterName,
			ConsoleURL:  consoleUrl,
			LogsURL:     logsUrl,
		}
		jobs = append(jobs, job)
	}
	return jobs, nil
}

// GetJob gets a single job.
func (s *Source) GetJob(ctx context.Context, jobId string) (any, error) {
	client := s.GetJobControllerClient()

	req := &dataprocpb.GetJobRequest{
		ProjectId: s.Project,
		Region:    s.Region,
		JobId:     jobId,
	}

	jobPb, err := client.GetJob(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get job: %w", err)
	}

	jsonBytes, err := protojson.Marshal(jobPb)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal job to JSON: %w", err)
	}

	var result map[string]any
	if err := json.Unmarshal(jsonBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal job JSON: %w", err)
	}

	consoleUrl := JobConsoleURLFromProto(jobPb, s.Region)
	logsUrl, err := JobLogsURLFromProto(jobPb, s.Region)
	if err != nil {
		return nil, fmt.Errorf("error generating logs url: %v", err)
	}

	wrappedResult := map[string]any{
		"consoleUrl": consoleUrl,
		"logsUrl":    logsUrl,
		"job":        result,
	}

	return wrappedResult, nil
}
