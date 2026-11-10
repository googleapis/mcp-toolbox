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

package redis

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
	"github.com/redis/go-redis/v9"
)

var (
	RedisSourceType = "redis"
	RedisToolType   = "redis"
	RedisAddress    = os.Getenv("REDIS_ADDRESS")
	RedisPass       = os.Getenv("REDIS_PASS")
)

func getRedisVars(t *testing.T) map[string]any {
	switch "" {
	case RedisAddress:
		t.Fatal("'REDIS_ADDRESS' not set")
	case RedisPass:
		t.Fatal("'REDIS_PASS' not set")
	}
	return map[string]any{
		"type":     RedisSourceType,
		"address":  []string{RedisAddress},
		"password": RedisPass,
	}
}

func initRedisClient(ctx context.Context, address, pass string) (*redis.Client, error) {
	// Create a new Redis client
	standaloneClient := redis.NewClient(&redis.Options{
		Addr:            address,
		PoolSize:        10,
		ConnMaxIdleTime: 60 * time.Second,
		MinIdleConns:    1,
		Password:        pass,
	})
	_, err := standaloneClient.Ping(ctx).Result()
	if err != nil {
		return nil, fmt.Errorf("unable to connect to redis: %s", err)
	}
	return standaloneClient, nil
}

func TestRedisToolEndpoints(t *testing.T) {
	sourceConfig := getRedisVars(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	args := []string{"--enable-api"}

	client, err := initRedisClient(ctx, RedisAddress, RedisPass)
	if err != nil {
		t.Fatalf("unable to create Redis connection: %s", err)
	}

	// set up data for param tool
	teardownDB := setupRedisDB(t, ctx, client)
	defer teardownDB(t)

	// Write config into a file and pass it to command
	toolsFile := tests.GetRedisValkeyToolsConfig(sourceConfig, RedisToolType)

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

func setupRedisDB(t *testing.T, ctx context.Context, client *redis.Client) func(*testing.T) {
	keys := []string{"row1", "row2", "row3", "row4", "null"}
	commands := [][]any{
		{"HSET", keys[0], "id", 1, "name", "Alice"},
		{"HSET", keys[1], "id", 2, "name", "Jane"},
		{"HSET", keys[2], "id", 3, "name", "Sid"},
		{"HSET", keys[3], "id", 4, "name", nil},
		{"SET", keys[4], "null"},
		{"HSET", tests.ServiceAccountEmail, "name", "Alice"},
	}
	for _, c := range commands {
		resp := client.Do(ctx, c...)
		if err := resp.Err(); err != nil {
			t.Fatalf("unable to insert test data: %s", err)
		}
	}

	return func(t *testing.T) {
		// tear down test
		_, err := client.Del(ctx, keys...).Result()
		if err != nil {
			t.Errorf("Teardown failed: %s", err)
		}
	}

}
