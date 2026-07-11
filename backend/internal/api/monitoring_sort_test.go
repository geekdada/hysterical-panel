package api

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

func TestSortAlertsHistoryUsesNewestStartedAtFirst(t *testing.T) {
	newerWarning := testAlertRecord("warning", "2026-07-11T09:00:00Z")
	olderCritical := testAlertRecord("critical", "2026-07-11T08:00:00Z")
	records := []*core.Record{olderCritical, newerWarning}

	sortAlerts(records, true)

	if records[0] != newerWarning {
		t.Fatal("expected newer warning alert before older critical alert in history")
	}
}

func TestSortAlertsCurrentStillPrioritizesSeverity(t *testing.T) {
	newerWarning := testAlertRecord("warning", "2026-07-11T09:00:00Z")
	olderCritical := testAlertRecord("critical", "2026-07-11T08:00:00Z")
	records := []*core.Record{newerWarning, olderCritical}

	sortAlerts(records, false)

	if records[0] != olderCritical {
		t.Fatal("expected critical alert before warning alert in current-alert ordering")
	}
}

func testAlertRecord(severity, startedAt string) *core.Record {
	record := core.NewRecord(core.NewBaseCollection("alerts"))
	record.Set("status", "cancelled")
	record.Set("severity_snapshot", severity)
	record.Set("started_at", startedAt)
	return record
}
