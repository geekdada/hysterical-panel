package oidc

import (
	"fmt"
	"log"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
)

const cleanupSchedule = "*/30 * * * *"

type cleanupRule struct {
	collection string
	filter     string
	cutoff     time.Duration
}

// Expired tokens stay for expiredTokenRetention so a late refresh token
// replay still finds its row and revokes the family.
var cleanupRules = []cleanupRule{
	{AuthRequestsCollection, "expires_at < {:cutoff}", 0},
	{CodesCollection, "expires_at < {:cutoff}", 0},
	{SessionAuthTimesCollection, "expires_at < {:cutoff}", 0},
	{TokensCollection, "expires_at < {:cutoff}", expiredTokenRetention},
	{SigningKeysCollection, "state = 'retired' && retired_at < {:cutoff}", RetiredKeyRetention},
}

// Cleanup deletes provider rows that can no longer be used.
func Cleanup(app core.App, now time.Time) error {
	for _, rule := range cleanupRules {
		cutoff := now.UTC().Add(-rule.cutoff).Format(types.DefaultDateLayout)
		recs, err := app.FindRecordsByFilter(rule.collection, rule.filter, "", 0, 0, dbx.Params{"cutoff": cutoff})
		if err != nil {
			return fmt.Errorf("cleanup %s: %w", rule.collection, err)
		}
		for _, rec := range recs {
			if err := app.Delete(rec); err != nil {
				return fmt.Errorf("cleanup %s: %w", rule.collection, err)
			}
		}
	}
	return nil
}

// ScheduleCleanup runs Cleanup every 30 minutes on the PocketBase cron, which
// the serve command starts.
func ScheduleCleanup(app core.App) {
	app.Cron().MustAdd("oidcCleanup", cleanupSchedule, func() {
		if err := Cleanup(app, time.Now()); err != nil {
			log.Printf("[oidc] cleanup: %v", err)
		}
	})
}
