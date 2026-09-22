package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"alpha_proxy/internal/config"
	"alpha_proxy/internal/contract"
)

// resultProcessor returns a fixed successful result.
type resultProcessor struct {
	result string
}

func (p *resultProcessor) Process(context.Context, contract.ProcessRequest) (contract.ProcessResponse, error) {
	return contract.ProcessResponse{Result: p.result}, nil
}

func TestMetricsWiringIntegration(t *testing.T) {
	const (
		payload   = "UNIQUE_PAYLOAD_7f3a"
		payloadID = "UNIQUE_PAYLOAD_ID_9b2c"
		apiKey    = "UNIQUE_API_KEY_5a6f"
		requestID = "UNIQUE_REQUEST_ID_1d4e"
		result    = "UNIQUE_RESULT_8c0d"
	)

	cfg := baseConfig()
	cfg.MetricsEnabled = true
	cfg.GlobalRateLimitRPS = 0
	cfg.GlobalRateLimitBurst = 0
	cfg.ConsumerRateLimitRPS = 0
	cfg.ConsumerRateLimitBurst = 0
	cfg.Systems = []config.System{{ID: "sys-a", Enabled: true, APIKey: apiKey}}

	handler := newTestRuntime(t, cfg, &resultProcessor{result: result}).Handler

	// Authorized process request.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/process",
		strings.NewReader(`{"payload":"`+payload+`","payload_id":"`+payloadID+`"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("X-Request-ID", requestID)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("process status = %d, want 200", rec.Code)
	}

	// Scrape metrics.
	mrec := httptest.NewRecorder()
	mreq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	handler.ServeHTTP(mrec, mreq)
	if mrec.Code != http.StatusOK {
		t.Fatalf("metrics status = %d, want 200", mrec.Code)
	}
	body := mrec.Body.String()

	// Safe fragments must be present.
	for _, fragment := range []string{
		`alpha_proxy_http_requests_total{method="POST",route="POST /process",status="200"} 1`,
		`alpha_proxy_processor_calls_total{outcome="success"} 1`,
		`alpha_proxy_http_request_duration_seconds_count{method="POST",route="POST /process"} 1`,
		`alpha_proxy_processor_duration_seconds_count{outcome="success"} 1`,
	} {
		if !strings.Contains(body, fragment) {
			t.Errorf("metrics output missing fragment %q", fragment)
		}
	}

	// Unique test values must not leak into the metrics output.
	for _, leak := range []struct {
		name  string
		value string
	}{
		{name: "payload", value: payload},
		{name: "payload_id", value: payloadID},
		{name: "api_key", value: apiKey},
		{name: "request_id", value: requestID},
		{name: "result", value: result},
	} {
		if strings.Contains(body, leak.value) {
			t.Errorf("metrics leaked %s", leak.name)
		}
	}
}
