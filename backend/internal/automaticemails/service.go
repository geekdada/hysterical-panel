package automaticemails

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"hysterical-panel/internal/sendouts"
	"hysterical-panel/internal/subscriptions"
)

const (
	scanInterval = 60 * time.Second

	templatesCollection = "automatic_email_templates"
	statesCollection    = "automatic_email_states"

	defaultLowThresholdPercent = 10
	scopeFilter                = "status = 'active' && verified = true && subscription_required = true"
)

// Notifier wakes the Sendout worker after rows are queued.
type Notifier interface{ Notify() }

type Service struct {
	app         core.App
	notifier    Notifier
	frontendURL string
}

func New(app core.App, notifier Notifier, frontendURL string) *Service {
	return &Service{app: app, notifier: notifier, frontendURL: frontendURL}
}

type kindTemplate struct {
	enabled bool
	content any
	Template
}

type scanTemplates struct {
	byKind           map[Kind]kindTemplate
	thresholdPercent int64
}

func (s *Service) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(scanInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := s.Scan(time.Now().UTC()); err != nil {
					log.Printf("[automatic-emails] scan: %v", err)
				}
			}
		}
	}()
}

// Scan compares each in-scope User's allowance with the last recorded state and
// queues one Sendout per enabled transition. A failure for one User does not
// stop the others.
func (s *Service) Scan(now time.Time) error {
	templates, err := s.loadTemplates()
	if err != nil {
		return err
	}
	users, err := s.app.FindRecordsByFilter("users", scopeFilter, "", 0, 0)
	if err != nil {
		return err
	}
	queued := false
	for _, user := range users {
		sent, err := s.scanUser(user, templates, now)
		if err != nil {
			log.Printf("[automatic-emails] user=%s: %v", user.Id, err)
		}
		queued = queued || sent
	}
	if queued {
		s.notifier.Notify()
	}
	return nil
}

func (s *Service) loadTemplates() (scanTemplates, error) {
	rows, err := s.app.FindAllRecords(templatesCollection)
	if err != nil {
		return scanTemplates{}, err
	}
	loaded := scanTemplates{byKind: map[Kind]kindTemplate{}, thresholdPercent: defaultLowThresholdPercent}
	for _, row := range rows {
		kind, ok := ParseKind(row.GetString("kind"))
		if !ok {
			continue
		}
		loaded.byKind[kind] = kindTemplate{
			enabled:  row.GetBool("enabled"),
			content:  row.Get("content"),
			Template: Template{Subject: row.GetString("subject"), HTML: row.GetString("html"), Text: row.GetString("text")},
		}
		if kind == KindLowAllowance && row.GetInt("threshold_percent") > 0 {
			loaded.thresholdPercent = int64(row.GetInt("threshold_percent"))
		}
	}
	return loaded, nil
}

func (s *Service) scanUser(user *core.Record, templates scanTemplates, now time.Time) (bool, error) {
	state, err := subscriptions.Current(s.app, user.Id, now)
	if err != nil || state == nil {
		return false, err
	}
	current := Condition{
		Low:       subscriptions.IsLowAllowance(state.Remaining, state.Allowance, templates.thresholdPercent),
		Exhausted: state.Remaining <= 0,
	}
	row, previous, err := s.loadState(user.Id)
	if err != nil {
		return false, err
	}
	kinds := pickKinds(previous, current, templates)
	if row != nil && previous == current && len(kinds) == 0 {
		return false, nil
	}
	err = s.app.RunInTransaction(func(tx core.App) error {
		if err := saveState(tx, row, user.Id, current); err != nil {
			return err
		}
		return s.queue(tx, user, state, kinds, templates, now)
	})
	return err == nil && len(kinds) > 0, err
}

// pickKinds applies Decide, then falls back to the Low email when the
// exhausted email is disabled but the same transition also crossed Low.
func pickKinds(previous, current Condition, templates scanTemplates) []Kind {
	picked := []Kind{}
	for _, kind := range Decide(previous, current) {
		if kind == KindAllowanceExhausted && !templates.byKind[kind].enabled && current.Low && !previous.Low {
			kind = KindLowAllowance
		}
		if templates.byKind[kind].enabled {
			picked = append(picked, kind)
		}
	}
	return picked
}

func (s *Service) loadState(userID string) (*core.Record, Condition, error) {
	row, err := s.app.FindFirstRecordByData(statesCollection, "user", userID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, Condition{}, nil
	}
	if err != nil {
		return nil, Condition{}, err
	}
	return row, Condition{Low: row.GetBool("low"), Exhausted: row.GetBool("exhausted")}, nil
}

func saveState(app core.App, row *core.Record, userID string, current Condition) error {
	if row == nil {
		collection, err := app.FindCollectionByNameOrId(statesCollection)
		if err != nil {
			return err
		}
		row = core.NewRecord(collection)
		row.Set("user", userID)
	}
	row.Set("low", current.Low)
	row.Set("exhausted", current.Exhausted)
	return app.Save(row)
}

func (s *Service) queue(tx core.App, user *core.Record, state *subscriptions.State, kinds []Kind, templates scanTemplates, now time.Time) error {
	vars := Variables(user.Email(), state, s.frontendURL)
	for _, kind := range kinds {
		tmpl := templates.byKind[kind]
		rendered := Render(tmpl.Template, vars)
		draft := sendouts.Draft{
			Subject:       rendered.Subject,
			HTML:          rendered.HTML,
			Text:          rendered.Text,
			Content:       tmpl.content,
			Language:      sendouts.LanguageBilingual,
			Audience:      sendouts.AudienceSingle,
			Source:        sendouts.SourceAutomatic,
			AutomaticKind: string(kind),
		}
		if _, err := sendouts.Create(tx, draft, []*core.Record{user}, nil, now); err != nil {
			return err
		}
	}
	return nil
}
