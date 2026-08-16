package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	plateapi "github.com/PlateAPI/plateapi-go"
)

func main() {
	apiKey := os.Getenv("PLATEAPI_KEY")
	if apiKey == "" {
		apiKey = "pk_live_your_api_key"
	}

	client := plateapi.NewClient(apiKey)
	ctx := context.Background()

	health, err := client.Health(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Health check failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("API status: %s\n", health.Status)

	result, err := client.Lookup(ctx, "TEST123", "VIC", false)
	if err != nil {
		var authErr *plateapi.AuthenticationError
		var quotaErr *plateapi.QuotaExceededError
		var rateErr *plateapi.RateLimitError
		var serverErr *plateapi.ServerError

		switch {
		case errors.As(err, &authErr):
			fmt.Fprintf(os.Stderr, "Invalid API key\n")
		case errors.As(err, &quotaErr):
			fmt.Fprintf(os.Stderr, "Monthly quota exceeded\n")
		case errors.As(err, &rateErr):
			fmt.Fprintf(os.Stderr, "Rate limited")
			if rateErr.RetryAfter != nil {
				fmt.Fprintf(os.Stderr, " (retry after %.0fs)", *rateErr.RetryAfter)
			}
			fmt.Fprintln(os.Stderr)
		case errors.As(err, &serverErr):
			fmt.Fprintf(os.Stderr, "Server error: %v\n", serverErr)
		default:
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}
		os.Exit(1)
	}

	if result.Success && result.Vehicle != nil {
		v := result.Vehicle
		fmt.Printf("Make: %s\n", deref(v.Make))
		fmt.Printf("Model: %s\n", deref(v.Model))
		fmt.Printf("Year range: %s\n", deref(v.YearRange))
		fmt.Printf("Body: %s\n", deref(v.Body))
		fmt.Printf("Engine: %s\n", deref(v.Engine))
		if result.DurationMs != nil {
			fmt.Printf("Duration: %.1fms\n", *result.DurationMs)
		}
		fmt.Printf("Request ID: %s\n", deref(result.RequestID))
	}

	for _, alt := range result.Alternatives {
		fmt.Printf("Also matched: %s %s (%s)\n",
			deref(alt.Make), deref(alt.Model), deref(alt.YearRange))
	}

	if result.RateLimit.Remaining != nil {
		fmt.Printf("Lookups remaining: %d\n", *result.RateLimit.Remaining)
	}
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
