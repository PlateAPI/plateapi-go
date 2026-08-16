package plateapi

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
}

type RateLimit struct {
	Limit     *int    `json:"limit,omitempty"`
	Remaining *int    `json:"remaining,omitempty"`
	Plan      *string `json:"plan,omitempty"`
}

type LookupResult struct {
	Success      bool      `json:"success"`
	Vehicle      *Vehicle  `json:"vehicle,omitempty"`
	Alternatives []Vehicle `json:"alternatives,omitempty"`
	Source       *string   `json:"source,omitempty"`
	DurationMs   *float64  `json:"duration_ms,omitempty"`
	Sandbox      bool      `json:"sandbox"`
	Code         *string   `json:"code,omitempty"`
	Error        *string   `json:"error,omitempty"`
	RequestID    *string   `json:"request_id,omitempty"`
	RateLimit    RateLimit `json:"-"`
}

type VehiclesOptions struct {
	Make    *string
	Model   *string
	Year    *int
	Series  *string
	Engine  *string
	Variant *string
}

type VehiclesResult struct {
	Success    bool     `json:"success"`
	Type       *string  `json:"type,omitempty"`
	Data       []string `json:"data"`
	Total      int      `json:"total"`
	DurationMs *float64 `json:"duration_ms,omitempty"`
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
