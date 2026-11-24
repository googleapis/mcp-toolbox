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
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
	"github.com/redis/go-redis/v9"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/log"
	"github.com/testcontainers/testcontainers-go/wait"
)

var (
	RedisSourceType = "redis"
	RedisToolType   = "redis"
	RedisAddress    = os.Getenv("REDIS_ADDRESS")
	RedisPass       = os.Getenv("REDIS_PASS")
)

const redisImage = "redis:7.2.17-alpine"

func setupRedisContainer(ctx context.Context, t *testing.T) (address, pass string) {
	t.Helper()

	pass = strings.ReplaceAll(uuid.New().String(), "-", "")
	req := testcontainers.ContainerRequest{
		Image:        redisImage,
		Cmd:          []string{"redis-server", "--requirepass", pass},
		ExposedPorts: []string{"6379/tcp"},
		WaitingFor: wait.ForAll(
			wait.ForListeningPort("6379/tcp"),
			wait.ForLog("Ready to accept connections"),
		).WithStartupTimeoutDefault(time.Minute).WithDeadline(time.Minute),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
		Logger:           log.TestLogger(t),
	})
	testcontainers.CleanupContainer(t, container)
	if err != nil {
		t.Fatalf("failed to start Redis container: %s", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("failed to get Redis container host: %s", err)
	}
	mappedPort, err := container.MappedPort(ctx, "6379/tcp")
	if err != nil {
		t.Fatalf("failed to get Redis container mapped port: %s", err)
	}
	return fmt.Sprintf("%s:%s", host, mappedPort.Port()), pass
}

func useRedis(ctx context.Context, t *testing.T) {
	t.Helper()
	if RedisAddress != "" {
		return
	}
	t.Logf("REDIS_ADDRESS not set; starting ephemeral %s container", redisImage)
	address, pass := setupRedisContainer(ctx, t)
	origAddress, origPass := RedisAddress, RedisPass
	t.Cleanup(func() { RedisAddress, RedisPass = origAddress, origPass })
	RedisAddress, RedisPass = address, pass
}

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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	useRedis(ctx, t)
	sourceConfig := getRedisVars(t)

	args := []string{"--enable-api"}

	client, err := initRedisClient(ctx, RedisAddress, RedisPass)
	if err != nil {
		t.Fatalf("unable to create Redis connection: %s", err)
	}
	t.Cleanup(func() { client.Close() })

	// set up data for param tool
	setupRedisDB(t, ctx, client)

	// Write config into a file and pass it to command
	toolsFile := tests.GetRedisValkeyToolsConfig(sourceConfig, RedisToolType)

	cmd, cleanup, err := tests.StartCmd(ctx, toolsFile, args...)
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	defer func() {
		cmd.Close()
		cleanup()
	}()

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

func setupRedisDB(t *testing.T, ctx context.Context, client *redis.Client) {
	keys := []string{"row1", "row2", "row3", "row4", "null", tests.ServiceAccountEmail}
	t.Cleanup(func() {
		// tear down test
		_, err := client.Del(context.WithoutCancel(ctx), keys...).Result()
		if err != nil {
			t.Errorf("Teardown failed: %s", err)
		}
	})
	commands := [][]any{
		{"HSET", keys[0], "id", 1, "name", "Alice"},
		{"HSET", keys[1], "id", 2, "name", "Jane"},
		{"HSET", keys[2], "id", 3, "name", "Sid"},
		{"HSET", keys[3], "id", 4, "name", nil},
		{"SET", keys[4], "null"},
		{"HSET", keys[5], "name", "Alice"},
	}
	for _, c := range commands {
		resp := client.Do(ctx, c...)
		if err := resp.Err(); err != nil {
			t.Fatalf("unable to insert test data: %s", err)
		}
	}
}
