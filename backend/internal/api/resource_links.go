package api

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	"hysterical-panel/internal/linkmeta"
)

const (
	resourceLinkFetchTimeout = 10 * time.Second
	resourceLinkMaxURLBytes  = 2048
)

var (
	errInvalidResourceLinkURL = errors.New("invalid url")
	errResourceLinkTitleLong  = errors.New("title is too long")
)

func resourceLinkDisplayTitle(manual, fetched, rawURL string) string {
	if manual != "" {
		return manual
	}
	if fetched != "" {
		return fetched
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Hostname() == "" {
		return rawURL
	}
	return parsed.Hostname()
}

func normalizeResourceLinkURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if len(trimmed) > resourceLinkMaxURLBytes {
		return "", errInvalidResourceLinkURL
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return "", errInvalidResourceLinkURL
	}
	return trimmed, nil
}

func normalizeResourceLinkTitle(raw string) (string, error) {
	title := strings.TrimSpace(raw)
	if utf8.RuneCountInString(title) > linkmeta.MaxTitleRunes {
		return "", errResourceLinkTitleLong
	}
	return title, nil
}

func publicResourceLink(rec *core.Record) ResourceLink {
	return ResourceLink{
		ID:             rec.Id,
		Title:          resourceLinkDisplayTitle(rec.GetString("title"), rec.GetString("fetched_title"), rec.GetString("url")),
		FaviconVersion: rec.GetString("favicon_hash"),
	}
}

func managedResourceLink(rec *core.Record) ManagedResourceLink {
	return ManagedResourceLink{
		ResourceLink:  publicResourceLink(rec),
		URL:           rec.GetString("url"),
		ManualTitle:   rec.GetString("title"),
		FetchedTitle:  rec.GetString("fetched_title"),
		VisitCount:    int64(rec.GetInt("visit_count")),
		LastVisitedAt: rec.GetString("last_visited_at"),
		Created:       rec.GetString("created"),
	}
}

// fetchResourceLinkMetadata gives the fetch its own deadline. A slow target
// never blocks the save for longer than resourceLinkFetchTimeout.
func fetchResourceLinkMetadata(e *core.RequestEvent, pageURL string) linkmeta.Metadata {
	ctx, cancel := context.WithTimeout(e.Request.Context(), resourceLinkFetchTimeout)
	defer cancel()
	return linkmeta.Fetch(ctx, pageURL)
}

func applyResourceLinkMetadata(rec *core.Record, meta linkmeta.Metadata) {
	rec.Set("fetched_title", meta.Title)
	if len(meta.Icon) == 0 {
		rec.Set("favicon_data", "")
		rec.Set("favicon_type", "")
		rec.Set("favicon_hash", "")
		return
	}
	rec.Set("favicon_data", base64.StdEncoding.EncodeToString(meta.Icon))
	rec.Set("favicon_type", meta.IconType)
	rec.Set("favicon_hash", resourceLinkFaviconHash(meta.Icon))
}

func resourceLinkFaviconHash(icon []byte) string {
	sum := sha256.Sum256(icon)
	return hex.EncodeToString(sum[:])[:16]
}

func (h *Handlers) listResourceLinks(e *core.RequestEvent) error {
	recs, err := h.app.FindRecordsByFilter("resource_links", "", "sort_order,created", 0, 0)
	if err != nil {
		return apis.NewBadRequestError("failed to list resource links", err)
	}
	out := make([]ResourceLink, 0, len(recs))
	for _, rec := range recs {
		out = append(out, publicResourceLink(rec))
	}
	return ok(e, out)
}

func (h *Handlers) listManagedResourceLinks(e *core.RequestEvent) error {
	recs, err := h.app.FindRecordsByFilter("resource_links", "", "sort_order,created", 0, 0)
	if err != nil {
		return apis.NewBadRequestError("failed to list resource links", err)
	}
	out := make([]ManagedResourceLink, 0, len(recs))
	for _, rec := range recs {
		out = append(out, managedResourceLink(rec))
	}
	return ok(e, out)
}

func (h *Handlers) createResourceLink(e *core.RequestEvent) error {
	var in ResourceLinkCreateRequest
	if err := e.BindBody(&in); err != nil {
		return apis.NewBadRequestError("invalid body", err)
	}
	pageURL, err := normalizeResourceLinkURL(in.URL)
	if err != nil {
		return apis.NewBadRequestError(err.Error(), nil)
	}
	title, err := normalizeResourceLinkTitle(in.Title)
	if err != nil {
		return apis.NewBadRequestError(err.Error(), nil)
	}
	order, err := h.nextResourceLinkSortOrder()
	if err != nil {
		return apis.NewBadRequestError("failed to read resource links", err)
	}
	rec, err := h.newResourceLinkRecord(pageURL, title, order, fetchResourceLinkMetadata(e, pageURL))
	if err != nil {
		return apis.NewBadRequestError("failed to create resource link", err)
	}
	if err := h.app.Save(rec); err != nil {
		return apis.NewBadRequestError("failed to save resource link", err)
	}
	return ok(e, managedResourceLink(rec))
}

func (h *Handlers) nextResourceLinkSortOrder() (int, error) {
	recs, err := h.app.FindRecordsByFilter("resource_links", "", "-sort_order", 1, 0)
	if err != nil {
		return 0, err
	}
	if len(recs) == 0 {
		return 0, nil
	}
	return recs[0].GetInt("sort_order") + 1, nil
}

func (h *Handlers) newResourceLinkRecord(pageURL, title string, order int, meta linkmeta.Metadata) (*core.Record, error) {
	coll, err := h.app.FindCollectionByNameOrId("resource_links")
	if err != nil {
		return nil, err
	}
	rec := core.NewRecord(coll)
	rec.Set("url", pageURL)
	rec.Set("title", title)
	rec.Set("sort_order", order)
	applyResourceLinkMetadata(rec, meta)
	return rec, nil
}

func (h *Handlers) updateResourceLink(e *core.RequestEvent) error {
	rec, err := h.app.FindRecordById("resource_links", e.Request.PathValue("id"))
	if err != nil {
		return apis.NewNotFoundError("resource link not found", err)
	}
	var in ResourceLinkUpdateRequest
	if err := e.BindBody(&in); err != nil {
		return apis.NewBadRequestError("invalid body", err)
	}
	if err := applyResourceLinkUpdate(e, rec, in); err != nil {
		return apis.NewBadRequestError(err.Error(), nil)
	}
	if err := h.app.Save(rec); err != nil {
		return apis.NewBadRequestError("failed to save resource link", err)
	}
	return ok(e, managedResourceLink(rec))
}

func applyResourceLinkUpdate(e *core.RequestEvent, rec *core.Record, in ResourceLinkUpdateRequest) error {
	if in.Title != nil {
		title, err := normalizeResourceLinkTitle(*in.Title)
		if err != nil {
			return err
		}
		rec.Set("title", title)
	}
	if in.URL == nil {
		return nil
	}
	pageURL, err := normalizeResourceLinkURL(*in.URL)
	if err != nil {
		return err
	}
	if pageURL != rec.GetString("url") {
		applyResourceLinkMetadata(rec, fetchResourceLinkMetadata(e, pageURL))
	}
	rec.Set("url", pageURL)
	return nil
}

func (h *Handlers) deleteResourceLink(e *core.RequestEvent) error {
	rec, err := h.app.FindRecordById("resource_links", e.Request.PathValue("id"))
	if err != nil {
		return apis.NewNotFoundError("resource link not found", err)
	}
	if err := h.app.Delete(rec); err != nil {
		return apis.NewBadRequestError("failed to delete resource link", err)
	}
	return ok(e, DeleteResponse{Deleted: true})
}

func (h *Handlers) reorderResourceLinks(e *core.RequestEvent) error {
	var in ResourceLinkOrderRequest
	if err := e.BindBody(&in); err != nil {
		return apis.NewBadRequestError("invalid body", err)
	}
	ordered := make([]ManagedResourceLink, 0, len(in.IDs))
	err := h.app.RunInTransaction(func(txApp core.App) error {
		recs, err := txApp.FindAllRecords("resource_links")
		if err != nil {
			return err
		}
		if !resourceLinkOrderMatches(in.IDs, recs) {
			return apis.NewApiError(409, "resource link order is out of date", nil)
		}
		byID := make(map[string]*core.Record, len(recs))
		for _, rec := range recs {
			byID[rec.Id] = rec
		}
		for index, id := range in.IDs {
			rec := byID[id]
			rec.Set("sort_order", index)
			if err := txApp.Save(rec); err != nil {
				return err
			}
			ordered = append(ordered, managedResourceLink(rec))
		}
		return nil
	})
	if err != nil {
		return err
	}
	return ok(e, ordered)
}

// resourceLinkOrderMatches rejects stale orders. The id list must name every
// stored link exactly once so a concurrent add or delete is never half-applied.
func resourceLinkOrderMatches(ids []string, recs []*core.Record) bool {
	if len(ids) != len(recs) {
		return false
	}
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if _, duplicate := seen[id]; duplicate {
			return false
		}
		seen[id] = struct{}{}
	}
	for _, rec := range recs {
		if _, ok := seen[rec.Id]; !ok {
			return false
		}
	}
	return true
}
