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

var validStates = map[string]bool{
	"NSW": true, "VIC": true, "QLD": true, "SA": true,
	"WA": true, "TAS": true, "NT": true, "ACT": true,
}

type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
	maxRetries int
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

func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) { c.httpClient = hc }
}

func NewClient(apiKey string, opts ...Option) *Client {
	c := &Client{
		apiKey:     apiKey,
		baseURL:    "https://api.plateapi.com.au",
		httpClient: &http.Client{Timeout: 30 * time.Second},
		maxRetries: 3,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

type httpResponse struct {
	statusCode           int
	body                 []byte
	rateLimitLimit       string
	rateLimitRemaining   string
	rateLimitPlan        string
	retryAfter           string
}

func (c *Client) doRequest(ctx context.Context, path string, params url.Values, auth bool) (*httpResponse, error) {
	u := c.baseURL + path
	if len(params) > 0 {
		u += "?" + params.Encode()
	}

	var lastErr error

	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, &APIError{Message: fmt.Sprintf("failed to create request: %v", err)}
		}
		req.Header.Set("Accept", "application/json")
		if auth {
			req.Header.Set("X-API-Key", c.apiKey)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = &APIError{Message: fmt.Sprintf("request failed: %v", err)}
			if attempt < c.maxRetries {
				sleepWithJitter(ctx, attempt)
				continue
			}
			return nil, lastErr
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = &APIError{Message: fmt.Sprintf("failed to read response: %v", err)}
			if attempt < c.maxRetries {
				sleepWithJitter(ctx, attempt)
				continue
			}
			return nil, lastErr
		}

		hr := &httpResponse{
			statusCode:         resp.StatusCode,
			body:               body,
			rateLimitLimit:     resp.Header.Get("X-RateLimit-Limit"),
			rateLimitRemaining: resp.Header.Get("X-RateLimit-Remaining"),
			rateLimitPlan:      resp.Header.Get("X-RateLimit-Plan"),
			retryAfter:         resp.Header.Get("Retry-After"),
		}

		if resp.StatusCode == 401 {
			return nil, &AuthenticationError{APIError: APIError{Message: "Invalid API key", StatusCode: 401}}
		}

		if resp.StatusCode == 429 {
			var parsed map[string]interface{}
			json.Unmarshal(body, &parsed)
			if code, ok := parsed["code"].(string); ok && code == "quota_exceeded" {
				return nil, &QuotaExceededError{APIError: APIError{Message: "Monthly quota exceeded", StatusCode: 429}}
			}

			if attempt < c.maxRetries {
				wait := backoffDelay(attempt)
				if ra := parseRetryAfter(hr.retryAfter); ra > 0 {
					wait = ra
				}
				sleepDuration(ctx, wait)
				continue
			}

			ra := parseRetryAfterPtr(hr.retryAfter)
			return nil, &RateLimitError{
				APIError:   APIError{Message: "Rate limit exceeded", StatusCode: 429},
				RetryAfter: ra,
			}
		}

		if resp.StatusCode >= 500 {
			if attempt < c.maxRetries {
				wait := backoffDelay(attempt)
				if ra := parseRetryAfter(hr.retryAfter); ra > 0 {
					wait = ra
				}
				sleepDuration(ctx, wait)
				continue
			}

			ra := parseRetryAfterPtr(hr.retryAfter)
			return nil, &ServerError{
				APIError:   APIError{Message: fmt.Sprintf("Server error (%d)", resp.StatusCode), StatusCode: resp.StatusCode},
				RetryAfter: ra,
			}
		}

		if resp.StatusCode >= 400 {
			msg := "Request failed"
			var parsed map[string]interface{}
			if json.Unmarshal(body, &parsed) == nil {
				if detail, ok := parsed["detail"].(string); ok {
					msg = detail
				} else if errMsg, ok := parsed["error"].(string); ok {
					msg = errMsg
				}
			}
			return nil, &APIError{Message: msg, StatusCode: resp.StatusCode}
		}

		return hr, nil
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, &APIError{Message: "request failed"}
}

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

	resp, err := c.doRequest(ctx, "/api/v1/lookup", params, true)
	if err != nil {
		return nil, err
	}

	var result LookupResult
	if err := json.Unmarshal(resp.body, &result); err != nil {
		return nil, &APIError{Message: fmt.Sprintf("failed to parse response: %v", err)}
	}

	if result.Vehicle != nil && result.Vehicle.LowestYear != nil {
		result.Vehicle.Year = result.Vehicle.LowestYear
	}
	for i := range result.Alternatives {
		if result.Alternatives[i].LowestYear != nil {
			result.Alternatives[i].Year = result.Alternatives[i].LowestYear
		}
	}

	result.RateLimit = parseRateLimit(resp)
	return &result, nil
}

func (c *Client) Vehicles(ctx context.Context, opts *VehiclesOptions) (*VehiclesResult, error) {
	params := url.Values{}
	if opts != nil {
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

	resp, err := c.doRequest(ctx, "/api/v1/vehicles", params, true)
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
	}

	if len(raw.Data) > 0 {
		t := raw.Data[0].Type
		result.Type = &t
		for _, item := range raw.Data[0].Data {
			var s string
			if json.Unmarshal(item, &s) == nil {
				result.Data = append(result.Data, s)
			} else {
				result.Data = append(result.Data, string(item))
			}
		}
	}

	return result, nil
}

func (c *Client) Usage(ctx context.Context) (*Usage, error) {
	resp, err := c.doRequest(ctx, "/api/v1/keys/usage", nil, true)
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

	resp, err := c.doRequest(ctx, "/api/v1/keys/logs", params, true)
	if err != nil {
		return nil, err
	}

	var result LogsResult
	if err := json.Unmarshal(resp.body, &result); err != nil {
		return nil, &APIError{Message: fmt.Sprintf("failed to parse response: %v", err)}
	}
	return &result, nil
}

func (c *Client) Health(ctx context.Context) (*HealthStatus, error) {
	resp, err := c.doRequest(ctx, "/api/v1/health", nil, false)
	if err != nil {
		return nil, err
	}

	var result HealthStatus
	if err := json.Unmarshal(resp.body, &result); err != nil {
		return nil, &APIError{Message: fmt.Sprintf("failed to parse response: %v", err)}
	}
	return &result, nil
}

func parseRateLimit(resp *httpResponse) RateLimit {
	rl := RateLimit{}
	if v := resp.rateLimitLimit; v != "" && v != "unlimited" {
		if n, err := strconv.Atoi(v); err == nil {
			rl.Limit = &n
		}
	}
	if v := resp.rateLimitRemaining; v != "" && v != "unlimited" {
		if n, err := strconv.Atoi(v); err == nil {
			rl.Remaining = &n
		}
	}
	if v := resp.rateLimitPlan; v != "" {
		rl.Plan = &v
	}
	return rl
}

func backoffDelay(attempt int) time.Duration {
	base := math.Pow(2, float64(attempt))
	jitter := rand.Float64() * 0.5
	return time.Duration((base + jitter) * float64(time.Second))
}

func sleepWithJitter(ctx context.Context, attempt int) {
	sleepDuration(ctx, backoffDelay(attempt))
}

func sleepDuration(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

func parseRetryAfter(s string) time.Duration {
	if s == "" {
		return 0
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return time.Duration(f * float64(time.Second))
	}
	return 0
}

func parseRetryAfterPtr(s string) *float64 {
	if s == "" {
		return nil
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return &f
	}
	return nil
}

func String(s string) *string { return &s }
func Int(i int) *int          { return &i }
func Bool(b bool) *bool       { return &b }
