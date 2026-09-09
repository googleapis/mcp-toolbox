// Copyright 2024 Google LLC
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

package server

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/go-chi/httplog/v3"
	"github.com/go-chi/render"
	"github.com/googleapis/mcp-toolbox/internal/auth"
	"github.com/googleapis/mcp-toolbox/internal/embeddingmodels"
	"github.com/googleapis/mcp-toolbox/internal/group"
	"github.com/googleapis/mcp-toolbox/internal/log"
	"github.com/googleapis/mcp-toolbox/internal/prompts"
	"github.com/googleapis/mcp-toolbox/internal/server/mcp"
	"github.com/googleapis/mcp-toolbox/internal/server/mcp/jsonrpc"
	"github.com/googleapis/mcp-toolbox/internal/server/primitives"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/telemetry"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// Server contains info for running an instance of Toolbox. Should be instantiated with NewServer().
type Server struct {
	version             string
	sqlCommenterEnabled bool
	responseEncoding    string
	toolboxUrl          string
	prmURL              string
	srv                 *http.Server
	listener            net.Listener
	root                chi.Router
	logger              log.Logger
	instrumentation     *telemetry.Instrumentation
	sseManager          *sseManager
	PrimitiveMgr        *primitives.PrimitiveManager
	mcpPrmFile          string
	httpMaxRequestBytes int64
	enableDraftSpecs    bool
}

func InitializeConfigs(ctx context.Context, cfg ServerConfig) (
	map[string]sources.Source,
	map[string]auth.AuthService,
	map[string]embeddingmodels.EmbeddingModel,
	map[string]tools.Tool,
	map[string]prompts.Prompt,
	map[string]group.Group,
	error,
) {
	if cfg.EnableAPI {
		for _, sc := range cfg.AuthServiceConfigs {
			if sc.IsMCPEnabled() {
				return nil, nil, nil, nil, nil, nil, fmt.Errorf("MCP Auth cannot be enabled together with the legacy HTTP API (EnableAPI)")
			}
		}
	}

	metadataStr := cfg.Version
	if len(cfg.UserAgentMetadata) > 0 {
		metadataStr += "+" + strings.Join(cfg.UserAgentMetadata, "+")
	}
	ctx = util.WithUserAgent(ctx, metadataStr)
	instrumentation, err := util.InstrumentationFromContext(ctx)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, fmt.Errorf("failed to get instrumentation from context: %w", err)
	}

	l, err := util.LoggerFromContext(ctx)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, fmt.Errorf("failed to get logger from context: %w", err)
	}

	// initialize and validate the sources from configs
	sourcesMap := make(map[string]sources.Source)
	for name, sc := range cfg.SourceConfigs {
		s, err := func() (sources.Source, error) {
			childCtx, span := instrumentation.Tracer.Start(
				ctx,
				"toolbox/server/source/init",
				trace.WithAttributes(attribute.String("source_type", sc.SourceConfigType())),
				trace.WithAttributes(attribute.String("source_name", name)),
			)
			defer span.End()
			s, err := sc.Initialize(childCtx, instrumentation.Tracer)
			if err != nil {
				return nil, fmt.Errorf("unable to initialize source %q: %w", name, err)
			}
			return s, nil
		}()
		if err != nil {
			return nil, nil, nil, nil, nil, nil, err
		}
		sourcesMap[name] = s
	}
	sourceNames := make([]string, 0, len(sourcesMap))
	for name := range sourcesMap {
		sourceNames = append(sourceNames, name)
	}
	l.InfoContext(ctx, fmt.Sprintf("Initialized %d sources: %s", len(sourcesMap), strings.Join(sourceNames, ", ")))

	// initialize and validate the auth services from configs
	authServicesMap := make(map[string]auth.AuthService)
	for name, sc := range cfg.AuthServiceConfigs {
		a, err := func() (auth.AuthService, error) {
			_, span := instrumentation.Tracer.Start(
				ctx,
				"toolbox/server/auth/init",
				trace.WithAttributes(attribute.String("auth_type", sc.AuthServiceConfigType())),
				trace.WithAttributes(attribute.String("auth_name", name)),
			)
			defer span.End()
			a, err := sc.Initialize()
			if err != nil {
				return nil, fmt.Errorf("unable to initialize auth service %q: %w", name, err)
			}
			return a, nil
		}()
		if err != nil {
			return nil, nil, nil, nil, nil, nil, err
		}
		authServicesMap[name] = a
	}

	authServiceNames := make([]string, 0, len(authServicesMap))
	for name := range authServicesMap {
		authServiceNames = append(authServiceNames, name)
	}
	l.InfoContext(ctx, fmt.Sprintf("Initialized %d authServices: %s", len(authServicesMap), strings.Join(authServiceNames, ", ")))

	// Initialize and validate embedding models from configs.
	embeddingModelsMap := make(map[string]embeddingmodels.EmbeddingModel)
	for name, ec := range cfg.EmbeddingModelConfigs {
		em, err := func() (embeddingmodels.EmbeddingModel, error) {
			_, span := instrumentation.Tracer.Start(
				ctx,
				"toolbox/server/embeddingmodel/init",
				trace.WithAttributes(attribute.String("model_type", ec.EmbeddingModelConfigType())),
				trace.WithAttributes(attribute.String("model_name", name)),
			)
			defer span.End()
			em, err := ec.Initialize(ctx)
			if err != nil {
				return nil, fmt.Errorf("unable to initialize embedding model %q: %w", name, err)
			}
			return em, nil
		}()
		if err != nil {
			return nil, nil, nil, nil, nil, nil, err
		}
		embeddingModelsMap[name] = em
	}
	embeddingModelNames := make([]string, 0, len(embeddingModelsMap))
	for name := range embeddingModelsMap {
		embeddingModelNames = append(embeddingModelNames, name)
	}
	l.InfoContext(ctx, fmt.Sprintf("Initialized %d embeddingModels: %s", len(embeddingModelsMap), strings.Join(embeddingModelNames, ", ")))

	toolsMap, err := initializeTools(ctx, cfg, sourcesMap, instrumentation, l)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, err
	}

	// initialize and validate the prompts from configs
	promptsMap := make(map[string]prompts.Prompt)
	for name, pc := range cfg.PromptConfigs {
		p, err := func() (prompts.Prompt, error) {
			_, span := instrumentation.Tracer.Start(
				ctx,
				"toolbox/server/prompt/init",
				trace.WithAttributes(attribute.String("prompt_type", pc.PromptConfigType())),
				trace.WithAttributes(attribute.String("prompt_name", name)),
			)
			defer span.End()
			p, err := pc.Initialize()
			if err != nil {
				return nil, fmt.Errorf("unable to initialize prompt %q: %w", name, err)
			}
			return p, nil
		}()
		if err != nil {
			return nil, nil, nil, nil, nil, nil, err
		}
		promptsMap[name] = p
	}
	promptNames := make([]string, 0, len(promptsMap))
	for name := range promptsMap {
		promptNames = append(promptNames, name)
	}
	l.InfoContext(ctx, fmt.Sprintf("Initialized %d prompts: %s", len(promptsMap), strings.Join(promptNames, ", ")))

	groupsMap, err := initializeGroups(ctx, cfg, toolsMap, promptsMap, instrumentation, l)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, err
	}

	return sourcesMap, authServicesMap, embeddingModelsMap, toolsMap, promptsMap, groupsMap, nil
}

// InitializeOfflineConfigs initializes only tools, prompts, and groups from the
// config, skipping sources, auth services, and embedding models. It backs flows
// like skills-generate that need tool metadata without live source connections.
func InitializeOfflineConfigs(ctx context.Context, cfg ServerConfig) (
	map[string]tools.Tool,
	map[string]group.Group,
	error,
) {
	instrumentation, err := util.InstrumentationFromContext(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get instrumentation from context: %w", err)
	}

	l, err := util.LoggerFromContext(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get logger from context: %w", err)
	}
	// Automatically skip source validation
	cfg.SkipSourceValidation = true
	toolsMap, err := initializeTools(ctx, cfg, map[string]sources.Source{}, instrumentation, l)
	if err != nil {
		return nil, nil, err
	}

	// Prompts are initialized so group prompt validation succeeds offline.
	promptsMap := make(map[string]prompts.Prompt)
	for name, pc := range cfg.PromptConfigs {
		p, err := pc.Initialize()
		if err != nil {
			return nil, nil, fmt.Errorf("unable to initialize prompt %q: %w", name, err)
		}
		promptsMap[name] = p
	}

	groupsMap, err := initializeGroups(ctx, cfg, toolsMap, promptsMap, instrumentation, l)
	if err != nil {
		return nil, nil, err
	}

	return toolsMap, groupsMap, nil
}

// initializeTools initializes and validates the tools from the config.
func initializeTools(ctx context.Context, cfg ServerConfig, sourcesMap map[string]sources.Source, instrumentation *telemetry.Instrumentation, l log.Logger) (map[string]tools.Tool, error) {
	toolsMap := make(map[string]tools.Tool)
	for name, tc := range cfg.ToolConfigs {
		var src sources.Source
		t, err := func() (tools.Tool, error) {
			_, span := instrumentation.Tracer.Start(
				ctx,
				"toolbox/server/tool/init",
				trace.WithAttributes(attribute.String("tool_type", tc.ToolConfigType())),
				trace.WithAttributes(attribute.String("tool_name", name)),
			)
			defer span.End()
			t, err := tc.Initialize(ctx)
			if err != nil {
				return nil, fmt.Errorf("unable to initialize tool %q: %w", name, err)
			}

			if srcName := t.GetSourceName(); srcName != "" && sourcesMap != nil {
				var ok bool
				src, ok = sourcesMap[srcName]
				if !ok && !cfg.SkipSourceValidation {
					return nil, fmt.Errorf("unable to retrieve source %q for tool %q", srcName, name)
				}
			}

			if !cfg.SkipSourceValidation {
				err = t.ValidateSource(src)
				if err != nil {
					return nil, err
				}
			}
			return t, nil
		}()
		if err != nil {
			return nil, err
		}

		if tools.ShouldSuppress(ctx, t, src) {
			continue
		}

		toolsMap[name] = t
	}
	toolNames := make([]string, 0, len(toolsMap))
	for name := range toolsMap {
		toolNames = append(toolNames, name)
	}
	l.InfoContext(ctx, fmt.Sprintf("Initialized %d tools: %s", len(toolsMap), strings.Join(toolNames, ", ")))
	return toolsMap, nil
}

// initializeGroups seeds a default nameless group containing all tools and all
// prompts, converts each legacy kind: toolsets config into a tools-only group,
// then initializes and validates every group. The default group's derived
// toolset/promptset views preserve the legacy behavior of returning everything
// for clients that connect without naming a collection.
func initializeGroups(ctx context.Context, cfg ServerConfig, toolsMap map[string]tools.Tool, promptsMap map[string]prompts.Prompt, instrumentation *telemetry.Instrumentation, l log.Logger) (map[string]group.Group, error) {
	allToolNames := make([]string, 0, len(toolsMap))
	for name := range toolsMap {
		allToolNames = append(allToolNames, name)
	}
	slices.Sort(allToolNames)
	allPromptNames := make([]string, 0, len(promptsMap))
	for name := range promptsMap {
		allPromptNames = append(allPromptNames, name)
	}
	slices.Sort(allPromptNames)

	// Legacy `kind: toolset` configs are already folded into cfg.GroupConfigs at
	// unmarshal. Copy them over, then seed the default nameless group with all tools
	// and all prompts.
	groupConfigs := make(map[string]group.GroupConfig, len(cfg.GroupConfigs)+1)
	var defaultDescription string
	for name, gc := range cfg.GroupConfigs {
		if name == "" {
			// The default group's tools and prompts are fixed; only its
			// description carries over as the default server instruction.
			defaultDescription = gc.Description
			continue
		}
		groupConfigs[name] = gc
	}
	groupConfigs[""] = group.GroupConfig{Name: "", Description: defaultDescription, ToolNames: allToolNames, PromptNames: allPromptNames}

	groupsMap := make(map[string]group.Group)
	for name, gc := range groupConfigs {
		filteredToolNames := make([]string, 0, len(gc.ToolNames))
		for _, tn := range gc.ToolNames {
			if _, ok := toolsMap[tn]; ok {
				filteredToolNames = append(filteredToolNames, tn)
			} else if _, isTool := cfg.ToolConfigs[tn]; isTool {
				l.InfoContext(ctx, fmt.Sprintf("Removing suppressed tool %q from group %q", tn, name))
			} else if cfg.IgnoreUnknownTools {
				l.WarnContext(ctx, fmt.Sprintf("Skipping missing tool %q in group %q", tn, name))
			} else {
				// Keep it so that Initialize returns the expected error
				filteredToolNames = append(filteredToolNames, tn)
			}
		}
		gc.ToolNames = filteredToolNames

		g, err := func() (group.Group, error) {
			_, span := instrumentation.Tracer.Start(
				ctx,
				"toolbox/server/group/init",
				trace.WithAttributes(attribute.String("group.name", name)),
			)
			defer span.End()
			g, err := gc.Initialize(toolsMap, promptsMap)
			if err != nil {
				return group.Group{}, fmt.Errorf("unable to initialize group %q: %w", name, err)
			}
			return g, nil
		}()
		if err != nil {
			return nil, err
		}
		groupsMap[name] = g
	}
	groupNames := make([]string, 0, len(groupsMap))
	for name := range groupsMap {
		if name == "" {
			groupNames = append(groupNames, "default")
		} else {
			groupNames = append(groupNames, name)
		}
	}
	l.InfoContext(ctx, fmt.Sprintf("Initialized %d groups: %s", len(groupsMap), strings.Join(groupNames, ", ")))

	return groupsMap, nil
}

func hostCheck(allowedHosts map[string]struct{}) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip host validation for health check probes. Container
			// orchestrators (Kubernetes, Docker, Cloud Run) typically hit
			// /healthz via the pod IP or localhost, which would otherwise
			// trip a strict AllowedHosts setting and break liveness probes.
			if r.URL.Path == "/healthz" {
				next.ServeHTTP(w, r)
				return
			}
			_, hasWildcard := allowedHosts["*"]
			hostname := r.Host
			if host, _, err := net.SplitHostPort(r.Host); err == nil {
				hostname = host
			}
			_, hostIsAllowed := allowedHosts[hostname]
			if !hasWildcard && !hostIsAllowed {
				// Return 403 Forbidden to block the attack
				http.Error(w, "Invalid Host header", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// NewServer returns a Server object based on provided Config.
func NewServer(ctx context.Context, cfg ServerConfig) (*Server, error) {
	instrumentation, err := util.InstrumentationFromContext(ctx)
	if err != nil {
		return nil, err
	}

	ctx, span := instrumentation.Tracer.Start(ctx, "toolbox/server/init")
	defer span.End()

	l, err := util.LoggerFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// set up http serving
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)

	// logging
	logLevel, err := log.SeverityToLevel(cfg.LogLevel.String())
	if err != nil {
		return nil, fmt.Errorf("unable to initialize http log: %w", err)
	}

	schema := *httplog.SchemaGCP
	schema.Level = cfg.LogLevel.String()
	schema.Concise(true)
	httpOpts := &httplog.Options{
		Level:  logLevel,
		Schema: &schema,
	}
	logger := l.SlogLogger()
	r.Use(httplog.RequestLogger(logger, httpOpts))

	sourcesMap, authServicesMap, embeddingModelsMap, toolsMap, promptsMap, groupsMap, err := InitializeConfigs(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("unable to initialize configs: %w", err)
	}

	addr := net.JoinHostPort(cfg.Address, strconv.Itoa(cfg.Port))
	srv := &http.Server{Addr: addr, Handler: r}

	sseManager := newSseManager(ctx)

	primitiveManager := primitives.NewPrimitiveManager(sourcesMap, authServicesMap, embeddingModelsMap, toolsMap, promptsMap, groupsMap)

	limit := cfg.HttpMaxRequestBytes
	if limit <= 0 {
		limit = DefaultHTTPMaxRequestBytes
	}

	mcp.InitializeProtocols(mcp.ProtocolOptions{
		DisableExt: cfg.DisableExt,
	})

	prmURLStr, err := parsePRMURL(cfg.ToolboxUrl)
	if err != nil {
		return nil, fmt.Errorf("unable to initialize server: %w", err)
	}
	prmURL, err := url.Parse(prmURLStr)
	if err != nil {
		return nil, fmt.Errorf("unable to initialize server: %w", err)
	}

	s := &Server{
		version:             cfg.Version,
		sqlCommenterEnabled: cfg.SQLCommenter,
		responseEncoding:    cfg.ResponseEncoding.String(),
		srv:                 srv,
		root:                r,
		logger:              l,
		instrumentation:     instrumentation,
		sseManager:          sseManager,
		PrimitiveMgr:        primitiveManager,
		toolboxUrl:          cfg.ToolboxUrl,
		prmURL:              prmURLStr,
		mcpPrmFile:          cfg.McpPrmFile,
		httpMaxRequestBytes: limit,
		enableDraftSpecs:    cfg.EnableDraftSpecs,
	}

	if s.enableDraftSpecs {
		s.logger.WarnContext(ctx, "Flag --enable-draft-specs is active. Please note that draft specs are subject to breaking changes and will be completely removed (not redirected) once stable MCP specifications are released. Do not use this configuration in production.")
	}

	// cors
	if slices.Contains(cfg.AllowedOrigins, "*") {
		s.logger.WarnContext(ctx, "wildcard (*) allows any website to access the primitives. This creates a security risk regardless of whether you are in a production or local development environment. Recommended to use --allowed-origins with specific local addresses.")
	}
	corsOpts := cors.Options{
		AllowedOrigins:   cfg.AllowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "DELETE", "OPTIONS"},
		AllowCredentials: true, // required since Toolbox uses auth headers
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token", "Mcp-Session-Id", "MCP-Protocol-Version"},
		ExposedHeaders:   []string{"Mcp-Session-Id"}, // headers that are sent to clients
		MaxAge:           300,                        // cache preflight results for 5 minutes
	}
	r.Use(cors.Handler(corsOpts))
	// validate hosts for DNS rebinding attacks
	if slices.Contains(cfg.AllowedHosts, "*") {
		s.logger.WarnContext(ctx, "wildcard (*) hosts allow any domain to access this resource, making it vulnerable to DNS rebinding attacks regardless of whether you are in a production or local development environment. For improved security, use the --allowed-hosts flag to specify trusted domains.")
	}
	allowedHostsMap := make(map[string]struct{}, len(cfg.AllowedHosts))
	for _, h := range cfg.AllowedHosts {
		hostname := h
		if host, _, err := net.SplitHostPort(h); err == nil {
			hostname = host
		}
		allowedHostsMap[hostname] = struct{}{}
	}
	r.Use(hostCheck(allowedHostsMap))

	// Host OAuth Protected Resource Metadata endpoint
	mcpAuthEnabled := false
	for _, authSvc := range s.PrimitiveMgr.AuthServices() {
		if mSvc, ok := authSvc.(auth.MCPAuthService); ok && mSvc.IsMCPEnabled() {
			mcpAuthEnabled = true
			break
		}
	}

	// Manual PRM override
	var cachedPrmBytes []byte
	var prmConfig ProtectedResourceMetadata
	if s.mcpPrmFile != "" {
		var err error
		cachedPrmBytes, err = os.ReadFile(s.mcpPrmFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read manual PRM file at startup: %w", err)
		}
		// Unmarshal into the struct to strictly validate the schema
		if err := json.Unmarshal(cachedPrmBytes, &prmConfig); err != nil {
			return nil, fmt.Errorf("manual PRM file does not match expected schema: %w", err)
		}
	}

	// Register route if auth is enabled or a manual file is provided
	if mcpAuthEnabled || s.mcpPrmFile != "" {
		r.Get(prmURL.Path, func(w http.ResponseWriter, req *http.Request) {
			// Serve from memory if file was loaded
			if s.mcpPrmFile != "" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				if _, err := w.Write(cachedPrmBytes); err != nil {
					s.logger.ErrorContext(req.Context(), "failed to write manual PRM file response", "error", err)
				}
				return
			}

			prmHandler(s, w, req)
		})
	}

	// control plane
	mcpR, err := mcpRouter(s)
	if err != nil {
		return nil, err
	}

	r.Mount("/mcp", mcpR)
	if cfg.EnableAPI {
		apiR, err := apiRouter(s)
		if err != nil {
			return nil, err
		}
		r.Mount("/api", apiR)
	} else {
		r.Handle("/api/*", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			err := errors.New("/api native endpoints are disabled by default. Please use the standard /mcp JSON-RPC endpoint")
			_ = render.Render(w, r, newErrResponse(err, http.StatusGone))
		}))
	}
	if cfg.UI {
		webR, err := webRouter()
		if err != nil {
			return nil, err
		}
		r.Mount("/ui", webR)
	}
	// default endpoint for validating server is running
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("🧰 Hello, World! 🧰"))
	})

	// healthz endpoint for container orchestration health checks
	// (Kubernetes liveness/readiness probes, Docker HEALTHCHECK, etc.).
	// Returns 200 OK with a small JSON body so probes can rely on both
	// status code and payload.
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		render.JSON(w, r, map[string]string{"status": "ok"})
	})

	return s, nil
}

func mcpAuthMiddleware(s *Server) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Find McpEnabled auth service
			var mcpSvc auth.MCPAuthService
			for _, authSvc := range s.PrimitiveMgr.AuthServices() {
				if mSvc, ok := authSvc.(auth.MCPAuthService); ok && mSvc.IsMCPEnabled() {
					mcpSvc = mSvc
					break
				}
			}

			// MCP Auth not enabled
			if mcpSvc == nil {
				next.ServeHTTP(w, r)
				return
			}

			claims, err := mcpSvc.ValidateMCPAuth(r.Context(), r.Header)
			if err != nil {
				var mcpErr *auth.MCPAuthError
				if errors.As(err, &mcpErr) {
					switch mcpErr.Code {
					case http.StatusUnauthorized:
						scopesArg := ""
						if len(mcpErr.ScopesRequired) > 0 {
							scopesArg = fmt.Sprintf(`, scope="%s"`, strings.Join(mcpErr.ScopesRequired, " "))
						}
						w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer resource_metadata="%s"%s`, s.getPRMURL(), scopesArg))
						render.Status(r, http.StatusUnauthorized)
						render.JSON(w, r, jsonrpc.NewError(nil, jsonrpc.UNAUTHORIZED, mcpErr.Message, nil))
						return
					case http.StatusForbidden:
						w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer error="insufficient_scope", scope="%s", resource_metadata="%s", error_description="%s"`, strings.Join(mcpErr.ScopesRequired, " "), s.getPRMURL(), mcpErr.Message))
						render.Status(r, http.StatusForbidden)
						render.JSON(w, r, jsonrpc.NewError(nil, jsonrpc.FORBIDDEN, mcpErr.Message, nil))
						return
					}
				}
				// Fail closed on unexpected errors
				s.logger.ErrorContext(r.Context(), "unexpected error during MCP auth validation", "error", err)
				render.Status(r, http.StatusInternalServerError)
				render.JSON(w, r, jsonrpc.NewError(nil, jsonrpc.INTERNAL_ERROR, "Internal Server Error", nil))
				return
			}

			ctx := util.WithAuthTokenClaims(r.Context(), claims)
			r = r.WithContext(ctx)

			next.ServeHTTP(w, r)
		})
	}
}

// Listen starts a listener for the given Server instance.
func (s *Server) Listen(ctx context.Context, certFile, keyFile string) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	if s.listener != nil {
		return fmt.Errorf("server is already listening: %s", s.listener.Addr().String())
	}
	lc := net.ListenConfig{KeepAlive: 30 * time.Second}
	ln, err := lc.Listen(ctx, "tcp", s.srv.Addr)
	if err != nil {
		if errors.Is(err, syscall.EADDRINUSE) {
			return fmt.Errorf("failed to open listener for %q. Use `--port=<number>` to specify a different port: %w", s.srv.Addr, err)
		}
		return fmt.Errorf("failed to open listener for %q: %w", s.srv.Addr, err)
	}

	if certFile != "" || keyFile != "" {
		// Load the certificates
		cert, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			ln.Close()
			return fmt.Errorf("failed to load TLS key pair (cert: %q, key: %q): %w", certFile, keyFile, err)
		}
		// Wrap the listener with TLS
		config := &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}
		s.listener = tls.NewListener(ln, config)
		s.logger.DebugContext(ctx, fmt.Sprintf("secure server listening on %s", s.srv.Addr))
	} else {
		s.listener = ln
		s.logger.DebugContext(ctx, fmt.Sprintf("server listening on %s", s.srv.Addr))
	}
	return nil
}

// Serve starts an HTTP server for the given Server instance.
func (s *Server) Serve(ctx context.Context) error {
	s.logger.DebugContext(ctx, "Starting a HTTP server.")
	return s.srv.Serve(s.listener)
}

// ServeStdio starts a new stdio session for mcp.
func (s *Server) ServeStdio(ctx context.Context, stdin io.Reader, stdout io.Writer) error {
	stdioServer := NewStdioSession(s, stdin, stdout)
	return stdioServer.Start(ctx)
}

// Shutdown gracefully shuts down the server without interrupting any active
// connections. It uses http.Server.Shutdown() and has the same functionality.
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.DebugContext(ctx, "shutting down the server.")
	return s.srv.Shutdown(ctx)
}

func (s *Server) Addr() string {
	return s.listener.Addr().String()
}
