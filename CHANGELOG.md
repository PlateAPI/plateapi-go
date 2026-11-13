# Changelog

## 0.3.0 -- 2026-11-13

Every 0.2.0 program keeps working. New methods and types only; no behaviour changes.

### Added

- `Client.VehicleChanges(ctx, opts)` -> `*VehicleChangesResult`: one page of
  `GET /api/v1/vehicles/changes`, the vehicles added, updated or merged since a date
  (`Since`, `VehicleType`, `Limit`, `Cursor`). Argument errors (no `Since` or `Cursor`, an
  unknown `VehicleType`, `Limit` outside 1 to 100) are returned before any request.
- `Client.AllVehicleChanges(ctx, opts)`: follows `NextCursor` and returns every change in the
  window as one result (`Count` the total, `DurationMs` the pages added up). Returns an
  `*APIError` instead of looping if the API says there is more without a cursor or repeats one.
- `VehicleChangesOptions`, `VehicleChange` (`MergedInto *int`, `YearRange *string`, `NewYears`
  never nil, `Raw`) and `VehicleChangesResult` types.
- README section "Keeping vehicle IDs current": store `Until`, pass it as `Since` next time,
  what to do for added, updated and merged vehicles.

## 0.2.0 -- 2026-10-18

Every 0.1.0 program keeps working. New fields and methods only; see "Upgrading from 0.1.0"
in the README for the few behaviour changes.

### Added

- `Vehicle.VehicleID`: PlateAPI's stable ID for the exact variant, on the primary vehicle and
  on every entry in `Alternatives`.
- `LookupResult.Match` (`exact`, `multiple`, `ambiguous`) and `LookupResult.CandidateIDs()`.
- `Client.VehicleByID(ctx, id)` -> `*VehicleRecord` for `GET /api/v1/vehicles/{vehicle_id}`.
- `VehiclesOptions.VehicleType` (`"motorcycle"`) for the motorcycle cascade.
- `VehiclesResult.Vehicles` and `VehiclesResult.VehicleIDs()` for the last cascade level.
- `Client.WalkVehicles(ctx, make, model, opts, fn)`: walks the cascade for one make and model
  (and its trims) and calls `fn` for each `VehicleID` once, with `YearFrom` / `YearTo`,
  `EngineContains`, `PerYear`, `Known` (resume) and `Delay` options.
- `RateLimit.TopupRemaining` from the `X-Topup-Remaining` header.
- `*BadRequestError` (400), `*PermissionDeniedError` (403) and `*NotFoundError` (404).
- `Code` and `RequestID` on every `APIError`, from the body or the `X-Request-ID` header, and
  `Unwrap()` on every typed error so `errors.As` finds `*APIError` through it.
- `WithMaxWait` option: the longest `Retry-After` the SDK will sleep (default 60 s).
- `Duration` and `KnownIDs` helpers; `plateapi.Version`; `User-Agent: plateapi-go/<version>`.
- Offline test suite (`go test ./...`, httptest only).

### Fixed

- A monthly-quota 429 returns `*QuotaExceededError` at once instead of sleeping for an hour and
  retrying.
- A `Retry-After` longer than the max wait returns the error instead of sleeping.
- `Health()` makes a single attempt.
- Error messages are the API's own (`detail` / `error`), not a fixed string.
- A lookup that returns `code: "internal_error"` (not billed) is retried before it is returned.
- A sleep between retries ends when the context is cancelled, and no further attempt is made.

### Changed

- `*QuotaExceededError` now embeds `RateLimitError` (and still unwraps to `*APIError`).

## 0.1.0 -- 2026-08-16

- First release: `Lookup`, `Vehicles`, `Usage`, `Logs`, `Health`, retries with backoff.
