package subscriptions

import (
	"fmt"
	"math"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

const grantDays = 360

// WindowAt returns the zero-based allowance window for a grant's exact UTC start.
func WindowAt(start, now time.Time, resetDays int) (int, bool) {
	if (resetDays != 30 && resetDays != 360) || now.Before(start) || !now.Before(start.Add(grantDays*24*time.Hour)) {
		return 0, false
	}
	return int(now.Sub(start) / (time.Duration(resetDays) * 24 * time.Hour)), true
}

type State struct {
	Grant     *core.Record
	Type      *core.Record
	Window    int
	Used      int64
	Allowance int64
	Remaining int64
}

func Current(app core.App, userID string, at time.Time) (*State, error) {
	grants, err := app.FindRecordsByFilter("user_subscriptions", "user = {:user} && terminated_at = ''", "-starts_at", 0, 0, map[string]any{"user": userID})
	if err != nil {
		return nil, err
	}
	var grant *core.Record
	for _, candidate := range grants {
		if !at.Before(candidate.GetDateTime("starts_at").Time()) && at.Before(candidate.GetDateTime("ends_at").Time()) {
			grant = candidate
			break
		}
	}
	if grant == nil {
		return nil, nil
	}
	typ, err := app.FindRecordById("subscription_types", grant.GetString("subscription_type"))
	if err != nil {
		return nil, err
	}
	index, ok := WindowAt(grant.GetDateTime("starts_at").Time(), at, typ.GetInt("reset_days"))
	if !ok {
		return nil, fmt.Errorf("invalid subscription window for %s", grant.Id)
	}
	used, extra := int64(0), int64(0)
	if grant.GetInt("window_index") == index {
		used = int64(grant.GetInt("used_bytes"))
		extra = int64(grant.GetInt("extra_bytes"))
	}
	base := int64(typ.GetInt("allowance_bytes"))
	if base <= 0 || extra < 0 || used < 0 || base > math.MaxInt64-extra {
		return nil, fmt.Errorf("invalid subscription allowance for %s", grant.Id)
	}
	allowance := base + extra
	return &State{Grant: grant, Type: typ, Window: index, Used: used, Allowance: allowance, Remaining: allowance - used}, nil
}

func Allowed(app core.App, user *core.Record, at time.Time) (bool, error) {
	if !user.GetBool("subscription_required") {
		return true, nil
	}
	state, err := Current(app, user.Id, at)
	return err == nil && state != nil && state.Remaining > 0, err
}

// AddUsage persists a settled counter delta without changing lifetime User totals.
// The caller must run this in the same transaction as the cursor and totals.
func AddUsage(app core.App, userID string, at time.Time, bytes int64) (bool, error) {
	if bytes < 0 {
		return false, fmt.Errorf("negative usage")
	}
	state, err := Current(app, userID, at)
	if err != nil || state == nil {
		return false, err
	}
	if bytes > math.MaxInt64-state.Used {
		return false, fmt.Errorf("subscription usage overflow")
	}
	newWindow := state.Grant.GetInt("window_index") != state.Window
	state.Grant.Set("window_index", state.Window)
	state.Grant.Set("used_bytes", state.Used+bytes)
	if newWindow {
		state.Grant.Set("extra_bytes", 0)
	}
	if err := app.Save(state.Grant); err != nil {
		return false, err
	}
	return state.Remaining > 0 && state.Remaining-bytes <= 0, nil
}
