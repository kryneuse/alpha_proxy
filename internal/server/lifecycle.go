// Package server provides a testable lifecycle for the HTTP server: it runs
// Serve in a single goroutine and coordinates graceful shutdown with readiness.
// It logs nothing, spawns no extra workers and never touches OS signals.
package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"
)

// Server is the minimal surface the lifecycle needs from an HTTP server.
// *http.Server implements it.
type Server interface {
	Serve(net.Listener) error
	Shutdown(context.Context) error
	Close() error
}

// Readiness is the minimal surface the lifecycle needs to flip readiness.
// *health.Readiness implements it.
type Readiness interface {
	SetReady(bool)
}

// Serve runs srv.Serve(listener) in a single goroutine and returns when either
// Serve completes on its own or ctx is cancelled. On any exit readiness is set
// to false. On ctx cancellation it performs a graceful shutdown bounded by
// shutdownTimeout, falling back to a forced Close if Shutdown fails.
func Serve(ctx context.Context, srv Server, listener net.Listener, readiness Readiness, shutdownTimeout time.Duration) error {
	if ctx == nil {
		return errors.New("server: context must not be nil")
	}
	if srv == nil {
		return errors.New("server: server must not be nil")
	}
	if listener == nil {
		return errors.New("server: listener must not be nil")
	}
	if readiness == nil {
		return errors.New("server: readiness must not be nil")
	}
	if shutdownTimeout <= 0 {
		return errors.New("server: shutdown timeout must be positive")
	}

	serveDone := make(chan error, 1)
	go func() {
		serveDone <- srv.Serve(listener)
	}()

	select {
	case serveErr := <-serveDone:
		readiness.SetReady(false)
		return normalizeServeResult(serveErr)
	case <-ctx.Done():
		return gracefulShutdown(srv, readiness, serveDone, shutdownTimeout)
	}
}

// gracefulShutdown flips readiness off, drains in-flight requests with a fresh
// timeout context and, if Shutdown fails, force-closes the server.
func gracefulShutdown(srv Server, readiness Readiness, serveDone <-chan error, shutdownTimeout time.Duration) error {
	readiness.SetReady(false)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		closeErr := srv.Close()
		serveErr := <-serveDone
		return joinShutdownErrors(err, closeErr, serveErr)
	}

	serveErr := <-serveDone
	if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
		return fmt.Errorf("server: serve: %w", serveErr)
	}
	return nil
}

// joinShutdownErrors collects the significant causes of a failed shutdown. The
// shutdown error is always included; the close error and a non-ErrServerClosed
// serve error are included when present. nil and http.ErrServerClosed are never
// added.
func joinShutdownErrors(shutdownErr, closeErr, serveErr error) error {
	causes := []error{fmt.Errorf("server: shutdown: %w", shutdownErr)}
	if closeErr != nil {
		causes = append(causes, fmt.Errorf("server: close: %w", closeErr))
	}
	if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
		causes = append(causes, fmt.Errorf("server: serve: %w", serveErr))
	}
	return errors.Join(causes...)
}

// normalizeServeResult maps a self-terminated Serve result: ErrServerClosed and
// nil are normal, anything else is wrapped.
func normalizeServeResult(err error) error {
	if err == nil || errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return fmt.Errorf("server: serve: %w", err)
}
