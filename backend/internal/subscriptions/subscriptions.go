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

// Usage is Traffic counted against a User Subscription, split by direction.
type Usage struct {
	Tx int64
	Rx int64
}

// Total cannot overflow because every write rejects a Usage whose sum would.
func (u Usage) Total() int64 { return u.Tx + u.Rx }

func (u Usage) add(tx, rx int64) (Usage, error) {
	if tx > math.MaxInt64-u.Tx || rx > math.MaxInt64-u.Rx {
		return Usage{}, fmt.Errorf("subscription usage overflow")
	}
	next := Usage{Tx: u.Tx + tx, Rx: u.Rx + rx}
	if next.Rx > math.MaxInt64-next.Tx {
		return Usage{}, fmt.Errorf("subscription usage overflow")
	}
	return next, nil
}

func (u Usage) valid() bool {
	return u.Tx >= 0 && u.Rx >= 0 && u.Rx <= math.MaxInt64-u.Tx
}

// WindowUsage returns the usage and top-up stored for window index. A grant
// only keeps the window it was last written in, so any other window is empty.
func WindowUsage(grant *core.Record, index int) (Usage, int64) {
	if grant.GetInt("window_index") != index {
		return Usage{}, 0
	}
	used := Usage{Tx: storedBytes(grant, "used_tx_bytes"), Rx: storedBytes(grant, "used_rx_bytes")}
	return used, storedBytes(grant, "extra_bytes")
}

// SetWindow replaces the grant's window state without saving the record.
func SetWindow(grant *core.Record, index int, used Usage, extra int64) {
	grant.Set("window_index", index)
	grant.Set("used_tx_bytes", used.Tx)
	grant.Set("used_rx_bytes", used.Rx)
	grant.Set("extra_bytes", extra)
}

// GrantUsage is the Traffic counted across every window of the grant. Window
// rollover, top-ups and Type allowance edits do not reset it.
func GrantUsage(grant *core.Record) Usage {
	return Usage{Tx: storedBytes(grant, "grant_tx_bytes"), Rx: storedBytes(grant, "grant_rx_bytes")}
}

// storedBytes reads the decimal text byte counters; see the subscriptions
// migration for why they are not NumberFields.
func storedBytes(record *core.Record, field string) int64 {
	return int64(record.GetInt(field))
}

type State struct {
	Grant     *core.Record
	Type      *core.Record
	Window    int
	Used      Usage
	Extra     int64
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
	used, extra := WindowUsage(grant, index)
	base := storedBytes(typ, "allowance_bytes")
	if base <= 0 || extra < 0 || !used.valid() || !GrantUsage(grant).valid() || base > math.MaxInt64-extra {
		return nil, fmt.Errorf("invalid subscription allowance for %s", grant.Id)
	}
	allowance := base + extra
	return &State{Grant: grant, Type: typ, Window: index, Used: used, Extra: extra, Allowance: allowance, Remaining: allowance - used.Total()}, nil
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
func AddUsage(app core.App, userID string, at time.Time, tx, rx int64) (bool, error) {
	if tx < 0 || rx < 0 {
		return false, fmt.Errorf("negative usage")
	}
	state, err := Current(app, userID, at)
	if err != nil || state == nil {
		return false, err
	}
	used, err := state.Used.add(tx, rx)
	if err != nil {
		return false, err
	}
	grantUsed, err := GrantUsage(state.Grant).add(tx, rx)
	if err != nil {
		return false, err
	}
	SetWindow(state.Grant, state.Window, used, state.Extra)
	state.Grant.Set("grant_tx_bytes", grantUsed.Tx)
	state.Grant.Set("grant_rx_bytes", grantUsed.Rx)
	if err := app.Save(state.Grant); err != nil {
		return false, err
	}
	return state.Remaining > 0 && state.Allowance-used.Total() <= 0, nil
}
