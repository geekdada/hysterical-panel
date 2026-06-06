package api

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

const (
	trafficHourlyTable = "traffic_hourly"
	trafficDailyTable  = "traffic_daily"
)

// GET /database/stats
func (h *Handlers) databaseStats(e *core.RequestEvent) error {
	cutoff := trafficRetentionCutoff(time.Now())
	stats, err := databaseManagementStats(h.app, h.app.DataDir(), cutoff)
	if err != nil {
		return apis.NewBadRequestError("failed to read database stats", err)
	}
	return ok(e, stats)
}

// POST /database/prune
func (h *Handlers) pruneDatabaseTraffic(e *core.RequestEvent) error {
	cutoff := trafficRetentionCutoff(time.Now())
	deleted := make([]DatabaseTrafficPruneResult, 0, 2)

	err := h.app.RunInTransaction(func(txApp core.App) error {
		for _, table := range []string{trafficHourlyTable, trafficDailyTable} {
			count, err := deleteTrafficRowsBefore(txApp, table, cutoff)
			if err != nil {
				return err
			}
			deleted = append(deleted, DatabaseTrafficPruneResult{
				Table:       table,
				DeletedRows: count,
			})
		}
		return nil
	})
	if err != nil {
		return apis.NewBadRequestError("failed to prune traffic data", err)
	}

	return ok(e, DatabasePruneResponse{
		Cutoff:  cutoff,
		Deleted: deleted,
	})
}

func databaseManagementStats(app core.App, dataDir string, cutoff string) (DatabaseStatsResponse, error) {
	storage, err := databaseStorageStats(dataDir)
	if err != nil {
		return DatabaseStatsResponse{}, err
	}

	tables := make([]DatabaseTrafficTableStats, 0, 2)
	for _, table := range []string{trafficHourlyTable, trafficDailyTable} {
		points, err := countTrafficRows(app, table)
		if err != nil {
			return DatabaseStatsResponse{}, err
		}
		old, err := countTrafficRowsBefore(app, table, cutoff)
		if err != nil {
			return DatabaseStatsResponse{}, err
		}
		tables = append(tables, DatabaseTrafficTableStats{
			Table:           table,
			Points:          points,
			OlderThan30Days: old,
		})
	}

	return DatabaseStatsResponse{
		Cutoff:        cutoff,
		Storage:       storage,
		TrafficTables: tables,
	}, nil
}

func databaseStorageStats(dataDir string) (DatabaseStorageStats, error) {
	files := []string{"data.db", "data.db-wal", "data.db-shm"}
	stats := DatabaseStorageStats{Files: make([]DatabaseStorageFile, 0, len(files))}
	for _, name := range files {
		size, err := databaseFileSize(filepath.Join(dataDir, name))
		if err != nil {
			return DatabaseStorageStats{}, err
		}
		stats.Files = append(stats.Files, DatabaseStorageFile{Name: name, Bytes: size})
		stats.TotalBytes += size
	}
	return stats, nil
}

func databaseFileSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if info.IsDir() {
		return 0, nil
	}
	return info.Size(), nil
}

func trafficRetentionCutoff(now time.Time) string {
	return now.UTC().AddDate(0, 0, -30).Truncate(time.Second).Format("2006-01-02 15:04:05.000Z")
}

func countTrafficRows(app core.App, table string) (int64, error) {
	sqlTable, err := trafficTableSQL(table)
	if err != nil {
		return 0, err
	}

	var count int64
	err = app.DB().NewQuery(fmt.Sprintf("SELECT COUNT(*) FROM %s", sqlTable)).Row(&count)
	return count, err
}

func countTrafficRowsBefore(app core.App, table string, cutoff string) (int64, error) {
	sqlTable, err := trafficTableSQL(table)
	if err != nil {
		return 0, err
	}

	var count int64
	err = app.DB().
		NewQuery(fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE bucket < {:cutoff}", sqlTable)).
		Bind(dbx.Params{"cutoff": cutoff}).
		Row(&count)
	return count, err
}

func deleteTrafficRowsBefore(app core.App, table string, cutoff string) (int64, error) {
	sqlTable, err := trafficTableSQL(table)
	if err != nil {
		return 0, err
	}

	result, err := app.DB().
		NewQuery(fmt.Sprintf("DELETE FROM %s WHERE bucket < {:cutoff}", sqlTable)).
		Bind(dbx.Params{"cutoff": cutoff}).
		Execute()
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func trafficTableSQL(table string) (string, error) {
	switch table {
	case trafficHourlyTable:
		return "traffic_hourly", nil
	case trafficDailyTable:
		return "traffic_daily", nil
	default:
		return "", fmt.Errorf("unsupported traffic table %q", table)
	}
}
