// Command server runs the HTTP contour of the alpha_proxy service.
//
// The temporary MockProcessor is used only when explicitly selected in dev mode.
// In verify/final mode a real Processor is required; without one the server
// refuses to start with a safe error.
package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/kryneuse/alpha_proxy/internal/app"
	"github.com/kryneuse/alpha_proxy/internal/config"
	"github.com/kryneuse/alpha_proxy/internal/contract"
	"github.com/kryneuse/alpha_proxy/internal/observability"
	"github.com/kryneuse/alpha_proxy/internal/server"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "startup error:", err)
		os.Exit(1)
	}
}

// run loads configuration, builds the runtime and serves HTTP until ctx is
// cancelled. It never calls os.Exit.
func run(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("run: context must not be nil")
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logger := observability.NewLogger()

	processor, err := buildProcessor(cfg)
	if err != nil {
		return fmt.Errorf("build processor: %w", err)
	}

	runtime, err := app.NewRuntime(cfg, logger, processor)
	if err != nil {
		return fmt.Errorf("build runtime: %w", err)
	}

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           runtime.Handler,
		ReadTimeout:       cfg.ReadTimeout,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
	}

	listener, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	logger.Info("server listening", "addr", listener.Addr().String())

	if err := server.Serve(ctx, srv, listener, runtime.Readiness, cfg.ShutdownTimeout); err != nil {
		return fmt.Errorf("serve lifecycle: %w", err)
	}

	logger.Info("server stopped")
	return nil
}

// buildProcessor returns the Processor for the configured mode. The mock is a
// temporary stub and is allowed only in dev mode; verify/final modes require a
// real Processor, which is not yet implemented, so startup fails safely.
func buildProcessor(cfg config.Config) (contract.Processor, error) {
	if cfg.ProcessorMode == config.ProcessorMock {
		if cfg.RunMode != config.RunModeDev {
			return nil, fmt.Errorf("mock processor is allowed only in dev mode")
		}
		return contract.MockProcessor{}, nil
	}
	return nil, fmt.Errorf("no real processor configured; refusing to start in %s mode", cfg.RunMode)
}
