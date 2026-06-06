package api

import (
	"github.com/pocketbase/pocketbase/core"
)

func (h *Handlers) handlePanelConfig(e *core.RequestEvent) error {
	return ok(e, h.publicConfig)
}
