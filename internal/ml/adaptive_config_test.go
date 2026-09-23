package ml

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestBatchConfigEnvironment(t *testing.T) {
	t.Setenv("ALPHA_PROXY_ML_RPC_WORKERS", "12")
	t.Setenv("ALPHA_PROXY_ML_BATCH_WAIT", "1ms")
	t.Setenv("ALPHA_PROXY_ML_RPC_TIMEOUT", "500ms")
	c, err := BatchConfigFromEnv()
	if err != nil || c.Workers != 12 || c.MaxWait != time.Millisecond || c.RPCTimeout != 500*time.Millisecond {
		t.Fatalf("unexpected config: %+v %v", c, err)
	}
	t.Setenv("ALPHA_PROXY_ML_RPC_WORKERS", "0")
	if _, err := BatchConfigFromEnv(); err == nil {
		t.Fatal("zero workers accepted")
	}
}

type deadlineClient struct{}

func (deadlineClient) ProcessBatch(ctx context.Context, req BatchRequest) (BatchResponse, error) {
	return deadlineClient{}.ProcessBatchV2(ctx, req)
}
func (deadlineClient) ProcessBatchV2(ctx context.Context, _ BatchRequest) (BatchResponse, error) {
	<-ctx.Done()
	return BatchResponse{}, ctx.Err()
}

func TestBatchRPCDeadlineDoesNotRequireCallerDeadline(t *testing.T) {
	c := DefaultBatchConfig()
	c.MaxWait = time.Millisecond
	c.RPCTimeout = 20 * time.Millisecond
	b, err := NewBatcher(deadlineClient{}, c)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err = b.Process(ctx, RequestItem{ChunkID: "c", Text: "text", GateText: "text"})
	if !errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
		t.Fatalf("shared RPC deadline not applied: %v", err)
	}
}

func TestConnectionRejectsEmptyReplica(t *testing.T) {
	if _, err := NewConnection("127.0.0.1:50051,"); err == nil {
		t.Fatal("empty replica accepted")
	}
}
