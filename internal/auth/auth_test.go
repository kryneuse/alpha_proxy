package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"alpha_proxy/internal/config"
)

func TestAPIKeyValid(t *testing.T) {
	a := New(apiKeyConfig())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/process", nil)
	req.Header.Set("X-API-Key", "secret-a")

	var got string
	a.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = ConsumerID(r.Context())
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got != "sys-a" {
		t.Fatalf("ConsumerID = %q, want sys-a", got)
	}
}

func TestAPIKeyMissing(t *testing.T) {
	a := New(apiKeyConfig())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/process", nil)

	a.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestAPIKeyInvalid(t *testing.T) {
	a := New(apiKeyConfig())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/process", nil)
	req.Header.Set("X-API-Key", "wrong")

	a.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestDisabledSystemForbidden(t *testing.T) {
	cfg := apiKeyConfig()
	cfg.Systems = []config.System{
		{ID: "sys-a", Enabled: true, APIKey: "secret-a"},
		{ID: "sys-b", Enabled: false, APIKey: "secret-b"},
	}
	a := New(cfg)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/process", nil)
	req.Header.Set("X-API-Key", "secret-b")

	a.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestVerifyModeNoKey(t *testing.T) {
	cfg := apiKeyConfig()
	cfg.AuthMode = config.AuthModeVerify
	a := New(cfg)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/process", nil)

	var got string
	a.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = ConsumerID(r.Context())
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got != VerifyConsumerID {
		t.Fatalf("ConsumerID = %q, want %q", got, VerifyConsumerID)
	}
}

func TestRawKeyNotStored(t *testing.T) {
	a := New(apiKeyConfig())
	// The raw key must not be retained anywhere in the Authenticator.
	if _, ok := a.byFp["secret-a"]; ok {
		t.Fatal("raw api key stored as a map key")
	}
	for k := range a.byFp {
		if k == "secret-a" {
			t.Fatal("raw api key found in fingerprint map")
		}
	}
}

func TestFingerprintIsDeterministic(t *testing.T) {
	a := New(apiKeyConfig())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/process", nil)
	req.Header.Set("X-API-Key", "secret-a")

	a.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func apiKeyConfig() config.Config {
	return config.Config{
		AuthMode: config.AuthModeAPIKey,
		Systems: []config.System{
			{ID: "sys-a", Enabled: true, APIKey: "secret-a"},
		},
	}
}
