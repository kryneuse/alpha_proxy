package observability

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kryneuse/alpha_proxy/internal/contract"
	"github.com/kryneuse/alpha_proxy/internal/requestmeta"
)

func TestNewInstrumentedProcessorRejectsNilProcessor(t *testing.T) {
	m, err := NewMetrics()
	if err != nil {
		t.Fatalf("NewMetrics() error: %v", err)
	}
	if _, err := NewInstrumentedProcessor(nil, m); err == nil {
		t.Fatal("NewInstrumentedProcessor(nil, m) error = nil, want error")
	}
}

func TestNewInstrumentedProcessorRejectsNilMetrics(t *testing.T) {
	if _, err := NewInstrumentedProcessor(&stubProcessor{}, nil); err == nil {
		t.Fatal("NewInstrumentedProcessor(p, nil) error = nil, want error")
	}
}

func TestNewInstrumentedProcessorRejectsNilClock(t *testing.T) {
	m, err := NewMetrics()
	if err != nil {
		t.Fatalf("NewMetrics() error: %v", err)
	}
	if _, err := newInstrumentedProcessor(&stubProcessor{}, m, nil); err == nil {
		t.Fatal("newInstrumentedProcessor(p, m, nil) error = nil, want error")
	}
}

func TestInstrumentedProcessorSuccess(t *testing.T) {
	m, err := NewMetrics()
	if err != nil {
		t.Fatalf("NewMetrics() error: %v", err)
	}

	start := time.Unix(100, 0)
	finish := time.Unix(100, 500_000_000) // 500ms later
	clock := fakeClock(start, finish)

	ctx := context.Background()
	req := contract.ProcessRequest{Payload: "raw", PayloadID: "id", ConsumerID: "consumer"}
	stub := &stubProcessor{result: "masked"}
	proc, err := newInstrumentedProcessor(stub, m, clock)
	if err != nil {
		t.Fatalf("newInstrumentedProcessor() error: %v", err)
	}

	resp, err := proc.Process(ctx, req)
	if err != nil {
		t.Fatalf("Process() error: %v", err)
	}

	if stub.calls != 1 {
		t.Errorf("next called %d times, want 1", stub.calls)
	}
	if stub.gotCtx != ctx {
		t.Error("next received a different context")
	}
	if !reflect.DeepEqual(stub.gotReq, req) {
		t.Errorf("next received request %+v, want %+v", stub.gotReq, req)
	}
	if resp.Result != "masked" {
		t.Errorf("result = %q, want %q", resp.Result, "masked")
	}

	if got := counterValue(t, m, "alpha_proxy_processor_calls_total", "success"); got != 1 {
		t.Errorf("success counter = %v, want 1", got)
	}
	count, sum := histogramStats(t, m, "alpha_proxy_processor_duration_seconds",
		map[string]string{"outcome": "success"})
	if count != 1 {
		t.Errorf("histogram sample_count = %d, want 1", count)
	}
	if want := finish.Sub(start).Seconds(); sum != want {
		t.Errorf("histogram sample_sum = %v, want %v", sum, want)
	}
}

func TestInstrumentedProcessorSetsOperationProcess(t *testing.T) {
	m, err := NewMetrics()
	if err != nil {
		t.Fatalf("NewMetrics() error: %v", err)
	}

	clock := fakeClock(time.Unix(100, 0), time.Unix(100, 100_000_000))
	meta := &requestmeta.Meta{}
	ctx := requestmeta.With(context.Background(), meta)

	stub := &stubProcessor{result: "masked"}
	proc, err := newInstrumentedProcessor(stub, m, clock)
	if err != nil {
		t.Fatalf("newInstrumentedProcessor() error: %v", err)
	}

	if _, err := proc.Process(ctx, contract.ProcessRequest{}); err != nil {
		t.Fatalf("Process() error: %v", err)
	}
	if meta.Operation != "process" {
		t.Errorf("Operation = %q, want process", meta.Operation)
	}
}

func TestInstrumentedProcessorUnavailableWrapped(t *testing.T) {
	m, err := NewMetrics()
	if err != nil {
		t.Fatalf("NewMetrics() error: %v", err)
	}

	clock := fakeClock(time.Unix(100, 0), time.Unix(100, 100_000_000))
	wrapped := fmt.Errorf("wrapped: %w", contract.ErrUnavailable)
	stub := &stubProcessor{err: wrapped}
	proc, err := newInstrumentedProcessor(stub, m, clock)
	if err != nil {
		t.Fatalf("newInstrumentedProcessor() error: %v", err)
	}

	_, err = proc.Process(context.Background(), contract.ProcessRequest{})
	if !errors.Is(err, contract.ErrUnavailable) {
		t.Fatalf("Process() error = %v, want errors.Is ErrUnavailable", err)
	}
	//nolint:errorlint // Identity comparison verifies that instrumentation preserves the exact error value.
	if err != wrapped {
		t.Error("Process() returned a different error value")
	}

	if got := counterValue(t, m, "alpha_proxy_processor_calls_total", "unavailable"); got != 1 {
		t.Errorf("unavailable counter = %v, want 1", got)
	}
}

func TestInstrumentedProcessorDeadlineExceededIsTimeout(t *testing.T) {
	m, err := NewMetrics()
	if err != nil {
		t.Fatalf("NewMetrics() error: %v", err)
	}

	clock := fakeClock(time.Unix(100, 0), time.Unix(100, 100_000_000))
	stub := &stubProcessor{err: context.DeadlineExceeded}
	proc, err := newInstrumentedProcessor(stub, m, clock)
	if err != nil {
		t.Fatalf("newInstrumentedProcessor() error: %v", err)
	}

	_, err = proc.Process(context.Background(), contract.ProcessRequest{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Process() error = %v, want DeadlineExceeded", err)
	}

	if got := counterValue(t, m, "alpha_proxy_processor_calls_total", "timeout"); got != 1 {
		t.Errorf("timeout counter = %v, want 1", got)
	}
}

func TestInstrumentedProcessorCanceledWithCancelledContextIsTimeout(t *testing.T) {
	m, err := NewMetrics()
	if err != nil {
		t.Fatalf("NewMetrics() error: %v", err)
	}

	clock := fakeClock(time.Unix(100, 0), time.Unix(100, 100_000_000))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	stub := &stubProcessor{err: context.Canceled}
	proc, err := newInstrumentedProcessor(stub, m, clock)
	if err != nil {
		t.Fatalf("newInstrumentedProcessor() error: %v", err)
	}

	_, err = proc.Process(ctx, contract.ProcessRequest{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Process() error = %v, want Canceled", err)
	}

	if got := counterValue(t, m, "alpha_proxy_processor_calls_total", "timeout"); got != 1 {
		t.Errorf("timeout counter = %v, want 1", got)
	}
}

func TestInstrumentedProcessorCanceledWithoutCancelledContextIsError(t *testing.T) {
	m, err := NewMetrics()
	if err != nil {
		t.Fatalf("NewMetrics() error: %v", err)
	}

	clock := fakeClock(time.Unix(100, 0), time.Unix(100, 100_000_000))
	// A non-cancelled context with a Canceled error is not a timeout.
	stub := &stubProcessor{err: context.Canceled}
	proc, err := newInstrumentedProcessor(stub, m, clock)
	if err != nil {
		t.Fatalf("newInstrumentedProcessor() error: %v", err)
	}

	_, err = proc.Process(context.Background(), contract.ProcessRequest{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Process() error = %v, want Canceled", err)
	}

	if got := counterValue(t, m, "alpha_proxy_processor_calls_total", "error"); got != 1 {
		t.Errorf("error counter = %v, want 1", got)
	}
}

func TestInstrumentedProcessorSentinelErrorIsError(t *testing.T) {
	m, err := NewMetrics()
	if err != nil {
		t.Fatalf("NewMetrics() error: %v", err)
	}

	clock := fakeClock(time.Unix(100, 0), time.Unix(100, 100_000_000))
	sentinel := errors.New("boom")
	stub := &stubProcessor{err: sentinel}
	proc, err := newInstrumentedProcessor(stub, m, clock)
	if err != nil {
		t.Fatalf("newInstrumentedProcessor() error: %v", err)
	}

	_, err = proc.Process(context.Background(), contract.ProcessRequest{})
	if !errors.Is(err, sentinel) {
		t.Fatalf("Process() error = %v, want sentinel", err)
	}

	if got := counterValue(t, m, "alpha_proxy_processor_calls_total", "error"); got != 1 {
		t.Errorf("error counter = %v, want 1", got)
	}
}

func TestInstrumentedProcessorPanicPropagatesAndRecordsError(t *testing.T) {
	m, err := NewMetrics()
	if err != nil {
		t.Fatalf("NewMetrics() error: %v", err)
	}

	clock := fakeClock(time.Unix(100, 0), time.Unix(100, 100_000_000))
	panicVal := "boom"
	stub := &stubProcessor{panicVal: panicVal}
	proc, err := newInstrumentedProcessor(stub, m, clock)
	if err != nil {
		t.Fatalf("newInstrumentedProcessor() error: %v", err)
	}

	func() {
		defer func() {
			if r := recover(); r != panicVal {
				t.Errorf("recovered %v, want %v", r, panicVal)
			}
		}()
		_, _ = proc.Process(context.Background(), contract.ProcessRequest{})
		t.Error("Process() did not panic")
	}()

	if stub.calls != 1 {
		t.Errorf("next called %d times, want 1", stub.calls)
	}
	if got := counterValue(t, m, "alpha_proxy_processor_calls_total", "error"); got != 1 {
		t.Errorf("error counter = %v, want 1", got)
	}
	count, _ := histogramStats(t, m, "alpha_proxy_processor_duration_seconds",
		map[string]string{"outcome": "error"})
	if count != 1 {
		t.Errorf("histogram sample_count = %d, want 1", count)
	}
}

func TestInstrumentedProcessorRecordsExactlyOnce(t *testing.T) {
	m, err := NewMetrics()
	if err != nil {
		t.Fatalf("NewMetrics() error: %v", err)
	}

	clock := fakeClock(time.Unix(100, 0), time.Unix(100, 100_000_000))
	stub := &stubProcessor{result: "ok"}
	proc, err := newInstrumentedProcessor(stub, m, clock)
	if err != nil {
		t.Fatalf("newInstrumentedProcessor() error: %v", err)
	}

	if _, err := proc.Process(context.Background(), contract.ProcessRequest{}); err != nil {
		t.Fatalf("Process() error: %v", err)
	}

	if got := counterValue(t, m, "alpha_proxy_processor_calls_total", "success"); got != 1 {
		t.Errorf("success counter = %v, want 1", got)
	}
	count, _ := histogramStats(t, m, "alpha_proxy_processor_duration_seconds",
		map[string]string{"outcome": "success"})
	if count != 1 {
		t.Errorf("histogram sample_count = %d, want 1", count)
	}
}

func TestInstrumentedProcessorDoesNotLeakSensitiveData(t *testing.T) {
	m, err := NewMetrics()
	if err != nil {
		t.Fatalf("NewMetrics() error: %v", err)
	}

	const (
		payload    = "UNIQUE_PAYLOAD_7f3a"
		payloadID  = "UNIQUE_PAYLOAD_ID_9b2c"
		result     = "UNIQUE_RESULT_1d4e"
		consumerID = "UNIQUE_CONSUMER_5a6f"
		errText    = "UNIQUE_ERR_8c0d"
	)

	clock := fakeClock(time.Unix(100, 0), time.Unix(100, 100_000_000))
	stub := &stubProcessor{result: result, err: errors.New(errText)}
	proc, err := newInstrumentedProcessor(stub, m, clock)
	if err != nil {
		t.Fatalf("newInstrumentedProcessor() error: %v", err)
	}

	req := contract.ProcessRequest{Payload: payload, PayloadID: payloadID, ConsumerID: consumerID}
	if _, err := proc.Process(context.Background(), req); err == nil {
		t.Fatal("Process() error = nil, want error")
	}

	handler := m.Handler()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	body := rec.Body.String()
	for _, leak := range []string{payload, payloadID, result, consumerID, errText} {
		if strings.Contains(body, leak) {
			t.Errorf("metrics exposition leaked %q: %q", leak, body)
		}
	}
}

// stubProcessor records calls and returns a fixed result/error, or panics.
type stubProcessor struct {
	mu       sync.Mutex
	calls    int
	gotCtx   context.Context
	gotReq   contract.ProcessRequest
	result   string
	err      error
	panicVal any
}

func (s *stubProcessor) Process(ctx context.Context, req contract.ProcessRequest) (contract.ProcessResponse, error) {
	s.mu.Lock()
	s.calls++
	s.gotCtx = ctx
	s.gotReq = req
	s.mu.Unlock()
	if s.panicVal != nil {
		panic(s.panicVal)
	}
	return contract.ProcessResponse{Result: s.result}, s.err
}

// fakeClock returns a clock function that yields the given times in sequence,
// holding the last value for any further calls.
func fakeClock(times ...time.Time) func() time.Time {
	i := 0
	return func() time.Time {
		if i < len(times)-1 {
			t := times[i]
			i++
			return t
		}
		return times[len(times)-1]
	}
}

// counterValue returns the value of a counter metric family matching the given
// outcome label, or 0 when absent.
func counterValue(t *testing.T, m *Metrics, familyName string, outcome string) float64 {
	t.Helper()
	families, err := m.Gatherer().Gather()
	if err != nil {
		t.Fatalf("Gather() error: %v", err)
	}
	for _, f := range families {
		if f.GetName() != familyName {
			continue
		}
		for _, metric := range f.GetMetric() {
			for _, lp := range metric.GetLabel() {
				if lp.GetName() == "outcome" && lp.GetValue() == outcome {
					return metric.GetCounter().GetValue()
				}
			}
		}
	}
	return 0
}
