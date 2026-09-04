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

package dataplexupdatedataproductaspects

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	dataplexpb "cloud.google.com/go/dataplex/apiv1/dataplexpb"
	"github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/structpb"
)

const resourceType string = "dataplex-update-data-product-aspects"

func init() {
	if !tools.Register(resourceType, newConfig) {
		panic(fmt.Sprintf("tool type %q already registered", resourceType))
	}
}

func newConfig(ctx context.Context, name string, decoder *yaml.Decoder) (tools.ToolConfig, error) {
	actual := Config{ConfigBase: tools.ConfigBase{Name: name}}
	if err := decoder.DecodeContext(ctx, &actual); err != nil {
		return nil, err
	}
	return actual, nil
}

type compatibleSource interface {
	ProjectID() string
	ProjectNumber() int64
	ProjectNumberContext(ctx context.Context) (int64, error)
	UpdateEntry(ctx context.Context, entry *dataplexpb.Entry, updateMask *fieldmaskpb.FieldMask) (*dataplexpb.Entry, error)
}

type Config struct {
	tools.ConfigBase `yaml:",inline"`
	Type             string                 `yaml:"type" validate:"required"`
	Source           string                 `yaml:"source" validate:"required"`
	Parameters       parameters.Parameters  `yaml:"parameters"`
	Annotations      *tools.ToolAnnotations `yaml:"annotations,omitempty"`
}

// validate interface
var _ tools.ToolConfig = Config{}

func (cfg Config) ToolConfigType() string {
	return resourceType
}

type Aspect struct {
	ProjectID    string         `json:"projectId" validate:"required"`
	LocationID   string         `json:"locationId" validate:"required"`
	AspectTypeId string         `json:"aspectTypeId" validate:"required"`
	Data         map[string]any `json:"data,omitempty"`
}

type EntrySource struct {
	Resource    string `json:"resource,omitempty"`
	DisplayName string `json:"displayName,omitempty"`
	Description string `json:"description,omitempty"`
}

type UpdateDataProductAspectsResponse struct {
	Name        string       `json:"name"`
	EntrySource *EntrySource `json:"entrySource,omitempty"`
	EntryType   string       `json:"entryType,omitempty"`
	Aspects     []Aspect     `json:"aspects"`
}

func (cfg Config) Initialize(context.Context) (tools.Tool, error) {
	locationID := parameters.NewStringParameter("locationId", "The location ID (e.g. 'us', 'us-central1') of the Data Product.")
	dataProductID := parameters.NewStringParameter("dataProductId", "The unique ID of the Data Product.")

	aspectSchema := parameters.NewMapParameter(
		"aspect",
		"Aspect details containing: projectId (string, required, 'dataplex-types' for system aspects), locationId (string, required, 'global' for system aspects), aspectTypeId (string, required, e.g. 'overview' or 'refresh-cadence'), and data (object, required, the aspect payload details. For 'overview' (documentation), data accepts: content (string, required, markdown or text), contentType (string, optional, MARKDOWN or HTML), and links (array of objects with url and title). For 'refresh-cadence' (contract), data accepts: frequency (string, required: Daily, Weekly, Monthly, etc.), refreshTime (string, optional, e.g. '09:00 PST'), thresholdInMinutes (int, optional), and cronSchedule (string, optional)).",
		"",
	)
	aspects := parameters.NewArrayParameter(
		"aspects",
		"The list of aspects to update on the Data Product Entry.",
		aspectSchema,
	)

	allParameters := parameters.Parameters{locationID, dataProductID, aspects}

	return Tool{
		BaseTool: tools.NewBaseTool(
			cfg,
			tools.GetAnnotationsOrDefault(cfg.Annotations, tools.NewDestructiveAnnotations),
			tools.Manifest{Description: cfg.Description, Parameters: allParameters.Manifest(), AuthRequired: cfg.AuthRequired},
			allParameters,
		),
	}, nil
}

// validate interface
var _ tools.Tool = Tool{}

type Tool struct {
	tools.BaseTool[Config]
}

func (t Tool) GetSourceName() string {
	return t.Cfg.Source
}

func (t Tool) ToConfig() tools.ToolConfig {
	return t.Cfg
}

func (t Tool) ValidateSource(source sources.Source) error {
	_, ok := source.(compatibleSource)
	if !ok {
		return fmt.Errorf("invalid source for %q tool: source %q is not a compatible type", t.Cfg.Type, t.Cfg.Source)
	}
	return nil
}

func (t Tool) Invoke(ctx context.Context, s sources.Source, params parameters.ParamValues, accessToken tools.AccessToken) (any, util.ToolboxError) {
	source, ok := s.(compatibleSource)
	if !ok {
		return nil, util.NewClientServerError("source used is not compatible with the tool", http.StatusInternalServerError, nil)
	}

	paramsMap := params.AsMap()

	locationID, _ := paramsMap["locationId"].(string)
	if locationID == "" {
		return nil, util.NewAgentError("locationId parameter is required and must be a non-empty string", nil)
	}

	dataProductID, _ := paramsMap["dataProductId"].(string)
	if dataProductID == "" {
		return nil, util.NewAgentError("dataProductId parameter is required and must be a non-empty string", nil)
	}

	rawAspects, ok := paramsMap["aspects"].([]any)
	if !ok {
		return nil, util.NewAgentError("aspects parameter is required and must be an array", nil)
	}

	rawAspectsBytes, err := json.Marshal(rawAspects)
	if err != nil {
		return nil, util.NewAgentError("failed to marshal aspects parameter", err)
	}

	var parsedAspects []Aspect
	if err := json.Unmarshal(rawAspectsBytes, &parsedAspects); err != nil {
		return nil, util.NewAgentError("failed to unmarshal aspects parameter into required format", err)
	}

	projectID := source.ProjectID()

	// Convert input array of aspects to aspects map
	aspectsMap := make(map[string]*dataplexpb.Aspect)

	for i, aspect := range parsedAspects {
		if aspect.AspectTypeId == "" {
			return nil, util.NewAgentError(fmt.Sprintf("aspectTypeId is required for aspect at index %d", i), nil)
		}

		if aspect.Data == nil {
			return nil, util.NewAgentError(fmt.Sprintf("data is required for aspect at index %d", i), nil)
		}

		aspectProjID := aspect.ProjectID
		if aspectProjID == "" {
			if aspect.AspectTypeId == "overview" || aspect.AspectTypeId == "refresh-cadence" {
				aspectProjID = "dataplex-types"
			} else {
				aspectProjID = projectID
			}
		}

		aspectLocID := aspect.LocationID
		if aspectLocID == "" {
			if aspect.AspectTypeId == "overview" || aspect.AspectTypeId == "refresh-cadence" {
				aspectLocID = "global"
			} else {
				aspectLocID = locationID
			}
		}

		aspectType := fmt.Sprintf("projects/%s/locations/%s/aspectTypes/%s", aspectProjID, aspectLocID, aspect.AspectTypeId)
		aspectKey := fmt.Sprintf("%s.%s.%s", aspectProjID, aspectLocID, aspect.AspectTypeId)

		structData, err := structpb.NewStruct(aspect.Data)
		if err != nil {
			return nil, util.NewAgentError(fmt.Sprintf("failed to serialize data for aspect %q: %s", aspect.AspectTypeId, err), err)
		}

		aspectsMap[aspectKey] = &dataplexpb.Aspect{
			AspectType: aspectType,
			Data:       structData,
		}
	}

	projectNumber, err := source.ProjectNumberContext(ctx)
	if err != nil {
		return nil, util.ProcessGcpError(err)
	}

	entryName := fmt.Sprintf(
		"projects/%s/locations/%s/entryGroups/@dataplex/entries/projects/%d/locations/%s/dataProducts/%s",
		projectID, locationID, projectNumber, locationID, dataProductID,
	)

	entry := &dataplexpb.Entry{
		Name:    entryName,
		Aspects: aspectsMap,
	}

	updateMask, _ := fieldmaskpb.New(entry, "aspects")

	returnedEntry, err := source.UpdateEntry(ctx, entry, updateMask)
	if err != nil {
		return nil, util.ProcessGcpError(err)
	}

	// Format returned entry aspects back to the expected output Aspects format
	var returnedAspects []Aspect
	for _, aspectProto := range returnedEntry.Aspects {
		parts := strings.Split(aspectProto.AspectType, "/")
		if len(parts) < 6 {
			continue
		}
		aspectProjID := parts[1]
		aspectLocID := parts[3]
		aspectTypeName := parts[5]
		data := aspectProto.Data.AsMap()
		returnedAspects = append(returnedAspects, Aspect{
			AspectTypeId: aspectTypeName,
			Data:         data,
			ProjectID:    aspectProjID,
			LocationID:   aspectLocID,
		})
	}

	var entrySource *EntrySource
	if returnedEntry.GetEntrySource() != nil {
		entrySource = &EntrySource{
			Resource:    returnedEntry.GetEntrySource().GetResource(),
			DisplayName: returnedEntry.GetEntrySource().GetDisplayName(),
			Description: returnedEntry.GetEntrySource().GetDescription(),
		}
	}

	return UpdateDataProductAspectsResponse{
		Name:        returnedEntry.GetName(),
		EntrySource: entrySource,
		EntryType:   returnedEntry.GetEntryType(),
		Aspects:     returnedAspects,
	}, nil
}
