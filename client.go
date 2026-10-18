package plateapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Version is the SDK version, sent as User-Agent: plateapi-go/<Version>.
const Version = "0.2.0"

const (
	// DefaultMaxWait is the longest Retry-After the SDK sleeps before giving up with an error.
	DefaultMaxWait = 60 * time.Second
	// DefaultWalkDelay is the pause after each request in WalkVehicles.
	DefaultWalkDelay = 150 * time.Millisecond

	userAgent = "plateapi-go/" + Version
)

var validStates = map[string]bool{
	"NSW": true, "VIC": true, "QLD": true, "SA": true,
	"WA": true, "TAS": true, "NT": true, "ACT": true,
}

var validVehicleTypes = map[string]bool{"car": true, "motorcycle": true}

type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
	maxRetries int
	maxWait    time.Duration
	sleep      func(ctx context.Context, d time.Duration)
}

type Option func(*Client)

func WithBaseURL(url string) Option {
	return func(c *Client) { c.baseURL = strings.TrimRight(url, "/") }
}

func WithTimeout(d time.Duration) Option {
	return func(c *Client) { c.httpClient.Timeout = d }
}

func WithMaxRetries(n int) Option {
	return func(c *Client) { c.maxRetries = n }
}

// WithMaxWait sets the longest Retry-After the SDK will sleep (default 60 s). A Retry-After
// above it returns the error at once instead of sleeping. d <= 0 removes the limit.
func WithMaxWait(d time.Duration) Option {
	return func(c *Client) { c.maxWait = d }
}

func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) { c.httpClient = hc }
}

func NewClient(apiKey string, opts ...Option) *Client {
	c := &Client{
		apiKey:     apiKey,
		baseURL:    "https://api.plateapi.com.au",
		httpClient: &http.Client{Timeout: 30 * time.Second},
		maxRetries: 3,
		maxWait:    DefaultMaxWait,
		sleep:      sleepDuration,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

type httpResponse struct {
	statusCode int
	body       []byte
	header     http.Header
}

func (c *Client) canWait(d time.Duration) bool {
	return c.maxWait <= 0 || d <= c.maxWait
}

func (c *Client) doRequest(ctx context.Context, path string, params url.Values, auth bool, maxRetries int) (*httpResponse, error) {
	u := c.baseURL + path
	if len(params) > 0 {
		u += "?" + params.Encode()
	}

	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, &APIError{Message: fmt.Sprintf("failed to create request: %v", err)}
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", userAgent)
		if auth {
			req.Header.Set("X-API-Key", c.apiKey)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			if ctx.Err() == nil && attempt < maxRetries {
				c.sleep(ctx, backoffDelay(attempt))
				continue
			}
			return nil, &APIError{Message: fmt.Sprintf("request failed: %v", err)}
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			if ctx.Err() == nil && attempt < maxRetries {
				c.sleep(ctx, backoffDelay(attempt))
				continue
			}
			return nil, &APIError{Message: fmt.Sprintf("failed to read response: %v", err)}
		}

		hr := &httpResponse{statusCode: resp.StatusCode, body: body, header: resp.Header}
		if resp.StatusCode < 400 {
			return hr, nil
		}

		var parsed map[string]interface{}
		if json.Unmarshal(body, &parsed) != nil {
			parsed = nil
		}
		base := APIError{
			Message:    errorMessage(parsed, resp.StatusCode),
			StatusCode: resp.StatusCode,
			Code:       stringField(parsed, "code"),
			RequestID:  stringField(parsed, "request_id"),
		}
		if base.RequestID == "" {
			base.RequestID = resp.Header.Get("X-Request-ID")
		}
		retryAfter := parseRetryAfterPtr(resp.Header.Get("Retry-After"))

		if resp.StatusCode == 429 {
			if isQuotaExceeded(parsed) {
				base.Code = "quota_exceeded"
				return nil, &QuotaExceededError{RateLimitError: RateLimitError{APIError: base, RetryAfter: retryAfter}}
			}
			wait := backoffDelay(attempt)
			if retryAfter != nil {
				wait = time.Duration(*retryAfter * float64(time.Second))
			}
			if attempt < maxRetries && c.canWait(wait) {
				c.sleep(ctx, wait)
				if ctx.Err() != nil {
					return nil, &APIError{Message: fmt.Sprintf("request failed: %v", ctx.Err())}
				}
				continue
			}
			return nil, &RateLimitError{APIError: base, RetryAfter: retryAfter}
		}

		if resp.StatusCode >= 500 {
			wait := backoffDelay(attempt)
			if retryAfter != nil {
				wait = time.Duration(*retryAfter * float64(time.Second))
			}
			if attempt < maxRetries && c.canWait(wait) {
				c.sleep(ctx, wait)
				if ctx.Err() != nil {
					return nil, &APIError{Message: fmt.Sprintf("request failed: %v", ctx.Err())}
				}
				continue
			}
			return nil, &ServerError{APIError: base, RetryAfter: retryAfter}
		}

		switch resp.StatusCode {
		case 400:
			return nil, &BadRequestError{APIError: base}
		case 401:
			return nil, &AuthenticationError{APIError: base}
		case 403:
			return nil, &PermissionDeniedError{APIError: base}
		case 404:
			if base.Code == "" {
				base.Code = "not_found"
			}
			return nil, &NotFoundError{APIError: base}
		default:
			return nil, &base
		}
	}
}

// Lookup resolves a plate. A plate that does not resolve is a normal result with Success
// false (and Code "not_found"), not an error.
func (c *Client) Lookup(ctx context.Context, plate, state string, detailed bool) (*LookupResult, error) {
	s := strings.ToUpper(strings.TrimSpace(state))
	p := strings.ToUpper(strings.TrimSpace(plate))

	if !validStates[s] {
		return nil, fmt.Errorf("invalid state %q, must be one of: ACT, NSW, NT, QLD, SA, TAS, VIC, WA", s)
	}
	if p == "" {
		return nil, fmt.Errorf("plate cannot be empty")
	}

	params := url.Values{"plate": {p}, "state": {s}}
	if detailed {
		params.Set("detailed", "true")
	}

	var resp *httpResponse
	var result LookupResult
	for attempt := 0; ; attempt++ {
		var err error
		resp, err = c.doRequest(ctx, "/api/v1/lookup", params, true, c.maxRetries)
		if err != nil {
			return nil, err
		}
		result = LookupResult{}
		if err := json.Unmarshal(resp.body, &result); err != nil {
			return nil, &APIError{Message: fmt.Sprintf("failed to parse response: %v", err)}
		}
		// internal_error is a 200 that is not billed; one more try usually succeeds.
		if !result.Success && result.Code != nil && *result.Code == "internal_error" && attempt < c.maxRetries {
			c.sleep(ctx, backoffDelay(attempt))
			if ctx.Err() != nil {
				return nil, &APIError{Message: fmt.Sprintf("request failed: %v", ctx.Err())}
			}
			continue
		}
		break
	}

	if result.Vehicle != nil && result.Vehicle.LowestYear != nil {
		result.Vehicle.Year = result.Vehicle.LowestYear
	}
	for i := range result.Alternatives {
		if result.Alternatives[i].LowestYear != nil {
			result.Alternatives[i].Year = result.Alternatives[i].LowestYear
		}
	}
	if result.RequestID == nil {
		if rid := resp.header.Get("X-Request-ID"); rid != "" {
			result.RequestID = &rid
		}
	}

	result.RateLimit = parseRateLimit(resp.header)
	return &result, nil
}

// Vehicles returns one level of the vehicle database cascade.
func (c *Client) Vehicles(ctx context.Context, opts *VehiclesOptions) (*VehiclesResult, error) {
	params := url.Values{}
	if opts != nil {
		if opts.VehicleType != nil {
			if !validVehicleTypes[*opts.VehicleType] {
				return nil, fmt.Errorf("invalid vehicle type %q, must be one of: car, motorcycle", *opts.VehicleType)
			}
			params.Set("type", *opts.VehicleType)
		}
		if opts.Make != nil {
			params.Set("make", *opts.Make)
		}
		if opts.Model != nil {
			params.Set("model", *opts.Model)
		}
		if opts.Year != nil {
			params.Set("year", strconv.Itoa(*opts.Year))
		}
		if opts.Series != nil {
			params.Set("series", *opts.Series)
		}
		if opts.Engine != nil {
			params.Set("engine", *opts.Engine)
		}
		if opts.Variant != nil {
			params.Set("variant", *opts.Variant)
		}
	}

	resp, err := c.doRequest(ctx, "/api/v1/vehicles", params, true, c.maxRetries)
	if err != nil {
		return nil, err
	}

	var raw struct {
		Success    bool     `json:"success"`
		Total      int      `json:"total"`
		DurationMs *float64 `json:"duration_ms"`
		Data       []struct {
			Type string            `json:"type"`
			Data []json.RawMessage `json:"data"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.body, &raw); err != nil {
		return nil, &APIError{Message: fmt.Sprintf("failed to parse response: %v", err)}
	}

	result := &VehiclesResult{
		Success:    raw.Success,
		Total:      raw.Total,
		DurationMs: raw.DurationMs,
		Data:       []string{},
		Vehicles:   []CascadeVehicle{},
	}

	if len(raw.Data) > 0 {
		t := raw.Data[0].Type
		result.Type = &t
		for _, item := range raw.Data[0].Data {
			var s string
			if json.Unmarshal(item, &s) == nil {
				result.Data = append(result.Data, s)
				continue
			}
			result.Data = append(result.Data, string(item))
			if len(item) > 0 && item[0] == '{' {
				var v CascadeVehicle
				if json.Unmarshal(item, &v) == nil {
					v.Raw = item
					result.Vehicles = append(result.Vehicles, v)
				}
			}
		}
	}

	return result, nil
}

// VehicleByID returns one vehicle record. An unknown ID returns a *NotFoundError.
func (c *Client) VehicleByID(ctx context.Context, vehicleID int) (*VehicleRecord, error) {
	if vehicleID < 0 {
		return nil, fmt.Errorf("vehicle ID must be a non-negative integer")
	}
	resp, err := c.doRequest(ctx, "/api/v1/vehicles/"+strconv.Itoa(vehicleID), nil, true, c.maxRetries)
	if err != nil {
		return nil, err
	}
	var raw struct {
		Vehicle json.RawMessage `json:"vehicle"`
	}
	if err := json.Unmarshal(resp.body, &raw); err != nil {
		return nil, &APIError{Message: fmt.Sprintf("failed to parse response: %v", err)}
	}
	record := &VehicleRecord{Years: []int{}}
	if len(raw.Vehicle) > 0 && string(raw.Vehicle) != "null" {
		if err := json.Unmarshal(raw.Vehicle, record); err != nil {
			return nil, &APIError{Message: fmt.Sprintf("failed to parse response: %v", err)}
		}
		if record.Years == nil {
			record.Years = []int{}
		}
		record.Raw = raw.Vehicle
	}
	return record, nil
}

func (c *Client) Usage(ctx context.Context) (*Usage, error) {
	resp, err := c.doRequest(ctx, "/api/v1/keys/usage", nil, true, c.maxRetries)
	if err != nil {
		return nil, err
	}

	var result Usage
	if err := json.Unmarshal(resp.body, &result); err != nil {
		return nil, &APIError{Message: fmt.Sprintf("failed to parse response: %v", err)}
	}
	return &result, nil
}

func (c *Client) Logs(ctx context.Context, opts *LogsOptions) (*LogsResult, error) {
	limit := 100
	offset := 0
	if opts != nil {
		if opts.Limit > 0 {
			limit = opts.Limit
		}
		offset = opts.Offset
	}

	params := url.Values{
		"limit":  {strconv.Itoa(limit)},
		"offset": {strconv.Itoa(offset)},
	}
	if opts != nil {
		if opts.Since != nil {
			params.Set("since", *opts.Since)
		}
		if opts.Until != nil {
			params.Set("until", *opts.Until)
		}
		if opts.Plate != nil {
			params.Set("plate", *opts.Plate)
		}
		if opts.Success != nil {
			if *opts.Success {
				params.Set("success", "true")
			} else {
				params.Set("success", "false")
			}
		}
	}

	resp, err := c.doRequest(ctx, "/api/v1/keys/logs", params, true, c.maxRetries)
	if err != nil {
		return nil, err
	}

	var result LogsResult
	if err := json.Unmarshal(resp.body, &result); err != nil {
		return nil, &APIError{Message: fmt.Sprintf("failed to parse response: %v", err)}
	}
	return &result, nil
}

// Health checks the API. No key is sent, no quota is used, and a single attempt is made.
func (c *Client) Health(ctx context.Context) (*HealthStatus, error) {
	resp, err := c.doRequest(ctx, "/api/v1/health", nil, false, 0)
	if err != nil {
		return nil, err
	}

	var result HealthStatus
	if err := json.Unmarshal(resp.body, &result); err != nil {
		return nil, &APIError{Message: fmt.Sprintf("failed to parse response: %v", err)}
	}
	return &result, nil
}

func headerInt(h http.Header, name string) *int {
	v := h.Get(name)
	if v == "" || v == "unlimited" {
		return nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return nil
	}
	return &n
}

func parseRateLimit(h http.Header) RateLimit {
	rl := RateLimit{
		Limit:          headerInt(h, "X-RateLimit-Limit"),
		Remaining:      headerInt(h, "X-RateLimit-Remaining"),
		TopupRemaining: headerInt(h, "X-Topup-Remaining"),
	}
	if v := h.Get("X-RateLimit-Plan"); v != "" {
		rl.Plan = &v
	}
	return rl
}

func stringField(parsed map[string]interface{}, key string) string {
	if parsed == nil {
		return ""
	}
	if s, ok := parsed[key].(string); ok {
		return s
	}
	return ""
}

func errorMessage(parsed map[string]interface{}, status int) string {
	for _, key := range []string{"detail", "error"} {
		if s := stringField(parsed, key); s != "" {
			return s
		}
	}
	return fmt.Sprintf("Request failed (HTTP %d)", status)
}

func isQuotaExceeded(parsed map[string]interface{}) bool {
	if stringField(parsed, "code") == "quota_exceeded" {
		return true
	}
	return strings.HasPrefix(strings.ToLower(stringField(parsed, "detail")), "monthly quota exceeded")
}

func backoffDelay(attempt int) time.Duration {
	base := math.Pow(2, float64(attempt))
	jitter := rand.Float64() * 0.5
	return time.Duration((base + jitter) * float64(time.Second))
}

func sleepDuration(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

func parseRetryAfterPtr(s string) *float64 {
	if s == "" {
		return nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || f < 0 {
		return nil
	}
	return &f
}

func String(s string) *string { return &s }
func Int(i int) *int          { return &i }
func Bool(b bool) *bool       { return &b }

// Duration returns a pointer to d, for WalkOptions.Delay.
func Duration(d time.Duration) *time.Duration { return &d }

// KnownIDs builds a WalkOptions.Known map from vehicle IDs (for walks without PerYear).
func KnownIDs(ids ...int) map[WalkKey]bool {
	m := make(map[WalkKey]bool, len(ids))
	for _, id := range ids {
		m[WalkKey{VehicleID: id}] = true
	}
	return m
}
