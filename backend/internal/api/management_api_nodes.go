package api

import (
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

// mgmtUpdateNode is the Management API PATCH for a node. The Collector sends
// the stored secret to whatever api_url the node has, so a token holder must
// not move a node without replacing its secret (ADR-0011).
func (h *Handlers) mgmtUpdateNode(e *core.RequestEvent) error {
	n, err := h.findActiveNode(e.Request.PathValue("id"))
	if err != nil {
		return err
	}
	var in nodeInput
	if err := e.BindBody(&in); err != nil {
		return apis.NewBadRequestError("invalid body", err)
	}
	if nodeURLChanged(n, in) && in.APISecret == nil {
		return apis.NewBadRequestError("api_secret is required when api_url changes", nil)
	}
	return h.applyNodeUpdate(e, n, in)
}

func nodeURLChanged(n *core.Record, in nodeInput) bool {
	if in.APIURL == nil {
		return false
	}
	return *in.APIURL != n.GetString("api_url")
}
