package ml

import (
	"context"
	"net"
	"testing"

	mlv1 "github.com/kryneuse/alpha_proxy/gen/ml/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

const bufSize = 1024 * 1024

type fakeServer struct {
	mlv1.UnimplementedPIIDetectorServer
	req  *mlv1.DetectBatchRequest
	resp *mlv1.DetectBatchResponse
	err  error
}

func (f *fakeServer) DetectBatch(_ context.Context, in *mlv1.DetectBatchRequest) (*mlv1.DetectBatchResponse, error) {
	f.req = in
	return f.resp, f.err
}

func startTestServer(t *testing.T, srv *fakeServer) (mlv1.PIIDetectorClient, func()) {
	t.Helper()
	lis := bufconn.Listen(bufSize)
	gs := grpc.NewServer()
	mlv1.RegisterPIIDetectorServer(gs, srv)

	go func() {
		_ = gs.Serve(lis)
	}()

	ctx := context.Background()
	conn, err := grpc.DialContext(ctx, "bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("grpc.DialContext returned error: %v", err)
	}

	cleanup := func() {
		_ = conn.Close()
		gs.Stop()
		_ = lis.Close()
	}
	return mlv1.NewPIIDetectorClient(conn), cleanup
}

func TestGRPCTransportSuccess(t *testing.T) {
	srv := &fakeServer{resp: &mlv1.DetectBatchResponse{
		BatchId:      "b1",
		ModelVersion: "v1",
		OffsetUnit:   mlv1.OffsetUnit_OFFSET_UNIT_UNICODE_CODE_POINTS,
		Results: []*mlv1.ChunkResult{
			{
				ChunkId:   "c1",
				ErrorCode: mlv1.ChunkErrorCode_CHUNK_ERROR_CODE_NONE,
				Entities: []*mlv1.Entity{
					{Type: mlv1.EntityType_ENTITY_TYPE_PHONE, Start: 6, End: 17, Confidence: 0.9},
				},
			},
		},
	}}
	client, cleanup := startTestServer(t, srv)
	t.Cleanup(cleanup)

	c, err := NewGRPCClient(client)
	if err != nil {
		t.Fatalf("NewGRPCClient returned error: %v", err)
	}

	resp, err := c.ProcessBatch(context.Background(), BatchRequest{
		BatchID:    "b1",
		OffsetUnit: OffsetsUnicodeCodePoints,
		Items:      []RequestItem{{ChunkID: "c1", Text: "звони 79123456789"}},
	})
	if err != nil {
		t.Fatalf("ProcessBatch returned error: %v", err)
	}

	// Сервер должен получить правильные поля запроса.
	if srv.req.BatchId != "b1" {
		t.Fatalf("unexpected batch id: %q", srv.req.BatchId)
	}
	if srv.req.OffsetUnit != mlv1.OffsetUnit_OFFSET_UNIT_UNICODE_CODE_POINTS {
		t.Fatalf("unexpected offset unit: %v", srv.req.OffsetUnit)
	}
	if len(srv.req.Chunks) != 1 || srv.req.Chunks[0].ChunkId != "c1" || srv.req.Chunks[0].Text != "звони 79123456789" {
		t.Fatalf("unexpected chunks: %+v", srv.req.Chunks)
	}

	// Клиент должен вернуть корректный BatchResponse.
	if resp.BatchID != "b1" || resp.ModelVersion != "v1" {
		t.Fatalf("unexpected response header: %+v", resp)
	}
	if len(resp.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(resp.Results))
	}
	r := resp.Results[0]
	if r.ChunkID != "c1" || len(r.Entities) != 1 {
		t.Fatalf("unexpected result: %+v", r)
	}
	e := r.Entities[0]
	if e.Type != "phone" || e.Start != 6 || e.End != 17 || float32(e.Confidence) != 0.9 {
		t.Fatalf("unexpected entity: %+v", e)
	}
}

func TestGRPCTransportResourceExhausted(t *testing.T) {
	srv := &fakeServer{err: status.Error(codes.ResourceExhausted, "queue full")}
	client, cleanup := startTestServer(t, srv)
	t.Cleanup(cleanup)

	c, err := NewGRPCClient(client)
	if err != nil {
		t.Fatalf("NewGRPCClient returned error: %v", err)
	}

	_, err = c.ProcessBatch(context.Background(), BatchRequest{
		BatchID:    "b1",
		OffsetUnit: OffsetsUnicodeCodePoints,
		Items:      []RequestItem{{ChunkID: "c1", Text: "звони 79123456789"}},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("expected ResourceExhausted, got %v", status.Code(err))
	}
}
