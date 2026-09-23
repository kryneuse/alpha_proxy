package server

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"
)

func TestServeRejectsInvalidArguments(t *testing.T) {
	ln := dummyListener(t)
	readiness := newFakeReadiness()
	srv := newFakeServer()

	cases := []struct {
		name      string
		ctx       context.Context
		srv       Server
		listener  net.Listener
		readiness Readiness
		timeout   time.Duration
	}{
		{name: "nil context", ctx: nil, srv: srv, listener: ln, readiness: readiness, timeout: time.Second},
		{name: "nil server", ctx: context.Background(), srv: nil, listener: ln, readiness: readiness, timeout: time.Second},
		{name: "nil listener", ctx: context.Background(), srv: srv, listener: nil, readiness: readiness, timeout: time.Second},
		{name: "nil readiness", ctx: context.Background(), srv: srv, listener: ln, readiness: nil, timeout: time.Second},
		{name: "zero timeout", ctx: context.Background(), srv: srv, listener: ln, readiness: readiness, timeout: 0},
		{name: "negative timeout", ctx: context.Background(), srv: srv, listener: ln, readiness: readiness, timeout: -time.Second},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := Serve(tc.ctx, tc.srv, tc.listener, tc.readiness, tc.timeout); err == nil {
				t.Fatal("Serve() error = nil, want error")
			}
		})
	}
}

func TestServeReturnsNil(t *testing.T) {
	srv := newFakeServer()
	readiness := newFakeReadiness()
	result := runServe(t, context.Background(), srv, dummyListener(t), readiness, time.Second)

	<-srv.serveStarted
	srv.finishServe()

	if err := <-result; err != nil {
		t.Fatalf("Serve() error = %v, want nil", err)
	}
	if readiness.isReady() {
		t.Error("readiness still ready, want false")
	}
}

func TestServeReturnsErrServerClosed(t *testing.T) {
	srv := newFakeServer()
	srv.serveErr = http.ErrServerClosed
	readiness := newFakeReadiness()
	result := runServe(t, context.Background(), srv, dummyListener(t), readiness, time.Second)

	<-srv.serveStarted
	srv.finishServe()

	if err := <-result; err != nil {
		t.Fatalf("Serve() error = %v, want nil for ErrServerClosed", err)
	}
	if readiness.isReady() {
		t.Error("readiness still ready, want false")
	}
}

func TestServeReturnsSentinelError(t *testing.T) {
	sentinel := errors.New("serve boom")
	srv := newFakeServer()
	srv.serveErr = sentinel
	readiness := newFakeReadiness()
	result := runServe(t, context.Background(), srv, dummyListener(t), readiness, time.Second)

	<-srv.serveStarted
	srv.finishServe()

	err := <-result
	if err == nil {
		t.Fatal("Serve() error = nil, want sentinel")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("errors.Is(err, sentinel) = false, want true: %v", err)
	}
	if readiness.isReady() {
		t.Error("readiness still ready, want false")
	}
}

func TestServeContextCancelledGraceful(t *testing.T) {
	srv := newFakeServer()
	readiness := newFakeReadiness()
	ctx, cancel := context.WithCancel(context.Background())

	// Capture readiness state and the shutdown context at Shutdown entry.
	observed := make(chan shutdownObservation, 1)
	srv.shutdownHook = func(shutdownCtx context.Context) {
		_, hasDeadline := shutdownCtx.Deadline()
		observed <- shutdownObservation{
			ready:       readiness.isReady(),
			ctxNil:      shutdownCtx == nil,
			ctxErr:      shutdownCtx.Err(),
			hasDeadline: hasDeadline,
		}
	}

	result := runServe(t, ctx, srv, dummyListener(t), readiness, time.Second)

	<-srv.serveStarted
	cancel()

	<-srv.shutdownCalled
	obs := <-observed
	if obs.ready {
		t.Error("readiness was still ready at Shutdown entry, want false")
	}
	if obs.ctxNil {
		t.Error("Shutdown received a nil context")
	}
	if obs.ctxErr != nil {
		t.Errorf("Shutdown context already cancelled: %v", obs.ctxErr)
	}
	if !obs.hasDeadline {
		t.Error("Shutdown context has no deadline, want a timeout deadline")
	}
	if got := srv.shutdownCount(); got != 1 {
		t.Errorf("Shutdown called %d times, want 1", got)
	}

	srv.finishServe()

	if err := <-result; err != nil {
		t.Fatalf("Serve() error = %v, want nil", err)
	}
	if readiness.isReady() {
		t.Error("readiness still ready, want false")
	}
}

func TestServeShutdownErrorForcesClose(t *testing.T) {
	shutdownSentinel := errors.New("shutdown boom")
	srv := newFakeServer()
	srv.shutdownErr = shutdownSentinel
	readiness := newFakeReadiness()
	ctx, cancel := context.WithCancel(context.Background())
	result := runServe(t, ctx, srv, dummyListener(t), readiness, time.Second)

	<-srv.serveStarted
	cancel()

	<-readiness.notReady
	<-srv.closeCalled
	if got := srv.closeCount(); got != 1 {
		t.Errorf("Close called %d times, want 1", got)
	}

	srv.finishServe()

	err := <-result
	if err == nil {
		t.Fatal("Serve() error = nil, want shutdown sentinel")
	}
	if !errors.Is(err, shutdownSentinel) {
		t.Errorf("errors.Is(err, shutdownSentinel) = false, want true: %v", err)
	}
}

func TestServeShutdownAndCloseErrorsJoined(t *testing.T) {
	shutdownSentinel := errors.New("shutdown boom")
	closeSentinel := errors.New("close boom")
	srv := newFakeServer()
	srv.shutdownErr = shutdownSentinel
	srv.closeErr = closeSentinel
	readiness := newFakeReadiness()
	ctx, cancel := context.WithCancel(context.Background())
	result := runServe(t, ctx, srv, dummyListener(t), readiness, time.Second)

	<-srv.serveStarted
	cancel()

	<-readiness.notReady
	<-srv.closeCalled
	srv.finishServe()

	err := <-result
	if err == nil {
		t.Fatal("Serve() error = nil, want joined errors")
	}
	if !errors.Is(err, shutdownSentinel) {
		t.Errorf("errors.Is(err, shutdownSentinel) = false, want true: %v", err)
	}
	if !errors.Is(err, closeSentinel) {
		t.Errorf("errors.Is(err, closeSentinel) = false, want true: %v", err)
	}
}

func TestServeShutdownErrorWithServeSentinel(t *testing.T) {
	shutdownSentinel := errors.New("shutdown boom")
	serveSentinel := errors.New("serve boom")
	srv := newFakeServer()
	srv.shutdownErr = shutdownSentinel
	srv.serveErr = serveSentinel
	readiness := newFakeReadiness()
	ctx, cancel := context.WithCancel(context.Background())
	result := runServe(t, ctx, srv, dummyListener(t), readiness, time.Second)

	<-srv.serveStarted
	cancel()

	<-readiness.notReady
	<-srv.closeCalled
	if got := srv.closeCount(); got != 1 {
		t.Errorf("Close called %d times, want 1", got)
	}

	srv.finishServe()

	err := <-result
	if err == nil {
		t.Fatal("Serve() error = nil, want joined errors")
	}
	if !errors.Is(err, shutdownSentinel) {
		t.Errorf("errors.Is(err, shutdownSentinel) = false, want true: %v", err)
	}
	if !errors.Is(err, serveSentinel) {
		t.Errorf("errors.Is(err, serveSentinel) = false, want true: %v", err)
	}
}

func TestServeShutdownCloseServeErrorsJoined(t *testing.T) {
	shutdownSentinel := errors.New("shutdown boom")
	closeSentinel := errors.New("close boom")
	serveSentinel := errors.New("serve boom")
	srv := newFakeServer()
	srv.shutdownErr = shutdownSentinel
	srv.closeErr = closeSentinel
	srv.serveErr = serveSentinel
	readiness := newFakeReadiness()
	ctx, cancel := context.WithCancel(context.Background())
	result := runServe(t, ctx, srv, dummyListener(t), readiness, time.Second)

	<-srv.serveStarted
	cancel()

	<-readiness.notReady
	<-srv.closeCalled
	srv.finishServe()

	err := <-result
	if err == nil {
		t.Fatal("Serve() error = nil, want joined errors")
	}
	if !errors.Is(err, shutdownSentinel) {
		t.Errorf("errors.Is(err, shutdownSentinel) = false, want true: %v", err)
	}
	if !errors.Is(err, closeSentinel) {
		t.Errorf("errors.Is(err, closeSentinel) = false, want true: %v", err)
	}
	if !errors.Is(err, serveSentinel) {
		t.Errorf("errors.Is(err, serveSentinel) = false, want true: %v", err)
	}
}

func TestServeGracefulDrain(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen error: %v", err)
	}
	defer func() {
		_ = ln.Close()
	}()

	started := make(chan struct{})
	release := make(chan struct{})
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		w.WriteHeader(http.StatusOK)
	})

	srv := &http.Server{Handler: handler}
	readiness := newFakeReadiness()
	ctx, cancel := context.WithCancel(context.Background())
	result := runServe(t, ctx, srv, ln, readiness, 5*time.Second)

	client := &http.Client{Timeout: 5 * time.Second}
	clientResultCh := make(chan clientResult, 1)
	go func() {
		resp, err := client.Get("http://" + ln.Addr().String() + "/")
		if err != nil {
			clientResultCh <- clientResult{err: err}
			return
		}
		_, readErr := io.Copy(io.Discard, resp.Body)
		closeErr := resp.Body.Close()
		clientResultCh <- clientResult{
			statusCode: resp.StatusCode,
			err:        errors.Join(readErr, closeErr),
		}
	}()

	<-started
	cancel()

	<-readiness.notReady

	// The lifecycle must still be running while the handler is blocked.
	select {
	case err := <-result:
		t.Fatalf("lifecycle returned early: %v", err)
	default:
	}

	close(release)

	res := <-clientResultCh
	if res.err != nil {
		t.Fatalf("client error: %v", res.err)
	}
	if res.statusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", res.statusCode)
	}

	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("Serve() error = %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("lifecycle did not finish after drain")
	}
}

// runServe runs Serve in a goroutine and returns a channel with its result.
func runServe(t *testing.T, ctx context.Context, srv Server, listener net.Listener, readiness Readiness, timeout time.Duration) <-chan error {
	t.Helper()
	result := make(chan error, 1)
	go func() {
		result <- Serve(ctx, srv, listener, readiness, timeout)
	}()
	return result
}

// dummyListener returns a closed TCP listener that is non-nil but never used.
func dummyListener(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen error: %v", err)
	}
	if err := ln.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}
	return ln
}

// shutdownObservation captures the state observed at Shutdown entry.
type shutdownObservation struct {
	ready       bool
	ctxNil      bool
	ctxErr      error
	hasDeadline bool
}

// clientResult carries the outcome of a client request performed in a goroutine.
type clientResult struct {
	statusCode int
	err        error
}

// fakeReadiness records the last state and signals SetReady(false) once.
type fakeReadiness struct {
	mu       sync.Mutex
	ready    bool
	notReady chan struct{}
	closed   bool
}

func newFakeReadiness() *fakeReadiness {
	return &fakeReadiness{ready: true, notReady: make(chan struct{})}
}

func (f *fakeReadiness) SetReady(ready bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ready = ready
	if !ready && !f.closed {
		close(f.notReady)
		f.closed = true
	}
}

func (f *fakeReadiness) isReady() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.ready
}

// fakeServer counts calls and lets tests control errors and Serve completion.
type fakeServer struct {
	mu             sync.Mutex
	serveCalls     int
	shutdownCalls  int
	closeCalls     int
	serveErr       error
	shutdownErr    error
	closeErr       error
	shutdownHook   func(ctx context.Context)
	serveRelease   chan struct{}
	releaseOnce    sync.Once
	serveStarted   chan struct{}
	serveStartOnce sync.Once
	shutdownCalled chan struct{}
	shutdownOnce   sync.Once
	closeCalled    chan struct{}
	closeOnce      sync.Once
}

func newFakeServer() *fakeServer {
	return &fakeServer{
		serveRelease:   make(chan struct{}),
		serveStarted:   make(chan struct{}),
		shutdownCalled: make(chan struct{}),
		closeCalled:    make(chan struct{}),
	}
}

func (f *fakeServer) Serve(net.Listener) error {
	f.mu.Lock()
	f.serveCalls++
	f.mu.Unlock()
	f.serveStartOnce.Do(func() { close(f.serveStarted) })
	<-f.serveRelease
	f.mu.Lock()
	err := f.serveErr
	f.mu.Unlock()
	return err
}

func (f *fakeServer) Shutdown(ctx context.Context) error {
	f.mu.Lock()
	f.shutdownCalls++
	err := f.shutdownErr
	f.mu.Unlock()
	if f.shutdownHook != nil {
		f.shutdownHook(ctx)
	}
	f.shutdownOnce.Do(func() { close(f.shutdownCalled) })
	return err
}

func (f *fakeServer) Close() error {
	f.mu.Lock()
	f.closeCalls++
	err := f.closeErr
	f.mu.Unlock()
	f.closeOnce.Do(func() { close(f.closeCalled) })
	return err
}

func (f *fakeServer) finishServe() {
	f.releaseOnce.Do(func() { close(f.serveRelease) })
}

func (f *fakeServer) shutdownCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.shutdownCalls
}

func (f *fakeServer) closeCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closeCalls
}
