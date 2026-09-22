package contract

import "context"

// MockProcessor is a TEMPORARY STUB used only for wiring and tests.
//
// It returns the Payload unchanged and performs NO masking, tokenization or
// state tracking. It MUST NOT be presented as working data protection, and it
// MUST NOT be silently used in the final verification mode. Replace it with a
// real Processor before any production or acceptance run.
type MockProcessor struct{}

// Process returns the input Payload unchanged.
func (MockProcessor) Process(_ context.Context, req ProcessRequest) (ProcessResponse, error) {
	return ProcessResponse{Result: req.Payload}, nil
}