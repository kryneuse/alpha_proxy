package contract

import (
	"context"
	"testing"
)

func TestMockProcessorReturnsPayloadUnchanged(t *testing.T) {
	p := MockProcessor{}
	resp, err := p.Process(context.Background(), ProcessRequest{
		Payload:    "hello",
		PayloadID:  "id-1",
		ConsumerID: "consumer-1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Result != "hello" {
		t.Fatalf("expected result %q, got %q", "hello", resp.Result)
	}
}