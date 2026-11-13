package plateapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
)

const maxChangesLimit = 100

// VehicleChanges returns one page of GET /api/v1/vehicles/changes: the vehicles added, updated or
// merged since opts.Since, oldest first. Pass Since for the first page, then Cursor set to
// result.NextCursor while result.HasMore (VehicleType and Limit the same; Since can be left
// out). Store result.Until and pass it as Since on the next sync. AllVehicleChanges does the
// paging for you. Starter plan or above; never counted against the lookup quota.
func (c *Client) VehicleChanges(ctx context.Context, opts *VehicleChangesOptions) (*VehicleChangesResult, error) {
	if opts == nil {
		opts = &VehicleChangesOptions{}
	}
	if opts.Since == "" && opts.Cursor == "" {
		return nil, fmt.Errorf("Since is required unless Cursor is given")
	}
	if opts.VehicleType != nil && !validVehicleTypes[*opts.VehicleType] {
		return nil, fmt.Errorf("invalid vehicle type %q, must be one of: car, motorcycle", *opts.VehicleType)
	}
	if opts.Limit < 0 || opts.Limit > maxChangesLimit {
		return nil, fmt.Errorf("Limit must be from 1 to %d (0 for the default)", maxChangesLimit)
	}

	params := url.Values{}
	if opts.Since != "" {
		params.Set("since", opts.Since)
	}
	if opts.Cursor != "" {
		params.Set("cursor", opts.Cursor)
	}
	if opts.VehicleType != nil {
		params.Set("type", *opts.VehicleType)
	}
	if opts.Limit > 0 {
		params.Set("limit", strconv.Itoa(opts.Limit))
	}

	resp, err := c.doRequest(ctx, "/api/v1/vehicles/changes", params, true, c.maxRetries)
	if err != nil {
		return nil, err
	}
	return parseVehicleChanges(resp.body)
}

// AllVehicleChanges returns every change since opts.Since, following NextCursor until the last
// page, as one result: Changes holds every page's changes in order, Count their total and
// DurationMs the pages' durations added up; HasMore is false and NextCursor "". opts.Cursor is
// ignored. Store the result's Until and pass it as Since on the next sync. If the API says there
// is more but sends no cursor, or sends the same cursor twice, an *APIError is returned instead
// of looping.
func (c *Client) AllVehicleChanges(ctx context.Context, opts *VehicleChangesOptions) (*VehicleChangesResult, error) {
	if opts == nil || opts.Since == "" {
		return nil, fmt.Errorf("Since is required")
	}
	page := VehicleChangesOptions{Since: opts.Since, VehicleType: opts.VehicleType, Limit: opts.Limit}
	result, err := c.VehicleChanges(ctx, &page)
	if err != nil {
		return nil, err
	}
	changes := result.Changes
	total := 0.0
	if result.DurationMs != nil {
		total = *result.DurationMs
	}
	sent := map[string]bool{}
	for result.HasMore {
		cursor := result.NextCursor
		if cursor == "" {
			return nil, &APIError{Message: "changes feed said has_more but sent no next_cursor"}
		}
		if sent[cursor] {
			return nil, &APIError{Message: "changes feed sent the same next_cursor twice"}
		}
		sent[cursor] = true
		if ctx.Err() != nil {
			return nil, &APIError{Message: fmt.Sprintf("request failed: %v", ctx.Err())}
		}
		page = VehicleChangesOptions{Cursor: cursor, VehicleType: opts.VehicleType, Limit: opts.Limit}
		result, err = c.VehicleChanges(ctx, &page)
		if err != nil {
			return nil, err
		}
		changes = append(changes, result.Changes...)
		if result.DurationMs != nil {
			total += *result.DurationMs
		}
	}

	result.Changes = changes
	result.Count = len(changes)
	result.HasMore = false
	result.NextCursor = ""
	result.DurationMs = &total
	return result, nil
}

func parseVehicleChanges(body []byte) (*VehicleChangesResult, error) {
	var raw struct {
		Success    bool              `json:"success"`
		Since      string            `json:"since"`
		Until      string            `json:"until"`
		Type       string            `json:"type"`
		Count      *int              `json:"count"`
		Changes    []json.RawMessage `json:"changes"`
		HasMore    bool              `json:"has_more"`
		NextCursor string            `json:"next_cursor"`
		DurationMs *float64          `json:"duration_ms"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, &APIError{Message: fmt.Sprintf("failed to parse response: %v", err)}
	}

	result := &VehicleChangesResult{
		Success:    raw.Success,
		Since:      raw.Since,
		Until:      raw.Until,
		Type:       raw.Type,
		Changes:    []VehicleChange{},
		HasMore:    raw.HasMore,
		NextCursor: raw.NextCursor,
		DurationMs: raw.DurationMs,
	}
	for _, item := range raw.Changes {
		if len(item) == 0 || item[0] != '{' {
			continue
		}
		var change VehicleChange
		if err := json.Unmarshal(item, &change); err != nil {
			return nil, &APIError{Message: fmt.Sprintf("failed to parse response: %v", err)}
		}
		if change.NewYears == nil {
			change.NewYears = []int{}
		}
		change.Raw = item
		result.Changes = append(result.Changes, change)
	}
	result.Count = len(result.Changes)
	if raw.Count != nil {
		result.Count = *raw.Count
	}
	return result, nil
}
