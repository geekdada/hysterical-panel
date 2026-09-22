package api

import (
	"strconv"
	"strings"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/search"
	"github.com/pocketbase/pocketbase/tools/types"

	"hysterical-panel/internal/authstrings"
)

var (
	allowedUserListPerPage = []int{25, 50, 100}
	allowedUserListSorts   = map[string]struct{}{
		"created":            {},
		"-created":           {},
		"email":              {},
		"-email":             {},
		"role":               {},
		"-role":              {},
		"status":             {},
		"-status":            {},
		"last_connected_at":  {},
		"-last_connected_at": {},
		"subscription_used":  {},
		"-subscription_used": {},
		"subscription_tx":    {},
		"-subscription_tx":   {},
		"subscription_rx":    {},
		"-subscription_rx":   {},
	}
	// Columns of the current window usage subquery, keyed by sort field.
	subscriptionUsageSortColumns = map[string]string{
		"subscription_used": "used",
		"subscription_tx":   "tx",
		"subscription_rx":   "rx",
	}
)

// currentWindowUsageSQL yields one row per User with a grant covering
// {:usage_now}, holding that grant's current window usage. It mirrors
// subscriptions.WindowUsage: a stored window other than the current one
// counts as empty. Dates share PocketBase's UTC text format, so they compare
// as strings.
const currentWindowUsageSQL = `(
	SELECT user, MAX(tx) AS tx, MAX(rx) AS rx, MAX(tx + rx) AS used FROM (
		SELECT g.user AS user,
			CASE WHEN g.window_index = CAST((julianday({:usage_now}) - julianday(g.starts_at)) / t.reset_days AS INTEGER)
				THEN CAST(g.used_tx_bytes AS INTEGER) ELSE 0 END AS tx,
			CASE WHEN g.window_index = CAST((julianday({:usage_now}) - julianday(g.starts_at)) / t.reset_days AS INTEGER)
				THEN CAST(g.used_rx_bytes AS INTEGER) ELSE 0 END AS rx
		FROM user_subscriptions g
		JOIN subscription_types t ON t.id = g.subscription_type
		WHERE g.terminated_at = '' AND g.starts_at <= {:usage_now} AND g.ends_at > {:usage_now}
	) GROUP BY user
) usage`

type userListQuery struct {
	Page    int
	PerPage int
	Search  string
	Sort    string
}

func parseUserListQuery(pageRaw, perPageRaw, searchRaw, sortRaw string) userListQuery {
	q := userListQuery{
		Page:    clampUserListPage(pageRaw),
		PerPage: clampUserListPerPage(perPageRaw),
		Search:  strings.TrimSpace(searchRaw),
		Sort:    normalizeUserListSort(sortRaw),
	}
	return q
}

func clampUserListPage(raw string) int {
	if raw == "" {
		return 1
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return 1
	}
	return n
}

func clampUserListPerPage(raw string) int {
	if raw == "" {
		return allowedUserListPerPage[0]
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return allowedUserListPerPage[0]
	}
	best := allowedUserListPerPage[0]
	bestDist := absInt(n - best)
	for _, allowed := range allowedUserListPerPage[1:] {
		dist := absInt(n - allowed)
		if dist < bestDist {
			best = allowed
			bestDist = dist
		}
	}
	return best
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func normalizeUserListSort(raw string) string {
	sort := strings.TrimSpace(raw)
	if sort == "" {
		return "created"
	}
	if _, ok := allowedUserListSorts[sort]; ok {
		return sort
	}
	return "created"
}

func buildUserListFilter(search, authUserID string) (string, dbx.Params) {
	search = strings.TrimSpace(search)
	if search == "" {
		return "", nil
	}
	return "(email ~ {:like} || role ~ {:like} || status ~ {:like} || id = {:search} || id = {:auth_user})",
		dbx.Params{
			"like":      "%" + search + "%",
			"search":    search,
			"auth_user": authUserID,
		}
}

func (h *Handlers) countRecordsByFilter(collection, filter string, params dbx.Params) (int64, error) {
	coll, err := h.app.FindCollectionByNameOrId(collection)
	if err != nil {
		return 0, err
	}

	resolver := core.NewRecordFieldResolver(h.app, coll, nil, true)
	q := h.app.RecordQuery(coll).Select("count(*)").Limit(1)

	if filter != "" {
		expr, err := search.FilterData(filter).BuildExpr(resolver, params)
		if err != nil {
			return 0, err
		}
		q.AndWhere(expr)
	}

	if err := resolver.UpdateQuery(q); err != nil {
		return 0, err
	}

	var total int64
	if err := q.Row(&total); err != nil {
		return 0, err
	}
	return total, nil
}

func (h *Handlers) listUsers(e *core.RequestEvent) error {
	q := parseUserListQuery(
		e.Request.URL.Query().Get("page"),
		e.Request.URL.Query().Get("per_page"),
		e.Request.URL.Query().Get("search"),
		e.Request.URL.Query().Get("sort"),
	)

	authUserID := ""
	if q.Search != "" {
		if user, err := authstrings.FindCurrentUserByAuthString(h.app, q.Search); err == nil && user != nil {
			authUserID = user.Id
		}
	}
	filter, params := buildUserListFilter(q.Search, authUserID)

	total, err := h.countRecordsByFilter("users", filter, params)
	if err != nil {
		return apis.NewBadRequestError("failed to count users", err)
	}

	offset := (q.Page - 1) * q.PerPage
	now := time.Now().UTC()
	var users []*core.Record
	if column, ok := subscriptionUsageSortColumns[strings.TrimPrefix(q.Sort, "-")]; ok {
		users, err = findUsersBySubscriptionUsage(h.app, filter, params, column, strings.HasPrefix(q.Sort, "-"), q.PerPage, offset, now)
	} else {
		users, err = h.app.FindRecordsByFilter("users", filter, q.Sort, q.PerPage, offset, params)
	}
	if err != nil {
		return apis.NewBadRequestError("failed to list users", err)
	}

	subscriptionsByUser, err := currentSubscriptionsByUser(h.app, users, now)
	if err != nil {
		return apis.NewBadRequestError("failed to load subscriptions", err)
	}

	items := make([]UserListItem, 0, len(users))
	ignored := h.loadIgnoredConnectionIPSet()
	resolver, err := authstrings.LoadResolver(h.app)
	if err != nil {
		return apis.NewBadRequestError("failed to load auth strings", err)
	}
	for _, u := range users {
		authString, err := resolver.CurrentForUser(u.Id)
		if err != nil {
			return apis.NewBadRequestError("failed to load auth string", err)
		}
		items = append(items, userListItem(u, userProfile(u, authString, h.ipLookup, ignored), subscriptionsByUser[u.Id]))
	}

	return ok(e, UserListResponse{
		Items:   items,
		Total:   total,
		Page:    q.Page,
		PerPage: q.PerPage,
	})
}

// findUsersBySubscriptionUsage orders Users with a current grant by that
// grant's window usage. Users without one follow in both directions, ordered
// by creation, so the sort only ranks subscribed Users.
func findUsersBySubscriptionUsage(app core.App, filter string, params dbx.Params, column string, desc bool, limit, offset int, now time.Time) ([]*core.Record, error) {
	coll, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		return nil, err
	}
	direction := "ASC"
	if desc {
		direction = "DESC"
	}
	query := app.RecordQuery(coll).
		LeftJoin(currentWindowUsageSQL, dbx.NewExp("[[usage.user]] = [[users.id]]")).
		AndBind(dbx.Params{"usage_now": now.Format(types.DefaultDateLayout)}).
		OrderBy("([[usage.user]] IS NULL) ASC", "([[usage."+column+"]]) "+direction, "users.created ASC", "users.id ASC").
		Limit(int64(limit)).
		Offset(int64(offset))
	if filter != "" {
		resolver := core.NewRecordFieldResolver(app, coll, nil, true)
		expr, err := search.FilterData(filter).BuildExpr(resolver, params)
		if err != nil {
			return nil, err
		}
		query.AndWhere(expr)
		if err := resolver.UpdateQuery(query); err != nil {
			return nil, err
		}
	}
	var users []*core.Record
	if err := query.All(&users); err != nil {
		return nil, err
	}
	return users, nil
}

// userListItem exposes lifetime Traffic only for Legacy Unmetered Users,
// who have no Allowance Window to show instead.
func userListItem(u *core.Record, profile UserProfile, current *UserSubscription) UserListItem {
	item := UserListItem{UserProfile: profile, CurrentSubscription: current}
	if !u.GetBool("subscription_required") {
		tx, rx := int64(u.GetInt("used_tx")), int64(u.GetInt("used_rx"))
		item.UsedTx, item.UsedRx = &tx, &rx
	}
	return item
}

// currentSubscriptionsByUser loads the page's grants in one query and keeps
// the one covering now for each User.
func currentSubscriptionsByUser(app core.App, users []*core.Record, now time.Time) (map[string]*UserSubscription, error) {
	out := make(map[string]*UserSubscription, len(users))
	if len(users) == 0 {
		return out, nil
	}
	ids := make([]any, 0, len(users))
	for _, u := range users {
		ids = append(ids, u.Id)
	}
	var grants []*core.Record
	err := app.RecordQuery("user_subscriptions").
		AndWhere(dbx.In("user", ids...)).
		AndWhere(dbx.HashExp{"terminated_at": ""}).
		All(&grants)
	if err != nil {
		return nil, err
	}
	for _, grant := range grants {
		if now.Before(grant.GetDateTime("starts_at").Time()) || !now.Before(grant.GetDateTime("ends_at").Time()) {
			continue
		}
		view, err := grantView(app, grant, now)
		if err != nil {
			return nil, err
		}
		out[grant.GetString("user")] = &view
	}
	return out, nil
}

func (h *Handlers) getUserStats(e *core.RequestEvent) error {
	total, err := h.app.CountRecords("users")
	if err != nil {
		return apis.NewBadRequestError("failed to count users", err)
	}
	active, err := h.app.CountRecords("users", dbx.HashExp{"status": "active"})
	if err != nil {
		return apis.NewBadRequestError("failed to count active users", err)
	}
	return ok(e, UserStatsResponse{Total: total, Active: active})
}
