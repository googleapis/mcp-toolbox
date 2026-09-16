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

package valkey

import (
	"context"
	"log"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
	"github.com/valkey-io/valkey-go"
)

var (
	ValkeySourceType = "valkey"
	ValkeyToolType   = "valkey"
	ValkeyAddress    = os.Getenv("VALKEY_ADDRESS")
)

func getValkeyVars(t *testing.T) map[string]any {
	switch "" {
	case ValkeyAddress:
		t.Fatal("'VALKEY_ADDRESS' not set")
	}
	return map[string]any{
		"type":         ValkeySourceType,
		"address":      []string{ValkeyAddress},
		"disableCache": true,
	}
}

func initValkeyClient(ctx context.Context, addr []string) (valkey.Client, error) {
	// Pass in an access token getter fn for IAM auth
	client, err := valkey.NewClient(valkey.ClientOption{
		InitAddress:       addr,
		ForceSingleClient: true,
		DisableCache:      true,
	})

	if err != nil {
		log.Fatalf("error creating client: %v", err)
	}

	// Ping the server to check connectivity (using Do)
	pingCmd := client.B().Ping().Build()
	_, err = client.Do(ctx, pingCmd).ToString()
	if err != nil {
		log.Fatalf("Failed to execute PING command: %v", err)
	}
	log.Println("Successfully connected to Valkey")
	return client, nil
}

func TestValkeyToolEndpoints(t *testing.T) {
	sourceConfig := getValkeyVars(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	args := []string{"--enable-api"}

	client, err := initValkeyClient(ctx, []string{ValkeyAddress})
	if err != nil {
		t.Fatalf("unable to create Valkey connection: %s", err)
	}

	// set up data for param tool
	teardownDB := setupValkeyDB(t, ctx, client)
	defer teardownDB(t)

	// Write config into a file and pass it to command
	toolsFile := tests.GetRedisValkeyToolsConfig(sourceConfig, ValkeyToolType)

	cmd, cleanup, err := tests.StartCmd(ctx, toolsFile, args...)
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	defer cleanup()

	waitCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}

	// Get configs for tests
	select1Want, mcpMyFailToolWant, invokeParamWant, invokeIdNullWant, nullWant, mcpSelect1Want, mcpInvokeParamWant := tests.GetRedisValkeyWants()

	// Run tests
	tests.RunToolGetTest(t)
	tests.RunToolInvokeTest(t, select1Want,
		tests.WithMyToolId3NameAliceWant(invokeParamWant),
		tests.WithMyArrayToolWant(invokeParamWant),
		tests.WithMyToolById4Want(invokeIdNullWant),
		tests.WithNullWant(nullWant),
	)
	tests.RunMCPToolCallMethod(t, mcpMyFailToolWant, mcpSelect1Want,
		tests.WithMcpMyToolId3NameAliceWant(mcpInvokeParamWant),
		tests.WithMcpMySecureToolWant(invokeParamWant),
	)
}

func setupValkeyDB(t *testing.T, ctx context.Context, client valkey.Client) func(*testing.T) {
	keys := []string{"row1", "row2", "row3", "row4", "null"}
	commands := [][]string{
		{"HSET", keys[0], "name", "Alice", "id", "1"},
		{"HSET", keys[1], "name", "Jane", "id", "2"},
		{"HSET", keys[2], "name", "Sid", "id", "3"},
		{"HSET", keys[3], "name", "", "id", "4"},
		{"SET", keys[4], "null"},
		{"HSET", tests.ServiceAccountEmail, "name", "Alice"},
	}
	builtCmds := make(valkey.Commands, len(commands))

	for i, cmd := range commands {
		builtCmds[i] = client.B().Arbitrary(cmd...).Build()
	}

	responses := client.DoMulti(ctx, builtCmds...)
	for _, resp := range responses {
		if err := resp.Error(); err != nil {
			t.Fatalf("unable to insert test data: %s", err)
		}
	}

	return func(t *testing.T) {
		// tear down test
		_, err := client.Do(ctx, client.B().Del().Key(keys...).Build()).AsInt64()
		if err != nil {
			t.Errorf("Teardown failed: %s", err)
		}
	}

}
