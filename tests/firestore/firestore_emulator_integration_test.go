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

package firestore

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
	"github.com/testcontainers/testcontainers-go"
	tclog "github.com/testcontainers/testcontainers-go/log"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	firestoreEmulatorImage   = "gcr.io/google.com/cloudsdktool/google-cloud-cli:592.0.0-emulators"
	firestoreEmulatorPort    = "8080"
	firestoreEmulatorProject = "toolbox-firestore-emulator"
)

// setupFirestoreEmulator starts the Cloud SDK Firestore emulator in memory,
// schedules container termination via t.Cleanup, and configures
// FIRESTORE_EMULATOR_HOST via t.Setenv.
func setupFirestoreEmulator(ctx context.Context, t *testing.T) string {
	t.Helper()

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        firestoreEmulatorImage,
			Cmd:          []string{"gcloud", "beta", "emulators", "firestore", "start", "--host-port=0.0.0.0:" + firestoreEmulatorPort},
			ExposedPorts: []string{firestoreEmulatorPort + "/tcp"},
			WaitingFor: wait.ForAll(
				wait.ForLog("Dev App Server is now running").WithStartupTimeout(3*time.Minute),
				wait.ForListeningPort(firestoreEmulatorPort+"/tcp").WithStartupTimeout(3*time.Minute),
			).WithDeadline(3 * time.Minute),
		},
		Started: true,
		Logger:  tclog.TestLogger(t),
	})
	testcontainers.CleanupContainer(t, container)
	if err != nil {
		t.Fatalf("failed to start Firestore emulator container: %s", err)
	}

	endpoint, err := container.PortEndpoint(ctx, firestoreEmulatorPort+"/tcp", "")
	if err != nil {
		t.Fatalf("failed to get Firestore emulator endpoint: %s", err)
	}
	t.Setenv(firestoreEmulatorHostEnv, endpoint)
	return firestoreEmulatorProject
}

// TestFirestoreEmulatorToolEndpoints runs document CRUD, query, and
// list-collections tools against the Firestore emulator without cloud
// credentials. Firebase Rules tools and Query Explain (analyzeQuery) require
// real Cloud Firestore and remain covered by TestFirestoreToolEndpoints.
func TestFirestoreEmulatorToolEndpoints(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	project := setupFirestoreEmulator(ctx, t)
	client := newFirestoreTestClient(t, project, "")

	testCollectionName := fmt.Sprintf("test_collection_emu_%s", strings.ReplaceAll(uuid.New().String(), "-", "")[:16])
	testSubCollectionName := fmt.Sprintf("test_subcollection_%s", strings.ReplaceAll(uuid.New().String(), "-", "")[:16])
	testDocID1 := fmt.Sprintf("doc_%s", strings.ReplaceAll(uuid.New().String(), "-", "")[:16])
	testDocID2 := fmt.Sprintf("doc_%s", strings.ReplaceAll(uuid.New().String(), "-", "")[:16])
	testDocID3 := fmt.Sprintf("doc_%s", strings.ReplaceAll(uuid.New().String(), "-", "")[:16])

	docPath1 := fmt.Sprintf("%s/%s", testCollectionName, testDocID1)
	docPath2 := fmt.Sprintf("%s/%s", testCollectionName, testDocID2)
	docPath3 := fmt.Sprintf("%s/%s", testCollectionName, testDocID3)

	// Ephemeral in-memory emulator state is discarded when CleanupContainer
	// terminates the container, so skip API-level teardown RPCs here.
	seedFirestoreTestCollection(t, ctx, client, testCollectionName, testSubCollectionName,
		testDocID1, testDocID2, testDocID3)

	sourceConfig := map[string]any{
		"type":    FirestoreSourceType,
		"project": project,
	}
	toolsFile := getFirestoreToolsConfig(sourceConfig)

	cmd, cleanup, err := tests.StartCmd(ctx, toolsFile, "--enable-api")
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	t.Cleanup(cleanup)

	waitCtx, waitCancel := context.WithTimeout(ctx, 30*time.Second)
	defer waitCancel()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}

	runFirestoreToolGetTest(t)
	runFirestoreMCPToolCallMethod(t, docPath1, docPath2)
	runFirestoreGetDocumentsTest(t, docPath1, docPath2)
	runFirestoreQueryCollectionTest(t, testCollectionName, false)
	runFirestoreQueryTest(t, testCollectionName)
	runFirestoreQuerySelectArrayTest(t, testCollectionName)
	runFirestoreListCollectionsTest(t, testCollectionName, testSubCollectionName, docPath1)
	runFirestoreAddDocumentsTest(t, testCollectionName)
	runFirestoreUpdateDocumentTest(t, testCollectionName, testDocID1)
	runFirestoreDeleteDocumentsTest(t, docPath3)
	runFirestoreRulesUnavailableOnEmulatorTest(t)
}

func runFirestoreRulesUnavailableOnEmulatorTest(t *testing.T) {
	t.Run("get rules on emulator returns clear unavailable error", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodPost, "http://127.0.0.1:5000/api/tool/firestore-get-rules/invoke", strings.NewReader(`{}`))
		if err != nil {
			t.Fatalf("unable to create request: %s", err)
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("unable to send request: %s", err)
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("unable to read response body: %s", err)
		}
		const want = "Firebase Rules API is not available"
		if !strings.Contains(string(body), want) {
			t.Fatalf("response (status %d) does not contain %q: %s", resp.StatusCode, want, body)
		}
	})
}
