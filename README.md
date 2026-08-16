# PlateAPI Go SDK

Go client for [PlateAPI](https://plateapi.com.au) -- Australian vehicle registration plate lookup.

## Install

```bash
go get github.com/PlateAPI/plateapi-go
```

Requires Go 1.21+. Zero external dependencies -- stdlib only.

## Quick start

```go
package main

import (
    "context"
    "fmt"
    plateapi "github.com/PlateAPI/plateapi-go"
)

func main() {
    client := plateapi.NewClient("pk_live_your_api_key")

    result, err := client.Lookup(context.Background(), "ABC123", "VIC", false)
    if err != nil {
        panic(err)
    }
    if result.Success && result.Vehicle != nil {
        fmt.Printf("%s %s\n", *result.Vehicle.Make, *result.Vehicle.Model)
    }
}
```

## Plate lookup

```go
result, err := client.Lookup(ctx, "ABC123", "VIC", false)

fmt.Println(result.Success)                            // true
fmt.Println(*result.Vehicle.Make)                      // "TOYOTA"
fmt.Println(*result.Vehicle.Model)                     // "HILUX"
fmt.Println(*result.Vehicle.LowestYear)                // 2015
fmt.Println(*result.Vehicle.HighestYear)               // 2023
fmt.Println(*result.Vehicle.YearRange)                 // "2015 - 2023"
fmt.Println(*result.Vehicle.Body)                      // "UTILITY"
fmt.Println(*result.Vehicle.Engine)                    // "2.8L"
fmt.Println(*result.Vehicle.Description)               // "TOYOTA HILUX UTILITY 2.8L"
fmt.Println(*result.DurationMs)                        // 2451.3
fmt.Println(*result.Source)                            // "plateapi"
fmt.Println(*result.RequestID)                         // "req_7f3a9c1b4e..."
```

Valid states: `NSW`, `VIC`, `QLD`, `SA`, `WA`, `TAS`, `NT`, `ACT`.

## Detailed lookup

```go
result, err := client.Lookup(ctx, "ABC123", "NSW", true)
if result.Success {
    fmt.Println(*result.Vehicle.DetailedDescription)
    fmt.Println(*result.Vehicle.Series)
}
```

## Multiple matches

```go
result, _ := client.Lookup(ctx, "ABC123", "VIC", false)
for _, alt := range result.Alternatives {
    fmt.Printf("Also matched: %s %s (%s)\n",
        *alt.Make, *alt.Model, *alt.YearRange)
}
```

## Vehicle database

Browse the full vehicle database (32,000+ vehicles, 213 makes). Each call narrows the cascade through all 7 levels. Paid plans only, no quota consumed.

```go
// Step 1: All makes
makes, _ := client.Vehicles(ctx, nil)
// makes.Type == "make", makes.Data == ["ABARTH", "AC", ...]

// Step 2: Models for a make
models, _ := client.Vehicles(ctx, &plateapi.VehiclesOptions{
    Make: plateapi.String("TOYOTA"),
})

// Step 3: Years
years, _ := client.Vehicles(ctx, &plateapi.VehiclesOptions{
    Make:  plateapi.String("TOYOTA"),
    Model: plateapi.String("HILUX"),
})

// Step 4: Series
series, _ := client.Vehicles(ctx, &plateapi.VehiclesOptions{
    Make:  plateapi.String("TOYOTA"),
    Model: plateapi.String("HILUX"),
    Year:  plateapi.Int(2020),
})

// Step 5: Engines
engines, _ := client.Vehicles(ctx, &plateapi.VehiclesOptions{
    Make:   plateapi.String("TOYOTA"),
    Model:  plateapi.String("HILUX"),
    Year:   plateapi.Int(2020),
    Series: plateapi.String("SR5"),
})

// Step 6: Variants
variants, _ := client.Vehicles(ctx, &plateapi.VehiclesOptions{
    Make:   plateapi.String("TOYOTA"),
    Model:  plateapi.String("HILUX"),
    Year:   plateapi.Int(2020),
    Series: plateapi.String("SR5"),
    Engine: plateapi.String("2.8L"),
})

// Step 7: Full vehicle details
vehicles, _ := client.Vehicles(ctx, &plateapi.VehiclesOptions{
    Make:    plateapi.String("TOYOTA"),
    Model:   plateapi.String("HILUX"),
    Year:    plateapi.Int(2020),
    Series:  plateapi.String("SR5"),
    Engine:  plateapi.String("2.8L"),
    Variant: plateapi.String("4x4 Double Cab"),
})
```

For vehicles without a series code, pass an empty string:

```go
result, _ := client.Vehicles(ctx, &plateapi.VehiclesOptions{
    Make:   plateapi.String("TOYOTA"),
    Model:  plateapi.String("HILUX"),
    Year:   plateapi.Int(2020),
    Series: plateapi.String(""),
})
```

## Check usage

```go
usage, _ := client.Usage(ctx)
fmt.Printf("%d/%d lookups used\n", usage.UsedThisMonth, usage.MonthlyLimit)
fmt.Printf("%d remaining\n", usage.Remaining)
fmt.Printf("%.1f%% used\n", *usage.PercentUsed)
fmt.Printf("Plan: %s\n", *usage.Plan)
fmt.Printf("Rate limit: %d/min\n", usage.RateLimitPerMin)
fmt.Printf("Period: %s to %s\n", *usage.PeriodStart, *usage.PeriodEnd)
fmt.Printf("Days remaining: %d\n", *usage.DaysRemaining)
fmt.Printf("Top-up credits: %d\n", usage.TopupCredits)
```

## Request logs

```go
// Last 10 lookups
logs, _ := client.Logs(ctx, &plateapi.LogsOptions{Limit: 10})
for _, entry := range logs.Logs {
    fmt.Printf("%s | %s (%s) | %v | %.0fms\n",
        *entry.CreatedAt, *entry.Plate, *entry.State,
        entry.Success != 0, *entry.DurationMs)
}
fmt.Printf("Showing %d of %d total\n", logs.Count, logs.Total)
```

### Filtering

```go
// Filter by plate
plateLogs, _ := client.Logs(ctx, &plateapi.LogsOptions{Plate: plateapi.String("ABC123")})

// Only failed lookups
failed, _ := client.Logs(ctx, &plateapi.LogsOptions{Success: plateapi.Bool(false)})

// Time range
july, _ := client.Logs(ctx, &plateapi.LogsOptions{
    Since: plateapi.String("2026-07-01T00:00:00"),
    Until: plateapi.String("2026-07-31T23:59:59"),
})

// Pagination
page1, _ := client.Logs(ctx, &plateapi.LogsOptions{Limit: 50, Offset: 0})
page2, _ := client.Logs(ctx, &plateapi.LogsOptions{Limit: 50, Offset: 50})
```

## Health check

No authentication required, no quota consumed.

```go
health, _ := client.Health(ctx)
fmt.Println(health.Status) // "ok"
```

## Rate limits

```go
result, _ := client.Lookup(ctx, "ABC123", "VIC", false)
if result.RateLimit.Remaining != nil {
    fmt.Printf("Lookups remaining: %d\n", *result.RateLimit.Remaining)
}
```

## Error handling

```go
import "errors"

result, err := client.Lookup(ctx, "ABC123", "VIC", false)
if err != nil {
    var authErr *plateapi.AuthenticationError
    var quotaErr *plateapi.QuotaExceededError
    var rateErr *plateapi.RateLimitError
    var serverErr *plateapi.ServerError
    var apiErr *plateapi.APIError

    switch {
    case errors.As(err, &authErr):
        fmt.Println("Invalid API key")
    case errors.As(err, &quotaErr):
        fmt.Println("Monthly quota exceeded")
    case errors.As(err, &rateErr):
        fmt.Print("Rate limited")
        if rateErr.RetryAfter != nil {
            fmt.Printf(", retry after %.0fs", *rateErr.RetryAfter)
        }
        fmt.Println()
    case errors.As(err, &serverErr):
        fmt.Printf("Server error (%d)\n", serverErr.StatusCode)
    case errors.As(err, &apiErr):
        fmt.Printf("API error: %s (status %d)\n", apiErr.Message, apiErr.StatusCode)
    default:
        fmt.Printf("Error: %v\n", err)
    }
}
```

## Retry behaviour

The SDK automatically retries on:
- Connection errors
- Timeouts
- 429 rate limit responses (waits for Retry-After header)
- 5xx server errors

Default: 3 retries with exponential backoff and jitter. Configure with:

```go
client := plateapi.NewClient("pk_live_your_api_key",
    plateapi.WithMaxRetries(5),
    plateapi.WithTimeout(60 * time.Second),
)
```

## Context support

All methods accept `context.Context` for cancellation and timeouts:

```go
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()

result, err := client.Lookup(ctx, "ABC123", "VIC", false)
```

## Custom HTTP client

```go
client := plateapi.NewClient("pk_live_your_api_key",
    plateapi.WithHTTPClient(&http.Client{
        Transport: &http.Transport{
            MaxIdleConns: 10,
        },
    }),
)
```

## Sandbox

Use plate `TEST123` with any state for testing. Returns a fixed response instantly, no quota consumed.

```go
result, _ := client.Lookup(ctx, "TEST123", "VIC", false)
// result.Sandbox == true
// result.Success == true
// result.Vehicle.Make == "TOYOTA"
```

## Helper functions

The SDK provides pointer helpers for building options:

```go
plateapi.String("TOYOTA")  // *string
plateapi.Int(2020)          // *int
plateapi.Bool(true)         // *bool
```

## Links

- [API Documentation](https://plateapi.com.au/docs)
- [Pricing](https://plateapi.com.au/pricing)
- [Dashboard](https://plateapi.com.au/dashboard)
- [Sign up for free](https://plateapi.com.au/register)
- [Status page](https://plateapi.com.au/status)
- [GitHub](https://github.com/PlateAPI)
