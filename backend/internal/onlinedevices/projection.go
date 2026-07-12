// Package onlinedevices persists the latest online client-instance projection.
package onlinedevices

import (
	"github.com/pocketbase/pocketbase/core"
)

// DeleteNodeCounts removes every per-user count currently attributed to nodeID.
// Callers that need atomic lifecycle changes should pass their transaction app.
func DeleteNodeCounts(app core.App, nodeID string) error {
	counts, err := app.FindRecordsByFilter(
		"online_device_counts",
		"node = {:node}",
		"",
		0,
		0,
		map[string]any{"node": nodeID},
	)
	if err != nil {
		return err
	}
	for _, count := range counts {
		if err := app.Delete(count); err != nil {
			return err
		}
	}
	return nil
}
