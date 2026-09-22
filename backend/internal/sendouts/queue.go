package sendouts

import (
	"errors"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

var (
	ErrCancelled       = errors.New("email sendout is cancelled")
	ErrNothingToCancel = errors.New("email sendout has no pending recipients")
)

// Draft is a validated Sendout as submitted by the browser. HTML and Text are
// stored and sent unchanged.
type Draft struct {
	Subject  string
	Language string
	Audience string
	HTML     string
	Text     string
	Content  any
}

// Create stores a Sendout and queues one pending Sendout Recipient per User in
// one transaction, so a Sendout never exists with a partial audience.
func Create(app core.App, draft Draft, users []*core.Record, createdBy *core.Record, now time.Time) (*core.Record, error) {
	var sendout *core.Record
	err := app.RunInTransaction(func(tx core.App) error {
		rec, err := saveSendout(tx, draft, createdBy)
		if err != nil {
			return err
		}
		sendout = rec
		return queueRecipients(tx, rec.Id, users, now)
	})
	return sendout, err
}

func saveSendout(app core.App, draft Draft, createdBy *core.Record) (*core.Record, error) {
	coll, err := app.FindCollectionByNameOrId(SendoutsCollection)
	if err != nil {
		return nil, err
	}
	rec := core.NewRecord(coll)
	rec.Set("subject", draft.Subject)
	rec.Set("language", draft.Language)
	rec.Set("audience", draft.Audience)
	rec.Set("html", draft.HTML)
	rec.Set("text", draft.Text)
	rec.Set("content", draft.Content)
	if createdBy != nil {
		rec.Set("created_by", createdBy.Id)
		rec.Set("created_by_email", createdBy.Email())
	}
	return rec, app.Save(rec)
}

func queueRecipients(app core.App, sendoutID string, users []*core.Record, now time.Time) error {
	coll, err := app.FindCollectionByNameOrId(RecipientsCollection)
	if err != nil {
		return err
	}
	for _, user := range users {
		rec := core.NewRecord(coll)
		rec.Set("sendout", sendoutID)
		rec.Set("user", user.Id)
		rec.Set("email", user.Email())
		rec.Set("status", StatusPending)
		rec.Set("queued_at", now)
		if err := app.Save(rec); err != nil {
			return err
		}
	}
	return nil
}

// Counts summarises a Sendout's recipients. Pending includes rows the worker
// is sending right now.
type Counts struct {
	Total     int64
	Pending   int64
	Sent      int64
	Failed    int64
	Skipped   int64
	Cancelled int64
}

func (c *Counts) add(status string, n int64) {
	c.Total += n
	switch status {
	case StatusPending, StatusSending:
		c.Pending += n
	case StatusSent:
		c.Sent += n
	case StatusFailed:
		c.Failed += n
	case StatusSkipped:
		c.Skipped += n
	case StatusCancelled:
		c.Cancelled += n
	}
}

type statusCount struct {
	Sendout string `db:"sendout"`
	Status  string `db:"status"`
	N       int64  `db:"n"`
}

// CountRecipients returns per-status counts keyed by Sendout ID. An empty
// sendoutID counts every Sendout.
func CountRecipients(app core.App, sendoutID string) (map[string]Counts, error) {
	query := "SELECT sendout, status, COUNT(*) AS n FROM " + RecipientsCollection
	params := dbx.Params{}
	if sendoutID != "" {
		query += " WHERE sendout = {:id}"
		params["id"] = sendoutID
	}
	var rows []statusCount
	if err := app.DB().NewQuery(query + " GROUP BY sendout, status").Bind(params).All(&rows); err != nil {
		return nil, err
	}
	out := map[string]Counts{}
	for _, row := range rows {
		c := out[row.Sendout]
		c.add(row.Status, row.N)
		out[row.Sendout] = c
	}
	return out, nil
}

// Status is derived from the recipients rather than stored, so it always
// matches what the worker, cancel and resend did.
func Status(sendout *core.Record, counts Counts) string {
	if !sendout.GetDateTime("cancelled_at").IsZero() {
		return SendoutStatusCancelled
	}
	if counts.Pending > 0 {
		return SendoutStatusSending
	}
	return SendoutStatusCompleted
}

// Cancel stops the rows the worker has not claimed yet. A row already in
// `sending` finishes normally.
func Cancel(app core.App, sendout *core.Record, now time.Time) error {
	if !sendout.GetDateTime("cancelled_at").IsZero() {
		return ErrCancelled
	}
	counts, err := CountRecipients(app, sendout.Id)
	if err != nil {
		return err
	}
	if counts[sendout.Id].Pending == 0 {
		return ErrNothingToCancel
	}
	return app.RunInTransaction(func(tx core.App) error {
		_, err := tx.DB().Update(RecipientsCollection,
			dbx.Params{"status": StatusCancelled, "updated": now.Format(dbDateLayout)},
			dbx.HashExp{"sendout": sendout.Id, "status": StatusPending},
		).Execute()
		if err != nil {
			return err
		}
		sendout.Set("cancelled_at", now)
		return tx.Save(sendout)
	})
}

// Requeue puts failed rows back at the tail of the queue. With no IDs it
// requeues every failed row of the Sendout; other statuses are never touched.
func Requeue(app core.App, sendout *core.Record, recipientIDs []string, now time.Time) (int64, error) {
	if !sendout.GetDateTime("cancelled_at").IsZero() {
		return 0, ErrCancelled
	}
	var where dbx.Expression = dbx.HashExp{"sendout": sendout.Id, "status": StatusFailed}
	if len(recipientIDs) > 0 {
		ids := make([]any, len(recipientIDs))
		for i, id := range recipientIDs {
			ids[i] = id
		}
		where = dbx.And(where, dbx.In("id", ids...))
	}
	stamp := now.Format(dbDateLayout)
	res, err := app.DB().Update(RecipientsCollection,
		dbx.Params{"status": StatusPending, "reason": "", "queued_at": stamp, "updated": stamp},
		where,
	).Execute()
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
