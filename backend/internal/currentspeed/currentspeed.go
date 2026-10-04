// Package currentspeed persists each User's Current Speed on a Node.
package currentspeed

import (
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// ResetNode zeroes the Current Speed of every User on nodeID except keepUserIDs.
// Callers that need atomic lifecycle changes should pass their transaction app.
func ResetNode(app core.App, nodeID string, keepUserIDs ...string) error {
	keep := make([]any, len(keepUserIDs))
	for i, id := range keepUserIDs {
		keep[i] = id
	}
	_, err := app.DB().Update(
		"traffic_cursor",
		dbx.Params{"current_tx_speed": 0, "current_rx_speed": 0},
		dbx.And(
			dbx.HashExp{"node": nodeID},
			dbx.Or(dbx.NewExp("current_tx_speed != 0"), dbx.NewExp("current_rx_speed != 0")),
			dbx.NotIn("user", keep...),
		),
	).Execute()
	return err
}
