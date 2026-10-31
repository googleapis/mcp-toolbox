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

package cloudhealthcare

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
)

// startHealthcareMCPServer starts a Toolbox server serving the given config
// over the MCP endpoint. Teardown is registered with t.Cleanup, so nothing
// leaks if startup fails. StartCmd's cleanup only removes the temporary config
// file, so Close is what stops the server and closes its pipes.
func startHealthcareMCPServer(t *testing.T, ctx context.Context, config map[string]any) {
	cmd, cleanup, err := tests.StartCmd(ctx, config)
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	t.Cleanup(func() {
		cmd.Close()
		cleanup()
	})

	waitCtx, cancelWait := context.WithTimeout(ctx, 10*time.Second)
	defer cancelWait()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: %s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}
}

func TestHealthcareMCPCallTool(t *testing.T) {
	sourceConfig := getHealthcareVars(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	t.Cleanup(cancel)

	healthcareService, err := newHealthcareService(ctx)
	if err != nil {
		t.Fatalf("failed to create healthcare service: %v", err)
	}

	fhirStoreID := "fhir-store-" + uuid.New().String()
	dicomStoreID := "dicom-store-" + uuid.New().String()

	patient1ID, patient2ID := setupHealthcareResources(t, healthcareService, healthcareDataset, fhirStoreID, dicomStoreID)

	toolsFile := getToolsConfig(sourceConfig)
	toolsFile = addClientAuthSourceConfig(t, toolsFile)

	startHealthcareMCPServer(t, ctx, toolsFile)

	datasetWant := fmt.Sprintf(`"name":"projects/%s/locations/%s/datasets/%s"`, healthcareProject, healthcareRegion, healthcareDataset)
	fhirStoreWant := fmt.Sprintf(`"name":"projects/%s/locations/%s/datasets/%s/fhirStores/%s"`, healthcareProject, healthcareRegion, healthcareDataset, fhirStoreID)
	dicomStoreWant := fmt.Sprintf(`"name":"projects/%s/locations/%s/datasets/%s/dicomStores/%s"`, healthcareProject, healthcareRegion, healthcareDataset, dicomStoreID)

	// Every helper below runs the same cases it runs over /api in the legacy
	// test, with the endpoint switched by WithMCPExec.
	runGetDatasetToolInvokeTest(t, datasetWant, withMCPExec())
	runListFHIRStoresToolInvokeTest(t, fhirStoreWant, withMCPExec())
	runListDICOMStoresToolInvokeTest(t, withMCPExec())
	runGetFHIRStoreToolInvokeTest(t, fhirStoreID, fhirStoreWant, withMCPExec())
	runGetFHIRStoreMetricsToolInvokeTest(t, fhirStoreID, `"metrics"`, withMCPExec())
	runGetFHIRResourceToolInvokeTest(t, fhirStoreID, "Patient", patient1ID, `"id":"`+patient1ID+`"`, withMCPExec())
	runFHIRPatientSearchToolInvokeTest(t, fhirStoreID, []string{patient1ID, patient2ID}, withMCPExec())
	runFHIRPatientEverythingToolInvokeTest(t, fhirStoreID, patient1ID, `"resourceType":"Bundle"`, withMCPExec())

	nextURL := getNextPageURLForPatientEverything(t, fhirStoreID, patient2ID, withMCPExec())
	runFHIRFetchPageToolInvokeTest(t, nextURL, `"total":1`, withMCPExec())

	runGetDICOMStoreToolInvokeTest(t, dicomStoreID, dicomStoreWant, withMCPExec())
	runGetDICOMStoreMetricsToolInvokeTest(t, dicomStoreID, dicomStoreWant, withMCPExec())
	runSearchDICOMStudiesToolInvokeTest(t, dicomStoreID, withMCPExec())
	runSearchDICOMSeriesToolInvokeTest(t, dicomStoreID, withMCPExec())
	runSearchDICOMInstancesToolInvokeTest(t, dicomStoreID, withMCPExec())
	runRetrieveRenderedDICOMInstanceToolInvokeTest(t, dicomStoreID, withMCPExec())
}

func TestHealthcareMCPCallToolWithStoreRestriction(t *testing.T) {
	sourceConfig := getHealthcareVars(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	t.Cleanup(cancel)

	healthcareService, err := newHealthcareService(ctx)
	if err != nil {
		t.Fatalf("failed to create healthcare service: %v", err)
	}

	// Create stores
	allowedFHIRStoreID := "fhir-store-allowed-" + uuid.New().String()
	allowedDICOMStoreID := "dicom-store-allowed-" + uuid.New().String()
	disallowedFHIRStoreID := "fhir-store-disallowed-" + uuid.New().String()
	disallowedDICOMStoreID := "dicom-store-disallowed-" + uuid.New().String()

	setupHealthcareStoresOnly(t, healthcareService, healthcareDataset, allowedFHIRStoreID, allowedDICOMStoreID)
	setupHealthcareStoresOnly(t, healthcareService, healthcareDataset, disallowedFHIRStoreID, disallowedDICOMStoreID)

	// Configure source with dataset restriction.
	sourceConfig["allowedFhirStores"] = []string{allowedFHIRStoreID}
	sourceConfig["allowedDicomStores"] = []string{allowedDICOMStoreID}

	config := map[string]any{
		"sources": map[string]any{
			"my-instance": sourceConfig,
		},
		"tools": map[string]any{
			"list-fhir-stores-restricted": map[string]any{
				"type":        "cloud-healthcare-list-fhir-stores",
				"source":      "my-instance",
				"description": "Tool to list fhir stores",
			},
			"list-dicom-stores-restricted": map[string]any{
				"type":        "cloud-healthcare-list-dicom-stores",
				"source":      "my-instance",
				"description": "Tool to list dicom stores",
			},
		},
	}

	startHealthcareMCPServer(t, ctx, config)

	runListFHIRStoresWithRestriction(t, allowedFHIRStoreID, disallowedFHIRStoreID, withMCPExec())
	runListDICOMStoresWithRestriction(t, allowedDICOMStoreID, disallowedDICOMStoreID, withMCPExec())
}

type chcExecConfig struct {
	isMCP bool
}

type chcExecOption func(*chcExecConfig)

func withMCPExec() chcExecOption {
	return func(c *chcExecConfig) {
		c.isMCP = true
	}
}

func getMCPResultJSON(t *testing.T, resp *tests.MCPCallToolResponse) string {
	t.Helper()
	if resp == nil || len(resp.Result.Content) == 0 {
		return "[]"
	}
	rows := make([]any, 0, len(resp.Result.Content))
	for _, block := range resp.Result.Content {
		if block.Type != "text" {
			continue
		}
		var obj any
		if err := json.Unmarshal([]byte(block.Text), &obj); err == nil {
			if slice, ok := obj.([]any); ok {
				rows = append(rows, slice...)
			} else {
				rows = append(rows, obj)
			}
		} else {
			rows = append(rows, block.Text)
		}
	}
	if len(rows) == 0 {
		return "[]"
	}
	b, err := json.Marshal(rows)
	if err != nil {
		t.Fatalf("error marshalling mcp result: %v", err)
	}
	return string(b)
}
