// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package redis

import (
	"context"
	"crypto/tls"
	"fmt"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel/trace"
)

const SourceType string = "redis"

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
	Name           string    `yaml:"name" validate:"required"`
	Type           string    `yaml:"type" validate:"required"`
	Address        []string  `yaml:"address" validate:"required"`
	Username       string    `yaml:"username"`
	Password       string    `yaml:"password"`
	Database       int       `yaml:"database"`
	UseGCPIAM      bool      `yaml:"useGCPIAM"`
	ClusterEnabled bool      `yaml:"clusterEnabled"`
	TLS            TLSConfig `yaml:"tls"`
}

type TLSConfig struct {
	Enabled            bool `yaml:"enabled"`
	InsecureSkipVerify bool `yaml:"insecureSkipVerify"`
}

func (r Config) SourceConfigType() string {
	return SourceType
}

// RedisClient is an interface for `redis.Client` and `redis.ClusterClient
type RedisClient interface {
	Do(context.Context, ...any) *redis.Cmd
}

var _ RedisClient = (*redis.Client)(nil)
var _ RedisClient = (*redis.ClusterClient)(nil)

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
		conn:   sources.NewConnectOnce[RedisClient](ctx, r.Name, SourceType, tracer),
	}
}

func initRedisClient(ctx context.Context, r Config) (RedisClient, error) {
	var authFn func(ctx context.Context) (username string, password string, err error)
	if r.UseGCPIAM {
		// Pass in an access token getter fn for IAM auth
		authFn = func(ctx context.Context) (username string, password string, err error) {
			token, err := sources.GetIAMAccessToken(ctx)
			if err != nil {
				return "", "", err
			}
			return "default", token, nil
		}
	}

	var tlsConfig *tls.Config
	if r.TLS.Enabled {
		tlsConfig = &tls.Config{
			InsecureSkipVerify: r.TLS.InsecureSkipVerify,
		}
	}

	var client RedisClient
	var err error
	if r.ClusterEnabled {
		// Create a new Redis Cluster client
		clusterClient := redis.NewClusterClient(&redis.ClusterOptions{
			Addrs: r.Address,
			// PoolSize applies per cluster node and not for the whole cluster.
			PoolSize:                   10,
			ConnMaxIdleTime:            60 * time.Second,
			MinIdleConns:               1,
			CredentialsProviderContext: authFn,
			Username:                   r.Username,
			Password:                   r.Password,
			TLSConfig:                  tlsConfig,
		})
		err = clusterClient.ForEachShard(ctx, func(ctx context.Context, shard *redis.Client) error {
			return shard.Ping(ctx).Err()
		})
		if err != nil {
			return nil, fmt.Errorf("unable to connect to redis cluster: %s", err)
		}
		client = clusterClient
		return client, nil
	}

	// Create a new Redis client
	standaloneClient := redis.NewClient(&redis.Options{
		Addr:                       r.Address[0],
		PoolSize:                   10,
		ConnMaxIdleTime:            60 * time.Second,
		MinIdleConns:               1,
		DB:                         r.Database,
		CredentialsProviderContext: authFn,
		Username:                   r.Username,
		Password:                   r.Password,
		TLSConfig:                  tlsConfig,
	})
	_, err = standaloneClient.Ping(ctx).Result()
	if err != nil {
		return nil, fmt.Errorf("unable to connect to redis: %s", err)
	}
	client = standaloneClient
	return client, nil
}

var _ sources.Source = &Source{}

type Source struct {
	Config
	conn *sources.ConnectOnce[RedisClient]
}

func (s *Source) client(ctx context.Context) (RedisClient, error) {
	return s.conn.Do(ctx, func(ctx context.Context) (RedisClient, error) {
		client, err := initRedisClient(ctx, s.Config)
		if err != nil {
			return nil, fmt.Errorf("error initializing Redis client: %s", err)
		}
		return client, nil
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

// RedisClient reports the client once connected.
func (s *Source) RedisClient() RedisClient {
	client, _ := s.conn.Get()
	return client
}

func (s *Source) RunCommand(ctx context.Context, cmds [][]any) (any, error) {
	client, err := s.client(ctx)
	if err != nil {
		return nil, err
	}
	// Execute commands
	responses := make([]*redis.Cmd, len(cmds))
	for i, cmd := range cmds {
		responses[i] = client.Do(ctx, cmd...)
	}
	// Parse responses
	out := make([]any, len(cmds))
	for i, resp := range responses {
		if err := resp.Err(); err != nil {
			// Add error from each command to `errSum`
			errString := fmt.Sprintf("error from executing command at index %d: %s", i, err)
			out[i] = errString
			continue
		}
		val, err := resp.Result()
		if err != nil {
			return nil, fmt.Errorf("error getting result: %s", err)
		}
		out[i] = convertRedisResult(val)
	}

	return out, nil
}

// convertRedisResult recursively converts redis results (map[any]any) to be
// JSON-marshallable (map[string]any).
// It converts map[any]any to map[string]any and handles nested structures.
func convertRedisResult(v any) any {
	switch val := v.(type) {
	case map[any]any:
		m := make(map[string]any)
		for k, v := range val {
			m[fmt.Sprint(k)] = convertRedisResult(v)
		}
		return m
	case []any:
		s := make([]any, len(val))
		for i, v := range val {
			s[i] = convertRedisResult(v)
		}
		return s
	default:
		return v
	}
}
