# PlateAPI Go SDK

Go client for [PlateAPI](https://plateapi.com.au) -- Australian vehicle registration plate
lookup and the PlateAPI vehicle database.

## Install

```bash
go get github.com/PlateAPI/plateapi-go@v0.2.0
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
        fmt.Println(*result.Vehicle.Make, *result.Vehicle.Model, *result.Vehicle.YearRange)
        fmt.Println(*result.Vehicle.VehicleID)   // PlateAPI's stable ID for this variant
        fmt.Println(*result.Match)               // "exact", "multiple" or "ambiguous"
    }
}
```

Get a free API key (20 lookups a month, no card) at [plateapi.com.au/register](https://plateapi.com.au/register/).
Full API reference: [plateapi.com.au/docs](https://plateapi.com.au/docs/).

Nullable fields are pointers (`nil` when the API sent `null` or nothing); the `String`, `Int`,
`Bool` and `Duration` helpers build option values.

## Plate lookup

```go
result, err := client.Lookup(ctx, "ABC123", "VIC", false)

result.Success                    // true when a vehicle was found
*result.Match                     // "exact" | "multiple" | "ambiguous" (nil when not found)
*result.Vehicle.Make              // "TOYOTA"
*result.Vehicle.Model             // "HILUX"
*result.Vehicle.YearRange         // "2015-2020" ("2020-ON" for models still in production)
*result.Vehicle.LowestYear        // 2015
*result.Vehicle.HighestYear       // 2020 (nil for an open range)
*result.Vehicle.Year              // same as LowestYear
*result.Vehicle.Body              // "UTE" (may be nil)
*result.Vehicle.Engine            // "2.8L" (may be nil)
*result.Vehicle.Description       // "15~20 TOYOTA HILUX 2.8L DIESEL"
*result.Vehicle.VehicleID         // 1380801 (nil when the variant is not certain)
result.Alternatives               // other candidate variants, same fields, each with its own VehicleID
result.CandidateIDs()             // every non-nil VehicleID, primary first: [1380801]
*result.RequestID                 // "req_7f3a9c1b4e8d2f6a0c5e" -- quote this when contacting support
*result.DurationMs                // 1509.96
result.Sandbox                    // true only for the TEST123 sandbox plate
```

Valid states: `NSW`, `VIC`, `QLD`, `SA`, `WA`, `TAS`, `NT`, `ACT`. Plate and state are
upper-cased and trimmed for you; an unknown state returns an error before any request.

### Not found

A plate that does not resolve is a normal result, not an error. It is still HTTP 200 and it
still counts against your monthly quota (typos are common, so validate input first).

```go
result, err := client.Lookup(ctx, "ZZZ999", "NSW", false)
if err == nil && !result.Success {
    fmt.Println(*result.Code)    // "not_found"
    fmt.Println(*result.Error)   // human-readable message
}
```

### `VehicleID` and `Match`

Every vehicle in a lookup carries `VehicleID`: PlateAPI's ID for the exact variant. It is stable
(the same variant always returns the same ID) and it is the same ID the vehicle database uses, so
you can link your own records (products, SKUs, fitment rows) to it. It is `nil` when the variant
could not be identified with certainty; an ID is never guessed.

`Match` tells you how to read the result:

| `Match` | Meaning | What to store |
|---------|---------|---------------|
| `exact` | one variant | `result.Vehicle.VehicleID` |
| `multiple` | several trims of one make and model, all on sale at the same time; `Vehicle` is the first with its ID, the rest are in `Alternatives` | all of `result.CandidateIDs()`, or let the customer pick |
| `ambiguous` | candidates are different makes or models, or different generations; `Vehicle.VehicleID` is `nil` and `Alternatives` lists every candidate (including the one shown as `Vehicle`) with its ID | `result.CandidateIDs()` (a picker is best) |

```go
result, _ := client.Lookup(ctx, "ABC123", "VIC", false)
if result.Match != nil && *result.Match == "exact" {
    save(*result.Vehicle.VehicleID)
} else {
    for _, alt := range result.Alternatives {
        fmt.Println(*alt.VehicleID, *alt.Description, *alt.YearRange)
    }
    saveAll(result.CandidateIDs())
}
```

With `multiple` the primary `VehicleID` identifies the first candidate, not necessarily the
plate's own trim. Lookup text (`Make`, `Model`, `Description`) can be worded differently from the
record the ID points to: integrate on `VehicleID`, not on text.

### Detailed lookup

```go
result, err := client.Lookup(ctx, "ABC123", "NSW", true)
fmt.Println(*result.Vehicle.DetailedDescription)
// "TOYOTA HILUX SR Auto or Manual AN120 AN130 05/2015~12/2020 4 Door UTE 4x4 DIESEL 2.8 litre, ..."
```

`DetailedDescription` may be `nil` for some vehicles even when requested.

### Rate limit headers

Each lookup result carries the quota headers from the response:

```go
*result.RateLimit.Limit            // monthly lookup allowance
*result.RateLimit.Remaining        // lookups left this period
*result.RateLimit.Plan             // "starter"
*result.RateLimit.TopupRemaining   // remaining top-up credits, only while they are being consumed
```

## Vehicle database

Starter plans and above. The database holds 23,000+ Australian-market car variants (225+ makes)
and 15,000+ motorcycles, scooters and ATVs (420+ makes). Queries do not consume lookup quota; they
have their own limit of 500 requests per minute.

### Cascade

`Vehicles()` returns one level of the seven-level cascade. Each call narrows it: no options gives
the makes, `Make` gives models, and so on down to full vehicle records with a `VehicleID`.

```go
r, _ := client.Vehicles(ctx, nil)                                                     // *r.Type == "make"
r.Data[:3]                                                                             // ["ABARTH", "ACE EV", "ACURA"]

r, _ = client.Vehicles(ctx, &plateapi.VehiclesOptions{Make: plateapi.String("TOYOTA")})           // "model"
r, _ = client.Vehicles(ctx, &plateapi.VehiclesOptions{Make: plateapi.String("TOYOTA"),
    Model: plateapi.String("HILUX")})                                                              // "year"  (newest first)
r, _ = client.Vehicles(ctx, &plateapi.VehiclesOptions{Make: plateapi.String("TOYOTA"),
    Model: plateapi.String("HILUX"), Year: plateapi.Int(2019)})                                    // "series"
r, _ = client.Vehicles(ctx, &plateapi.VehiclesOptions{Make: plateapi.String("TOYOTA"),
    Model: plateapi.String("HILUX"), Year: plateapi.Int(2019),
    Series: plateapi.String("GUN126R -GUN126")})                                                   // "engine"
r, _ = client.Vehicles(ctx, &plateapi.VehiclesOptions{Make: plateapi.String("TOYOTA"),
    Model: plateapi.String("HILUX"), Year: plateapi.Int(2019),
    Series: plateapi.String("GUN126R -GUN126"),
    Engine: plateapi.String("2.8L,  DIE 1GDFTV I4 16v DOHC I/C Turbo CRD {130kW}")})               // "variant"
r, _ = client.Vehicles(ctx, &plateapi.VehiclesOptions{Make: plateapi.String("TOYOTA"),
    Model: plateapi.String("HILUX"), Year: plateapi.Int(2019),
    Series: plateapi.String("GUN126R -GUN126"),
    Engine: plateapi.String("2.8L,  DIE 1GDFTV I4 16v DOHC I/C Turbo CRD {130kW}"),
    Variant: plateapi.String("4D Ute -, 4WD MR0HA3CD  [THAILAND], AT/MT")})                        // "vehicle"
r.Vehicles        // [{VehicleID: 1346446, SelectText: "...", Description: "...", LongDescription: "..."}]
r.VehicleIDs()    // [1346446]
```

`VehiclesResult` fields: `Success`, `Type` (the level returned), `Data` (each value as text:
names, years as digits, and the JSON of each record at the vehicle level), `Vehicles` and
`VehicleIDs()` (filled at the vehicle level), `Total`, `DurationMs`.

Rules worth knowing:

- Pass each value exactly as the previous level returned it. Values contain commas, double spaces,
  braces and brackets, and matching is case-sensitive. The SDK URL-encodes them for you.
- An empty-string series is a real value. Pass `Series: plateapi.String("")` to continue below it.
- Unknown values are not errors: you get `Success == true` with an empty `Data` slice. A `503`
  (database briefly unavailable) is retried by the SDK and then returned as a `*ServerError`.
- Trims are filed as separate models (`HILUX`, `HILUX SR`, `HILUX SR5 EXTRA CAB`), so to cover a
  base model walk every model name that starts with it. `WalkVehicles()` below does this.
- The vehicle level is a list: a few paths return two records whose catalogue entries overlap in
  that year. Use each record's description and `VehicleByID()` to tell them apart.
- `VehicleID` identifies the variant, not the year: a variant returns the same ID under every
  model year it was sold.

### Motorcycles

Motorcycles have their own cascade. Pass `VehicleType: plateapi.String("motorcycle")` on every
level (`"car"` is the default).

```go
r, _ = client.Vehicles(ctx, &plateapi.VehiclesOptions{VehicleType: plateapi.String("motorcycle")})   // 420+ makes
r, _ = client.Vehicles(ctx, &plateapi.VehiclesOptions{Make: plateapi.String("YAMAHA"),
    Model: plateapi.String("MT-07"), VehicleType: plateapi.String("motorcycle")})
```

A motorcycle plate lookup returns the same ID as the motorcycle cascade. Any other `VehicleType`
returns an error before any request.

### One vehicle by ID

```go
record, err := client.VehicleByID(ctx, 1380801)

*record.VehicleID        // 1380801
*record.VehicleType      // "car" or "motorcycle"
*record.Make             // "TOYOTA"
*record.Model            // "HILUX SR"
*record.YearRange        // "2015-2020"
*record.LowestYear       // 2015
*record.HighestYear      // 2020 (nil for an open range)
record.Years             // [2015 2016 2017 2018 2019 2020]
*record.Description      // "15~20 TOYOTA HILUX 2.8L DIESEL"
*record.LongDescription  // "TOYOTA HILUX 10/2015~04/2020"
*record.Series           // "GUN126R -GUN126"
*record.Engine           // "2.8L,  DIE 1GDFTV I4 16v DOHC I/C Turbo CRD {130kW}"
*record.Variant          // "SR,  4D Ute -, 4WD MR0KA3CD  [THAILAND], AT/MT"
*record.Body             // "UTE"
*record.BodySize         // "COMPACT" / "MEDIUM" / "FULL-SIZE" for SUVs, else nil
*record.Doors            // 4
*record.Drive            // "4WD", "AWD", "RWD", "FWD", "6X4", ...
*record.Transmission     // "AT", "MT", "AT/MT", "CVT", "DCT", "AMT"
*record.Detail           // "full" or "basic"
record.Raw               // json.RawMessage of the vehicle object exactly as returned
```

- `Detail == "basic"` means the vehicle has an ID but its full record has not been added yet:
  `Series`, `Variant` and `LongDescription` are `nil` and `Years` is empty. The ID does not
  change when the full record is added.
- `Body`, `BodySize`, `Doors`, `Drive` and `Transmission` are read from the variant name and are
  `nil` when it does not carry them.
- A retired ID that was merged into another returns the current record, so `record.VehicleID` can
  differ from the ID you asked for. Compare them if you care.
- An unknown ID returns a `*NotFoundError` (code `not_found`). A negative ID returns an error
  before any request.
- The sandbox plate `TEST123` returns `VehicleID` 1380801, a real record, so you can test this
  call without using quota.

## Walking the cascade

`WalkVehicles()` collects every `VehicleID` for one make and model: it walks models, years,
series, engines, variants and vehicles for you and calls your function for each vehicle the first
time it is seen. This is how you map existing fitment text (make, model, years, engine) to
PlateAPI IDs once.

```go
err := client.WalkVehicles(ctx, "TOYOTA", "HILUX", &plateapi.WalkOptions{
    YearFrom: plateapi.Int(2015), YearTo: plateapi.Int(2020), EngineContains: plateapi.String("2.8L"),
}, func(v plateapi.WalkedVehicle) error {
    fmt.Println(v.VehicleID, v.Model, v.Year, v.Series, v.Engine, v.Variant)
    return nil
})
```

| Option | Default | Meaning |
|--------|---------|---------|
| `make`, `model` (arguments) | required | base model; must be non-empty |
| `Trims` | `nil` (true) | also walk every model name starting with `model + " "` (`HILUX SR`, `HILUX SR5 EXTRA CAB`). `plateapi.Bool(false)` walks `model` only and skips the models request |
| `YearFrom`, `YearTo` | `nil` | inclusive model-year window; an error if from > to |
| `EngineContains` | `nil` | only engines whose name contains this text (case-insensitive), e.g. `"2.8L"` or `"DIE"` |
| `VehicleType` | `nil` | `"motorcycle"` for the motorcycle cascade |
| `PerYear` | `false` | deliver one row per vehicle per model year instead of one row per vehicle (for year-level master lists) |
| `Known` | `nil` | a `map[WalkKey]bool` of vehicles already handled (`plateapi.KnownIDs(1, 2, 3)` builds one); they are not delivered again and the map is updated in place (resume an interrupted walk). With `PerYear` the key's `Year` is set |
| `Delay` | `nil` (150 ms) | pause after each request (`plateapi.Duration(0)` for none); the default stays under 500 requests/minute |

Each `WalkedVehicle` has `VehicleID`, `Make`, `Model` (the exact model name it was found under),
`Year`, `Series`, `Engine`, `Variant` (the cascade's `select_text`), `Description`,
`LongDescription`, `VehicleType` and `Raw` (the `CascadeVehicle` record). The path fields are
where the vehicle was first seen; with `PerYear` you get a row for every year.

Results arrive as they are found. Return a non-nil error from your function to stop early (it is
returned to you unchanged), or cancel `ctx`. API errors are returned at the point they happen;
everything already delivered stays with you and `Known` is up to date. A `429` whose
`Retry-After` is longer than the max wait (60 s by default) returns a `*RateLimitError`
immediately rather than sleeping, so a long walk never sits silently. Save your progress and
resume later with `Known`.

A Hilux walk (all trims, all years) is about 350 requests and takes a minute or two at the default
delay; a single make and model with a year window is usually a few seconds.

### Example: stream IDs to a CSV, resumable

```go
package main

import (
    "context"
    "encoding/csv"
    "errors"
    "fmt"
    "os"
    "strconv"

    plateapi "github.com/PlateAPI/plateapi-go"
)

func main() {
    const filename = "hilux.csv"
    known := map[plateapi.WalkKey]bool{}
    if f, err := os.Open(filename); err == nil {
        rows, _ := csv.NewReader(f).ReadAll()
        f.Close()
        for _, row := range rows[1:] {
            id, _ := strconv.Atoi(row[0])
            known[plateapi.WalkKey{VehicleID: id}] = true
        }
    }

    out, _ := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
    defer out.Close()
    w := csv.NewWriter(out)
    if len(known) == 0 {
        w.Write([]string{"vehicle_id", "make", "model", "year", "series", "engine", "variant"})
    }

    client := plateapi.NewClient(os.Getenv("PLATEAPI_KEY"))
    err := client.WalkVehicles(context.Background(), "TOYOTA", "HILUX", &plateapi.WalkOptions{Known: known},
        func(v plateapi.WalkedVehicle) error {
            w.Write([]string{strconv.Itoa(v.VehicleID), v.Make, v.Model, strconv.Itoa(v.Year), v.Series, v.Engine, v.Variant})
            w.Flush()
            return nil
        })
    var rl *plateapi.RateLimitError
    if errors.As(err, &rl) {
        fmt.Printf("rate limited, retry in %v s; %d rows saved\n", *rl.RetryAfter, len(known))
    } else if err != nil {
        panic(err)
    }
}
```

Run it again later and it continues where it stopped.

### Linking products (SKUs) to vehicles

Keep a many-to-many table `(sku, vehicle_id)`. Walk the cascade once per fitment line to fill it,
then on each plate lookup query the table by `result.Vehicle.VehicleID` (or every ID in
`result.CandidateIDs()` when `Match` is `multiple`). Store IDs, not text. The full guide is at
[plateapi.com.au/docs/vehicles](https://plateapi.com.au/docs/vehicles/#vehicles-skus).

## Account

### Usage

```go
usage, _ := client.Usage(ctx)
*usage.Plan               // "starter"
usage.MonthlyLimit        // 600
usage.UsedThisMonth       // 12
usage.Remaining           // 588
*usage.PercentUsed        // 2.0
usage.RateLimitPerMin     // 10
*usage.PeriodStart        // "2026-09-07 00:00:00" (UTC)
usage.PeriodEnd           // nil when there is no billing period (free plan)
usage.DaysRemaining       // nil when there is no billing period
*usage.LastLookupAt       // "2026-10-03 03:20:02"
usage.TopupCredits        // remaining purchased top-up lookups
usage.CancelAtPeriodEnd, usage.CancelAt
```

### Lookup history

```go
logs, _ := client.Logs(ctx, &plateapi.LogsOptions{Limit: 50, Offset: 0, Since: plateapi.String("2026-10-01T00:00:00")})
for _, entry := range logs.Logs {
    fmt.Println(*entry.CreatedAt, *entry.Plate, *entry.State, entry.Success, *entry.DurationMs, *entry.RequestID)
}
fmt.Println(logs.Count, "of", logs.Total)
```

`Limit` is at most 500. `Since` / `Until` are ISO 8601 UTC. `Success: plateapi.Bool(true)` /
`false` filters found / not found. `LogEntry` fields: `Plate`, `State`, `Success` (1 or 0),
`Error`, `DurationMs`, `Make`, `Model`, `Year`, `ClientIP`, `RequestID`, `CreatedAt`.

### Health

```go
health, _ := client.Health(ctx)
health.Status   // "ok"
```

No key is sent, no quota is used, and the call makes a single attempt.

## Sandbox

Plate `TEST123` with any state returns a fixed vehicle instantly: free, no quota, no rate limit.
A valid key is still required. Use it in integration tests and CI.

```go
result, _ := client.Lookup(ctx, "TEST123", "VIC", false)
result.Sandbox                 // true
*result.Vehicle.VehicleID      // 1380801
record, _ := client.VehicleByID(ctx, *result.Vehicle.VehicleID)
*record.Model                  // "HILUX SR"
```

## Errors

Every error from the SDK unwraps to `*plateapi.APIError`, which carries `Message` (the API's own
text), `StatusCode`, `Code` (the API's machine code when it sent one) and `RequestID`. Use
`errors.As` with the typed errors:

| Type | HTTP | When |
|------|------|------|
| `*BadRequestError` | 400 | invalid plate, state, cascade parameter or vehicle ID |
| `*AuthenticationError` | 401 | missing, invalid or deactivated key; email not verified |
| `*PermissionDeniedError` | 403 | IP not on the key's allowlist, account banned, or vehicle database on a Free / Lite plan |
| `*NotFoundError` | 404 | `VehicleByID()` with an unknown ID |
| `*RateLimitError` | 429 | per-minute limit; `RetryAfter` in seconds |
| `*QuotaExceededError` | 429 | monthly quota used up (also matches `*RateLimitError`; `RetryAfter` is 3600) |
| `*ServerError` | 5xx | after retries; `Code` is `lookup_failed` or `unavailable` when the API sent one, `RetryAfter` when it sent a header |
| `*APIError` | other | connection failures, timeouts, unexpected statuses |

```go
import "errors"

result, err := client.Lookup(ctx, "ABC123", "VIC", false)
if err != nil {
    var quota *plateapi.QuotaExceededError
    var rate *plateapi.RateLimitError
    var auth *plateapi.AuthenticationError
    var server *plateapi.ServerError
    var api *plateapi.APIError

    switch {
    case errors.As(err, &quota):
        fmt.Println("monthly quota used up:", quota.Message)
    case errors.As(err, &rate):
        fmt.Println("slow down, retry in", *rate.RetryAfter, "seconds")
    case errors.As(err, &auth):
        fmt.Println("key problem:", auth.Message)
    case errors.As(err, &server):
        fmt.Println("temporary failure, not billed:", server.Code, server.RequestID)
    case errors.As(err, &api):
        fmt.Println("error", api.StatusCode, api.Message)
    default:
        fmt.Println("invalid input:", err)
    }
}
```

Check `*QuotaExceededError` before `*RateLimitError`, as above. A lookup that cannot be completed
(`503`, code `lookup_failed`) is never billed; retry it after a few seconds. A `200` with
`code: "internal_error"` is also not billed and is retried by the SDK before it is returned.

## Retries and waiting

| Response | Behaviour |
|----------|-----------|
| connection error, timeout | retried up to `WithMaxRetries` with backoff (1 s, 2 s, 4 s plus jitter), then `*APIError` |
| 400 / 401 / 403 / 404 | returned at once, no retry |
| 429 monthly quota | `*QuotaExceededError` at once, no sleep |
| 429 other | sleeps `Retry-After` (backoff if absent) and retries; if `Retry-After` is longer than the max wait or retries are used up, `*RateLimitError` |
| 5xx | same rule, then `*ServerError` |
| 200 lookup with `internal_error` | retried with backoff, then returned |

```go
client := plateapi.NewClient("pk_live_your_api_key",
    plateapi.WithBaseURL("https://api.plateapi.com.au"),  // default
    plateapi.WithTimeout(30*time.Second),                 // per request
    plateapi.WithMaxRetries(3),                           // retries after the first attempt
    plateapi.WithMaxWait(60*time.Second),                 // longest Retry-After to sleep; 0 waits any length
    plateapi.WithHTTPClient(&http.Client{}),              // your own transport, proxies, etc.
)
```

Every request sends `User-Agent: plateapi-go/<version>` (`plateapi.Version`). All methods take a
`context.Context` for cancellation and deadlines; a sleep between retries ends when the context
does.

## Upgrading from 0.1.0

Everything written against 0.1.0 keeps working: no public name, signature, field or error type
was removed. New in 0.2.0: `VehicleID`, `Match`, `CandidateIDs()`, `VehicleByID()`,
`WalkVehicles()`, `VehicleType: "motorcycle"`, `Vehicles` / `VehicleIDs()` on cascade results,
`TopupRemaining`, `*BadRequestError`, `*PermissionDeniedError`, `*NotFoundError`, `Code` and
`RequestID` on every error, `Unwrap()` on every typed error, `WithMaxWait`, `Duration` and
`KnownIDs` helpers.

Behaviour changes to be aware of:

- A monthly-quota `429` now returns `*QuotaExceededError` immediately. 0.1.0 only recognised a
  body code the API does not send, so it slept for an hour and retried. `*QuotaExceededError`
  now embeds `RateLimitError` and unwraps to it, so `errors.As(err, &rateLimitErr)` matches both.
- A `Retry-After` longer than the max wait (60 s) returns the error instead of sleeping. Pass
  `plateapi.WithMaxWait(0)` for the old behaviour.
- `Health()` makes one attempt.
- Error messages are the API's `detail` / `error` text instead of fixed strings.
- 400, 403 and 404 return `*BadRequestError`, `*PermissionDeniedError` and `*NotFoundError`
  (all unwrap to `*APIError`, which 0.1.0 returned).

See [CHANGELOG.md](CHANGELOG.md).

## Links

- [API documentation](https://plateapi.com.au/docs/)
- [Vehicle database and vehicle IDs](https://plateapi.com.au/docs/vehicles/)
- [OpenAPI spec](https://plateapi.com.au/openapi.json)
- [Pricing](https://plateapi.com.au/pricing/)
- [Dashboard](https://plateapi.com.au/dashboard/)
- [Status page](https://plateapi.com.au/status/)
- [Support](mailto:support@plateapi.com.au)
