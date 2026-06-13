package api

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	"hysterical-panel/internal/token"
)

// generateUniqueInviteCode mints a short alphanumeric invite code, retrying on
// the chance of a collision with an existing code. Six characters keep the code
// easy to share by hand while the unique check guards against the larger
// collision odds of the smaller space.
func (h *Handlers) generateUniqueInviteCode() (string, error) {
	for i := 0; i < 8; i++ {
		candidate, err := token.Alphanumeric(6)
		if err != nil {
			return "", err
		}
		existing, _ := h.app.FindFirstRecordByFilter(
			"invitations",
			"code = {:c}",
			map[string]any{"c": candidate},
		)
		if existing == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("could not generate a unique invite code")
}

// invalidInviteReason is the pure core of invite validity, kept record-free so
// it can be unit-tested directly. It returns "" when the invite is usable.
func invalidInviteReason(revoked bool, expiresAt time.Time, maxUses, usedCount int, now time.Time) string {
	if revoked {
		return "revoked"
	}
	if !expiresAt.IsZero() && now.After(expiresAt) {
		return "expired"
	}
	if maxUses > 0 && usedCount >= maxUses {
		return "exhausted"
	}
	return ""
}

// inviteValid reports whether an invitation can still be used to register, and
// a short reason string when it cannot.
func inviteValid(inv *core.Record, now time.Time) (bool, string) {
	reason := invalidInviteReason(
		inv.GetBool("revoked"),
		inv.GetDateTime("expires_at").Time(),
		inv.GetInt("max_uses"),
		inv.GetInt("used_count"),
		now,
	)
	return reason == "", reason
}

// publicInvitation renders an invitation record for admin clients. The code is
// intentionally included — admins need it to share the invite.
func (h *Handlers) publicInvitation(inv *core.Record) Invitation {
	valid, reason := inviteValid(inv, time.Now().UTC())
	return Invitation{
		ID:            inv.Id,
		Code:          inv.GetString("code"),
		Email:         inv.GetString("email"),
		MaxUses:       inv.GetInt("max_uses"),
		UsedCount:     inv.GetInt("used_count"),
		ExpiresAt:     inv.GetString("expires_at"),
		Revoked:       inv.GetBool("revoked"),
		Note:          inv.GetString("note"),
		LastUsedAt:    inv.GetString("last_used_at"),
		Created:       inv.GetString("created"),
		Valid:         valid,
		InvalidReason: reason,
		Link:          h.inviteLink(inv.GetString("code")),
	}
}

func (h *Handlers) listInvitations(e *core.RequestEvent) error {
	recs, err := h.app.FindRecordsByFilter("invitations", "", "-created", 0, 0)
	if err != nil {
		return apis.NewBadRequestError("failed to list invitations", err)
	}
	out := make([]Invitation, 0, len(recs))
	for _, inv := range recs {
		out = append(out, h.publicInvitation(inv))
	}
	return ok(e, out)
}

func (h *Handlers) createInvitation(e *core.RequestEvent) error {
	if !h.loadSettings().InvitationsEnabled {
		return apis.NewBadRequestError("invitations are disabled", nil)
	}
	var in InvitationCreateRequest
	if err := e.BindBody(&in); err != nil {
		return apis.NewBadRequestError("invalid body", err)
	}
	if in.MaxUses != nil && *in.MaxUses < 0 {
		return apis.NewBadRequestError("max_uses must be >= 0", nil)
	}
	if in.ExpiresInHours != nil && *in.ExpiresInHours < 0 {
		return apis.NewBadRequestError("expires_in_hours must be >= 0", nil)
	}

	coll, err := h.app.FindCollectionByNameOrId("invitations")
	if err != nil {
		return err
	}
	code, err := h.generateUniqueInviteCode()
	if err != nil {
		return apis.NewBadRequestError("failed to generate invite code", err)
	}

	inv := core.NewRecord(coll)
	inv.Set("code", code)
	inv.Set("used_count", 0)
	inv.Set("revoked", false)
	if in.Email != nil {
		inv.Set("email", strings.TrimSpace(*in.Email))
	}
	if in.MaxUses != nil {
		inv.Set("max_uses", *in.MaxUses)
	}
	if in.Note != nil {
		inv.Set("note", strings.TrimSpace(*in.Note))
	}
	if in.ExpiresInHours != nil && *in.ExpiresInHours > 0 {
		inv.Set("expires_at", time.Now().UTC().Add(time.Duration(*in.ExpiresInHours)*time.Hour))
	}
	if e.Auth != nil {
		inv.Set("created_by", e.Auth.Id)
	}
	if err := h.app.Save(inv); err != nil {
		return apis.NewBadRequestError("failed to create invitation", err)
	}

	resp := h.publicInvitation(inv)
	if in.SendEmail != nil && *in.SendEmail && inv.GetString("email") != "" {
		sent, mailErr := h.sendInviteEmail(inv.GetString("email"), resp.Link)
		resp.EmailSent = &sent
		if mailErr != nil {
			log.Printf("[invitations] failed to send invite email: %v", mailErr)
		}
	}
	return ok(e, resp)
}

func (h *Handlers) deleteInvitation(e *core.RequestEvent) error {
	inv, err := h.app.FindRecordById("invitations", e.Request.PathValue("id"))
	if err != nil {
		return apis.NewNotFoundError("invitation not found", err)
	}
	if err := h.app.Delete(inv); err != nil {
		return apis.NewBadRequestError("failed to delete invitation", err)
	}
	return ok(e, map[string]any{"deleted": true})
}
