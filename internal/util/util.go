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
package util

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"unicode"

	"github.com/go-playground/validator/v10"
	yaml "github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/log"
	"github.com/googleapis/mcp-toolbox/internal/telemetry"
)

// GDAClientID is the client ID for Gemini Data Analytics
const GDAClientID = "GENAI_TOOLBOX"

// DecodeJSON decodes a given reader into an interface using the json decoder.
func DecodeJSON(r io.Reader, v interface{}) error {
	defer io.Copy(io.Discard, r) //nolint:errcheck
	d := json.NewDecoder(r)
	// specify JSON numbers should get parsed to json.Number instead of float64 by default.
	// This prevents loss between floats/ints.
	d.UseNumber()
	return d.Decode(v)
}

// ConvertNumbers traverses an interface and converts all json.Number
// instances to int64 or float64.
func ConvertNumbers(data any) (any, error) {
	switch v := data.(type) {
	// If it's a map, recursively convert the values.
	case map[string]any:
		for key, val := range v {
			convertedVal, err := ConvertNumbers(val)
			if err != nil {
				return nil, err
			}
			v[key] = convertedVal
		}
		return v, nil

	// If it's a slice, recursively convert the elements.
	case []any:
		for i, val := range v {
			convertedVal, err := ConvertNumbers(val)
			if err != nil {
				return nil, err
			}
			v[i] = convertedVal
		}
		return v, nil

	// If it's a json.Number, convert it to int or float. Prefer an integer,
	// and fall back to float for decimals, exponent notation (e.g. 1e5,
	// 1e-07), and values outside the int64 range. Deciding on the presence
	// of a "." alone misroutes exponent-form numbers to Int64, which rejects
	// them as invalid syntax.
	case json.Number:
		if i, err := v.Int64(); err == nil {
			return i, nil
		}
		f, err := v.Float64()
		if err != nil {
			return nil, err
		}
		return f, nil

	// For all other types, return them as is.
	default:
		return data, nil
	}
}

var _ yaml.InterfaceUnmarshalerContext = &DelayedUnmarshaler{}

// DelayedUnmarshaler is struct that saves the provided unmarshal function
// passed to UnmarshalYAML so it can be re-used later once the target interface
// is known.
type DelayedUnmarshaler struct {
	unmarshal func(interface{}) error
}

func (d *DelayedUnmarshaler) UnmarshalYAML(ctx context.Context, unmarshal func(interface{}) error) error {
	d.unmarshal = unmarshal
	return nil
}

func (d *DelayedUnmarshaler) Unmarshal(v interface{}) error {
	if d.unmarshal == nil {
		return fmt.Errorf("nothing to unmarshal")
	}
	return d.unmarshal(v)
}

type contextKey string

// userAgentKey is the key used to store userAgent within context
const userAgentKey contextKey = "userAgent"

// WithUserAgent adds a user agent into the context as a value
func WithUserAgent(ctx context.Context, versionString string) context.Context {
	userAgent := "genai-toolbox/" + versionString
	return context.WithValue(ctx, userAgentKey, userAgent)
}

// UserAgentFromContext retrieves the user agent or return an error
func UserAgentFromContext(ctx context.Context) (string, error) {
	if ua := ctx.Value(userAgentKey); ua != nil {
		return ua.(string), nil
	} else {
		return "", fmt.Errorf("unable to retrieve user agent")
	}
}

type UserAgentRoundTripper struct {
	userAgent string
	next      http.RoundTripper
}

func NewUserAgentRoundTripper(ua string, next http.RoundTripper) *UserAgentRoundTripper {
	return &UserAgentRoundTripper{
		userAgent: ua,
		next:      next,
	}
}

func (rt *UserAgentRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// create a deep copy of the request
	newReq := req.Clone(req.Context())
	ua := newReq.Header.Get("User-Agent")
	if ua == "" {
		newReq.Header.Set("User-Agent", rt.userAgent)
	} else {
		newReq.Header.Set("User-Agent", ua+" "+rt.userAgent)
	}
	return rt.next.RoundTrip(newReq)
}

func NewStrictDecoder(v interface{}) (*yaml.Decoder, error) {
	b, err := yaml.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("fail to marshal %q: %w", v, err)
	}

	dec := yaml.NewDecoder(
		bytes.NewReader(b),
		yaml.Strict(),
		yaml.Validator(validator.New()),
	)
	return dec, nil
}

// loggerKey is the key used to store logger within context
const loggerKey contextKey = "logger"

// WithLogger adds a logger into the context as a value
func WithLogger(ctx context.Context, logger log.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, logger)
}

// LoggerFromContext retrieves the logger or return an error
func LoggerFromContext(ctx context.Context) (log.Logger, error) {
	if logger, ok := ctx.Value(loggerKey).(log.Logger); ok {
		return logger, nil
	}
	return nil, fmt.Errorf("unable to retrieve logger")
}

// LogPrimitiveDeprecation is a helper function to mark a primitive as
// deprecated.
//
// kind: kind of primitive. E.g. source / tool / authService / etc.
// primitiveType: the type of primitive being deprecated. E.g. postgres-sql
// migrationStep: the migration step for this deprecation. E.g. "Please use
// [Alternative] instead." / "Please remove this from your configuration file."
func LogPrimitiveDeprecation(ctx context.Context, kind string, primitiveType string, migrationStep string) error {
	l, err := LoggerFromContext(ctx)
	if err != nil {
		return err
	}
	msg := fmt.Sprintf("%s '%s' is deprecated and will be removed in the next major release. %s", kind, primitiveType, migrationStep)
	l.WarnContext(ctx, msg)
	return nil
}

const instrumentationKey contextKey = "instrumentation"

// WithInstrumentation adds an instrumentation into the context as a value
func WithInstrumentation(ctx context.Context, instrumentation *telemetry.Instrumentation) context.Context {
	return context.WithValue(ctx, instrumentationKey, instrumentation)
}

// InstrumentationFromContext retrieves the instrumentation or return an error
func InstrumentationFromContext(ctx context.Context) (*telemetry.Instrumentation, error) {
	if instrumentation, ok := ctx.Value(instrumentationKey).(*telemetry.Instrumentation); ok {
		return instrumentation, nil
	}
	return nil, fmt.Errorf("unable to retrieve instrumentation")
}

// GenAIMetricAttrs holds gen_ai and network attributes for metrics
type GenAIMetricAttrs struct {
	OperationName          string
	ToolName               string
	PromptName             string
	GroupName              string
	NetworkProtocolName    string
	NetworkProtocolVersion string
}

const genAIMetricAttrsKey contextKey = "genAIMetricAttrs"

// WithGenAIMetricAttrs adds GenAIMetricAttrs to the context
func WithGenAIMetricAttrs(ctx context.Context, attrs *GenAIMetricAttrs) context.Context {
	return context.WithValue(ctx, genAIMetricAttrsKey, attrs)
}

// GenAIMetricAttrsFromContext retrieves GenAIMetricAttrs from context
func GenAIMetricAttrsFromContext(ctx context.Context) *GenAIMetricAttrs {
	if attrs, ok := ctx.Value(genAIMetricAttrsKey).(*GenAIMetricAttrs); ok {
		return attrs
	}
	return nil
}

const authTokenClaimsKey contextKey = "authTokenClaims"

// WithAuthTokenClaims adds auth token claims into the context as a value
func WithAuthTokenClaims(ctx context.Context, claims map[string]any) context.Context {
	return context.WithValue(ctx, authTokenClaimsKey, claims)
}

// AuthTokenClaimsFromContext retrieves the auth token claims from context
func AuthTokenClaimsFromContext(ctx context.Context) map[string]any {
	if claims, ok := ctx.Value(authTokenClaimsKey).(map[string]any); ok {
		return claims
	}
	return nil
}

const clientIPKey contextKey = "clientIP"

// WithClientIP adds a client IP address into the context as a value
func WithClientIP(ctx context.Context, clientIP string) context.Context {
	return context.WithValue(ctx, clientIPKey, clientIP)
}

// ClientIPFromContext retrieves the client IP address or returns false if not present
func ClientIPFromContext(ctx context.Context) (string, bool) {
	if ip, ok := ctx.Value(clientIPKey).(string); ok {
		return ip, true
	}
	return "", false
}

// ExtractClientIP retrieves the leftmost client IP from X-Forwarded-For or X-Real-IP header
func ExtractClientIP(header http.Header) string {
	if xff := header.Get("X-Forwarded-For"); xff != "" {
		for _, ip := range strings.Split(xff, ",") {
			if trimmed := strings.TrimSpace(ip); trimmed != "" {
				return trimmed
			}
		}
	}
	if xri := header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}
	return ""
}

// TelemetryAttributes holds client-provided telemetry metadata from _meta["dev.mcp-toolbox/telemetry"].
type TelemetryAttributes struct {
	ClientName    string
	ClientVersion string
	ClientModel   string
	ClientUserID  string
	ClientAgentID string
}

const telemetryAttrsKey contextKey = "telemetryAttrs"

// WithTelemetryAttributes adds TelemetryAttributes to the context
func WithTelemetryAttributes(ctx context.Context, attrs *TelemetryAttributes) context.Context {
	return context.WithValue(ctx, telemetryAttrsKey, attrs)
}

// TelemetryAttributesFromContext retrieves TelemetryAttributes from context
func TelemetryAttributesFromContext(ctx context.Context) *TelemetryAttributes {
	if attrs, ok := ctx.Value(telemetryAttrsKey).(*TelemetryAttributes); ok {
		return attrs
	}
	return nil
}

const sqlCommenterEnabledKey contextKey = "sqlCommenterEnabled"

const responseEncodingKey contextKey = "responseEncoding"

// WithResponseEncoding adds the tool-response output encoding to the context.
func WithResponseEncoding(ctx context.Context, encoding string) context.Context {
	return context.WithValue(ctx, responseEncodingKey, encoding)
}

// ResponseEncodingFromContext retrieves the tool-response output encoding; it
// returns "json" when unset or empty.
func ResponseEncodingFromContext(ctx context.Context) string {
	if e, ok := ctx.Value(responseEncodingKey).(string); ok && e != "" {
		return e
	}
	return "json"
}

// WithSQLCommenterEnabled adds the sql-commenter-enabled flag to the context
func WithSQLCommenterEnabled(ctx context.Context, enabled bool) context.Context {
	return context.WithValue(ctx, sqlCommenterEnabledKey, enabled)
}

// SQLCommenterEnabledFromContext retrieves the sql-commenter-enabled flag from context
func SQLCommenterEnabledFromContext(ctx context.Context) bool {
	if enabled, ok := ctx.Value(sqlCommenterEnabledKey).(bool); ok {
		return enabled
	}
	return false
}

// toolboxVersionKey is the key used to store toolbox version within context
const toolboxVersionKey contextKey = "toolboxVersion"

// WithToolboxVersionKey adds a toolbox version into the context as a value
func WithToolboxVersionKey(ctx context.Context, versionString string) context.Context {
	return context.WithValue(ctx, toolboxVersionKey, versionString)
}

// ToolboxVersionFromContext retrieves the toolbox version or return an error
func ToolboxVersionFromContext(ctx context.Context) (string, error) {
	if v, ok := ctx.Value(toolboxVersionKey).(string); ok && v != "" {
		return v, nil
	} else {
		return "", fmt.Errorf("unable to retrieve toolbox version")
	}
}

const ignoreUnknownToolsKey contextKey = "ignoreUnknownTools"

// WithIgnoreUnknownTools adds the ignore-unknown-tools flag to the context
func WithIgnoreUnknownTools(ctx context.Context, ignore bool) context.Context {
	return context.WithValue(ctx, ignoreUnknownToolsKey, ignore)
}

// IgnoreUnknownToolsFromContext retrieves the ignore-unknown-tools flag from context
func IgnoreUnknownToolsFromContext(ctx context.Context) bool {
	if ignore, ok := ctx.Value(ignoreUnknownToolsKey).(bool); ok {
		return ignore
	}
	return false
}

// urlParamsKey is the key used to store URL parameters within context
const urlParamsKey contextKey = "urlParams"

// WithUrlParams adds URL parameters into the context as a value
func WithUrlParams(ctx context.Context, params map[string]string) context.Context {
	return context.WithValue(ctx, urlParamsKey, params)
}

// UrlParamsFromContext retrieves URL parameters from context
func UrlParamsFromContext(ctx context.Context) (map[string]string, bool) {
	if params, ok := ctx.Value(urlParamsKey).(map[string]string); ok {
		return params, true
	}
	return nil, false
}

// SnakeFromCamelCase converts a camelCase string to snake_case.
func SnakeFromCamelCase(s string) string {
	var result strings.Builder
	for i, r := range s {
		if i > 0 && unicode.IsUpper(r) {
			result.WriteRune('_')
		}
		result.WriteRune(unicode.ToLower(r))
	}
	return result.String()
}

// enableDraftSpecs is the key to check if the server enabled mcp draft specs
const enableDraftSpecs contextKey = "enableDraftSpecs"

// WithEnableDraftSpecs adds enable draft specs bool into the context as a value
func WithEnableDraftSpecs(ctx context.Context, enableDraft bool) context.Context {
	return context.WithValue(ctx, enableDraftSpecs, enableDraft)
}

// EnableDraftSpecsFromContext retrieves enable draft specs bool from context
func EnableDraftSpecsFromContext(ctx context.Context) (bool, bool) {
	if enableDraft, ok := ctx.Value(enableDraftSpecs).(bool); ok {
		return enableDraft, true
	}
	return false, false
}
