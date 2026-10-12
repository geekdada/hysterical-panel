package api

import (
	"database/sql"
	"maps"
	"strings"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
)

// A daily bucket counts toward a range when its midpoint falls inside it, so
// local-day ranges in any time zone pick the UTC day they overlap most.
const dailyBucketMidpoint = 12 * time.Hour

// trafficRange is a UTC bucket range. A zero bound is open.
type trafficRange struct {
	from, to time.Time
}

// parseTrafficRange parses optional from/to query values. Required bounds are
// enforced by requiredTrafficRange.
func parseTrafficRange(from, to string) (trafficRange, error) {
	var r trafficRange
	for _, bound := range []struct {
		name, value string
		target      *time.Time
	}{{"from", from, &r.from}, {"to", to, &r.to}} {
		if bound.value == "" {
			continue
		}
		parsed, err := types.ParseDateTime(bound.value)
		if err != nil || parsed.IsZero() {
			return trafficRange{}, apis.NewBadRequestError(bound.name+" must be a UTC datetime", err)
		}
		*bound.target = parsed.Time().UTC()
	}
	return r, nil
}

// findTrafficRows reads traffic rows matching scope inside r, ordered by bucket.
// Hourly rows are used where hourly history is complete. Earlier days come from
// traffic_daily because pruning removes their hourly rows.
func findTrafficRows(app core.App, scope string, params map[string]any, r trafficRange) ([]*core.Record, error) {
	hourlyFrom, hasHourly, err := hourlyCompleteFrom(app)
	if err != nil {
		return nil, err
	}

	daily, err := findDailyTrafficRows(app, scope, params, r, hourlyFrom, hasHourly)
	if err != nil || !hasHourly {
		return daily, err
	}

	conditions, hourlyParams := scopedConditions(scope, params)
	conditions = append(conditions, "bucket >= {:hourlyFrom}")
	hourlyParams["hourlyFrom"] = formatBucket(hourlyFrom)
	if !r.from.IsZero() {
		conditions = append(conditions, "bucket >= {:from}")
		hourlyParams["from"] = formatBucket(r.from)
	}
	if !r.to.IsZero() {
		conditions = append(conditions, "bucket <= {:to}")
		hourlyParams["to"] = formatBucket(r.to)
	}
	hourly, err := app.FindRecordsByFilter(trafficHourlyTable, strings.Join(conditions, " && "), "bucket", 0, 0, hourlyParams)
	if err != nil {
		return nil, err
	}
	return append(daily, hourly...), nil
}

func findDailyTrafficRows(app core.App, scope string, params map[string]any, r trafficRange, hourlyFrom time.Time, hasHourly bool) ([]*core.Record, error) {
	conditions, dailyParams := scopedConditions(scope, params)
	if hasHourly {
		if !r.from.IsZero() && !r.from.Before(hourlyFrom) {
			return nil, nil
		}
		conditions = append(conditions, "bucket < {:hourlyFrom}")
		dailyParams["hourlyFrom"] = formatBucket(hourlyFrom)
	}
	if !r.from.IsZero() {
		conditions = append(conditions, "bucket >= {:dailyFrom}")
		dailyParams["dailyFrom"] = formatBucket(r.from.Add(-dailyBucketMidpoint))
	}
	if !r.to.IsZero() {
		conditions = append(conditions, "bucket <= {:dailyTo}")
		dailyParams["dailyTo"] = formatBucket(r.to.Add(-dailyBucketMidpoint))
	}
	return app.FindRecordsByFilter(trafficDailyTable, strings.Join(conditions, " && "), "bucket", 0, 0, dailyParams)
}

// hourlyCompleteFrom returns the first UTC day start from which traffic_hourly
// holds every row. Pruning only removes rows before a cutoff, so hourly history
// is complete from the day after its oldest row. The oldest day itself is
// complete unless pruning removed some of its hours.
func hourlyCompleteFrom(app core.App) (time.Time, bool, error) {
	var oldest sql.NullString
	if err := app.DB().NewQuery("SELECT MIN(bucket) FROM traffic_hourly").Row(&oldest); err != nil {
		return time.Time{}, false, err
	}
	if !oldest.Valid || oldest.String == "" {
		return time.Time{}, false, nil
	}
	parsed, err := types.ParseDateTime(oldest.String)
	if err != nil {
		return time.Time{}, false, err
	}
	day := parsed.Time().UTC().Truncate(24 * time.Hour)
	complete, err := hourlyDayMatchesDaily(app, day)
	if err != nil {
		return time.Time{}, false, err
	}
	if !complete {
		day = day.Add(24 * time.Hour)
	}
	return day, true, nil
}

// hourlyDayMatchesDaily reports whether every daily row of day equals the sum
// of its hourly rows. The collector writes both in one transaction, so a
// mismatch means pruning removed hours of that day.
func hourlyDayMatchesDaily(app core.App, day time.Time) (bool, error) {
	var mismatched int64
	err := app.DB().NewQuery(`
		SELECT COUNT(*) FROM (
			SELECT d.tx AS tx, d.rx AS rx, COALESCE(SUM(h.tx), 0) AS hourly_tx, COALESCE(SUM(h.rx), 0) AS hourly_rx
			FROM traffic_daily d
			LEFT JOIN traffic_hourly h
				ON h.user = d.user AND h.node = d.node AND h.bucket >= d.bucket AND h.bucket < {:dayEnd}
			WHERE d.bucket = {:day}
			GROUP BY d.id
		) WHERE tx != hourly_tx OR rx != hourly_rx`).
		Bind(dbx.Params{"day": formatBucket(day), "dayEnd": formatBucket(day.Add(24 * time.Hour))}).
		Row(&mismatched)
	return mismatched == 0, err
}

func scopedConditions(scope string, params map[string]any) ([]string, map[string]any) {
	conditions := []string{}
	if scope != "" {
		conditions = append(conditions, scope)
	}
	merged := make(map[string]any, len(params)+4)
	maps.Copy(merged, params)
	return conditions, merged
}

func formatBucket(t time.Time) string {
	return t.UTC().Format(types.DefaultDateLayout)
}
