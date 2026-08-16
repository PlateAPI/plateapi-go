package plateapi

import "fmt"

type APIError struct {
	Message    string
	StatusCode int
}

func (e *APIError) Error() string {
	if e.StatusCode > 0 {
		return fmt.Sprintf("%s (status %d)", e.Message, e.StatusCode)
	}
	return e.Message
}

type AuthenticationError struct {
	APIError
}

type RateLimitError struct {
	APIError
	RetryAfter *float64
}

type QuotaExceededError struct {
	APIError
}

type ServerError struct {
	APIError
	RetryAfter *float64
}
