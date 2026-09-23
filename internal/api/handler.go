// Package api implements the HTTP handlers of the contour. Handlers decode the
// request, forward it to a Processor, and encode the response. They never decide
// the processing phase based on a call counter, never store state per payload_id,
// never count calls, and never re-invoke the Processor.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"

	"github.com/kryneuse/alpha_proxy/internal/auth"
	"github.com/kryneuse/alpha_proxy/internal/contract"
	"github.com/kryneuse/alpha_proxy/internal/pii"
	"github.com/kryneuse/alpha_proxy/internal/requestmeta"
)

// ProcessRoute is the registered route pattern for the process endpoint.
const ProcessRoute = "POST /process"

// Handler serves the process endpoint.
type Handler struct {
	processor contract.Processor
}

// NewHandler returns a Handler backed by the given Processor.
func NewHandler(p contract.Processor) *Handler {
	return &Handler{processor: p}
}

// processRequest is the HTTP-layer DTO. Pointers distinguish a present field
// from an absent one. It is intentionally separate from contract.ProcessRequest.
type processRequest struct {
	Payload   *string   `json:"payload"`
	PayloadID *string   `json:"payload_id"`
	MaskKinds *[]string `json:"mask_kinds,omitempty"`
}

type processResponse struct {
	Result string `json:"result"`
}

// Process handles POST /process.
func (h *Handler) Process(w http.ResponseWriter, r *http.Request) {
	if !isJSONContentType(r.Header.Get("Content-Type")) {
		http.Error(w, "unsupported media type", http.StatusBadRequest)
		return
	}

	req, ok := decodeRequest(w, r)
	if !ok {
		return
	}

	resp, err := h.processor.Process(r.Context(), contract.ProcessRequest{
		Payload:      *req.Payload,
		PayloadID:    *req.PayloadID,
		ConsumerID:   auth.ConsumerID(r.Context()),
		MaskKinds:    maskKindsValue(req.MaskKinds),
		MaskKindsSet: req.MaskKinds != nil,
	})
	if err != nil {
		switch {
		case errors.Is(err, contract.ErrUnavailable):
			setErrorClass(r, "processor_unavailable")
			http.Error(w, "processor unavailable", http.StatusServiceUnavailable)
		case errors.Is(err, context.DeadlineExceeded):
			setErrorClass(r, "timeout")
			http.Error(w, "request timed out", http.StatusServiceUnavailable)
		case errors.Is(err, pii.ErrInvalidMaskKind):
			setErrorClass(r, "invalid_mask_kind")
			http.Error(w, "invalid mask kind", http.StatusBadRequest)
		case errors.Is(err, pii.ErrPolicyRejected):
			setErrorClass(r, "policy_rejected")
			http.Error(w, "policy rejected", http.StatusForbidden)
		case errors.Is(err, pii.ErrDemaskingDisabled):
			setErrorClass(r, "demasking_disabled")
			http.Error(w, "detokenization disabled", http.StatusForbidden)
		default:
			http.Error(w, "internal error", http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(processResponse{Result: resp.Result})
}

// maskKindsValue returns the slice contents when the field is present, or nil
// when it is absent.
func maskKindsValue(kinds *[]string) []string {
	if kinds == nil {
		return nil
	}
	return *kinds
}

// setErrorClass records a safe error classification in the logging metadata.
func setErrorClass(r *http.Request, class string) {
	if meta := requestmeta.From(r.Context()); meta != nil {
		meta.ErrorClass = class
	}
}

// isJSONContentType reports whether the Content-Type is application/json,
// allowing MIME parameters such as charset.
func isJSONContentType(ct string) bool {
	mt, _, err := mime.ParseMediaType(ct)
	return err == nil && mt == "application/json"
}

// decodeRequest decodes and validates the request body. It writes the error
// response and returns ok=false on any failure.
func decodeRequest(w http.ResponseWriter, r *http.Request) (processRequest, bool) {
	dec := json.NewDecoder(r.Body)

	var req processRequest
	if err := dec.Decode(&req); err != nil {
		if isTooLarge(err) {
			http.Error(w, "request too large", http.StatusRequestEntityTooLarge)
			return processRequest{}, false
		}
		http.Error(w, "invalid json", http.StatusBadRequest)
		return processRequest{}, false
	}

	// After the first JSON object only whitespace and EOF are allowed. Any
	// body-limit overflow while reading the tail must also be reported as 413.
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		if isTooLarge(err) {
			http.Error(w, "request too large", http.StatusRequestEntityTooLarge)
			return processRequest{}, false
		}
		http.Error(w, "invalid json", http.StatusBadRequest)
		return processRequest{}, false
	}

	if req.Payload == nil {
		http.Error(w, "missing payload", http.StatusBadRequest)
		return processRequest{}, false
	}
	if req.PayloadID == nil {
		http.Error(w, "missing payload_id", http.StatusBadRequest)
		return processRequest{}, false
	}
	if *req.PayloadID == "" {
		http.Error(w, "empty payload_id", http.StatusBadRequest)
		return processRequest{}, false
	}

	return req, true
}

// isTooLarge reports whether err is an *http.MaxBytesError.
func isTooLarge(err error) bool {
	var maxErr *http.MaxBytesError
	return errors.As(err, &maxErr)
}

// Routes registers the contour endpoints on mux.
func (h *Handler) Routes(mux *http.ServeMux) {
	mux.Handle(ProcessRoute, http.HandlerFunc(h.Process))
}
