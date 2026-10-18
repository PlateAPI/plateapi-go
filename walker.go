package plateapi

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// WalkVehicles walks the cascade for one make and model (and, by default, its trims) and calls
// fn once for every vehicle the first time it is seen. A non-nil error from fn stops the walk
// and is returned; cancelling ctx stops it too. Errors from the API are returned at the point
// they happen; vehicles already delivered stay with the caller, and opts.Known is kept up to
// date so the walk can be resumed later.
func (c *Client) WalkVehicles(ctx context.Context, make, model string, opts *WalkOptions, fn func(WalkedVehicle) error) error {
	if opts == nil {
		opts = &WalkOptions{}
	}
	if strings.TrimSpace(make) == "" {
		return fmt.Errorf("make must be a non-empty string")
	}
	if strings.TrimSpace(model) == "" {
		return fmt.Errorf("model must be a non-empty string")
	}
	if opts.YearFrom != nil && opts.YearTo != nil && *opts.YearFrom > *opts.YearTo {
		return fmt.Errorf("YearFrom must not be greater than YearTo")
	}
	if opts.VehicleType != nil && !validVehicleTypes[*opts.VehicleType] {
		return fmt.Errorf("invalid vehicle type %q, must be one of: car, motorcycle", *opts.VehicleType)
	}
	delay := DefaultWalkDelay
	if opts.Delay != nil {
		delay = *opts.Delay
	}
	if delay < 0 {
		return fmt.Errorf("Delay must not be negative")
	}
	if fn == nil {
		return fmt.Errorf("fn must not be nil")
	}
	trims := opts.Trims == nil || *opts.Trims
	if opts.Known == nil {
		opts.Known = map[WalkKey]bool{}
	}
	seen := opts.Known
	needle := ""
	if opts.EngineContains != nil {
		needle = strings.ToUpper(*opts.EngineContains)
	}
	vehicleType := "car"
	if opts.VehicleType != nil {
		vehicleType = *opts.VehicleType
	}

	fetch := func(o VehiclesOptions) (*VehiclesResult, error) {
		o.VehicleType = opts.VehicleType
		result, err := c.Vehicles(ctx, &o)
		if err != nil {
			return nil, err
		}
		if delay > 0 {
			c.sleep(ctx, delay)
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
		}
		return result, nil
	}

	var models []string
	if trims {
		r, err := fetch(VehiclesOptions{Make: &make})
		if err != nil {
			return err
		}
		for _, m := range r.Data {
			if m == model || strings.HasPrefix(m, model+" ") {
				models = append(models, m)
			}
		}
	} else {
		models = []string{model}
	}

	for _, modelName := range models {
		modelName := modelName
		years, err := fetch(VehiclesOptions{Make: &make, Model: &modelName})
		if err != nil {
			return err
		}
		for _, yearText := range years.Data {
			year, err := strconv.Atoi(strings.TrimSpace(yearText))
			if err != nil {
				continue
			}
			if opts.YearFrom != nil && year < *opts.YearFrom {
				continue
			}
			if opts.YearTo != nil && year > *opts.YearTo {
				continue
			}
			yearValue := year

			seriesList, err := fetch(VehiclesOptions{Make: &make, Model: &modelName, Year: &yearValue})
			if err != nil {
				return err
			}
			for _, series := range seriesList.Data {
				series := series
				engines, err := fetch(VehiclesOptions{Make: &make, Model: &modelName, Year: &yearValue, Series: &series})
				if err != nil {
					return err
				}
				for _, engine := range engines.Data {
					engine := engine
					if needle != "" && !strings.Contains(strings.ToUpper(engine), needle) {
						continue
					}
					variants, err := fetch(VehiclesOptions{Make: &make, Model: &modelName, Year: &yearValue, Series: &series, Engine: &engine})
					if err != nil {
						return err
					}
					for _, variant := range variants.Data {
						variant := variant
						records, err := fetch(VehiclesOptions{Make: &make, Model: &modelName, Year: &yearValue, Series: &series, Engine: &engine, Variant: &variant})
						if err != nil {
							return err
						}
						for _, record := range records.Vehicles {
							if record.VehicleID == nil {
								continue
							}
							key := WalkKey{VehicleID: *record.VehicleID}
							if opts.PerYear {
								key.Year = year
							}
							if seen[key] {
								continue
							}
							seen[key] = true
							v := WalkedVehicle{
								VehicleID:       *record.VehicleID,
								Make:            make,
								Model:           modelName,
								Year:            year,
								Series:          series,
								Engine:          engine,
								Variant:         variant,
								Description:     record.Description,
								LongDescription: record.LongDescription,
								VehicleType:     vehicleType,
								Raw:             record,
							}
							if record.SelectText != nil {
								v.Variant = *record.SelectText
							}
							if err := fn(v); err != nil {
								return err
							}
						}
					}
				}
			}
		}
	}
	return nil
}
