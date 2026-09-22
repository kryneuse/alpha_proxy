# Boundary of responsibility: HTTP contour vs Processor

## Scope of this document

This document fixes the boundary between the external HTTP contour and the
processing layer. It is intentionally short and normative.

## The HTTP layer

The HTTP layer (`cmd/server`, `internal/api`, `internal/auth`,
`internal/middleware`, `internal/ratelimit`, `internal/observability`,
`internal/app`) is responsible for:

- accepting and decoding HTTP requests;
- authenticating the caller and extracting the consumer ID;
- applying transport-level concerns (logging, recovery, rate limiting);
- forwarding a `contract.ProcessRequest` to a `contract.Processor`;
- encoding and returning the `contract.ProcessResponse`.

The HTTP layer **does not**:

- decide the processing phase based on a call counter;
- track state per `payload_id`;
- implement masking, tokenization, or any data-protection logic.

## The Processor

The `contract.Processor` is responsible for:

- all state associated with a `payload_id`;
- handling repeated requests for the same `payload_id`;
- handling concurrent access to the same `payload_id`;
- producing the processed result.

The HTTP layer treats the Processor as an opaque boundary: it passes a request
in and receives a response out, with no knowledge of how the result is derived.

## Contract

```go
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
```

## Temporary stub

`contract.MockProcessor` is a temporary stub that returns the payload unchanged.
It performs no masking and no state tracking. It must not be presented as
working data protection and must not be silently used in the final verification
mode.