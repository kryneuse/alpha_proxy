package contract

import "context"

type ProcessRequest struct {
	Payload    string
	PayloadID  string
	ConsumerID string
}

type ProcessResponse struct {
	Result string
}

type Processor interface {
	Process(ctx context.Context, req ProcessRequest) (ProcessResponse, error)
}
