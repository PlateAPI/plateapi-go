package plateapi

import "fmt"

// APIError is the base of every error returned by the SDK. Message is the API's own text when
// it sent one, Code its machine code (not_found, lookup_failed, unavailable, quota_exceeded)
// and RequestID the request ID from the body or the X-Request-ID header.
type APIError struct {
	Message    string
	StatusCode int
	Code       string
	RequestID  string
}

func (e *APIError) Error() string {
	if e.StatusCode > 0 {
		return fmt.Sprintf("%s (status %d)", e.Message, e.StatusCode)
	}
	return e.Message
}

// BadRequestError is HTTP 400: invalid plate, state, cascade value or vehicle ID.
type BadRequestError struct {
	APIError
}

func (e *BadRequestError) Unwrap() error { return &e.APIError }

// AuthenticationError is HTTP 401: missing, invalid or deactivated API key, or email not verified.
type AuthenticationError struct {
	APIError
}

func (e *AuthenticationError) Unwrap() error { return &e.APIError }

// PermissionDeniedError is HTTP 403: IP not on the key's allowlist, account banned, or the
// vehicle database needs a Starter plan or above.
type PermissionDeniedError struct {
	APIError
}

func (e *PermissionDeniedError) Unwrap() error { return &e.APIError }

// NotFoundError is HTTP 404: no vehicle with that vehicle ID.
type NotFoundError struct {
	APIError
}

func (e *NotFoundError) Unwrap() error { return &e.APIError }

// RateLimitError is HTTP 429. RetryAfter is the server's Retry-After in seconds, if sent.
type RateLimitError struct {
	APIError
	RetryAfter *float64
}

func (e *RateLimitError) Unwrap() error { return &e.APIError }

// QuotaExceededError is HTTP 429 because the monthly lookup quota is used up. errors.As
// also matches it as a *RateLimitError.
type QuotaExceededError struct {
	RateLimitError
}

func (e *QuotaExceededError) Unwrap() error { return &e.RateLimitError }

// ServerError is HTTP 5xx after retries. Code is lookup_failed or unavailable when the API
// sent one.
type ServerError struct {
	APIError
	RetryAfter *float64
}

func (e *ServerError) Unwrap() error { return &e.APIError }
