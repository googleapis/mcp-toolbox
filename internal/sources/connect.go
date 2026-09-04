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

package sources

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/singleflight"
)

// ConnectTimeout is the default ceiling on a connection attempt. It is sized
// for a cold cloud connector path rather than for a healthy connection.
const ConnectTimeout = 60 * time.Second

// Option configures a ConnectOnce.
type Option func(*options)

type options struct {
	timeout time.Duration
}

// WithMinConnectTimeout raises the ceiling for a source whose own configuration
// allows a longer connect. A shorter value is ignored: the source's own bound
// still applies inside the ceiling, and lowering the ceiling would not make it
// any tighter.
func WithMinConnectTimeout(d time.Duration) Option {
	return func(o *options) { o.timeout = d }
}

// ConnectOnce holds a connection a source builds on first use. A source keeps
// one of these instead of a bare handle and resolves it through Do.
type ConnectOnce[T any] struct {
	name       string
	sourceType string
	tracer     trace.Tracer
	timeout    time.Duration

	// startupCtx is the context Initialize ran under. The connect derives from
	// it rather than from the caller that triggers it, so a source that
	// connects on first use sees what one that connected at startup would.
	startupCtx context.Context

	// closeFn releases the connection. It is registered rather than discovered
	// through an io.Closer assertion because the handles sources hold do not
	// agree on a shape: pgxpool.Pool.Close takes no context and returns
	// nothing, neo4j's driver takes a context, and mongo spells it Disconnect.
	// An assertion would compile against all of them and silently skip the
	// ones that do not match.
	closeFn func(context.Context, T) error

	mu     sync.RWMutex
	value  T
	ready  bool
	closed bool

	// initGroup, not mu, serializes connecting. A mutex held for the length of
	// a connect would block a caller from abandoning a hung attempt.
	initGroup singleflight.Group
}

// NewConnectOnce returns a holder for a connection that has not been made yet.
// ctx must be the context Initialize was called with: every later connect runs
// under it, so the source reports the startup user agent and cannot pick up
// request-scoped values from whichever caller happens to trigger it.
func NewConnectOnce[T any](ctx context.Context, name, sourceType string, tracer trace.Tracer, opts ...Option) *ConnectOnce[T] {
	o := options{timeout: ConnectTimeout}
	for _, opt := range opts {
		opt(&o)
	}
	if o.timeout < ConnectTimeout {
		o.timeout = ConnectTimeout
	}
	return &ConnectOnce[T]{name: name, sourceType: sourceType, tracer: tracer, startupCtx: ctx, timeout: o.timeout}
}

// OnClose registers how to release the connection. A source that holds a
// handle with no teardown can leave it unset. It is meant to be chained onto
// NewConnectOnce, before the holder is reachable by another goroutine.
func (c *ConnectOnce[T]) OnClose(fn func(context.Context, T) error) *ConnectOnce[T] {
	c.closeFn = fn
	return c
}

// Get returns the connection if one has already been made. It never blocks and
// never fails, so a source's context-free accessors — the ones tools type
// assert on — can report a handle without being able to build one.
func (c *ConnectOnce[T]) Get() (T, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.value, c.ready
}

func (c *ConnectOnce[T]) isClosed() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.closed
}

// release hands a value to the registered closer. The caller must not hold mu:
// a source's teardown can be slow, and blocking Get behind it would stall the
// tools that only want to read the handle.
func (c *ConnectOnce[T]) release(ctx context.Context, value T) error {
	if c.closeFn == nil {
		return nil
	}
	return c.closeFn(ctx, value)
}

// Close releases the connection, if one was ever made, and stops another from
// being made. It is safe on a source that never connected and safe to call
// twice.
//
// Close does not wait for an attempt that is already in flight. That attempt
// releases its own result rather than caching it into a closed holder, so
// shutdown never blocks on a connect that may be hung for the full timeout.
func (c *ConnectOnce[T]) Close(ctx context.Context) error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	var zero T
	value, ready := c.value, c.ready
	c.value, c.ready, c.closed = zero, false, true
	c.mu.Unlock()

	if !ready {
		return nil
	}
	if err := c.release(ctx, value); err != nil {
		return fmt.Errorf("unable to close source %q: %w", c.name, err)
	}
	return nil
}

// Do returns the connection, making it on the first call. Concurrent callers
// share one attempt, and a failed attempt is not remembered.
//
// Because a failure is retried by the next caller rather than cached, connect
// must release whatever it had already built before it returns an error. A
// pool that fails its ping and is returned unclosed leaks once per tool call,
// not once per process.
func (c *ConnectOnce[T]) Do(ctx context.Context, connect func(context.Context) (T, error)) (T, error) {
	var zero T
	if value, ok := c.Get(); ok {
		return value, nil
	}
	if c.isClosed() {
		return zero, fmt.Errorf("unable to connect to source %q: source is closed", c.name)
	}

	ch := c.initGroup.DoChan("", func() (any, error) {
		// singleflight only shares an attempt that is still in flight, so a
		// caller queued behind a finished winner would start a second connect.
		if value, ok := c.Get(); ok {
			return value, nil
		}
		if c.isClosed() {
			return nil, fmt.Errorf("unable to connect to source %q: source is closed", c.name)
		}

		// The attempt is shared by every waiter and the handle outlives the
		// request that triggered it, so it runs under the startup context
		// rather than the caller's: a request context carries that caller's
		// auth claims and a user agent that omits --user-agent-metadata, and it
		// is cancelled when that one caller goes away. Only the span context
		// crosses over, so the connect still appears in the trace of the
		// request that paid for it.
		base := trace.ContextWithSpanContext(
			context.WithoutCancel(c.startupCtx),
			trace.SpanContextFromContext(ctx),
		)
		connectCtx, cancel := context.WithTimeout(base, c.timeout)
		defer cancel()

		// What is deferred is the connection, not the source, which is built
		// at startup either way and keeps server.go's init span. Emitting the
		// connect span from here is what lets a source drop its own: every
		// connect reaches this path, whether it runs at startup or later.
		childCtx, span := InitConnectionSpan(connectCtx, c.tracer, c.sourceType, c.name)
		defer span.End()

		value, err := connect(childCtx)
		if err != nil {
			span.SetStatus(codes.Error, err.Error())
			return nil, fmt.Errorf("unable to connect to source %q: %w", c.name, err)
		}

		c.mu.Lock()
		if c.closed {
			c.mu.Unlock()
			// Close ran while this attempt was in flight. Nothing will hand the
			// value out, so it is released here rather than left to outlive the
			// holder. The connect context is already near its deadline and the
			// caller's is irrelevant to a teardown, so neither bounds the close.
			if cerr := c.release(context.WithoutCancel(childCtx), value); cerr != nil {
				return nil, fmt.Errorf("unable to close source %q: %w", c.name, cerr)
			}
			return nil, fmt.Errorf("unable to connect to source %q: source is closed", c.name)
		}
		c.value, c.ready = value, true
		c.mu.Unlock()
		return value, nil
	})

	select {
	case res := <-ch:
		if res.Err != nil {
			return zero, res.Err
		}
		value, _ := res.Val.(T)
		return value, nil
	case <-ctx.Done():
		// Only this caller gives up; the shared attempt runs on for the others.
		return zero, fmt.Errorf("unable to connect to source %q: %w", c.name, ctx.Err())
	}
}
