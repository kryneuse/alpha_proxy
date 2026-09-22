// Package api implements the HTTP handlers of the contour. Handlers decode the
// request, forward it to a Processor, and encode the response. They never decide
// the processing phase based on a call counter.
package api

import (
	"encoding/json"
	"net/http"

	"alpha_proxy/internal/auth"
	"alpha_proxy/internal/contract"
)

// Handler serves the process endpoint.
type Handler struct {
	processor contract.Processor
}

// NewHandler returns a Handler backed by the given Processor.
func NewHandler(p contract.Processor) *Handler {
	return &Handler{processor: p}
}

type processRequest struct {
	Payload   string `json:"payload"`
	PayloadID string `json:"payload_id"`
}

type processResponse struct {
	Result string `json:"result"`
}

// Process handles POST /process.
func (h *Handler) Process(w http.ResponseWriter, r *http.Request) {
	var req processRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	resp, err := h.processor.Process(r.Context(), contract.ProcessRequest{
		Payload:    req.Payload,
		PayloadID:  req.PayloadID,
		ConsumerID: auth.ConsumerID(r.Context()),
	})
	if err != nil {
		http.Error(w, "processing failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(processResponse{Result: resp.Result})
}

// Routes registers the contour endpoints on mux.
func (h *Handler) Routes(mux *http.ServeMux) {
	mux.HandleFunc("POST /process", h.Process)
}