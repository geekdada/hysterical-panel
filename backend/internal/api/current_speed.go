package api

import (
	"sort"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

// currentSpeedTopUsers caps the per-user list on a Node's Current Speed.
const currentSpeedTopUsers = 10

const speedingCursorFilter = "(current_tx_speed > 0 || current_rx_speed > 0)"

// GET /users/:id/current-speed
// The User's nonzero Current Speed on each visible Node.
func (h *Handlers) userCurrentSpeed(e *core.RequestEvent) error {
	u, err := h.app.FindRecordById("users", e.Request.PathValue("id"))
	if err != nil {
		return apis.NewNotFoundError("user not found", err)
	}
	nodes, err := h.nodesForUser(u.Id)
	if err != nil {
		return apis.NewBadRequestError("failed to read nodes", err)
	}
	cursors, err := h.app.FindRecordsByFilter(
		"traffic_cursor", "user = {:u} && "+speedingCursorFilter, "", 0, 0,
		map[string]any{"u": u.Id},
	)
	if err != nil {
		return apis.NewBadRequestError("failed to read current speed", err)
	}

	visible := make(map[string]*core.Record, len(nodes))
	for _, n := range nodes {
		visible[n.Id] = n
	}
	out := UserCurrentSpeedResponse{ByNode: []NodeTraffic{}}
	for _, cursor := range cursors {
		n := visible[cursor.GetString("node")]
		if n == nil {
			continue
		}
		row := NodeTraffic{
			Node: NodeRef{ID: n.Id, Name: n.GetString("name")},
			Tx:   int64(cursor.GetInt("current_tx_speed")),
			Rx:   int64(cursor.GetInt("current_rx_speed")),
		}
		out.ByNode = append(out.ByNode, row)
		out.Total.Tx += row.Tx
		out.Total.Rx += row.Rx
	}
	sort.Slice(out.ByNode, func(i, j int) bool {
		return out.ByNode[i].Tx+out.ByNode[i].Rx > out.ByNode[j].Tx+out.ByNode[j].Rx
	})
	return ok(e, out)
}

// GET /nodes/:id/current-speed
// The Node's Current Speed and its fastest Users.
func (h *Handlers) nodeCurrentSpeed(e *core.RequestEvent) error {
	n, err := h.findActiveNode(e.Request.PathValue("id"))
	if err != nil {
		return err
	}
	out := NodeCurrentSpeedResponse{
		Total: ByteCount{
			Tx: int64(n.GetInt("current_tx_speed")),
			Rx: int64(n.GetInt("current_rx_speed")),
		},
		TopUsers: []UserTraffic{},
	}
	if !n.GetBool("enabled") {
		return ok(e, out)
	}
	cursors, err := h.app.FindRecordsByFilter(
		"traffic_cursor", "node = {:n} && "+speedingCursorFilter, "", 0, 0,
		map[string]any{"n": n.Id},
	)
	if err != nil {
		return apis.NewBadRequestError("failed to read current speed", err)
	}
	sort.Slice(cursors, func(i, j int) bool {
		return cursorSpeedTotal(cursors[i]) > cursorSpeedTotal(cursors[j])
	})
	if len(cursors) > currentSpeedTopUsers {
		cursors = cursors[:currentSpeedTopUsers]
	}
	emails, err := h.userEmailsByID(cursors)
	if err != nil {
		return apis.NewBadRequestError("failed to read users", err)
	}
	for _, cursor := range cursors {
		userID := cursor.GetString("user")
		out.TopUsers = append(out.TopUsers, UserTraffic{
			User: UserRef{ID: userID, Email: emails[userID]},
			Tx:   int64(cursor.GetInt("current_tx_speed")),
			Rx:   int64(cursor.GetInt("current_rx_speed")),
		})
	}
	return ok(e, out)
}

func cursorSpeedTotal(cursor *core.Record) int64 {
	return int64(cursor.GetInt("current_tx_speed")) + int64(cursor.GetInt("current_rx_speed"))
}

func (h *Handlers) userEmailsByID(cursors []*core.Record) (map[string]string, error) {
	ids := make([]string, 0, len(cursors))
	for _, cursor := range cursors {
		ids = append(ids, cursor.GetString("user"))
	}
	users, err := h.app.FindRecordsByIds("users", ids)
	if err != nil {
		return nil, err
	}
	emails := make(map[string]string, len(users))
	for _, u := range users {
		emails[u.Id] = u.GetString("email")
	}
	return emails, nil
}
