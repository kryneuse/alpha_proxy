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
	"sync"
	"syscall"
	"time"

	mlv1 "github.com/kryneuse/alpha_proxy/gen/ml/v1"
	"github.com/kryneuse/alpha_proxy/internal/app"
	"github.com/kryneuse/alpha_proxy/internal/auth"
	"github.com/kryneuse/alpha_proxy/internal/cascade"
	"github.com/kryneuse/alpha_proxy/internal/config"
	"github.com/kryneuse/alpha_proxy/internal/contract"
	"github.com/kryneuse/alpha_proxy/internal/engine"
	"github.com/kryneuse/alpha_proxy/internal/gate"
	"github.com/kryneuse/alpha_proxy/internal/masking"
	"github.com/kryneuse/alpha_proxy/internal/ml"
	"github.com/kryneuse/alpha_proxy/internal/observability"
	"github.com/kryneuse/alpha_proxy/internal/pii"
	"github.com/kryneuse/alpha_proxy/internal/policy"
	"github.com/kryneuse/alpha_proxy/internal/processor"
	"github.com/kryneuse/alpha_proxy/internal/server"
	"github.com/kryneuse/alpha_proxy/internal/store"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
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

	processor, cleanup, err := buildProcessor(cfg)
	if err != nil {
		return fmt.Errorf("build processor: %w", err)
	}
	defer cleanup()

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

	var listenConfig net.ListenConfig
	listener, err := listenConfig.Listen(ctx, "tcp", cfg.Addr)
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

// buildProcessor returns the Processor for the configured mode and a cleanup
// function. The mock is a temporary stub and is allowed only in dev mode;
// verify/final modes require a real Processor wired to the Python ML service.
func buildProcessor(cfg config.Config) (contract.Processor, func(), error) {
	if cfg.ProcessorMode == config.ProcessorMock {
		if cfg.RunMode != config.RunModeDev {
			return nil, nil, fmt.Errorf("mock processor is allowed only in dev mode")
		}
		return contract.MockProcessor{}, func() {}, nil
	}

	conn, err := grpc.NewClient(cfg.MLAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, nil, fmt.Errorf("create ml grpc client: %w", err)
	}

	var batcher *ml.Batcher
	var once sync.Once
	cleanup := func() {
		once.Do(func() {
			if batcher != nil {
				batcher.Close()
			}
			_ = conn.Close()
		})
	}
	success := false
	defer func() {
		if !success {
			cleanup()
		}
	}()

	grpcClient, err := ml.NewGRPCClient(mlv1.NewPIIDetectorClient(conn))
	if err != nil {
		return nil, nil, fmt.Errorf("create ml grpc client: %w", err)
	}
	batcher, err = ml.NewBatcher(grpcClient, ml.DefaultBatchConfig())
	if err != nil {
		return nil, nil, fmt.Errorf("create ml batcher: %w", err)
	}
	extractor, err := ml.NewExtractor(batcher)
	if err != nil {
		return nil, nil, fmt.Errorf("create ml extractor: %w", err)
	}

	eng := engine.New(engine.Options{})
	g := gate.New(gate.DefaultConfig())
	casc := cascade.New(eng, g, nil, extractor)
	cascadeMasker, err := masking.NewCascadeMasker(casc, ml.DefaultChunkConfig(), 8)
	if err != nil {
		return nil, nil, fmt.Errorf("create cascade masker: %w", err)
	}

	st := store.NewMemoryStore(100000)
	pp := policy.NewStaticProvider(buildPolicies(cfg))
	proc, err := processor.New(st, pp, cascadeMasker, 15*time.Minute)
	if err != nil {
		return nil, nil, fmt.Errorf("create processor: %w", err)
	}

	success = true
	return proc, cleanup, nil
}

// buildPolicies builds a policy for the verify consumer and every enabled
// system, allowing all PII kinds with detokenization enabled.
func buildPolicies(cfg config.Config) map[string]pii.Policy {
	allowed := allKinds()
	pol := pii.Policy{
		AllowedKinds:          allowed,
		DetokenizationAllowed: true,
		MinConfidence:         0.5,
	}
	policies := map[string]pii.Policy{
		auth.VerifyConsumerID: pol,
	}
	for _, s := range cfg.Systems {
		if s.Enabled {
			policies[s.ID] = pol
		}
	}
	return policies
}

func allKinds() map[pii.PIIKind]bool {
	return map[pii.PIIKind]bool{
		pii.PIIKindFullName:         true,
		pii.PIIKindFirstName:        true,
		pii.PIIKindLastName:         true,
		pii.PIIKindMiddleName:       true,
		pii.PIIKindAddress:          true,
		pii.PIIKindCity:             true,
		pii.PIIKindStreet:           true,
		pii.PIIKindHouse:            true,
		pii.PIIKindApartment:        true,
		pii.PIIKindBirthPlace:       true,
		pii.PIIKindCitizenship:      true,
		pii.PIIKindPassportIssuer:   true,
		pii.PIIKindCardHolderName:   true,
		pii.PIIKindEmail:            true,
		pii.PIIKindPhone:            true,
		pii.PIIKindINN:              true,
		pii.PIIKindBankCard:         true,
		pii.PIIKindPassport:         true,
		pii.PIIKindPassportDivision: true,
		pii.PIIKindDate:             true,
		pii.PIIKindDriverLicense:    true,
		pii.PIIKindCVV:              true,
		pii.PIIKindPIN:              true,
		pii.PIIKindPostalCode:       true,
	}
}
