package ml

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	mlv1 "github.com/kryneuse/alpha_proxy/gen/ml/v1"
	"github.com/kryneuse/alpha_proxy/internal/pii"
)

// grpcClient адаптирует сгенерированный gRPC клиент к internal/ml.Client.
type grpcClient struct {
	client mlv1.PIIDetectorClient
}

// NewGRPCClient создаёт grpcClient.
func NewGRPCClient(client mlv1.PIIDetectorClient) (*grpcClient, error) {
	if client == nil {
		return nil, errors.New("grpc client must not be nil")
	}
	return &grpcClient{client: client}, nil
}

var _ Client = (*grpcClient)(nil)

// ProcessBatch преобразует BatchRequest в mlv1.DetectBatchRequest, вызывает
// DetectBatch, проверяет ответ и преобразует его обратно в BatchResponse.
func (g *grpcClient) ProcessBatch(ctx context.Context, req BatchRequest) (BatchResponse, error) {
	offsetUnit, err := toOffsetUnit(req.OffsetUnit)
	if err != nil {
		return BatchResponse{}, err
	}

	grpcReq := &mlv1.DetectBatchRequest{
		BatchId:    req.BatchID,
		OffsetUnit: offsetUnit,
		Chunks:     make([]*mlv1.Chunk, 0, len(req.Items)),
	}
	for _, item := range req.Items {
		grpcReq.Chunks = append(grpcReq.Chunks, &mlv1.Chunk{
			ChunkId: item.ChunkID,
			Text:    item.Text,
		})
	}

	grpcResp, err := g.client.DetectBatch(ctx, grpcReq)
	if err != nil {
		return BatchResponse{}, err
	}

	return fromGRPCResponse(req, grpcResp)
}

func toOffsetUnit(unit string) (mlv1.OffsetUnit, error) {
	switch unit {
	case OffsetsUnicodeCodePoints:
		return mlv1.OffsetUnit_OFFSET_UNIT_UNICODE_CODE_POINTS, nil
	default:
		return mlv1.OffsetUnit_OFFSET_UNIT_UNSPECIFIED, fmt.Errorf("unknown offset unit: %w", pii.ErrInvalidMLResponse)
	}
}

func fromGRPCResponse(req BatchRequest, resp *mlv1.DetectBatchResponse) (BatchResponse, error) {
	if resp == nil {
		return BatchResponse{}, fmt.Errorf("nil response: %w", pii.ErrInvalidMLResponse)
	}
	if resp.BatchId != req.BatchID {
		return BatchResponse{}, fmt.Errorf("batch id mismatch: %w", pii.ErrInvalidMLResponse)
	}
	if resp.ModelVersion == "" {
		return BatchResponse{}, fmt.Errorf("empty model version: %w", pii.ErrInvalidMLResponse)
	}
	if resp.OffsetUnit != mlv1.OffsetUnit_OFFSET_UNIT_UNICODE_CODE_POINTS {
		return BatchResponse{}, fmt.Errorf("invalid offset unit: %w", pii.ErrInvalidMLResponse)
	}
	if len(resp.Results) != len(req.Items) {
		return BatchResponse{}, fmt.Errorf("result count mismatch: %w", pii.ErrInvalidMLResponse)
	}

	// Проверяем уникальность chunk_id в ответе и сопоставляем с запросом.
	seen := make(map[string]bool, len(resp.Results))
	chunkLen := make(map[string]int, len(req.Items))
	for _, item := range req.Items {
		chunkLen[item.ChunkID] = utf8.RuneCountInString(item.Text)
	}

	out := BatchResponse{
		BatchID:      resp.BatchId,
		ModelVersion: resp.ModelVersion,
		OffsetUnit:   OffsetsUnicodeCodePoints,
		Results:      make([]ChunkResult, 0, len(resp.Results)),
	}
	for _, r := range resp.Results {
		if r == nil {
			return BatchResponse{}, fmt.Errorf("nil result: %w", pii.ErrInvalidMLResponse)
		}
		if seen[r.ChunkId] {
			return BatchResponse{}, fmt.Errorf("duplicate chunk id: %w", pii.ErrInvalidMLResponse)
		}
		seen[r.ChunkId] = true
		if _, ok := chunkLen[r.ChunkId]; !ok {
			return BatchResponse{}, fmt.Errorf("unknown chunk id: %w", pii.ErrInvalidMLResponse)
		}
		if r.ErrorCode == mlv1.ChunkErrorCode_CHUNK_ERROR_CODE_UNSPECIFIED {
			return BatchResponse{}, fmt.Errorf("unspecified error code: %w", pii.ErrInvalidMLResponse)
		}
		if _, ok := mlv1.ChunkErrorCode_name[int32(r.ErrorCode)]; !ok {
			return BatchResponse{}, fmt.Errorf("unknown error code: %w", pii.ErrInvalidMLResponse)
		}

		cr := ChunkResult{
			ChunkID:  r.ChunkId,
			Entities: make([]MLEntity, 0, len(r.Entities)),
		}
		if r.ErrorCode != mlv1.ChunkErrorCode_CHUNK_ERROR_CODE_NONE {
			cr.ErrorCode = strings.TrimPrefix(r.ErrorCode.String(), "CHUNK_ERROR_CODE_")
			if len(r.Entities) != 0 {
				return BatchResponse{}, fmt.Errorf("entities present with error: %w", pii.ErrInvalidMLResponse)
			}
			out.Results = append(out.Results, cr)
			continue
		}

		textLen := chunkLen[r.ChunkId]
		for _, e := range r.Entities {
			if e == nil {
				return BatchResponse{}, fmt.Errorf("nil entity: %w", pii.ErrInvalidMLResponse)
			}
			if e.Type == mlv1.EntityType_ENTITY_TYPE_UNSPECIFIED {
				return BatchResponse{}, fmt.Errorf("unspecified entity type: %w", pii.ErrInvalidMLResponse)
			}
			if _, ok := mlv1.EntityType_name[int32(e.Type)]; !ok {
				return BatchResponse{}, fmt.Errorf("unknown entity type: %w", pii.ErrInvalidMLResponse)
			}
			if e.Confidence != e.Confidence {
				return BatchResponse{}, fmt.Errorf("entity confidence is NaN: %w", pii.ErrInvalidMLResponse)
			}
			if e.Confidence < 0 || e.Confidence > 1 {
				return BatchResponse{}, fmt.Errorf("entity confidence out of range: %w", pii.ErrInvalidMLResponse)
			}
			if e.Start < 0 || e.End <= e.Start || int64(e.End) > int64(textLen) {
				return BatchResponse{}, fmt.Errorf("entity span out of range: %w", pii.ErrInvalidMLResponse)
			}
			cr.Entities = append(cr.Entities, MLEntity{
				Type:       strings.ToLower(strings.TrimPrefix(e.Type.String(), "ENTITY_TYPE_")),
				Start:      int(e.Start),
				End:        int(e.End),
				Confidence: float64(e.Confidence),
			})
		}
		out.Results = append(out.Results, cr)
	}

	// Проверяем, что каждый chunk запроса имеет ровно один результат.
	for _, item := range req.Items {
		if !seen[item.ChunkID] {
			return BatchResponse{}, fmt.Errorf("missing chunk id: %w", pii.ErrInvalidMLResponse)
		}
	}

	return out, nil
}
