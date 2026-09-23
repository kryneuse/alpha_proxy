package ml

import (
	"context"
	"errors"
	"math"
	"testing"

	mlv1 "github.com/kryneuse/alpha_proxy/gen/ml/v1"
	"github.com/kryneuse/alpha_proxy/internal/pii"
	"google.golang.org/grpc"
)

type fakePIIDetectorClient struct {
	req  *mlv1.DetectBatchRequest
	resp *mlv1.DetectBatchResponse
	err  error
}

func (f *fakePIIDetectorClient) DetectBatch(_ context.Context, in *mlv1.DetectBatchRequest, _ ...grpc.CallOption) (*mlv1.DetectBatchResponse, error) {
	f.req = in
	return f.resp, f.err
}

func TestGRPCClientRequestConversion(t *testing.T) {
	fake := &fakePIIDetectorClient{resp: &mlv1.DetectBatchResponse{
		BatchId:      "b1",
		ModelVersion: "v1",
		OffsetUnit:   mlv1.OffsetUnit_OFFSET_UNIT_UNICODE_CODE_POINTS,
		Results: []*mlv1.ChunkResult{
			{ChunkId: "c1", ErrorCode: mlv1.ChunkErrorCode_CHUNK_ERROR_CODE_NONE},
			{ChunkId: "c2", ErrorCode: mlv1.ChunkErrorCode_CHUNK_ERROR_CODE_NONE},
		},
	}}
	c, err := NewGRPCClient(fake)
	if err != nil {
		t.Fatalf("NewGRPCClient returned error: %v", err)
	}

	_, err = c.ProcessBatch(context.Background(), BatchRequest{
		BatchID:    "b1",
		OffsetUnit: OffsetsUnicodeCodePoints,
		Items: []RequestItem{
			{ChunkID: "c1", Text: "hello"},
			{ChunkID: "c2", Text: "world"},
		},
	})
	if err != nil {
		t.Fatalf("ProcessBatch returned error: %v", err)
	}

	if fake.req.BatchId != "b1" {
		t.Fatalf("unexpected batch id: %q", fake.req.BatchId)
	}
	if fake.req.OffsetUnit != mlv1.OffsetUnit_OFFSET_UNIT_UNICODE_CODE_POINTS {
		t.Fatalf("unexpected offset unit: %v", fake.req.OffsetUnit)
	}
	if len(fake.req.Chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(fake.req.Chunks))
	}
	if fake.req.Chunks[0].ChunkId != "c1" || fake.req.Chunks[0].Text != "hello" {
		t.Fatalf("unexpected chunk 0: %+v", fake.req.Chunks[0])
	}
	if fake.req.Chunks[1].ChunkId != "c2" || fake.req.Chunks[1].Text != "world" {
		t.Fatalf("unexpected chunk 1: %+v", fake.req.Chunks[1])
	}
}

func TestGRPCClientFullResponse(t *testing.T) {
	fake := &fakePIIDetectorClient{resp: &mlv1.DetectBatchResponse{
		BatchId:      "b1",
		ModelVersion: "v1",
		OffsetUnit:   mlv1.OffsetUnit_OFFSET_UNIT_UNICODE_CODE_POINTS,
		Results: []*mlv1.ChunkResult{
			{
				ChunkId:   "c1",
				ErrorCode: mlv1.ChunkErrorCode_CHUNK_ERROR_CODE_NONE,
				Entities: []*mlv1.Entity{
					{Type: mlv1.EntityType_ENTITY_TYPE_PHONE, Start: 0, End: 11, Confidence: 0.9},
					{Type: mlv1.EntityType_ENTITY_TYPE_FULL_NAME, Start: 12, End: 20, Confidence: 0.8},
				},
			},
			{
				ChunkId:   "c2",
				ErrorCode: mlv1.ChunkErrorCode_CHUNK_ERROR_CODE_TOO_LARGE,
			},
		},
	}}
	c, err := NewGRPCClient(fake)
	if err != nil {
		t.Fatalf("NewGRPCClient returned error: %v", err)
	}

	resp, err := c.ProcessBatch(context.Background(), BatchRequest{
		BatchID:    "b1",
		OffsetUnit: OffsetsUnicodeCodePoints,
		Items:      []RequestItem{{ChunkID: "c1", Text: "01234567890123456789"}, {ChunkID: "c2", Text: "y"}},
	})
	if err != nil {
		t.Fatalf("ProcessBatch returned error: %v", err)
	}

	if resp.BatchID != "b1" || resp.ModelVersion != "v1" {
		t.Fatalf("unexpected response header: %+v", resp)
	}
	if resp.OffsetUnit != OffsetsUnicodeCodePoints {
		t.Fatalf("unexpected offset unit: %q", resp.OffsetUnit)
	}
	if len(resp.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(resp.Results))
	}
	r0 := resp.Results[0]
	if r0.ChunkID != "c1" || len(r0.Entities) != 2 {
		t.Fatalf("unexpected result 0: %+v", r0)
	}
	if r0.Entities[0].Type != "phone" || r0.Entities[0].Start != 0 || r0.Entities[0].End != 11 || float32(r0.Entities[0].Confidence) != 0.9 {
		t.Fatalf("unexpected entity 0: %+v", r0.Entities[0])
	}
	if r0.Entities[1].Type != "full_name" {
		t.Fatalf("unexpected entity 1 type: %q", r0.Entities[1].Type)
	}
	r1 := resp.Results[1]
	if r1.ChunkID != "c2" || r1.ErrorCode != "TOO_LARGE" {
		t.Fatalf("unexpected result 1: %+v", r1)
	}
}

func TestGRPCClientRPCError(t *testing.T) {
	fake := &fakePIIDetectorClient{err: errors.New("rpc failed")}
	c, err := NewGRPCClient(fake)
	if err != nil {
		t.Fatalf("NewGRPCClient returned error: %v", err)
	}

	_, err = c.ProcessBatch(context.Background(), BatchRequest{
		BatchID:    "b1",
		OffsetUnit: OffsetsUnicodeCodePoints,
		Items:      []RequestItem{{ChunkID: "c1", Text: "x"}},
	})
	if err == nil || err.Error() != "rpc failed" {
		t.Fatalf("expected rpc error, got %v", err)
	}
}

func TestGRPCClientUnknownOffsetUnit(t *testing.T) {
	fake := &fakePIIDetectorClient{}
	c, err := NewGRPCClient(fake)
	if err != nil {
		t.Fatalf("NewGRPCClient returned error: %v", err)
	}

	_, err = c.ProcessBatch(context.Background(), BatchRequest{
		BatchID:    "b1",
		OffsetUnit: "unknown",
		Items:      []RequestItem{{ChunkID: "c1", Text: "x"}},
	})
	if !errors.Is(err, pii.ErrInvalidMLResponse) {
		t.Fatalf("expected ErrInvalidMLResponse, got %v", err)
	}
}

func TestGRPCClientUnspecifiedValues(t *testing.T) {
	// UNSPECIFIED offset unit в ответе.
	fake := &fakePIIDetectorClient{resp: &mlv1.DetectBatchResponse{
		BatchId:      "b1",
		ModelVersion: "v1",
		OffsetUnit:   mlv1.OffsetUnit_OFFSET_UNIT_UNSPECIFIED,
	}}
	c, err := NewGRPCClient(fake)
	if err != nil {
		t.Fatalf("NewGRPCClient returned error: %v", err)
	}
	_, err = c.ProcessBatch(context.Background(), BatchRequest{
		BatchID:    "b1",
		OffsetUnit: OffsetsUnicodeCodePoints,
		Items:      []RequestItem{{ChunkID: "c1", Text: "x"}},
	})
	if !errors.Is(err, pii.ErrInvalidMLResponse) {
		t.Fatalf("expected ErrInvalidMLResponse for unspecified offset unit, got %v", err)
	}

	// UNSPECIFIED entity type в ответе.
	fake2 := &fakePIIDetectorClient{resp: &mlv1.DetectBatchResponse{
		BatchId:      "b1",
		ModelVersion: "v1",
		OffsetUnit:   mlv1.OffsetUnit_OFFSET_UNIT_UNICODE_CODE_POINTS,
		Results: []*mlv1.ChunkResult{
			{ChunkId: "c1", Entities: []*mlv1.Entity{{Type: mlv1.EntityType_ENTITY_TYPE_UNSPECIFIED, Start: 0, End: 1, Confidence: 0.9}}},
		},
	}}
	c2, err := NewGRPCClient(fake2)
	if err != nil {
		t.Fatalf("NewGRPCClient returned error: %v", err)
	}
	_, err = c2.ProcessBatch(context.Background(), BatchRequest{
		BatchID:    "b1",
		OffsetUnit: OffsetsUnicodeCodePoints,
		Items:      []RequestItem{{ChunkID: "c1", Text: "x"}},
	})
	if !errors.Is(err, pii.ErrInvalidMLResponse) {
		t.Fatalf("expected ErrInvalidMLResponse for unspecified entity type, got %v", err)
	}
}

func TestNewGRPCClientNil(t *testing.T) {
	if _, err := NewGRPCClient(nil); err == nil {
		t.Fatal("expected error for nil client")
	}
}

func validRequest() BatchRequest {
	return BatchRequest{
		BatchID:    "b1",
		OffsetUnit: OffsetsUnicodeCodePoints,
		Items: []RequestItem{
			{ChunkID: "c1", Text: "hello"},
			{ChunkID: "c2", Text: "world"},
		},
	}
}

func validResponse() *mlv1.DetectBatchResponse {
	return &mlv1.DetectBatchResponse{
		BatchId:      "b1",
		ModelVersion: "v1",
		OffsetUnit:   mlv1.OffsetUnit_OFFSET_UNIT_UNICODE_CODE_POINTS,
		Results: []*mlv1.ChunkResult{
			{ChunkId: "c1", ErrorCode: mlv1.ChunkErrorCode_CHUNK_ERROR_CODE_NONE},
			{ChunkId: "c2", ErrorCode: mlv1.ChunkErrorCode_CHUNK_ERROR_CODE_NONE},
		},
	}
}

func runProcess(t *testing.T, resp *mlv1.DetectBatchResponse) error {
	t.Helper()
	fake := &fakePIIDetectorClient{resp: resp}
	c, err := NewGRPCClient(fake)
	if err != nil {
		t.Fatalf("NewGRPCClient returned error: %v", err)
	}
	_, err = c.ProcessBatch(context.Background(), validRequest())
	return err
}

func expectInvalidMLResponse(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, pii.ErrInvalidMLResponse) {
		t.Fatalf("expected ErrInvalidMLResponse, got %v", err)
	}
}

func TestGRPCClientWrongBatchID(t *testing.T) {
	resp := validResponse()
	resp.BatchId = "wrong"
	expectInvalidMLResponse(t, runProcess(t, resp))
}

func TestGRPCClientEmptyModelVersion(t *testing.T) {
	resp := validResponse()
	resp.ModelVersion = ""
	expectInvalidMLResponse(t, runProcess(t, resp))
}

func TestGRPCClientUnknownResponseOffsetUnit(t *testing.T) {
	resp := validResponse()
	resp.OffsetUnit = mlv1.OffsetUnit(99)
	expectInvalidMLResponse(t, runProcess(t, resp))
}

func TestGRPCClientDuplicateChunkID(t *testing.T) {
	resp := validResponse()
	resp.Results = []*mlv1.ChunkResult{
		{ChunkId: "c1", ErrorCode: mlv1.ChunkErrorCode_CHUNK_ERROR_CODE_NONE},
		{ChunkId: "c1", ErrorCode: mlv1.ChunkErrorCode_CHUNK_ERROR_CODE_NONE},
	}
	expectInvalidMLResponse(t, runProcess(t, resp))
}

func TestGRPCClientMissingChunkID(t *testing.T) {
	resp := validResponse()
	resp.Results = []*mlv1.ChunkResult{
		{ChunkId: "c1", ErrorCode: mlv1.ChunkErrorCode_CHUNK_ERROR_CODE_NONE},
	}
	expectInvalidMLResponse(t, runProcess(t, resp))
}

func TestGRPCClientExtraChunkID(t *testing.T) {
	resp := validResponse()
	resp.Results = []*mlv1.ChunkResult{
		{ChunkId: "c1", ErrorCode: mlv1.ChunkErrorCode_CHUNK_ERROR_CODE_NONE},
		{ChunkId: "c2", ErrorCode: mlv1.ChunkErrorCode_CHUNK_ERROR_CODE_NONE},
		{ChunkId: "c3", ErrorCode: mlv1.ChunkErrorCode_CHUNK_ERROR_CODE_NONE},
	}
	expectInvalidMLResponse(t, runProcess(t, resp))
}

func TestGRPCClientNilResult(t *testing.T) {
	resp := validResponse()
	resp.Results = []*mlv1.ChunkResult{
		{ChunkId: "c1", ErrorCode: mlv1.ChunkErrorCode_CHUNK_ERROR_CODE_NONE},
		nil,
	}
	expectInvalidMLResponse(t, runProcess(t, resp))
}

func TestGRPCClientNilEntity(t *testing.T) {
	resp := validResponse()
	resp.Results = []*mlv1.ChunkResult{
		{ChunkId: "c1", ErrorCode: mlv1.ChunkErrorCode_CHUNK_ERROR_CODE_NONE, Entities: []*mlv1.Entity{nil}},
		{ChunkId: "c2", ErrorCode: mlv1.ChunkErrorCode_CHUNK_ERROR_CODE_NONE},
	}
	expectInvalidMLResponse(t, runProcess(t, resp))
}

func TestGRPCClientUnspecifiedErrorCode(t *testing.T) {
	resp := validResponse()
	resp.Results = []*mlv1.ChunkResult{
		{ChunkId: "c1", ErrorCode: mlv1.ChunkErrorCode_CHUNK_ERROR_CODE_UNSPECIFIED},
		{ChunkId: "c2", ErrorCode: mlv1.ChunkErrorCode_CHUNK_ERROR_CODE_NONE},
	}
	expectInvalidMLResponse(t, runProcess(t, resp))
}

func TestGRPCClientEntitiesWithError(t *testing.T) {
	resp := validResponse()
	resp.Results = []*mlv1.ChunkResult{
		{ChunkId: "c1", ErrorCode: mlv1.ChunkErrorCode_CHUNK_ERROR_CODE_TOO_LARGE, Entities: []*mlv1.Entity{
			{Type: mlv1.EntityType_ENTITY_TYPE_PHONE, Start: 0, End: 1, Confidence: 0.9},
		}},
		{ChunkId: "c2", ErrorCode: mlv1.ChunkErrorCode_CHUNK_ERROR_CODE_NONE},
	}
	expectInvalidMLResponse(t, runProcess(t, resp))
}

func TestGRPCClientConfidenceOutOfRange(t *testing.T) {
	resp := validResponse()
	resp.Results = []*mlv1.ChunkResult{
		{ChunkId: "c1", ErrorCode: mlv1.ChunkErrorCode_CHUNK_ERROR_CODE_NONE, Entities: []*mlv1.Entity{
			{Type: mlv1.EntityType_ENTITY_TYPE_PHONE, Start: 0, End: 1, Confidence: 1.5},
		}},
		{ChunkId: "c2", ErrorCode: mlv1.ChunkErrorCode_CHUNK_ERROR_CODE_NONE},
	}
	expectInvalidMLResponse(t, runProcess(t, resp))
}

func TestGRPCClientSpanOutOfText(t *testing.T) {
	resp := validResponse()
	// chunk "hello" имеет 5 unicode символов; span [0:6] выходит за границы.
	resp.Results = []*mlv1.ChunkResult{
		{ChunkId: "c1", ErrorCode: mlv1.ChunkErrorCode_CHUNK_ERROR_CODE_NONE, Entities: []*mlv1.Entity{
			{Type: mlv1.EntityType_ENTITY_TYPE_PHONE, Start: 0, End: 6, Confidence: 0.9},
		}},
		{ChunkId: "c2", ErrorCode: mlv1.ChunkErrorCode_CHUNK_ERROR_CODE_NONE},
	}
	expectInvalidMLResponse(t, runProcess(t, resp))
}

func TestGRPCClientNaNConfidence(t *testing.T) {
	resp := validResponse()
	resp.Results = []*mlv1.ChunkResult{
		{ChunkId: "c1", ErrorCode: mlv1.ChunkErrorCode_CHUNK_ERROR_CODE_NONE, Entities: []*mlv1.Entity{
			{Type: mlv1.EntityType_ENTITY_TYPE_PHONE, Start: 0, End: 1, Confidence: float32(math.NaN())},
		}},
		{ChunkId: "c2", ErrorCode: mlv1.ChunkErrorCode_CHUNK_ERROR_CODE_NONE},
	}
	expectInvalidMLResponse(t, runProcess(t, resp))
}

func TestGRPCClientUnknownEntityType(t *testing.T) {
	resp := validResponse()
	resp.Results = []*mlv1.ChunkResult{
		{ChunkId: "c1", ErrorCode: mlv1.ChunkErrorCode_CHUNK_ERROR_CODE_NONE, Entities: []*mlv1.Entity{
			{Type: mlv1.EntityType(99), Start: 0, End: 1, Confidence: 0.9},
		}},
		{ChunkId: "c2", ErrorCode: mlv1.ChunkErrorCode_CHUNK_ERROR_CODE_NONE},
	}
	expectInvalidMLResponse(t, runProcess(t, resp))
}

func TestGRPCClientUnknownErrorCode(t *testing.T) {
	resp := validResponse()
	resp.Results = []*mlv1.ChunkResult{
		{ChunkId: "c1", ErrorCode: mlv1.ChunkErrorCode(99)},
		{ChunkId: "c2", ErrorCode: mlv1.ChunkErrorCode_CHUNK_ERROR_CODE_NONE},
	}
	expectInvalidMLResponse(t, runProcess(t, resp))
}
