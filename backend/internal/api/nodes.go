package api

import (
	"context"
	"time"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	"hysterical-panel/internal/hysteria"
	"hysterical-panel/internal/token"
)

type nodeInput struct {
	Name         *string `json:"name"`
	APIURL       *string `json:"api_url"`
	APISecret    *string `json:"api_secret"`
	PollInterval *int    `json:"poll_interval"`
	Enabled      *bool   `json:"enabled"`
}

const activeNodesFilter = "deleted_at = ''"

func nodeDeleted(n *core.Record) bool {
	return !n.GetDateTime("deleted_at").IsZero()
}

func (h *Handlers) findActiveNode(id string) (*core.Record, error) {
	n, err := h.app.FindRecordById("nodes", id)
	if err != nil {
		return nil, apis.NewNotFoundError("node not found", err)
	}
	if nodeDeleted(n) {
		return nil, apis.NewNotFoundError("node not found", nil)
	}
	return n, nil
}

func (h *Handlers) nodeRefByID(nodeID string) map[string]any {
	n, err := h.app.FindRecordById("nodes", nodeID)
	if err != nil {
		return map[string]any{"id": nodeID, "name": nodeID, "deleted": true}
	}
	return map[string]any{
		"id":      n.Id,
		"name":    n.GetString("name"),
		"deleted": nodeDeleted(n),
	}
}

func (h *Handlers) listNodes(e *core.RequestEvent) error {
	nodes, err := h.app.FindRecordsByFilter("nodes", activeNodesFilter, "-created", 0, 0)
	if err != nil {
		return apis.NewBadRequestError("failed to list nodes", err)
	}
	out := make([]map[string]any, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, publicNode(n))
	}
	return ok(e, out)
}

func (h *Handlers) getNode(e *core.RequestEvent) error {
	n, err := h.findActiveNode(e.Request.PathValue("id"))
	if err != nil {
		return err
	}
	return ok(e, publicNode(n))
}

func (h *Handlers) createNode(e *core.RequestEvent) error {
	var in nodeInput
	if err := e.BindBody(&in); err != nil {
		return apis.NewBadRequestError("invalid body", err)
	}
	if in.Name == nil || *in.Name == "" || in.APIURL == nil || *in.APIURL == "" ||
		in.APISecret == nil || *in.APISecret == "" {
		return apis.NewBadRequestError("name, api_url and api_secret are required", nil)
	}
	enc, err := h.box.Encrypt(*in.APISecret)
	if err != nil {
		return apis.NewBadRequestError("failed to encrypt secret", err)
	}
	coll, err := h.app.FindCollectionByNameOrId("nodes")
	if err != nil {
		return err
	}
	n := core.NewRecord(coll)
	n.Set("name", *in.Name)
	n.Set("api_url", *in.APIURL)
	n.Set("api_secret", enc)
	if in.PollInterval != nil && *in.PollInterval > 0 {
		n.Set("poll_interval", *in.PollInterval)
	} else {
		n.Set("poll_interval", 30)
	}
	if in.Enabled != nil {
		n.Set("enabled", *in.Enabled)
	} else {
		n.Set("enabled", true)
	}
	if err := h.app.Save(n); err != nil {
		return apis.NewBadRequestError("failed to save node", err)
	}
	return ok(e, publicNode(n))
}

func (h *Handlers) updateNode(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	n, err := h.findActiveNode(id)
	if err != nil {
		return err
	}
	var in nodeInput
	if err := e.BindBody(&in); err != nil {
		return apis.NewBadRequestError("invalid body", err)
	}
	if in.Name != nil {
		n.Set("name", *in.Name)
	}
	if in.APIURL != nil {
		n.Set("api_url", *in.APIURL)
	}
	if in.APISecret != nil {
		// omitted => unchanged; empty string => explicit error (avoid wiping secret)
		if *in.APISecret == "" {
			return apis.NewBadRequestError("api_secret cannot be empty; omit the field to keep it unchanged", nil)
		}
		enc, err := h.box.Encrypt(*in.APISecret)
		if err != nil {
			return apis.NewBadRequestError("failed to encrypt secret", err)
		}
		n.Set("api_secret", enc)
	}
	if in.PollInterval != nil && *in.PollInterval > 0 {
		n.Set("poll_interval", *in.PollInterval)
	}
	if in.Enabled != nil {
		n.Set("enabled", *in.Enabled)
		if !*in.Enabled {
			n.Set("current_tx_speed", 0)
			n.Set("current_rx_speed", 0)
		}
	}
	if err := h.app.Save(n); err != nil {
		return apis.NewBadRequestError("failed to save node", err)
	}
	return ok(e, publicNode(n))
}

func (h *Handlers) deleteNode(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	n, err := h.findActiveNode(id)
	if err != nil {
		return err
	}
	n.Set("deleted_at", time.Now().UTC())
	n.Set("enabled", false)
	n.Set("current_tx_speed", 0)
	n.Set("current_rx_speed", 0)
	if err := h.app.Save(n); err != nil {
		return apis.NewBadRequestError("failed to delete node", err)
	}
	return ok(e, map[string]any{"deleted": true})
}

func (h *Handlers) testNode(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	n, err := h.findActiveNode(id)
	if err != nil {
		return err
	}
	secret, err := h.box.Decrypt(n.GetString("api_secret"))
	if err != nil {
		return ok(e, map[string]any{"ok": false, "error": "decrypt secret failed"})
	}
	cl := hysteria.New(n.GetString("api_url"), secret, 5*time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	latency, err := cl.Ping(ctx)
	if err != nil {
		return ok(e, map[string]any{"ok": false, "error": err.Error()})
	}
	return ok(e, map[string]any{"ok": true, "latency_ms": latency.Milliseconds()})
}

// resetNodeAPISecret rotates a node's Hysteria Stats API secret. The new value
// is server-generated; traffic history is unchanged. The old secret stops working
// on the next collector poll until the Hysteria server is reconfigured.
func (h *Handlers) resetNodeAPISecret(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	n, err := h.findActiveNode(id)
	if err != nil {
		return err
	}
	secret, err := token.New(32)
	if err != nil {
		return apis.NewBadRequestError("failed to reset api secret", err)
	}
	enc, err := h.box.Encrypt(secret)
	if err != nil {
		return apis.NewBadRequestError("failed to encrypt secret", err)
	}
	n.Set("api_secret", enc)
	if err := h.app.Save(n); err != nil {
		return apis.NewBadRequestError("failed to reset api secret", err)
	}
	return ok(e, map[string]any{
		"api_secret": secret,
		"node":       publicNode(n),
	})
}
