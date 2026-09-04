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
package valkey

import (
	"context"
	"fmt"
	"log"

	"github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/valkey-io/valkey-go"
	"go.opentelemetry.io/otel/trace"
)

const SourceType string = "valkey"

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
	Name         string   `yaml:"name" validate:"required"`
	Type         string   `yaml:"type" validate:"required"`
	Address      []string `yaml:"address" validate:"required"`
	Username     string   `yaml:"username"`
	Password     string   `yaml:"password"`
	Database     int      `yaml:"database"`
	UseGCPIAM    bool     `yaml:"useGCPIAM"`
	DisableCache bool     `yaml:"disableCache"`
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
		conn:   sources.NewConnectOnce[valkey.Client](ctx, r.Name, SourceType, tracer),
	}
}

func initValkeyClient(ctx context.Context, r Config) (valkey.Client, error) {
	var authFn func(valkey.AuthCredentialsContext) (valkey.AuthCredentials, error)
	if r.UseGCPIAM {
		// Pass in an access token getter fn for IAM auth
		authFn = func(valkey.AuthCredentialsContext) (valkey.AuthCredentials, error) {
			token, err := sources.GetIAMAccessToken(ctx)
			creds := valkey.AuthCredentials{Username: "default", Password: token}
			if err != nil {
				return creds, err
			}
			return creds, nil
		}
	}

	client, err := valkey.NewClient(valkey.ClientOption{
		InitAddress:       r.Address,
		SelectDB:          r.Database,
		Username:          r.Username,
		Password:          r.Password,
		AuthCredentialsFn: authFn,
		DisableCache:      r.DisableCache,
	})

	if err != nil {
		log.Fatalf("error creating Valkey client: %v", err)
	}

	// Ping the server to check connectivity
	pingCmd := client.B().Ping().Build()
	_, err = client.Do(ctx, pingCmd).ToString()
	if err != nil {
		log.Fatalf("Failed to execute PING command: %v", err)
	}
	return client, nil
}

var _ sources.Source = &Source{}

type Source struct {
	Config
	conn *sources.ConnectOnce[valkey.Client]
}

func (s *Source) client(ctx context.Context) (valkey.Client, error) {
	return s.conn.Do(ctx, func(ctx context.Context) (valkey.Client, error) {
		client, err := initValkeyClient(ctx, s.Config)
		if err != nil {
			return nil, fmt.Errorf("error initializing Valkey client: %s", err)
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

// ValkeyClient reports the client once connected.
func (s *Source) ValkeyClient() valkey.Client {
	client, _ := s.conn.Get()
	return client
}

func (s *Source) RunCommand(ctx context.Context, cmds [][]string) (any, error) {
	client, err := s.client(ctx)
	if err != nil {
		return nil, err
	}

	// Build commands
	builtCmds := make(valkey.Commands, len(cmds))

	for i, cmd := range cmds {
		builtCmds[i] = client.B().Arbitrary(cmd...).Build()
	}

	if len(builtCmds) == 0 {
		return nil, fmt.Errorf("no valid commands were built to execute")
	}

	// Execute commands
	responses := client.DoMulti(ctx, builtCmds...)

	// Parse responses
	out := make([]any, len(cmds))
	for i, resp := range responses {
		if err := resp.Error(); err != nil {
			// Store error message in the output for this command
			out[i] = fmt.Sprintf("error from executing command at index %d: %s", i, err)
			continue
		}
		val, err := resp.ToAny()
		if err != nil {
			out[i] = fmt.Sprintf("error parsing response: %s", err)
			continue
		}
		out[i] = val
	}

	return out, nil
}
