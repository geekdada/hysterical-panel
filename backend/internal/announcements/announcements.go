// Package announcements stores administrator-written Announcements and
// enforces their body, pin and hide rules (ADR-0015).
package announcements

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
)

const (
	Collection = "announcements"
	// MaxTitleRunes mirrors the Max of the title field.
	MaxTitleRunes   = 200
	MaxHTMLBytes    = 256 << 10
	MaxContentBytes = 1 << 20
)

var (
	ErrTitleRequired  = errors.New("title is required")
	ErrTitleTooLong   = errors.New("title is too long")
	ErrBodyEmpty      = errors.New("body is empty")
	ErrBodyTooLarge   = errors.New("body is too large")
	ErrContentInvalid = errors.New("editor content must be a JSON object")
	ErrPinnedHidden   = errors.New("a hidden announcement cannot be pinned")
)

// Body is the editor output. HTML is rendered to readers and Content lets the
// editor load the Announcement again.
type Body struct {
	HTML    string
	Content json.RawMessage
}

type Flags struct{ Pinned, Hidden bool }

type Input struct {
	Title string
	Body  Body
	Flags Flags
}

// Patch leaves a nil field unchanged.
type Patch struct {
	Title  *string
	Body   *Body
	Pinned *bool
	Hidden *bool
}

// ResolveFlags applies requested changes to the current flags. Hiding unpins
// in the same change, and pinning a hidden Announcement is refused.
func ResolveFlags(current Flags, pinned, hidden *bool) (Flags, error) {
	next := current
	if hidden != nil {
		next.Hidden = *hidden
	}
	if pinned != nil {
		if *pinned && next.Hidden {
			return Flags{}, ErrPinnedHidden
		}
		next.Pinned = *pinned
	}
	if next.Hidden {
		next.Pinned = false
	}
	return next, nil
}

func normalizeTitle(raw string) (string, error) {
	title := strings.TrimSpace(raw)
	if title == "" {
		return "", ErrTitleRequired
	}
	if utf8.RuneCountInString(title) > MaxTitleRunes {
		return "", ErrTitleTooLong
	}
	return title, nil
}

// prepareBody sanitizes the HTML and checks both parts of the editor output.
func prepareBody(body Body) (Body, error) {
	content := bytes.TrimSpace(body.Content)
	if len(content) > MaxContentBytes {
		return Body{}, ErrBodyTooLarge
	}
	if len(content) == 0 || content[0] != '{' || !json.Valid(content) {
		return Body{}, ErrContentInvalid
	}
	clean := SanitizeHTML(body.HTML)
	if len(clean) > MaxHTMLBytes {
		return Body{}, ErrBodyTooLarge
	}
	if !hasVisibleContent(clean) {
		return Body{}, ErrBodyEmpty
	}
	return Body{HTML: clean, Content: content}, nil
}

func Create(app core.App, createdBy string, in Input) (*core.Record, error) {
	title, err := normalizeTitle(in.Title)
	if err != nil {
		return nil, err
	}
	body, err := prepareBody(in.Body)
	if err != nil {
		return nil, err
	}
	flags, err := ResolveFlags(Flags{}, &in.Flags.Pinned, &in.Flags.Hidden)
	if err != nil {
		return nil, err
	}
	collection, err := app.FindCollectionByNameOrId(Collection)
	if err != nil {
		return nil, err
	}
	record := core.NewRecord(collection)
	record.Set("title", title)
	setBody(record, body)
	setFlags(record, flags)
	record.Set("created_by", createdBy)
	if err := app.Save(record); err != nil {
		return nil, err
	}
	return record, nil
}

// Update validates the whole patch before changing the record.
func Update(app core.App, record *core.Record, patch Patch) error {
	flags, err := ResolveFlags(FlagsOf(record), patch.Pinned, patch.Hidden)
	if err != nil {
		return err
	}
	if patch.Title != nil {
		title, err := normalizeTitle(*patch.Title)
		if err != nil {
			return err
		}
		record.Set("title", title)
	}
	if patch.Body != nil {
		body, err := prepareBody(*patch.Body)
		if err != nil {
			return err
		}
		setBody(record, body)
	}
	setFlags(record, flags)
	return app.Save(record)
}

func FlagsOf(record *core.Record) Flags {
	return Flags{Pinned: record.GetBool("pinned"), Hidden: record.GetBool("hidden")}
}

func setBody(record *core.Record, body Body) {
	record.Set("html", body.HTML)
	record.Set("content", types.JSONRaw(body.Content))
}

func setFlags(record *core.Record, flags Flags) {
	record.Set("pinned", flags.Pinned)
	record.Set("hidden", flags.Hidden)
}

// newestFirst breaks ties on id so pages stay stable when two share a timestamp.
const newestFirst = "-created,-id"

// ListAll returns every Announcement, hidden ones included, for administrators.
func ListAll(app core.App) ([]*core.Record, error) {
	return app.FindRecordsByFilter(Collection, "", newestFirst, 0, 0)
}

// ListPinned returns the Pinned Announcements readers see on their user page.
func ListPinned(app core.App) ([]*core.Record, error) {
	return app.FindRecordsByFilter(Collection, "pinned = true && hidden = false", newestFirst, 0, 0)
}

// ListPublished returns one page of Announcements visible to readers and
// the total count across all pages.
func ListPublished(app core.App, page, perPage int) ([]*core.Record, int, error) {
	total, err := app.CountRecords(Collection, dbx.HashExp{"hidden": false})
	if err != nil {
		return nil, 0, err
	}
	offset := (max(page, 1) - 1) * perPage
	records, err := app.FindRecordsByFilter(Collection, "hidden = false", newestFirst, perPage, offset)
	if err != nil {
		return nil, 0, err
	}
	return records, int(total), nil
}

// HasPublished reports whether readers can see any Announcement at all.
func HasPublished(app core.App) (bool, error) {
	total, err := app.CountRecords(Collection, dbx.HashExp{"hidden": false})
	return total > 0, err
}

// FindPublished returns a visible Announcement. A hidden one is reported as
// missing, so readers cannot tell it apart from an unknown id.
func FindPublished(app core.App, id string) (*core.Record, error) {
	return app.FindFirstRecordByFilter(Collection, "id = {:id} && hidden = false", dbx.Params{"id": id})
}
