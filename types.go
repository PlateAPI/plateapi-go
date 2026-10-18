package plateapi

import (
	"encoding/json"
	"time"
)

type Vehicle struct {
	Make                *string `json:"make,omitempty"`
	Model               *string `json:"model,omitempty"`
	Year                *int    `json:"-"`
	YearRange           *string `json:"year_range,omitempty"`
	LowestYear          *int    `json:"lowest_year,omitempty"`
	HighestYear         *int    `json:"highest_year,omitempty"`
	Body                *string `json:"body,omitempty"`
	Engine              *string `json:"engine,omitempty"`
	Series              *string `json:"series,omitempty"`
	Description         *string `json:"description,omitempty"`
	DetailedDescription *string `json:"detailed_description,omitempty"`
	// VehicleID is PlateAPI's stable ID for this exact variant. nil when the variant is not certain.
	VehicleID *int `json:"vehicle_id,omitempty"`
}

type RateLimit struct {
	Limit     *int    `json:"limit,omitempty"`
	Remaining *int    `json:"remaining,omitempty"`
	Plan      *string `json:"plan,omitempty"`
	// TopupRemaining is the remaining top-up credits, only while they are being consumed.
	TopupRemaining *int `json:"topup_remaining,omitempty"`
}

type LookupResult struct {
	Success      bool      `json:"success"`
	Vehicle      *Vehicle  `json:"vehicle,omitempty"`
	Alternatives []Vehicle `json:"alternatives,omitempty"`
	// Match is "exact", "multiple" or "ambiguous"; nil when not found.
	Match      *string   `json:"match,omitempty"`
	Source     *string   `json:"source,omitempty"`
	DurationMs *float64  `json:"duration_ms,omitempty"`
	Sandbox    bool      `json:"sandbox"`
	Code       *string   `json:"code,omitempty"`
	Error      *string   `json:"error,omitempty"`
	RequestID  *string   `json:"request_id,omitempty"`
	RateLimit  RateLimit `json:"-"`
}

// CandidateIDs returns every non-nil vehicle ID, the primary vehicle first then the
// alternatives, without duplicates. Empty when nothing was found.
func (r *LookupResult) CandidateIDs() []int {
	ids := []int{}
	seen := map[int]bool{}
	add := func(v *Vehicle) {
		if v != nil && v.VehicleID != nil && !seen[*v.VehicleID] {
			seen[*v.VehicleID] = true
			ids = append(ids, *v.VehicleID)
		}
	}
	add(r.Vehicle)
	for i := range r.Alternatives {
		add(&r.Alternatives[i])
	}
	return ids
}

type VehiclesOptions struct {
	Make    *string
	Model   *string
	Year    *int
	Series  *string
	Engine  *string
	Variant *string
	// VehicleType is "car" (default) or "motorcycle".
	VehicleType *string
}

// CascadeVehicle is one record at the vehicle level of the cascade.
type CascadeVehicle struct {
	VehicleID       *int            `json:"vehicle_id,omitempty"`
	SelectText      *string         `json:"select_text,omitempty"`
	Description     *string         `json:"description,omitempty"`
	LongDescription *string         `json:"long_description,omitempty"`
	Raw             json.RawMessage `json:"-"`
}

type VehiclesResult struct {
	Success bool    `json:"success"`
	Type    *string `json:"type,omitempty"`
	// Data holds each value as text: names at most levels, years as digits, and the JSON of each
	// record at the vehicle level (see Vehicles for the parsed form).
	Data []string `json:"data"`
	// Vehicles holds the parsed records at the vehicle level; empty at other levels.
	Vehicles   []CascadeVehicle `json:"vehicles,omitempty"`
	Total      int              `json:"total"`
	DurationMs *float64         `json:"duration_ms,omitempty"`
}

// VehicleIDs returns the vehicle ID of each record at the vehicle level; empty at other levels.
func (r *VehiclesResult) VehicleIDs() []int {
	ids := []int{}
	for _, v := range r.Vehicles {
		if v.VehicleID != nil {
			ids = append(ids, *v.VehicleID)
		}
	}
	return ids
}

// VehicleRecord is one vehicle from GET /api/v1/vehicles/{vehicle_id}.
type VehicleRecord struct {
	VehicleID       *int    `json:"vehicle_id,omitempty"`
	VehicleType     *string `json:"vehicle_type,omitempty"`
	Make            *string `json:"make,omitempty"`
	Model           *string `json:"model,omitempty"`
	YearRange       *string `json:"year_range,omitempty"`
	LowestYear      *int    `json:"lowest_year,omitempty"`
	HighestYear     *int    `json:"highest_year,omitempty"`
	Years           []int   `json:"years"`
	Description     *string `json:"description,omitempty"`
	LongDescription *string `json:"long_description,omitempty"`
	Series          *string `json:"series,omitempty"`
	Engine          *string `json:"engine,omitempty"`
	Variant         *string `json:"variant,omitempty"`
	Body            *string `json:"body,omitempty"`
	BodySize        *string `json:"body_size,omitempty"`
	Doors           *int    `json:"doors,omitempty"`
	Drive           *string `json:"drive,omitempty"`
	Transmission    *string `json:"transmission,omitempty"`
	// Detail is "full" or "basic".
	Detail *string `json:"detail,omitempty"`
	// Raw is the vehicle object exactly as returned.
	Raw json.RawMessage `json:"-"`
}

// WalkKey identifies a walked vehicle in WalkOptions.Known: Year is 0 unless PerYear is set.
type WalkKey struct {
	VehicleID int
	Year      int
}

type WalkOptions struct {
	// Trims also walks every model name starting with model + " ". nil means true.
	Trims *bool
	// YearFrom and YearTo give an inclusive model-year window.
	YearFrom *int
	YearTo   *int
	// EngineContains keeps only engines whose name contains this text (case-insensitive).
	EngineContains *string
	// VehicleType is "motorcycle" for the motorcycle cascade.
	VehicleType *string
	// PerYear delivers one vehicle per model year instead of one per vehicle.
	PerYear bool
	// Known holds vehicles already handled; they are not delivered again and the map is
	// updated in place, so an interrupted walk can be resumed. See KnownIDs.
	Known map[WalkKey]bool
	// Delay is the pause after each request. nil means DefaultWalkDelay (150 ms).
	Delay *time.Duration
}

type WalkedVehicle struct {
	VehicleID       int
	Make            string
	Model           string
	Year            int
	Series          string
	Engine          string
	Variant         string
	Description     *string
	LongDescription *string
	VehicleType     string
	Raw             CascadeVehicle
}

type Usage struct {
	Email             *string  `json:"email,omitempty"`
	Plan              *string  `json:"plan,omitempty"`
	MonthlyLimit      int      `json:"monthly_limit"`
	UsedThisMonth     int      `json:"used_this_month"`
	Remaining         int      `json:"remaining"`
	PercentUsed       *float64 `json:"percent_used,omitempty"`
	RateLimitPerMin   int      `json:"rate_limit_per_min"`
	LastLookupAt      *string  `json:"last_lookup_at,omitempty"`
	PeriodStart       *string  `json:"period_start,omitempty"`
	PeriodEnd         *string  `json:"period_end,omitempty"`
	DaysRemaining     *int     `json:"days_remaining,omitempty"`
	CancelAtPeriodEnd bool     `json:"cancel_at_period_end"`
	CancelAt          *string  `json:"cancel_at,omitempty"`
	TopupCredits      int      `json:"topup_credits"`
}

type LogEntry struct {
	Plate      *string  `json:"plate,omitempty"`
	State      *string  `json:"state,omitempty"`
	Success    int      `json:"success"`
	Error      *string  `json:"error,omitempty"`
	DurationMs *float64 `json:"duration_ms,omitempty"`
	Make       *string  `json:"make,omitempty"`
	Model      *string  `json:"model,omitempty"`
	Year       *int     `json:"year,omitempty"`
	ClientIP   *string  `json:"client_ip,omitempty"`
	RequestID  *string  `json:"request_id,omitempty"`
	CreatedAt  *string  `json:"created_at,omitempty"`
}

type LogsOptions struct {
	Limit   int
	Offset  int
	Since   *string
	Until   *string
	Plate   *string
	Success *bool
}

type LogsResult struct {
	Logs   []LogEntry `json:"logs"`
	Count  int        `json:"count"`
	Total  int        `json:"total"`
	Limit  int        `json:"limit"`
	Offset int        `json:"offset"`
}

type HealthStatus struct {
	Status  string  `json:"status"`
	Version *string `json:"version,omitempty"`
}
