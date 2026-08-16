package plateapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHealthLive(t *testing.T) {
	c := NewClient("dummy")
	health, err := c.Health(context.Background())
	if err != nil {
		t.Fatalf("health failed: %v", err)
	}
	if health.Status != "ok" {
		t.Errorf("expected status ok, got %q", health.Status)
	}
}

func TestLookupInvalidState(t *testing.T) {
	c := NewClient("dummy")
	_, err := c.Lookup(context.Background(), "ABC123", "XX", false)
	if err == nil {
		t.Fatal("expected error for invalid state")
	}
	if !contains(err.Error(), "invalid state") {
		t.Errorf("expected 'invalid state' error, got %q", err.Error())
	}
}

func TestLookupEmptyPlate(t *testing.T) {
	c := NewClient("dummy")
	_, err := c.Lookup(context.Background(), "", "VIC", false)
	if err == nil {
		t.Fatal("expected error for empty plate")
	}
}

func TestLookupWhitespacePlate(t *testing.T) {
	c := NewClient("dummy")
	_, err := c.Lookup(context.Background(), "   ", "VIC", false)
	if err == nil {
		t.Fatal("expected error for whitespace plate")
	}
}

func TestStateCaseInsensitive(t *testing.T) {
	c := NewClient("pk_live_invalid", WithMaxRetries(0))
	_, err := c.Lookup(context.Background(), "TEST123", "vic", false)
	if err == nil {
		t.Error("expected error with invalid key")
		return
	}
	if containsStr(err.Error(), "invalid state") {
		t.Error("lowercase state 'vic' was rejected -- case normalisation broken")
	}
}

func TestAllStatesAccepted(t *testing.T) {
	c := NewClient("pk_live_invalid", WithMaxRetries(0))
	states := []string{"NSW", "VIC", "QLD", "SA", "WA", "TAS", "NT", "ACT"}
	for _, s := range states {
		_, err := c.Lookup(context.Background(), "TEST123", s, false)
		if err == nil {
			t.Errorf("state %s: expected error, got nil", s)
			continue
		}
		if containsStr(err.Error(), "invalid state") {
			t.Errorf("state %s was rejected as invalid", s)
		}
	}
}

func TestLookupBadKey(t *testing.T) {
	c := NewClient("pk_live_invalid_key", WithMaxRetries(0))
	_, err := c.Lookup(context.Background(), "TEST123", "VIC", false)
	if err == nil {
		t.Fatal("expected error with invalid key")
	}
	var authErr *AuthenticationError
	var rateErr *RateLimitError
	if errors.As(err, &authErr) {
		if authErr.StatusCode != 401 {
			t.Errorf("expected status 401, got %d", authErr.StatusCode)
		}
	} else if errors.As(err, &rateErr) {
		t.Log("got rate limited instead of auth error (expected under rapid testing)")
	} else {
		t.Fatalf("expected AuthenticationError or RateLimitError, got %T: %v", err, err)
	}
}

func TestUsageBadKey(t *testing.T) {
	c := NewClient("pk_live_invalid_key", WithMaxRetries(0))
	_, err := c.Usage(context.Background())
	var authErr *AuthenticationError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected AuthenticationError, got %T: %v", err, err)
	}
}

func TestLogsBadKey(t *testing.T) {
	c := NewClient("pk_live_invalid_key", WithMaxRetries(0))
	_, err := c.Logs(context.Background(), nil)
	var authErr *AuthenticationError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected AuthenticationError, got %T: %v", err, err)
	}
}

func TestVehiclesBadKey(t *testing.T) {
	c := NewClient("pk_live_invalid_key", WithMaxRetries(0))
	_, err := c.Vehicles(context.Background(), nil)
	var authErr *AuthenticationError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected AuthenticationError, got %T: %v", err, err)
	}
}

func TestConnectionError(t *testing.T) {
	c := NewClient("dummy",
		WithBaseURL("http://127.0.0.1:19999"),
		WithTimeout(2*time.Second),
		WithMaxRetries(0),
	)
	_, err := c.Health(context.Background())
	if err == nil {
		t.Fatal("expected connection error")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Errorf("expected APIError, got %T", err)
	}
}

func TestCustomOptions(t *testing.T) {
	c := NewClient("dummy",
		WithTimeout(10*time.Second),
		WithMaxRetries(1),
	)
	health, err := c.Health(context.Background())
	if err != nil {
		t.Fatalf("health failed: %v", err)
	}
	if health.Status != "ok" {
		t.Errorf("expected ok, got %q", health.Status)
	}
}

func TestTrailingSlashStripped(t *testing.T) {
	c := NewClient("dummy", WithBaseURL("https://api.plateapi.com.au///"))
	health, err := c.Health(context.Background())
	if err != nil {
		t.Fatalf("health failed: %v", err)
	}
	if health.Status != "ok" {
		t.Errorf("expected ok, got %q", health.Status)
	}
}

func TestContextCancellation(t *testing.T) {
	c := NewClient("dummy", WithMaxRetries(0))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := c.Health(ctx)
	if err == nil {
		t.Fatal("expected error with cancelled context")
	}
}

func TestHelperFunctions(t *testing.T) {
	s := String("hello")
	if *s != "hello" {
		t.Errorf("String helper failed")
	}
	i := Int(42)
	if *i != 42 {
		t.Errorf("Int helper failed")
	}
	b := Bool(true)
	if *b != true {
		t.Errorf("Bool helper failed")
	}
}

func TestMock429QuotaExceeded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(429)
		w.Write([]byte(`{"code":"quota_exceeded","detail":"Monthly quota exceeded"}`))
	}))
	defer srv.Close()

	c := NewClient("dummy", WithBaseURL(srv.URL), WithMaxRetries(0))
	_, err := c.Lookup(context.Background(), "TEST123", "VIC", false)
	var quotaErr *QuotaExceededError
	if !errors.As(err, &quotaErr) {
		t.Fatalf("expected QuotaExceededError, got %T: %v", err, err)
	}
}

func TestMock429RateLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "5")
		w.WriteHeader(429)
		w.Write([]byte(`{"code":"rate_limited","detail":"Rate limit exceeded"}`))
	}))
	defer srv.Close()

	c := NewClient("dummy", WithBaseURL(srv.URL), WithMaxRetries(0))
	_, err := c.Lookup(context.Background(), "TEST123", "VIC", false)
	var rateErr *RateLimitError
	if !errors.As(err, &rateErr) {
		t.Fatalf("expected RateLimitError, got %T: %v", err, err)
	}
	if rateErr.RetryAfter == nil || *rateErr.RetryAfter != 5.0 {
		t.Errorf("expected RetryAfter=5, got %v", rateErr.RetryAfter)
	}
}

func TestMock500ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(502)
		w.Write([]byte(`{"error":"bad gateway"}`))
	}))
	defer srv.Close()

	c := NewClient("dummy", WithBaseURL(srv.URL), WithMaxRetries(0))
	_, err := c.Health(context.Background())
	var serverErr *ServerError
	if !errors.As(err, &serverErr) {
		t.Fatalf("expected ServerError, got %T: %v", err, err)
	}
	if serverErr.StatusCode != 502 {
		t.Errorf("expected 502, got %d", serverErr.StatusCode)
	}
}

func TestMockSuccessfulLookup(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/lookup" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("plate") != "TEST123" {
			t.Errorf("expected plate TEST123, got %s", r.URL.Query().Get("plate"))
		}
		if r.URL.Query().Get("state") != "VIC" {
			t.Errorf("expected state VIC, got %s", r.URL.Query().Get("state"))
		}
		if r.Header.Get("X-API-Key") != "pk_live_test" {
			t.Errorf("expected api key header")
		}
		w.Header().Set("X-RateLimit-Limit", "20")
		w.Header().Set("X-RateLimit-Remaining", "19")
		w.Header().Set("X-RateLimit-Plan", "free")
		w.Write([]byte(`{
			"success": true,
			"vehicle": {
				"make": "TOYOTA",
				"model": "CAMRY",
				"lowest_year": 2020,
				"highest_year": 2023,
				"year_range": "2020 - 2023",
				"body": "SEDAN",
				"engine": "2.5L",
				"description": "TOYOTA CAMRY SEDAN 2.5L"
			},
			"source": "plateapi",
			"duration_ms": 1234.5,
			"sandbox": true,
			"request_id": "req_abc123"
		}`))
	}))
	defer srv.Close()

	c := NewClient("pk_live_test", WithBaseURL(srv.URL))
	result, err := c.Lookup(context.Background(), "TEST123", "VIC", false)
	if err != nil {
		t.Fatalf("lookup failed: %v", err)
	}
	if !result.Success {
		t.Error("expected success")
	}
	if result.Vehicle == nil {
		t.Fatal("expected vehicle")
	}
	if *result.Vehicle.Make != "TOYOTA" {
		t.Errorf("expected TOYOTA, got %s", *result.Vehicle.Make)
	}
	if *result.Vehicle.Model != "CAMRY" {
		t.Errorf("expected CAMRY, got %s", *result.Vehicle.Model)
	}
	if result.Vehicle.Year == nil || *result.Vehicle.Year != 2020 {
		t.Errorf("expected Year=2020, got %v", result.Vehicle.Year)
	}
	if *result.Vehicle.LowestYear != 2020 {
		t.Errorf("expected LowestYear=2020")
	}
	if *result.Vehicle.HighestYear != 2023 {
		t.Errorf("expected HighestYear=2023")
	}
	if !result.Sandbox {
		t.Error("expected sandbox=true")
	}
	if *result.RequestID != "req_abc123" {
		t.Errorf("expected request_id req_abc123, got %s", *result.RequestID)
	}
	if result.RateLimit.Remaining == nil || *result.RateLimit.Remaining != 19 {
		t.Errorf("expected 19 remaining, got %v", result.RateLimit.Remaining)
	}
	if result.RateLimit.Plan == nil || *result.RateLimit.Plan != "free" {
		t.Errorf("expected plan free")
	}
}

func TestMockVehicles(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{
			"success": true,
			"data": [{"type": "make", "data": ["TOYOTA", "HONDA", "FORD"]}],
			"total": 3,
			"duration_ms": 12.5
		}`))
	}))
	defer srv.Close()

	c := NewClient("pk_live_test", WithBaseURL(srv.URL))
	result, err := c.Vehicles(context.Background(), nil)
	if err != nil {
		t.Fatalf("vehicles failed: %v", err)
	}
	if !result.Success {
		t.Error("expected success")
	}
	if *result.Type != "make" {
		t.Errorf("expected type make, got %s", *result.Type)
	}
	if len(result.Data) != 3 {
		t.Fatalf("expected 3 makes, got %d", len(result.Data))
	}
	if result.Data[0] != "TOYOTA" {
		t.Errorf("expected TOYOTA, got %s", result.Data[0])
	}
	if result.Total != 3 {
		t.Errorf("expected total 3, got %d", result.Total)
	}
}

func TestMockUsage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{
			"email": "test@example.com",
			"plan": "free",
			"monthly_limit": 20,
			"used_this_month": 5,
			"remaining": 15,
			"percent_used": 25.0,
			"rate_limit_per_min": 1,
			"period_start": "2026-08-01",
			"period_end": "2026-08-31",
			"days_remaining": 15,
			"cancel_at_period_end": false,
			"topup_credits": 100
		}`))
	}))
	defer srv.Close()

	c := NewClient("pk_live_test", WithBaseURL(srv.URL))
	u, err := c.Usage(context.Background())
	if err != nil {
		t.Fatalf("usage failed: %v", err)
	}
	if *u.Email != "test@example.com" {
		t.Errorf("expected email test@example.com")
	}
	if *u.Plan != "free" {
		t.Errorf("expected plan free")
	}
	if u.MonthlyLimit != 20 {
		t.Errorf("expected limit 20, got %d", u.MonthlyLimit)
	}
	if u.UsedThisMonth != 5 {
		t.Errorf("expected used 5, got %d", u.UsedThisMonth)
	}
	if u.Remaining != 15 {
		t.Errorf("expected remaining 15, got %d", u.Remaining)
	}
	if u.TopupCredits != 100 {
		t.Errorf("expected topup 100, got %d", u.TopupCredits)
	}
	if u.DaysRemaining == nil || *u.DaysRemaining != 15 {
		t.Errorf("expected days_remaining 15")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
