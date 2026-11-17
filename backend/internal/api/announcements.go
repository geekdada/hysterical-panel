package api

import (
	"encoding/json"
	"errors"
	"strconv"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"

	"hysterical-panel/internal/announcements"
)

const announcementsPerPage = 20

var errAnnouncementBodyIncomplete = errors.New("html and content must be sent together")

var announcementInputErrors = []error{
	announcements.ErrTitleRequired,
	announcements.ErrTitleTooLong,
	announcements.ErrBodyEmpty,
	announcements.ErrBodyTooLarge,
	announcements.ErrContentInvalid,
	announcements.ErrPinnedHidden,
}

// requireUsableAccount re-checks on every request what bindAuthGate checks at
// sign-in. A token issued before the account was disabled stays valid until
// it expires, and readers must lose access as soon as that happens.
func requireUsableAccount() *hook.Handler[*core.RequestEvent] {
	return &hook.Handler[*core.RequestEvent]{
		Func: func(e *core.RequestEvent) error {
			if e.Auth == nil || e.Auth.GetString("status") != "active" || !e.Auth.GetBool("verified") {
				return apis.NewForbiddenError("active account required", nil)
			}
			return e.Next()
		},
	}
}

func announcementSummary(rec *core.Record) AnnouncementSummary {
	return AnnouncementSummary{ID: rec.Id, Title: rec.GetString("title"), Created: rec.GetString("created")}
}

func announcementSummaries(recs []*core.Record) []AnnouncementSummary {
	out := make([]AnnouncementSummary, 0, len(recs))
	for _, rec := range recs {
		out = append(out, announcementSummary(rec))
	}
	return out
}

func managedAnnouncement(rec *core.Record) ManagedAnnouncement {
	return ManagedAnnouncement{
		ID:      rec.Id,
		Title:   rec.GetString("title"),
		Pinned:  rec.GetBool("pinned"),
		Hidden:  rec.GetBool("hidden"),
		Created: rec.GetString("created"),
		Updated: rec.GetString("updated"),
	}
}

func managedAnnouncementDetail(rec *core.Record) ManagedAnnouncementDetail {
	var content map[string]any
	if err := rec.UnmarshalJSONField("content", &content); err != nil || content == nil {
		content = map[string]any{}
	}
	return ManagedAnnouncementDetail{
		ManagedAnnouncement: managedAnnouncement(rec),
		HTML:                rec.GetString("html"),
		Content:             content,
	}
}

// announcementError maps validation failures to 400. Anything else is a
// storage failure and stays a server error.
func announcementError(err error) error {
	for _, known := range announcementInputErrors {
		if errors.Is(err, known) {
			return apis.NewBadRequestError(known.Error(), nil)
		}
	}
	return err
}

func announcementBody(html string, content map[string]any) (announcements.Body, error) {
	raw, err := json.Marshal(content)
	if err != nil {
		return announcements.Body{}, announcements.ErrContentInvalid
	}
	return announcements.Body{HTML: html, Content: raw}, nil
}

func (h *Handlers) listPinnedAnnouncements(e *core.RequestEvent) error {
	recs, err := announcements.ListPinned(h.app)
	if err != nil {
		return err
	}
	published, err := announcements.HasPublished(h.app)
	if err != nil {
		return err
	}
	return ok(e, PinnedAnnouncementsResponse{Items: announcementSummaries(recs), HasPublished: published})
}

func (h *Handlers) listAnnouncements(e *core.RequestEvent) error {
	page, _ := strconv.Atoi(e.Request.URL.Query().Get("page"))
	page = max(page, 1)
	recs, total, err := announcements.ListPublished(h.app, page, announcementsPerPage)
	if err != nil {
		return err
	}
	return ok(e, AnnouncementListResponse{
		Items:   announcementSummaries(recs),
		Total:   total,
		Page:    page,
		PerPage: announcementsPerPage,
	})
}

func (h *Handlers) getAnnouncement(e *core.RequestEvent) error {
	rec, err := announcements.FindPublished(h.app, e.Request.PathValue("id"))
	if err != nil {
		return apis.NewNotFoundError("announcement not found", nil)
	}
	return ok(e, Announcement{
		ID:      rec.Id,
		Title:   rec.GetString("title"),
		HTML:    rec.GetString("html"),
		Created: rec.GetString("created"),
		Updated: rec.GetString("updated"),
	})
}

func (h *Handlers) listManagedAnnouncements(e *core.RequestEvent) error {
	recs, err := announcements.ListAll(h.app)
	if err != nil {
		return err
	}
	out := make([]ManagedAnnouncement, 0, len(recs))
	for _, rec := range recs {
		out = append(out, managedAnnouncement(rec))
	}
	return ok(e, out)
}

func (h *Handlers) getManagedAnnouncement(e *core.RequestEvent) error {
	rec, err := h.app.FindRecordById(announcements.Collection, e.Request.PathValue("id"))
	if err != nil {
		return apis.NewNotFoundError("announcement not found", nil)
	}
	return ok(e, managedAnnouncementDetail(rec))
}

func (h *Handlers) createAnnouncement(e *core.RequestEvent) error {
	var in AnnouncementCreateRequest
	if err := e.BindBody(&in); err != nil {
		return apis.NewBadRequestError("invalid body", err)
	}
	body, err := announcementBody(in.HTML, in.Content)
	if err != nil {
		return announcementError(err)
	}
	rec, err := announcements.Create(h.app, e.Auth.Id, announcements.Input{
		Title: in.Title,
		Body:  body,
		Flags: announcements.Flags{Pinned: in.Pinned, Hidden: in.Hidden},
	})
	if err != nil {
		return announcementError(err)
	}
	return ok(e, managedAnnouncementDetail(rec))
}

func (h *Handlers) updateAnnouncement(e *core.RequestEvent) error {
	rec, err := h.app.FindRecordById(announcements.Collection, e.Request.PathValue("id"))
	if err != nil {
		return apis.NewNotFoundError("announcement not found", nil)
	}
	var in AnnouncementUpdateRequest
	if err := e.BindBody(&in); err != nil {
		return apis.NewBadRequestError("invalid body", err)
	}
	patch, err := announcementPatch(in)
	if err != nil {
		return announcementError(err)
	}
	if err := announcements.Update(h.app, rec, patch); err != nil {
		return announcementError(err)
	}
	return ok(e, managedAnnouncementDetail(rec))
}

func announcementPatch(in AnnouncementUpdateRequest) (announcements.Patch, error) {
	patch := announcements.Patch{Title: in.Title, Pinned: in.Pinned, Hidden: in.Hidden}
	if (in.HTML == nil) != (in.Content == nil) {
		return patch, apis.NewBadRequestError(errAnnouncementBodyIncomplete.Error(), nil)
	}
	if in.HTML != nil {
		body, err := announcementBody(*in.HTML, *in.Content)
		if err != nil {
			return patch, err
		}
		patch.Body = &body
	}
	return patch, nil
}

func (h *Handlers) deleteAnnouncement(e *core.RequestEvent) error {
	rec, err := h.app.FindRecordById(announcements.Collection, e.Request.PathValue("id"))
	if err != nil {
		return apis.NewNotFoundError("announcement not found", nil)
	}
	if err := h.app.Delete(rec); err != nil {
		return err
	}
	return ok(e, DeleteResponse{Deleted: true})
}
