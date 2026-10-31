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
	runGetDatasetToolInvokeTest(t, datasetWant, tests.WithMCPExec())
	runListFHIRStoresToolInvokeTest(t, fhirStoreWant, tests.WithMCPExec())
	runListDICOMStoresToolInvokeTest(t, tests.WithMCPExec())
	runGetFHIRStoreToolInvokeTest(t, fhirStoreID, fhirStoreWant, tests.WithMCPExec())
	runGetFHIRStoreMetricsToolInvokeTest(t, fhirStoreID, `"metrics"`, tests.WithMCPExec())
	runGetFHIRResourceToolInvokeTest(t, fhirStoreID, "Patient", patient1ID, `"id":"`+patient1ID+`"`, tests.WithMCPExec())
	runFHIRPatientSearchToolInvokeTest(t, fhirStoreID, []string{patient1ID, patient2ID}, tests.WithMCPExec())
	runFHIRPatientEverythingToolInvokeTest(t, fhirStoreID, patient1ID, `"resourceType":"Bundle"`, tests.WithMCPExec())

	nextURL := getNextPageURLForPatientEverything(t, fhirStoreID, patient2ID)
	runFHIRFetchPageToolInvokeTest(t, nextURL, `"total":1`, tests.WithMCPExec())

	runGetDICOMStoreToolInvokeTest(t, dicomStoreID, dicomStoreWant, tests.WithMCPExec())
	runGetDICOMStoreMetricsToolInvokeTest(t, dicomStoreID, dicomStoreWant, tests.WithMCPExec())
	runSearchDICOMStudiesToolInvokeTest(t, dicomStoreID, tests.WithMCPExec())
	runSearchDICOMSeriesToolInvokeTest(t, dicomStoreID, tests.WithMCPExec())
	runSearchDICOMInstancesToolInvokeTest(t, dicomStoreID, tests.WithMCPExec())
	runRetrieveRenderedDICOMInstanceToolInvokeTest(t, dicomStoreID, tests.WithMCPExec())
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

	runListFHIRStoresWithRestriction(t, allowedFHIRStoreID, disallowedFHIRStoreID, tests.WithMCPExec())
	runListDICOMStoresWithRestriction(t, allowedDICOMStoreID, disallowedDICOMStoreID, tests.WithMCPExec())
}
