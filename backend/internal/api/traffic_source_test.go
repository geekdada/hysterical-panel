package api

import (
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

type trafficFixture struct {
	app    core.App
	userID string
	nodeID string
}

func newTrafficFixture(t *testing.T) trafficFixture {
	t.Helper()
	app := newMigratedTestApp(t)
	user := newUsersTestRecord(t, app, "traffic@example.com", "traffic-auth-string")
	nodes, err := app.FindCollectionByNameOrId("nodes")
	if err != nil {
		t.Fatalf("find nodes: %v", err)
	}
	node := core.NewRecord(nodes)
	node.Set("name", "traffic node")
	node.Set("api_url", "http://127.0.0.1:1")
	node.Set("api_secret", "sealed")
	node.Set("enabled", true)
	if err := app.Save(node); err != nil {
		t.Fatalf("save node: %v", err)
	}
	return trafficFixture{app: app, userID: user.Id, nodeID: node.Id}
}

// record writes one hourly row and adds it to its daily row, like the collector.
func (f trafficFixture) record(t *testing.T, hour time.Time, tx int64) {
	t.Helper()
	for table, bucket := range map[string]time.Time{
		trafficHourlyTable: hour,
		trafficDailyTable:  hour.Truncate(24 * time.Hour),
	} {
		rec, err := f.app.FindFirstRecordByFilter(table, "bucket = {:b}", map[string]any{"b": formatBucket(bucket)})
		if err != nil {
			col, cerr := f.app.FindCollectionByNameOrId(table)
			if cerr != nil {
				t.Fatalf("find %s: %v", table, cerr)
			}
			rec = core.NewRecord(col)
			rec.Set("user", f.userID)
			rec.Set("node", f.nodeID)
			rec.Set("bucket", bucket)
		}
		rec.Set("tx", int64(rec.GetInt("tx"))+tx)
		rec.Set("rx", 0)
		if err := f.app.Save(rec); err != nil {
			t.Fatalf("save %s: %v", table, err)
		}
	}
}

func (f trafficFixture) pruneHourlyBefore(t *testing.T, cutoff time.Time) {
	t.Helper()
	if _, err := deleteTrafficRowsBefore(f.app, trafficHourlyTable, formatBucket(cutoff)); err != nil {
		t.Fatalf("prune hourly: %v", err)
	}
}

func (f trafficFixture) rows(t *testing.T, from, to time.Time) (total int64, tables map[string]int) {
	t.Helper()
	rows, err := findTrafficRows(f.app, "user = {:u}", map[string]any{"u": f.userID}, trafficRange{from: from, to: to})
	if err != nil {
		t.Fatalf("findTrafficRows: %v", err)
	}
	tables = map[string]int{}
	for _, r := range rows {
		total += int64(r.GetInt("tx"))
		tables[r.Collection().Name]++
	}
	return total, tables
}

var trafficDay1 = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// seedThreeDays records n bytes at 06:00 and 18:00 UTC on day n of three days.
func (f trafficFixture) seedThreeDays(t *testing.T) {
	t.Helper()
	for day := range 3 {
		start := trafficDay1.AddDate(0, 0, day)
		f.record(t, start.Add(6*time.Hour), int64(day+1))
		f.record(t, start.Add(18*time.Hour), int64(day+1))
	}
}

func TestFindTrafficRowsReadsHourlyWhenNothingWasPruned(t *testing.T) {
	f := newTrafficFixture(t)
	f.seedThreeDays(t)

	total, tables := f.rows(t, trafficDay1, trafficDay1.AddDate(0, 0, 3).Add(-time.Second))
	if total != 12 || tables[trafficHourlyTable] != 6 || tables[trafficDailyTable] != 0 {
		t.Fatalf("total=%d tables=%v, want 12 bytes from 6 hourly rows", total, tables)
	}
}

func TestFindTrafficRowsKeepsHourlyForFirstPartialDay(t *testing.T) {
	f := newTrafficFixture(t)
	f.record(t, trafficDay1.Add(13*time.Hour), 5)
	f.record(t, trafficDay1.Add(14*time.Hour), 7)

	total, tables := f.rows(t, trafficDay1.Add(12*time.Hour), trafficDay1.Add(15*time.Hour))
	if total != 12 || tables[trafficHourlyTable] != 2 || tables[trafficDailyTable] != 0 {
		t.Fatalf("total=%d tables=%v, want 12 bytes from 2 hourly rows", total, tables)
	}
}

func TestFindTrafficRowsFallsBackToDailyForPrunedDays(t *testing.T) {
	f := newTrafficFixture(t)
	f.seedThreeDays(t)
	// Cut inside day 2 so its 06:00 row is gone and its 18:00 row survives.
	f.pruneHourlyBefore(t, trafficDay1.AddDate(0, 0, 1).Add(12*time.Hour))

	total, tables := f.rows(t, trafficDay1, trafficDay1.AddDate(0, 0, 3).Add(-time.Second))
	if total != 12 {
		t.Fatalf("total=%d, want every recorded byte", total)
	}
	if tables[trafficDailyTable] != 2 || tables[trafficHourlyTable] != 2 {
		t.Fatalf("tables=%v, want days 1-2 from daily and day 3 from hourly", tables)
	}
}

func TestFindTrafficRowsAssignsDailyRowsByMidpoint(t *testing.T) {
	f := newTrafficFixture(t)
	f.seedThreeDays(t)
	f.pruneHourlyBefore(t, trafficDay1.AddDate(0, 0, 3))

	day2 := trafficDay1.AddDate(0, 0, 1)
	for name, offset := range map[string]time.Duration{"UTC+8": -8 * time.Hour, "UTC-5": 5 * time.Hour} {
		from := day2.Add(offset)
		total, tables := f.rows(t, from, from.Add(24*time.Hour-time.Second))
		if total != 4 || tables[trafficDailyTable] != 1 {
			t.Fatalf("%s local day 2: total=%d tables=%v, want only the day 2 daily row", name, total, tables)
		}
	}
}

func TestParseTrafficRangeRejectsInvalidDatetime(t *testing.T) {
	if _, err := parseTrafficRange("yesterday", ""); err == nil {
		t.Fatal("parseTrafficRange accepted an invalid from")
	}
	r, err := parseTrafficRange("", "2026-01-02 03:04:05.000Z")
	if err != nil || !r.from.IsZero() || !r.to.Equal(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)) {
		t.Fatalf("parseTrafficRange = %+v, %v", r, err)
	}
}
