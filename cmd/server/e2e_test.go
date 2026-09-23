package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	mlv1 "github.com/kryneuse/alpha_proxy/gen/ml/v1"
	"github.com/kryneuse/alpha_proxy/internal/app"
	"github.com/kryneuse/alpha_proxy/internal/config"
	"github.com/kryneuse/alpha_proxy/internal/observability"
	"google.golang.org/grpc"
)

// fakeMLServer — простой fake ML gRPC сервер без Python моделей.
type fakeMLServer struct {
	mlv1.UnimplementedPIIDetectorServer
	mu          sync.Mutex
	calls       int
	batchIDs    []string
	chunkIDs    []string
	offsetUnits []mlv1.OffsetUnit
}

func (f *fakeMLServer) DetectBatch(_ context.Context, req *mlv1.DetectBatchRequest) (*mlv1.DetectBatchResponse, error) {
	f.mu.Lock()
	f.calls++
	f.batchIDs = append(f.batchIDs, req.BatchId)
	f.offsetUnits = append(f.offsetUnits, req.OffsetUnit)
	for _, c := range req.Chunks {
		f.chunkIDs = append(f.chunkIDs, c.ChunkId)
	}
	f.mu.Unlock()

	results := make([]*mlv1.ChunkResult, 0, len(req.Chunks))
	for _, c := range req.Chunks {
		cr := &mlv1.ChunkResult{ChunkId: c.ChunkId, ErrorCode: mlv1.ChunkErrorCode_CHUNK_ERROR_CODE_NONE}
		if idx := strings.Index(c.Text, "СЕКРЕТ"); idx >= 0 {
			start := utf8.RuneCountInString(c.Text[:idx])
			end := start + utf8.RuneCountInString("СЕКРЕТ")
			cr.Entities = []*mlv1.Entity{
				{Type: mlv1.EntityType_ENTITY_TYPE_FULL_NAME, Start: int32(start), End: int32(end), Confidence: 0.9},
			}
		}
		results = append(results, cr)
	}
	return &mlv1.DetectBatchResponse{
		BatchId:      req.BatchId,
		ModelVersion: "test-v1",
		OffsetUnit:   mlv1.OffsetUnit_OFFSET_UNIT_UNICODE_CODE_POINTS,
		Results:      results,
	}, nil
}

func (f *fakeMLServer) DetectBatchV2(_ context.Context, req *mlv1.DetectBatchV2Request) (*mlv1.DetectBatchResponse, error) {
	f.mu.Lock()
	f.calls++
	f.batchIDs = append(f.batchIDs, req.BatchId)
	f.offsetUnits = append(f.offsetUnits, req.OffsetUnit)
	for _, c := range req.Chunks {
		f.chunkIDs = append(f.chunkIDs, c.ChunkId)
	}
	f.mu.Unlock()

	results := make([]*mlv1.ChunkResult, 0, len(req.Chunks))
	for _, c := range req.Chunks {
		cr := &mlv1.ChunkResult{ChunkId: c.ChunkId, ErrorCode: mlv1.ChunkErrorCode_CHUNK_ERROR_CODE_NONE}
		if idx := strings.Index(c.OriginalText, "СЕКРЕТ"); idx >= 0 {
			start := utf8.RuneCountInString(c.OriginalText[:idx])
			end := start + utf8.RuneCountInString("СЕКРЕТ")
			cr.Entities = []*mlv1.Entity{
				{Type: mlv1.EntityType_ENTITY_TYPE_FULL_NAME, Start: int32(start), End: int32(end), Confidence: 0.9},
			}
		}
		results = append(results, cr)
	}
	return &mlv1.DetectBatchResponse{
		BatchId:      req.BatchId,
		ModelVersion: "test-v1",
		OffsetUnit:   mlv1.OffsetUnit_OFFSET_UNIT_UNICODE_CODE_POINTS,
		Results:      results,
	}, nil
}

func (f *fakeMLServer) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func (f *fakeMLServer) snapshot() (batchIDs []string, chunkIDs []string, offsetUnits []mlv1.OffsetUnit) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.batchIDs...), append([]string(nil), f.chunkIDs...), append([]mlv1.OffsetUnit(nil), f.offsetUnits...)
}

type e2eResponse struct {
	Result string `json:"result"`
}

// startE2E поднимает fake ML gRPC сервер, настоящий Processor и HTTP handler.
func startE2E(t *testing.T) (*fakeMLServer, string) {
	t.Helper()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	gs := grpc.NewServer()
	fake := &fakeMLServer{}
	mlv1.RegisterPIIDetectorServer(gs, fake)
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(func() {
		gs.Stop()
		_ = lis.Close()
	})

	cfg := config.Config{
		Addr:               "127.0.0.1:0",
		ReadTimeout:        10 * time.Second,
		ReadHeaderTimeout:  5 * time.Second,
		WriteTimeout:       10 * time.Second,
		IdleTimeout:        60 * time.Second,
		BodyLimit:          1 << 20,
		ProcessingTimeout:  5 * time.Second,
		ParallelLimit:      16,
		OverloadRetryAfter: time.Second,
		ShutdownTimeout:    15 * time.Second,
		RunMode:            config.RunModeVerify,
		AuthMode:           config.AuthModeVerify,
		ProcessorMode:      config.ProcessorReal,
		MLAddress:          lis.Addr().String(),
		MetricsEnabled:     false,
	}

	proc, cleanup, err := buildProcessor(cfg)
	if err != nil {
		t.Fatalf("buildProcessor: %v", err)
	}
	t.Cleanup(cleanup)

	logger := observability.NewLogger()
	runtime, err := app.NewRuntime(cfg, logger, proc)
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}

	srv := httptest.NewServer(runtime.Handler)
	t.Cleanup(srv.Close)

	return fake, srv.URL
}

func postProcess(t *testing.T, url, payload, payloadID string) (int, string) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"payload": payload, "payload_id": payloadID})
	req, err := http.NewRequest(http.MethodPost, url+"/process", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()

	var out e2eResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return resp.StatusCode, out.Result
}

func extractToken(result string) string {
	start := strings.Index(result, "<")
	end := strings.Index(result, ">")
	if start < 0 || end < 0 || end <= start {
		return ""
	}
	return result[start : end+1]
}

func TestE2ERuleMaskRetryAndDetokenize(t *testing.T) {
	_, url := startE2E(t)

	payload := "call 79123456789"
	payloadID := "id-rule"

	status, result := postProcess(t, url, payload, payloadID)
	if status != http.StatusOK {
		t.Fatalf("first status = %d, want 200", status)
	}
	if strings.Contains(result, "79123456789") {
		t.Fatalf("phone must be masked, got %q", result)
	}
	if !strings.Contains(result, "PHONE") {
		t.Fatalf("expected PHONE token, got %q", result)
	}

	status2, result2 := postProcess(t, url, payload, payloadID)
	if status2 != http.StatusOK {
		t.Fatalf("retry status = %d, want 200", status2)
	}
	if result2 != result {
		t.Fatalf("retry result differs: %q vs %q", result2, result)
	}

	token := extractToken(result)
	if token == "" {
		t.Fatalf("could not extract token from %q", result)
	}

	status3, result3 := postProcess(t, url, "Ответ "+token, payloadID)
	if status3 != http.StatusOK {
		t.Fatalf("detokenize status = %d, want 200", status3)
	}
	if !strings.Contains(result3, "79123456789") {
		t.Fatalf("expected phone restored, got %q", result3)
	}
}

func TestE2EMLMasking(t *testing.T) {
	fake, url := startE2E(t)

	payload := "паспорт СЕКРЕТ"
	payloadID := "id-ml"

	status, result := postProcess(t, url, payload, payloadID)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if strings.Contains(result, "СЕКРЕТ") {
		t.Fatalf("СЕКРЕТ must be masked, got %q", result)
	}
	if !strings.Contains(result, "FULL_NAME") {
		t.Fatalf("expected FULL_NAME token, got %q", result)
	}

	if fake.callCount() < 1 {
		t.Fatal("expected at least one DetectBatch call")
	}
	batchIDs, chunkIDs, offsetUnits := fake.snapshot()
	if len(batchIDs) == 0 || batchIDs[0] == "" {
		t.Fatal("expected non-empty batch_id")
	}
	if len(offsetUnits) == 0 || offsetUnits[0] != mlv1.OffsetUnit_OFFSET_UNIT_UNICODE_CODE_POINTS {
		t.Fatalf("expected unicode_code_points offset unit, got %v", offsetUnits)
	}
	if len(chunkIDs) == 0 {
		t.Fatal("expected at least one chunk_id")
	}
	seen := make(map[string]bool)
	for _, id := range chunkIDs {
		if id == "" {
			t.Fatal("expected non-empty chunk_id")
		}
		if seen[id] {
			t.Fatalf("duplicate chunk_id %q", id)
		}
		seen[id] = true
	}
}
