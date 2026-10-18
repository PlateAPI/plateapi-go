package plateapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ---- fake server ----

type fakeResponse struct {
	status  int
	headers map[string]string
	body    string
	drop    bool
}

type call struct {
	path   string
	params map[string]string
	header http.Header
}

type fake struct {
	t      *testing.T
	srv    *httptest.Server
	queue  []fakeResponse
	calls  []call
	sleeps []time.Duration
	client *Client
}

func jsonBody(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func res(status int, body interface{}, headers map[string]string) fakeResponse {
	text := "<html>bad gateway</html>"
	if body != nil {
		text = jsonBody(body)
	}
	return fakeResponse{status: status, headers: headers, body: text}
}

func ok(body interface{}, headers map[string]string) fakeResponse {
	return res(200, body, headers)
}

func level(name string, items ...interface{}) fakeResponse {
	if items == nil {
		items = []interface{}{}
	}
	return ok(map[string]interface{}{
		"success": true, "data": []interface{}{map[string]interface{}{"type": name, "data": items}},
		"total": len(items), "duration_ms": 1.0,
	}, nil)
}

func newFake(t *testing.T, responses []fakeResponse, opts ...Option) *fake {
	t.Helper()
	f := &fake{t: t, queue: responses}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		params := map[string]string{}
		for k, v := range r.URL.Query() {
			params[k] = v[0]
		}
		f.calls = append(f.calls, call{path: r.URL.Path, params: params, header: r.Header.Clone()})
		if len(f.queue) == 0 {
			t.Errorf("unexpected request: %s", r.URL.String())
			w.WriteHeader(599)
			return
		}
		item := f.queue[0]
		f.queue = f.queue[1:]
		if item.drop {
			hj, _ := w.(http.Hijacker)
			conn, _, _ := hj.Hijack()
			conn.Close()
			return
		}
		for k, v := range item.headers {
			w.Header().Set(k, v)
		}
		w.WriteHeader(item.status)
		w.Write([]byte(item.body))
	}))
	t.Cleanup(f.srv.Close)
	all := append([]Option{WithBaseURL(f.srv.URL)}, opts...)
	f.client = NewClient("pk_live_test", all...)
	f.client.sleep = func(ctx context.Context, d time.Duration) { f.sleeps = append(f.sleeps, d) }
	return f
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func deref(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}

var hilux = map[string]interface{}{
	"year_range": "2015-2020", "lowest_year": 2015, "highest_year": 2020,
	"make": "TOYOTA", "model": "HILUX", "description": "15~20 TOYOTA HILUX 2.8L DIESEL",
	"body": "UTE", "engine": "2.8L", "vehicle_id": 1380801,
}

func corolla(id interface{}) map[string]interface{} {
	return map[string]interface{}{
		"year_range": "2003-2006", "lowest_year": 2003, "highest_year": 2006,
		"make": "TOYOTA", "model": "COROLLA", "description": "03~06 TOYOTA COROLLA 1.8L PETROL",
		"body": "HATCHBACK", "engine": "1.8L", "vehicle_id": id,
	}
}

var record = map[string]interface{}{
	"vehicle_id": 1380801, "vehicle_type": "car", "make": "TOYOTA", "model": "HILUX SR",
	"year_range": "2015-2020", "lowest_year": 2015, "highest_year": 2020,
	"years":       []int{2015, 2016, 2017, 2018, 2019, 2020},
	"description": "15~20 TOYOTA HILUX 2.8L DIESEL", "long_description": "TOYOTA HILUX 10/2015~04/2020",
	"series": "GUN126R -GUN126", "engine": "2.8L,  DIE 1GDFTV I4 16v DOHC I/C Turbo CRD {130kW}",
	"variant": "SR,  4D Ute -, 4WD MR0KA3CD  [THAILAND], AT/MT",
	"body":    "UTE", "body_size": nil, "doors": 4, "drive": "4WD", "transmission": "AT/MT", "detail": "full",
}

// ---- lookup ----

func TestExactLookup(t *testing.T) {
	f := newFake(t, []fakeResponse{ok(map[string]interface{}{
		"success": true, "vehicle": hilux, "alternatives": []interface{}{}, "match": "exact",
		"source": "plateapi", "duration_ms": 1509.9, "request_id": "req_abc",
	}, map[string]string{"X-RateLimit-Limit": "600", "X-RateLimit-Remaining": "599", "X-RateLimit-Plan": "starter"})})
	r, err := f.client.Lookup(context.Background(), "abc 123", "vic", false)
	must(t, err)
	c := f.calls[0]
	if c.path != "/api/v1/lookup" || c.params["plate"] != "ABC 123" || c.params["state"] != "VIC" {
		t.Errorf("bad request: %+v", c)
	}
	if c.header.Get("X-API-Key") != "pk_live_test" || c.header.Get("User-Agent") != "plateapi-go/"+Version {
		t.Errorf("bad headers: %v", c.header)
	}
	if !r.Success || deref(r.Match) != "exact" || *r.Vehicle.VehicleID != 1380801 || *r.Vehicle.Year != 2015 {
		t.Errorf("bad result: %+v", r)
	}
	if ids := r.CandidateIDs(); len(ids) != 1 || ids[0] != 1380801 {
		t.Errorf("candidate ids: %v", ids)
	}
	if deref(r.RequestID) != "req_abc" || *r.RateLimit.Limit != 600 || *r.RateLimit.Remaining != 599 || deref(r.RateLimit.Plan) != "starter" || r.RateLimit.TopupRemaining != nil {
		t.Errorf("bad meta: %+v %+v", r.RequestID, r.RateLimit)
	}
}

func TestMultipleAndAmbiguous(t *testing.T) {
	f := newFake(t, []fakeResponse{
		ok(map[string]interface{}{"success": true, "vehicle": corolla(5152708), "match": "multiple", "alternatives": []interface{}{corolla(1857621), corolla(5152708)}}, nil),
		ok(map[string]interface{}{"success": true, "vehicle": corolla(nil), "match": "ambiguous", "alternatives": []interface{}{corolla(5152708), hilux}}, nil),
	})
	r, err := f.client.Lookup(context.Background(), "ABC123", "VIC", false)
	must(t, err)
	if ids := r.CandidateIDs(); len(ids) != 2 || ids[0] != 5152708 || ids[1] != 1857621 {
		t.Errorf("multiple ids: %v", ids)
	}
	r, err = f.client.Lookup(context.Background(), "ABC123", "VIC", false)
	must(t, err)
	if r.Vehicle.VehicleID != nil || deref(r.Match) != "ambiguous" {
		t.Errorf("ambiguous: %+v", r)
	}
	if ids := r.CandidateIDs(); len(ids) != 2 || ids[0] != 5152708 || ids[1] != 1380801 {
		t.Errorf("ambiguous ids: %v", ids)
	}
}

func TestNotFoundSandboxAndHeaderFallbacks(t *testing.T) {
	f := newFake(t, []fakeResponse{
		ok(map[string]interface{}{"success": false, "vehicle": nil, "code": "not_found", "error": "No vehicle found", "request_id": "req_nf"}, nil),
		ok(map[string]interface{}{"success": true, "vehicle": hilux, "sandbox": true}, map[string]string{"X-Request-ID": "req_hdr", "X-Topup-Remaining": "17"}),
		ok(map[string]interface{}{"success": true, "vehicle": hilux}, map[string]string{"X-RateLimit-Limit": "unlimited"}),
	})
	r, err := f.client.Lookup(context.Background(), "ZZZ999", "NSW", false)
	must(t, err)
	if r.Success || deref(r.Code) != "not_found" || r.Vehicle != nil || len(r.CandidateIDs()) != 0 || r.Match != nil || deref(r.RequestID) != "req_nf" {
		t.Errorf("not found: %+v", r)
	}
	r, err = f.client.Lookup(context.Background(), "TEST123", "VIC", true)
	must(t, err)
	if f.calls[1].params["detailed"] != "true" || !r.Sandbox || deref(r.RequestID) != "req_hdr" || *r.RateLimit.TopupRemaining != 17 {
		t.Errorf("sandbox: %+v", r)
	}
	r, err = f.client.Lookup(context.Background(), "ABC123", "VIC", false)
	must(t, err)
	if r.RateLimit.Limit != nil {
		t.Errorf("unlimited should be nil")
	}
}

func TestInternalErrorRetried(t *testing.T) {
	internal := ok(map[string]interface{}{"success": false, "vehicle": nil, "code": "internal_error", "error": "Internal error"}, nil)
	f := newFake(t, []fakeResponse{internal, ok(map[string]interface{}{"success": true, "vehicle": hilux}, nil)})
	r, err := f.client.Lookup(context.Background(), "ABC123", "VIC", false)
	must(t, err)
	if !r.Success || len(f.calls) != 2 || len(f.sleeps) != 1 {
		t.Errorf("retry: %+v calls=%d sleeps=%d", r, len(f.calls), len(f.sleeps))
	}
	f = newFake(t, []fakeResponse{internal, internal, internal, internal})
	r, err = f.client.Lookup(context.Background(), "ABC123", "VIC", false)
	must(t, err)
	if r.Success || deref(r.Code) != "internal_error" || len(f.calls) != 4 {
		t.Errorf("exhausted: %+v calls=%d", r, len(f.calls))
	}
}

func TestLookupValidation(t *testing.T) {
	f := newFake(t, nil)
	if _, err := f.client.Lookup(context.Background(), "ABC123", "XX", false); err == nil || !strings.Contains(err.Error(), "invalid state") {
		t.Errorf("expected invalid state, got %v", err)
	}
	if _, err := f.client.Lookup(context.Background(), "   ", "VIC", false); err == nil {
		t.Error("expected empty plate error")
	}
	if len(f.calls) != 0 {
		t.Error("no request expected")
	}
}

// ---- errors ----

func TestErrorMapping(t *testing.T) {
	cases := []struct {
		status   int
		body     interface{}
		expected string
		check    func(error) bool
	}{
		{400, map[string]interface{}{"detail": "Invalid state. Use one of: ACT, NSW"}, "Invalid state. Use one of: ACT, NSW", func(e error) bool { var x *BadRequestError; return errors.As(e, &x) }},
		{401, map[string]interface{}{"detail": "API key has been deactivated"}, "API key has been deactivated", func(e error) bool { var x *AuthenticationError; return errors.As(e, &x) }},
		{401, nil, "Request failed (HTTP 401)", func(e error) bool { var x *AuthenticationError; return errors.As(e, &x) }},
		{403, map[string]interface{}{"detail": "This API key is not allowed from your IP address."}, "This API key is not allowed from your IP address.", func(e error) bool { var x *PermissionDeniedError; return errors.As(e, &x) }},
		{404, map[string]interface{}{"success": false, "code": "not_found", "error": "No vehicle with that vehicle_id."}, "No vehicle with that vehicle_id.", func(e error) bool { var x *NotFoundError; return errors.As(e, &x) && x.Code == "not_found" }},
		{418, map[string]interface{}{"detail": "teapot"}, "teapot", func(e error) bool { var x *APIError; return errors.As(e, &x) }},
	}
	for _, tc := range cases {
		f := newFake(t, []fakeResponse{res(tc.status, tc.body, map[string]string{"X-Request-ID": "req_x"})})
		_, err := f.client.Lookup(context.Background(), "ABC123", "VIC", false)
		if err == nil || !tc.check(err) {
			t.Errorf("status %d: wrong type %T %v", tc.status, err, err)
			continue
		}
		var api *APIError
		errors.As(err, &api)
		if api.Message != tc.expected || api.StatusCode != tc.status || api.RequestID != "req_x" {
			t.Errorf("status %d: %+v", tc.status, api)
		}
		if len(f.calls) != 1 || len(f.sleeps) != 0 {
			t.Errorf("status %d: retried", tc.status)
		}
	}
}

func TestRateLimitHandling(t *testing.T) {
	f := newFake(t, []fakeResponse{
		res(429, map[string]interface{}{"detail": "Per-minute rate limit exceeded (10/min on starter plan). Slow down."}, map[string]string{"Retry-After": "42"}),
		ok(map[string]interface{}{"success": true, "vehicle": hilux}, nil),
	})
	r, err := f.client.Lookup(context.Background(), "ABC123", "VIC", false)
	must(t, err)
	if !r.Success || len(f.sleeps) != 1 || f.sleeps[0] != 42*time.Second || len(f.calls) != 2 {
		t.Errorf("retry-after: sleeps=%v calls=%d", f.sleeps, len(f.calls))
	}

	f = newFake(t, []fakeResponse{res(429, map[string]interface{}{"detail": "Vehicle query rate limit exceeded. Try again later."}, map[string]string{"Retry-After": "61"})})
	_, err = f.client.Vehicles(context.Background(), &VehiclesOptions{Make: String("TOYOTA")})
	var rl *RateLimitError
	var quota *QuotaExceededError
	if !errors.As(err, &rl) || errors.As(err, &quota) || rl.RetryAfter == nil || *rl.RetryAfter != 61 || rl.Message != "Vehicle query rate limit exceeded. Try again later." {
		t.Errorf("above max wait: %T %v", err, err)
	}
	if len(f.sleeps) != 0 || len(f.calls) != 1 {
		t.Errorf("above max wait slept: %v", f.sleeps)
	}

	slow := res(429, map[string]interface{}{"detail": "slow down"}, nil)
	f = newFake(t, []fakeResponse{slow, slow, slow, slow})
	_, err = f.client.Lookup(context.Background(), "ABC123", "VIC", false)
	if !errors.As(err, &rl) || rl.RetryAfter != nil || len(f.sleeps) != 3 || len(f.calls) != 4 {
		t.Errorf("backoff: %v sleeps=%d calls=%d", err, len(f.sleeps), len(f.calls))
	}

	f = newFake(t, []fakeResponse{res(429, map[string]interface{}{"detail": "slow down"}, map[string]string{"Retry-After": "3600"}), ok(map[string]interface{}{"success": true, "vehicle": hilux}, nil)}, WithMaxWait(0))
	_, err = f.client.Lookup(context.Background(), "ABC123", "VIC", false)
	must(t, err)
	if len(f.sleeps) != 1 || f.sleeps[0] != time.Hour {
		t.Errorf("no max wait: %v", f.sleeps)
	}

	f = newFake(t, []fakeResponse{res(429, map[string]interface{}{"detail": "slow"}, map[string]string{"Retry-After": "Wed, 21 Oct 2026 07:28:00 GMT"}), ok(map[string]interface{}{"success": true, "vehicle": hilux}, nil)})
	_, err = f.client.Lookup(context.Background(), "ABC123", "VIC", false)
	must(t, err)
	if len(f.sleeps) != 1 || f.sleeps[0] < time.Second || f.sleeps[0] >= 2*time.Second {
		t.Errorf("junk retry-after: %v", f.sleeps)
	}
}

func TestQuotaExceeded(t *testing.T) {
	f := newFake(t, []fakeResponse{res(429, map[string]interface{}{"detail": "Monthly quota exceeded. Used 600/600 lookups on starter plan. Upgrade at plateapi.com.au"}, map[string]string{"Retry-After": "3600"})})
	_, err := f.client.Lookup(context.Background(), "ABC123", "VIC", false)
	var quota *QuotaExceededError
	var rl *RateLimitError
	var api *APIError
	if !errors.As(err, &quota) || !errors.As(err, &rl) || !errors.As(err, &api) {
		t.Fatalf("expected QuotaExceededError chain, got %T %v", err, err)
	}
	if quota.Code != "quota_exceeded" || *quota.RetryAfter != 3600 || !strings.HasPrefix(quota.Message, "Monthly quota exceeded") || rl.RetryAfter == nil {
		t.Errorf("quota: %+v", quota)
	}
	if len(f.sleeps) != 0 || len(f.calls) != 1 {
		t.Error("quota must not retry")
	}
	f = newFake(t, []fakeResponse{res(429, map[string]interface{}{"code": "quota_exceeded", "error": "Quota exceeded"}, nil)})
	_, err = f.client.Lookup(context.Background(), "ABC123", "VIC", false)
	if !errors.As(err, &quota) {
		t.Errorf("quota by code: %T", err)
	}
}

func TestServerErrors(t *testing.T) {
	failed := res(503, map[string]interface{}{"success": false, "vehicle": nil, "code": "lookup_failed", "error": "Lookup temporarily unavailable. Please retry.", "request_id": "req_lf"}, nil)
	f := newFake(t, []fakeResponse{failed, failed, failed, failed})
	_, err := f.client.Lookup(context.Background(), "ABC123", "VIC", false)
	var se *ServerError
	if !errors.As(err, &se) || se.Code != "lookup_failed" || se.StatusCode != 503 || se.RequestID != "req_lf" || se.Message != "Lookup temporarily unavailable. Please retry." {
		t.Errorf("lookup_failed: %T %v", err, err)
	}
	if len(f.calls) != 4 || len(f.sleeps) != 3 {
		t.Errorf("lookup_failed retries: calls=%d sleeps=%d", len(f.calls), len(f.sleeps))
	}

	f = newFake(t, []fakeResponse{
		res(503, map[string]interface{}{"success": false, "code": "unavailable", "data": []interface{}{}, "total": 0}, map[string]string{"Retry-After": "5"}),
		level("make", "TOYOTA"),
	})
	r, err := f.client.Vehicles(context.Background(), nil)
	must(t, err)
	if r.Data[0] != "TOYOTA" || len(f.sleeps) != 1 || f.sleeps[0] != 5*time.Second {
		t.Errorf("503 retry-after: %v %v", r.Data, f.sleeps)
	}

	html := res(502, nil, nil)
	f = newFake(t, []fakeResponse{html, html, html, html})
	_, err = f.client.Lookup(context.Background(), "ABC123", "VIC", false)
	if !errors.As(err, &se) || se.Message != "Request failed (HTTP 502)" || se.Code != "" {
		t.Errorf("html body: %v", err)
	}
}

func TestConnectionErrorsRetried(t *testing.T) {
	f := newFake(t, []fakeResponse{{drop: true}, {drop: true}, ok(map[string]interface{}{"success": true, "vehicle": hilux}, nil)})
	r, err := f.client.Lookup(context.Background(), "ABC123", "VIC", false)
	must(t, err)
	if !r.Success || len(f.sleeps) != 2 {
		t.Errorf("dropped connections: sleeps=%d", len(f.sleeps))
	}
	f = newFake(t, []fakeResponse{{drop: true}, {drop: true}, {drop: true}, {drop: true}})
	_, err = f.client.Lookup(context.Background(), "ABC123", "VIC", false)
	var api *APIError
	if !errors.As(err, &api) || !strings.HasPrefix(api.Message, "request failed") || api.StatusCode != 0 || len(f.calls) != 4 {
		t.Errorf("exhausted: %v calls=%d", err, len(f.calls))
	}
}

// ---- vehicles ----

func TestVehiclesParamsAndLevels(t *testing.T) {
	f := newFake(t, []fakeResponse{level("engine", "2.8L"), level("make", "ABARTH", "TOYOTA"), level("year", 2021, 2020), level("make", "YAMAHA"), ok(map[string]interface{}{"success": true, "data": []interface{}{}, "total": 0}, nil)})
	r, err := f.client.Vehicles(context.Background(), &VehiclesOptions{Make: String("TOYOTA"), Model: String("HILUX"), Year: Int(2020), Series: String("")})
	must(t, err)
	c := f.calls[0]
	if c.path != "/api/v1/vehicles" || c.params["make"] != "TOYOTA" || c.params["year"] != "2020" {
		t.Errorf("params: %+v", c)
	}
	if v, present := c.params["series"]; !present || v != "" {
		t.Errorf("empty series must be sent: %+v", c.params)
	}
	if !r.Success || deref(r.Type) != "engine" || len(r.Data) != 1 || r.Data[0] != "2.8L" || r.Total != 1 || len(r.VehicleIDs()) != 0 {
		t.Errorf("engine level: %+v", r)
	}
	r, _ = f.client.Vehicles(context.Background(), nil)
	if deref(r.Type) != "make" || len(f.calls[1].params) != 0 {
		t.Errorf("makes: %+v", r)
	}
	r, _ = f.client.Vehicles(context.Background(), &VehiclesOptions{Make: String("TOYOTA"), Model: String("HILUX")})
	if r.Data[0] != "2021" || r.Data[1] != "2020" {
		t.Errorf("years: %v", r.Data)
	}
	_, err = f.client.Vehicles(context.Background(), &VehiclesOptions{VehicleType: String("motorcycle")})
	must(t, err)
	if f.calls[3].params["type"] != "motorcycle" {
		t.Errorf("type param: %+v", f.calls[3].params)
	}
	r, _ = f.client.Vehicles(context.Background(), &VehiclesOptions{Make: String("NOPE")})
	if len(r.Data) != 0 || r.Type != nil {
		t.Errorf("empty: %+v", r)
	}
	if _, err := f.client.Vehicles(context.Background(), &VehiclesOptions{VehicleType: String("truck")}); err == nil || len(f.calls) != 5 {
		t.Errorf("bad type must not request: %v", err)
	}
}

func TestVehicleLevelAndByID(t *testing.T) {
	f := newFake(t, []fakeResponse{
		level("vehicle", map[string]interface{}{"vehicle_id": 5434441, "select_text": "a", "description": "d", "long_description": "l"}, map[string]interface{}{"vehicle_id": 2573335, "select_text": "b"}),
		ok(map[string]interface{}{"success": true, "vehicle": record, "duration_ms": 2.2}, nil),
		ok(map[string]interface{}{"success": true, "vehicle": map[string]interface{}{"vehicle_id": 7295503, "vehicle_type": "car", "make": "TOYOTA", "model": "X", "years": []int{}, "detail": "basic", "series": nil}}, nil),
		res(404, map[string]interface{}{"success": false, "code": "not_found", "error": "No vehicle with that vehicle_id."}, nil),
	})
	r, err := f.client.Vehicles(context.Background(), &VehiclesOptions{Make: String("KIA"), Model: String("SELTOS"), Year: Int(2022), Series: String("EP81A"), Engine: String("2.0L"), Variant: String("x")})
	must(t, err)
	if ids := r.VehicleIDs(); len(ids) != 2 || ids[0] != 5434441 || ids[1] != 2573335 || deref(r.Vehicles[0].LongDescription) != "l" || r.Vehicles[1].Description != nil || len(r.Data) != 2 {
		t.Errorf("vehicle level: %+v", r)
	}
	rec, err := f.client.VehicleByID(context.Background(), 1380801)
	must(t, err)
	if f.calls[1].path != "/api/v1/vehicles/1380801" || len(f.calls[1].params) != 0 {
		t.Errorf("by-id request: %+v", f.calls[1])
	}
	if *rec.VehicleID != 1380801 || deref(rec.VehicleType) != "car" || deref(rec.Model) != "HILUX SR" || len(rec.Years) != 6 || *rec.Doors != 4 || deref(rec.Drive) != "4WD" || rec.BodySize != nil || deref(rec.Detail) != "full" || len(rec.Raw) == 0 {
		t.Errorf("record: %+v", rec)
	}
	rec, err = f.client.VehicleByID(context.Background(), 7295503)
	must(t, err)
	if deref(rec.Detail) != "basic" || len(rec.Years) != 0 || rec.Series != nil {
		t.Errorf("basic: %+v", rec)
	}
	_, err = f.client.VehicleByID(context.Background(), 1)
	var nf *NotFoundError
	if !errors.As(err, &nf) || nf.Code != "not_found" {
		t.Errorf("404: %T %v", err, err)
	}
	if _, err := f.client.VehicleByID(context.Background(), -1); err == nil || len(f.calls) != 4 {
		t.Error("negative id must not request")
	}
}

// ---- walker ----

func vehicle(id int, text string) map[string]interface{} {
	return map[string]interface{}{"vehicle_id": id, "select_text": text, "description": "desc", "long_description": "long"}
}

func hiluxResponses(trims bool) []fakeResponse {
	var r []fakeResponse
	if trims {
		r = append(r, level("model", "HILUX", "HILUX SR", "HILUXX", "CAMRY"))
	}
	r = append(r,
		level("year", 2020, 2019),
		level("series", "GUN126R"), level("engine", "2.8L DIE"), level("variant", "4D Ute"), level("vehicle", vehicle(100, "4D Ute")),
		level("series", "GUN126R"), level("engine", "2.8L DIE"), level("variant", "4D Ute"), level("vehicle", vehicle(100, "4D Ute")),
	)
	if trims {
		r = append(r,
			level("year", 2019), level("series", ""), level("engine", "2.8L DIE", "2.4L DIE"),
			level("variant", "SR 4D Ute"), level("vehicle", vehicle(200, "SR 4D Ute"), vehicle(201, "SR 4D Ute")),
			level("variant", "SR 4D Ute"), level("vehicle", vehicle(300, "SR 4D Ute")),
		)
	}
	return r
}

func collect(t *testing.T, f *fake, make, model string, opts *WalkOptions) ([]WalkedVehicle, error) {
	t.Helper()
	var out []WalkedVehicle
	err := f.client.WalkVehicles(context.Background(), make, model, opts, func(v WalkedVehicle) error {
		out = append(out, v)
		return nil
	})
	return out, err
}

func ids(vs []WalkedVehicle) []int {
	out := []int{}
	for _, v := range vs {
		out = append(out, v.VehicleID)
	}
	return out
}

func TestWalkWithTrims(t *testing.T) {
	responses := hiluxResponses(true)
	f := newFake(t, responses)
	found, err := collect(t, f, "TOYOTA", "HILUX", nil)
	must(t, err)
	if got := ids(found); len(got) != 4 || got[0] != 100 || got[1] != 200 || got[2] != 201 || got[3] != 300 {
		t.Fatalf("ids: %v", got)
	}
	first := found[0]
	if first.Make != "TOYOTA" || first.Model != "HILUX" || first.Year != 2020 || first.Series != "GUN126R" || first.Engine != "2.8L DIE" || first.Variant != "4D Ute" || deref(first.Description) != "desc" || first.VehicleType != "car" || first.Raw.VehicleID == nil {
		t.Errorf("first: %+v", first)
	}
	if found[1].Model != "HILUX SR" || found[1].Series != "" || found[3].Engine != "2.4L DIE" {
		t.Errorf("trim rows: %+v %+v", found[1], found[3])
	}
	if len(f.calls[0].params) != 1 || f.calls[0].params["make"] != "TOYOTA" {
		t.Errorf("first call: %+v", f.calls[0].params)
	}
	if p := f.calls[7].params; p["model"] != "HILUX" || p["year"] != "2019" || p["series"] != "GUN126R" {
		t.Errorf("call 7: %+v", p)
	}
	if _, has := f.calls[11].params["series"]; has {
		t.Errorf("call 11 must not send series: %+v", f.calls[11].params)
	}
	if v, has := f.calls[12].params["series"]; !has || v != "" {
		t.Errorf("call 12 must send empty series: %+v", f.calls[12].params)
	}
	if len(f.calls) != len(responses) || len(f.sleeps) != len(responses) || f.sleeps[0] != DefaultWalkDelay {
		t.Errorf("calls=%d sleeps=%d", len(f.calls), len(f.sleeps))
	}
}

func TestWalkOptions(t *testing.T) {
	f := newFake(t, hiluxResponses(false))
	found, err := collect(t, f, "TOYOTA", "HILUX", &WalkOptions{Trims: Bool(false)})
	must(t, err)
	if got := ids(found); len(got) != 1 || got[0] != 100 || f.calls[0].params["model"] != "HILUX" {
		t.Errorf("no trims: %v", got)
	}

	f = newFake(t, []fakeResponse{level("year", 2021, 2020, 2019), level("series", "S"), level("engine", "2.8L DIE", "2.4L DIE"), level("variant", "V"), level("vehicle", vehicle(1, "V"))})
	found, err = collect(t, f, "TOYOTA", "HILUX", &WalkOptions{Trims: Bool(false), YearFrom: Int(2020), YearTo: Int(2020), EngineContains: String("2.8l")})
	must(t, err)
	if got := ids(found); len(got) != 1 || got[0] != 1 || f.calls[3].params["engine"] != "2.8L DIE" {
		t.Errorf("filters: %v", got)
	}
	for _, c := range f.calls[1:] {
		if c.params["year"] != "2020" {
			t.Errorf("year filter leaked: %+v", c.params)
		}
	}

	f = newFake(t, hiluxResponses(false))
	found, err = collect(t, f, "TOYOTA", "HILUX", &WalkOptions{Trims: Bool(false), PerYear: true})
	must(t, err)
	if len(found) != 2 || found[0].Year != 2020 || found[1].Year != 2019 || found[1].VehicleID != 100 {
		t.Errorf("per year: %+v", found)
	}

	f = newFake(t, hiluxResponses(true))
	known := KnownIDs(100, 200)
	found, err = collect(t, f, "TOYOTA", "HILUX", &WalkOptions{Known: known})
	must(t, err)
	if got := ids(found); len(got) != 2 || got[0] != 201 || got[1] != 300 || len(known) != 4 || !known[WalkKey{VehicleID: 300}] {
		t.Errorf("resume: %v known=%v", got, known)
	}

	f = newFake(t, []fakeResponse{level("model", "MT-07"), level("year", 2022), level("series", "RM33"), level("engine", "0.7L"), level("variant", "MT-07"), level("vehicle", vehicle(7076553, "MT-07"))})
	found, err = collect(t, f, "YAMAHA", "MT-07", &WalkOptions{VehicleType: String("motorcycle"), Delay: Duration(0)})
	must(t, err)
	if found[0].VehicleType != "motorcycle" || len(f.sleeps) != 0 {
		t.Errorf("motorcycle: %+v sleeps=%d", found[0], len(f.sleeps))
	}
	for _, c := range f.calls {
		if c.params["type"] != "motorcycle" {
			t.Errorf("type missing: %+v", c.params)
		}
	}
}

func TestWalkErrorsAndStop(t *testing.T) {
	responses := hiluxResponses(false)
	responses[5] = res(429, map[string]interface{}{"detail": "Vehicle query rate limit exceeded. Try again later."}, map[string]string{"Retry-After": "7200"})
	f := newFake(t, responses)
	found, err := collect(t, f, "TOYOTA", "HILUX", &WalkOptions{Trims: Bool(false)})
	var rl *RateLimitError
	if !errors.As(err, &rl) || *rl.RetryAfter != 7200 || len(found) != 1 || found[0].VehicleID != 100 {
		t.Errorf("429 mid walk: %v found=%v", err, ids(found))
	}

	stop := errors.New("stop")
	f = newFake(t, hiluxResponses(true))
	n := 0
	err = f.client.WalkVehicles(context.Background(), "TOYOTA", "HILUX", nil, func(v WalkedVehicle) error {
		n++
		if n == 2 {
			return stop
		}
		return nil
	})
	if !errors.Is(err, stop) || n != 2 {
		t.Errorf("stop: %v n=%d", err, n)
	}

	f = newFake(t, nil)
	for _, tc := range []struct {
		make, model string
		opts        *WalkOptions
	}{
		{"", "HILUX", nil}, {"TOYOTA", " ", nil},
		{"TOYOTA", "HILUX", &WalkOptions{YearFrom: Int(2020), YearTo: Int(2019)}},
		{"TOYOTA", "HILUX", &WalkOptions{Delay: Duration(-1)}},
		{"TOYOTA", "HILUX", &WalkOptions{VehicleType: String("bus")}},
	} {
		if _, err := collect(t, f, tc.make, tc.model, tc.opts); err == nil {
			t.Errorf("expected validation error for %+v", tc)
		}
	}
	if len(f.calls) != 0 {
		t.Error("validation must not request")
	}
}

// ---- account ----

func TestUsageLogsHealth(t *testing.T) {
	f := newFake(t, []fakeResponse{
		ok(map[string]interface{}{"email": "a@b.c", "plan": "starter", "monthly_limit": 600, "used_this_month": 12, "remaining": 588, "percent_used": 2.0, "rate_limit_per_min": 10, "period_start": "2026-09-07 00:00:00", "topup_credits": 40}, nil),
		ok(map[string]interface{}{"logs": []interface{}{map[string]interface{}{"plate": "ABC123", "success": 1, "request_id": "req_1"}}, "count": 1, "total": 9, "limit": 50, "offset": 10}, nil),
		ok(map[string]interface{}{"status": "ok"}, nil),
		res(503, map[string]interface{}{"detail": "down"}, nil),
	})
	u, err := f.client.Usage(context.Background())
	must(t, err)
	if deref(u.Plan) != "starter" || u.MonthlyLimit != 600 || u.TopupCredits != 40 || u.PeriodEnd != nil {
		t.Errorf("usage: %+v", u)
	}
	logs, err := f.client.Logs(context.Background(), &LogsOptions{Limit: 50, Offset: 10, Since: String("2026-10-01T00:00:00"), Plate: String("ABC123"), Success: Bool(false)})
	must(t, err)
	p := f.calls[1].params
	if p["limit"] != "50" || p["offset"] != "10" || p["since"] != "2026-10-01T00:00:00" || p["plate"] != "ABC123" || p["success"] != "false" {
		t.Errorf("logs params: %+v", p)
	}
	if deref(logs.Logs[0].RequestID) != "req_1" || logs.Total != 9 {
		t.Errorf("logs: %+v", logs)
	}
	h, err := f.client.Health(context.Background())
	must(t, err)
	if h.Status != "ok" || f.calls[2].header.Get("X-API-Key") != "" {
		t.Errorf("health: %+v key=%q", h, f.calls[2].header.Get("X-API-Key"))
	}
	if _, err := f.client.Health(context.Background()); err == nil || len(f.calls) != 4 || len(f.sleeps) != 0 {
		t.Errorf("health must make one attempt: %v", err)
	}
}

func TestContextCancellation(t *testing.T) {
	c := NewClient("dummy", WithBaseURL("http://127.0.0.1:1"), WithMaxRetries(0))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Health(ctx); err == nil {
		t.Fatal("expected error with cancelled context")
	}
}

func TestHelperFunctions(t *testing.T) {
	if *String("hello") != "hello" || *Int(42) != 42 || *Bool(true) != true || *Duration(time.Second) != time.Second {
		t.Error("helpers")
	}
	if k := KnownIDs(1, 2); len(k) != 2 || !k[WalkKey{VehicleID: 2}] {
		t.Error("KnownIDs")
	}
}
