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

package sources_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"go.opentelemetry.io/otel/trace/noop"
)

// handle stands in for whatever a source connects to.
type handle struct{ id int }

// connector records how a source's connect function was called.
type connector struct {
	mu       sync.Mutex
	calls    int
	err      error
	delay    time.Duration
	ctxErr   error
	userAg   string
	claims   map[string]any
	deadline time.Time

	// entered reports that a connect is under way, so a test can act on an
	// in-flight attempt without guessing at how long the scheduler will take.
	entered chan struct{}
}

func (c *connector) connect(ctx context.Context) (*handle, error) {
	ua, _ := util.UserAgentFromContext(ctx)
	claims := util.AuthTokenClaimsFromContext(ctx)
	deadline, _ := ctx.Deadline()

	if c.entered != nil {
		select {
		case c.entered <- struct{}{}:
		default:
		}
	}

	c.mu.Lock()
	c.calls++
	id, err, delay := c.calls, c.err, c.delay
	c.userAg, c.claims, c.deadline = ua, claims, deadline
	c.mu.Unlock()

	// Holding the connection open lets concurrent callers pile up behind it, so
	// a missing singleflight shows up as extra calls.
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
		c.mu.Lock()
		c.ctxErr = ctx.Err()
		c.mu.Unlock()
		return nil, ctx.Err()
	}

	if err != nil {
		return nil, err
	}
	return &handle{id: id}, nil
}

func (c *connector) callCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

func (c *connector) connectContextErr() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ctxErr
}

func (c *connector) observedUserAgent() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.userAg
}

func (c *connector) observedClaims() map[string]any {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.claims
}

func (c *connector) observedTimeout(from time.Time) time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.deadline.Sub(from)
}

func newConnectOnce(ctx context.Context, opts ...sources.Option) *sources.ConnectOnce[*handle] {
	return sources.NewConnectOnce[*handle](ctx, "my-source", "mock", noop.NewTracerProvider().Tracer("test"), opts...)
}

// releases records the handles a ConnectOnce handed to its closer.
type releases struct {
	mu   sync.Mutex
	got  []*handle
	fail error
}

func (r *releases) close(_ context.Context, h *handle) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.got = append(r.got, h)
	return r.fail
}

func (r *releases) all() []*handle {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]*handle(nil), r.got...)
}

func TestConnectOnceCoalescesConcurrentCallers(t *testing.T) {
	c := &connector{delay: 50 * time.Millisecond}
	once := newConnectOnce(context.Background())

	const callers = 16
	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make([]error, callers)
	for i := range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, errs[i] = once.Do(context.Background(), c.connect)
		}()
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("caller %d failed to connect: %s", i, err)
		}
	}
	if got := c.callCount(); got != 1 {
		t.Fatalf("expected the racing callers to share one connect, got %d", got)
	}
}

func TestConnectOnceSurvivesFirstCallerCancellation(t *testing.T) {
	c := &connector{delay: 100 * time.Millisecond, entered: make(chan struct{}, 1)}
	once := newConnectOnce(context.Background())

	// The first caller starts the shared attempt and then walks away. Its
	// context must not travel into the connect, or every other caller waiting
	// on the same attempt fails for a request that is no longer theirs.
	firstCtx, cancelFirst := context.WithCancel(context.Background())
	firstStarted := make(chan struct{})
	firstErr := make(chan error, 1)
	go func() {
		close(firstStarted)
		_, err := once.Do(firstCtx, c.connect)
		firstErr <- err
	}()
	<-firstStarted

	secondErr := make(chan error, 1)
	go func() {
		// Wait until the first caller is inside the connect. Sleeping instead
		// would assume how the scheduler orders the two, and on a loaded box
		// the cancellation could land before the attempt it is meant to test.
		<-c.entered
		cancelFirst()
		_, err := once.Do(context.Background(), c.connect)
		secondErr <- err
	}()

	if err := <-firstErr; !errors.Is(err, context.Canceled) {
		t.Fatalf("expected the cancelled caller to see its own cancellation, got %v", err)
	}
	if err := <-secondErr; err != nil {
		t.Fatalf("a cancelled caller must not fail the others waiting: %s", err)
	}
	if err := c.connectContextErr(); err != nil {
		t.Fatalf("the connect saw a cancelled context: %s", err)
	}
	if _, ok := once.Get(); !ok {
		t.Fatal("expected the shared attempt to finish and cache the connection")
	}
}

func TestConnectOnceRespectsCallerDeadline(t *testing.T) {
	// The delay only has to outlast the caller's deadline; a longer one would
	// leave the shared attempt running for the rest of the package's tests.
	c := &connector{delay: 2 * time.Second}
	once := newConnectOnce(context.Background())

	// A hung connect must not pin the request for ConnectTimeout.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := once.Do(ctx, c.connect)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected the caller's deadline to end the wait, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("caller waited %s, well past its own deadline", elapsed)
	}
}

func TestConnectOnceRetriesAfterFailure(t *testing.T) {
	c := &connector{err: errors.New("connection refused")}
	once := newConnectOnce(context.Background())

	if _, err := once.Do(context.Background(), c.connect); err == nil {
		t.Fatal("expected the first connect to fail")
	}
	if _, ok := once.Get(); ok {
		t.Fatal("a failed connection must not be cached")
	}

	c.mu.Lock()
	c.err = nil
	c.mu.Unlock()

	if _, err := once.Do(context.Background(), c.connect); err != nil {
		t.Fatalf("expected the retry to succeed, got %s", err)
	}
	if got := c.callCount(); got != 2 {
		t.Fatalf("expected 2 connect attempts, got %d", got)
	}
}

func TestConnectOnceReusesTheConnection(t *testing.T) {
	c := &connector{}
	once := newConnectOnce(context.Background())

	first, err := once.Do(context.Background(), c.connect)
	if err != nil {
		t.Fatalf("unexpected error connecting: %s", err)
	}
	second, err := once.Do(context.Background(), c.connect)
	if err != nil {
		t.Fatalf("unexpected error on the second call: %s", err)
	}
	if first != second {
		t.Fatalf("expected the same connection, got %v and %v", first, second)
	}
	if got := c.callCount(); got != 1 {
		t.Fatalf("expected the connection to be reused, got %d attempts", got)
	}
}

func TestConnectOnceCeiling(t *testing.T) {
	tests := []struct {
		name string
		opts []sources.Option
		want time.Duration
	}{
		{
			name: "default ceiling",
			want: sources.ConnectTimeout,
		},
		{
			name: "configured value longer than the ceiling raises it",
			opts: []sources.Option{sources.WithMinConnectTimeout(10 * time.Minute)},
			want: 10 * time.Minute,
		},
		{
			name: "configured value shorter than the ceiling is ignored",
			opts: []sources.Option{sources.WithMinConnectTimeout(time.Millisecond)},
			want: sources.ConnectTimeout,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := &connector{}
			once := newConnectOnce(context.Background(), tc.opts...)

			start := time.Now()
			if _, err := once.Do(context.Background(), c.connect); err != nil {
				t.Fatalf("unexpected error connecting: %s", err)
			}
			if got := c.observedTimeout(start); (got - tc.want).Abs() > time.Second {
				t.Errorf("shared attempt bounded at %s, want %s", got, tc.want)
			}
		})
	}
}

// The connection is shared by every caller and outlives the request that
// triggered it, so the connect must run under the startup context. A request
// context carries a user agent that omits --user-agent-metadata and auth claims
// belonging to whichever caller happened to arrive first.
func TestConnectOnceConnectsUnderTheStartupContext(t *testing.T) {
	tests := []struct {
		name      string
		startupUA string
		want      string
	}{
		{
			name:      "startup user agent is used, not the request one",
			startupUA: "1.2.3+custom-metadata",
			want:      "genai-toolbox/1.2.3+custom-metadata",
		},
		{
			name: "no user agent is invented when startup had none",
			want: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			startupCtx := context.Background()
			if tc.startupUA != "" {
				startupCtx = testutils.ContextWithUserAgent(startupCtx, tc.startupUA)
			}
			once := newConnectOnce(startupCtx)

			c := &connector{}
			callerCtx := util.WithAuthTokenClaims(
				testutils.ContextWithUserAgent(context.Background(), "1.2.3"),
				map[string]any{"sub": "caller@example.com"},
			)
			if _, err := once.Do(callerCtx, c.connect); err != nil {
				t.Fatalf("unexpected error connecting: %s", err)
			}
			if got := c.observedUserAgent(); got != tc.want {
				t.Errorf("connect saw user agent %q, want %q", got, tc.want)
			}
			if got := c.observedClaims(); got != nil {
				t.Errorf("connect saw the triggering caller's auth claims %v, want none", got)
			}
		})
	}
}

func TestConnectOnceCloseReleasesTheConnection(t *testing.T) {
	c := &connector{}
	r := &releases{}
	once := newConnectOnce(context.Background()).OnClose(r.close)

	value, err := once.Do(context.Background(), c.connect)
	if err != nil {
		t.Fatalf("unexpected error connecting: %s", err)
	}
	if err := once.Close(context.Background()); err != nil {
		t.Fatalf("unexpected error closing: %s", err)
	}

	got := r.all()
	if len(got) != 1 {
		t.Fatalf("expected the connection to be released once, got %d", len(got))
	}
	if got[0] != value {
		t.Fatalf("released %v, want the connection that was handed out, %v", got[0], value)
	}
	if _, ok := once.Get(); ok {
		t.Fatal("a closed holder must not still report a connection")
	}
}

func TestConnectOnceCloseWithoutAConnection(t *testing.T) {
	r := &releases{}
	once := newConnectOnce(context.Background()).OnClose(r.close)

	// Nothing was ever built, so there is nothing to hand the closer. A source
	// that is configured but never used still gets closed at shutdown.
	if err := once.Close(context.Background()); err != nil {
		t.Fatalf("unexpected error closing an unused source: %s", err)
	}
	if got := r.all(); len(got) != 0 {
		t.Fatalf("expected no releases, got %d", len(got))
	}
}

func TestConnectOnceCloseIsIdempotent(t *testing.T) {
	c := &connector{}
	r := &releases{}
	once := newConnectOnce(context.Background()).OnClose(r.close)

	if _, err := once.Do(context.Background(), c.connect); err != nil {
		t.Fatalf("unexpected error connecting: %s", err)
	}
	for i := range 3 {
		if err := once.Close(context.Background()); err != nil {
			t.Fatalf("close %d failed: %s", i, err)
		}
	}
	if got := r.all(); len(got) != 1 {
		t.Fatalf("expected one release across repeated closes, got %d", len(got))
	}
}

func TestConnectOnceRefusesToConnectAfterClose(t *testing.T) {
	c := &connector{}
	once := newConnectOnce(context.Background())

	if err := once.Close(context.Background()); err != nil {
		t.Fatalf("unexpected error closing: %s", err)
	}
	if _, err := once.Do(context.Background(), c.connect); err == nil {
		t.Fatal("expected a closed source to refuse to connect")
	}
	if got := c.callCount(); got != 0 {
		t.Fatalf("a closed source must not reconnect, got %d attempts", got)
	}
}

// A connect already in flight when Close runs has nowhere to put its result.
// It must release the value itself, or shutdown leaks exactly the connection
// it was trying to reclaim.
func TestConnectOnceReleasesAnAttemptThatOutlivesClose(t *testing.T) {
	c := &connector{delay: 50 * time.Millisecond, entered: make(chan struct{}, 1)}
	r := &releases{}
	once := newConnectOnce(context.Background()).OnClose(r.close)

	done := make(chan error, 1)
	go func() {
		_, err := once.Do(context.Background(), c.connect)
		done <- err
	}()

	<-c.entered
	if err := once.Close(context.Background()); err != nil {
		t.Fatalf("unexpected error closing: %s", err)
	}

	if err := <-done; err == nil {
		t.Fatal("expected the caller to be told the source closed")
	}
	if got := r.all(); len(got) != 1 {
		t.Fatalf("expected the in-flight connection to be released, got %d", len(got))
	}
	if _, ok := once.Get(); ok {
		t.Fatal("an attempt that finishes after Close must not be cached")
	}
}

func TestConnectOnceCloseReportsCloserFailure(t *testing.T) {
	c := &connector{}
	r := &releases{fail: errors.New("pool did not drain")}
	once := newConnectOnce(context.Background()).OnClose(r.close)

	if _, err := once.Do(context.Background(), c.connect); err != nil {
		t.Fatalf("unexpected error connecting: %s", err)
	}
	err := once.Close(context.Background())
	if err == nil {
		t.Fatal("expected the closer's failure to surface")
	}
	if !errors.Is(err, r.fail) {
		t.Fatalf("expected the closer's error to be wrapped, got %v", err)
	}
	if _, ok := once.Get(); ok {
		t.Fatal("a failed close must still drop the connection")
	}
}

func TestConnectOnceCloseWithoutACloser(t *testing.T) {
	c := &connector{}
	// A source whose handle needs no teardown leaves the closer unset.
	once := newConnectOnce(context.Background())

	if _, err := once.Do(context.Background(), c.connect); err != nil {
		t.Fatalf("unexpected error connecting: %s", err)
	}
	if err := once.Close(context.Background()); err != nil {
		t.Fatalf("unexpected error closing: %s", err)
	}
	if _, ok := once.Get(); ok {
		t.Fatal("a closed holder must not still report a connection")
	}
}
