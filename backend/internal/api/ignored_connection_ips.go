package api

import (
	"strings"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

func publicIgnoredConnectionIP(rec *core.Record) IgnoredConnectionIP {
	return IgnoredConnectionIP{
		ID:      rec.Id,
		IP:      rec.GetString("ip"),
		Created: rec.GetString("created"),
	}
}

func (h *Handlers) loadIgnoredConnectionIPSet() map[string]struct{} {
	recs, err := h.app.FindRecordsByFilter("ignored_connection_ips", "", "", 0, 0)
	if err != nil || len(recs) == 0 {
		return map[string]struct{}{}
	}
	out := make(map[string]struct{}, len(recs))
	for _, rec := range recs {
		ip, ok := normalizeStoredConnectionIP(rec.GetString("ip"))
		if !ok {
			continue
		}
		out[ip] = struct{}{}
	}
	return out
}

func (h *Handlers) isConnectionIPIgnored(ip string) bool {
	normalized, ok := normalizeStoredConnectionIP(ip)
	if !ok {
		return false
	}
	rec, err := h.app.FindFirstRecordByFilter(
		"ignored_connection_ips",
		"ip = {:ip}",
		map[string]any{"ip": normalized},
	)
	return err == nil && rec != nil
}

func (h *Handlers) purgeIPFromAllUsersRecentConnections(ip string) error {
	normalized, ok := normalizeStoredConnectionIP(ip)
	if !ok {
		return nil
	}
	recs, err := h.app.FindRecordsByFilter("users", "", "", 0, 0)
	if err != nil {
		return err
	}
	for _, u := range recs {
		if !removeIPFromRecentConnections(u, normalized) {
			continue
		}
		if err := h.app.Save(u); err != nil {
			return err
		}
	}
	return nil
}

func (h *Handlers) listIgnoredConnectionIPs(e *core.RequestEvent) error {
	recs, err := h.app.FindRecordsByFilter("ignored_connection_ips", "", "-created", 0, 0)
	if err != nil {
		return apis.NewBadRequestError("failed to list ignored connection IPs", err)
	}
	out := make([]IgnoredConnectionIP, 0, len(recs))
	for _, rec := range recs {
		out = append(out, publicIgnoredConnectionIP(rec))
	}
	return ok(e, out)
}

func (h *Handlers) createIgnoredConnectionIP(e *core.RequestEvent) error {
	var in IgnoredConnectionIPCreateRequest
	if err := e.BindBody(&in); err != nil {
		return apis.NewBadRequestError("invalid body", err)
	}
	ip, valid := normalizeStoredConnectionIP(strings.TrimSpace(in.IP))
	if !valid {
		return apis.NewBadRequestError("invalid ip", nil)
	}

	existing, err := h.app.FindFirstRecordByFilter(
		"ignored_connection_ips",
		"ip = {:ip}",
		map[string]any{"ip": ip},
	)
	if err == nil && existing != nil {
		if err := h.purgeIPFromAllUsersRecentConnections(ip); err != nil {
			return apis.NewBadRequestError("failed to purge ignored IP from users", err)
		}
		return ok(e, publicIgnoredConnectionIP(existing))
	}

	coll, err := h.app.FindCollectionByNameOrId("ignored_connection_ips")
	if err != nil {
		return err
	}
	rec := core.NewRecord(coll)
	rec.Set("ip", ip)
	if e.Auth != nil {
		rec.Set("created_by", e.Auth.Id)
	}
	if err := h.app.Save(rec); err != nil {
		return apis.NewBadRequestError("failed to save ignored connection IP", err)
	}
	if err := h.purgeIPFromAllUsersRecentConnections(ip); err != nil {
		return apis.NewBadRequestError("failed to purge ignored IP from users", err)
	}
	return ok(e, publicIgnoredConnectionIP(rec))
}

func (h *Handlers) deleteIgnoredConnectionIP(e *core.RequestEvent) error {
	rec, err := h.app.FindRecordById("ignored_connection_ips", e.Request.PathValue("id"))
	if err != nil {
		return apis.NewNotFoundError("ignored connection IP not found", err)
	}
	if err := h.app.Delete(rec); err != nil {
		return apis.NewBadRequestError("failed to delete ignored connection IP", err)
	}
	return ok(e, map[string]any{"deleted": true})
}
