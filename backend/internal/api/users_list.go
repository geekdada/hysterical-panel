package api

import (
	"strconv"
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/search"
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
		"used_tx":            {},
		"-used_tx":           {},
		"used_rx":            {},
		"-used_rx":           {},
		"last_connected_at":  {},
		"-last_connected_at": {},
	}
)

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

func buildUserListFilter(search string) (string, dbx.Params) {
	search = strings.TrimSpace(search)
	if search == "" {
		return "", nil
	}
	return "(email ~ {:like} || role ~ {:like} || status ~ {:like} || auth_string = {:auth})",
		dbx.Params{
			"like": "%" + search + "%",
			"auth": search,
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

	filter, params := buildUserListFilter(q.Search)

	total, err := h.countRecordsByFilter("users", filter, params)
	if err != nil {
		return apis.NewBadRequestError("failed to count users", err)
	}

	offset := (q.Page - 1) * q.PerPage
	users, err := h.app.FindRecordsByFilter("users", filter, q.Sort, q.PerPage, offset, params)
	if err != nil {
		return apis.NewBadRequestError("failed to list users", err)
	}

	items := make([]PanelUser, 0, len(users))
	ignored := h.loadIgnoredConnectionIPSet()
	for _, u := range users {
		items = append(items, panelUser(u, h.ipLookup, ignored))
	}

	return ok(e, UserListResponse{
		Items:   items,
		Total:   total,
		Page:    q.Page,
		PerPage: q.PerPage,
	})
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
