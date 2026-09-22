// Command server runs the HTTP contour of the alpha_proxy service.
//
// The temporary MockProcessor is used only when explicitly selected in dev mode.
// In verify/final mode a real Processor is required; without one the server
// refuses to start with a safe error.
package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/kryneuse/alpha_proxy/internal/app"
	"github.com/kryneuse/alpha_proxy/internal/config"
	"github.com/kryneuse/alpha_proxy/internal/contract"
	"github.com/kryneuse/alpha_proxy/internal/observability"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "startup error:", err)
		os.Exit(1)
	}

	logger := observability.NewLogger()

	processor, err := buildProcessor(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "startup error:", err)
		os.Exit(1)
	}

	handler := app.New(cfg, logger, processor)

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           handler,
		ReadTimeout:       cfg.ReadTimeout,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
	}

	logger.Info("server listening", "addr", cfg.Addr)
	if err := srv.ListenAndServe(); err != nil {
		fmt.Fprintln(os.Stderr, "server error:", err)
		os.Exit(1)
	}
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
