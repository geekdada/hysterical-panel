package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"
)

func TestManagementAPILooksUsersUpByID(t *testing.T) {
	app := newMigratedTestApp(t)
	user := newUsersTestRecord(t, app, "mgmt-id@example.com", "IDLookupSecret")
	h := &Handlers{app: app}

	cases := []struct {
		query  string
		status int
	}{
		{"id=" + user.Id, http.StatusOK},
		{"id=missinguser0000", http.StatusNotFound},
		{"id=" + user.Id + "&email=mgmt-id@example.com", http.StatusBadRequest},
		{"id=" + user.Id + "&auth_string=IDLookupSecret", http.StatusBadRequest},
		{"", http.StatusBadRequest},
	}
	for _, tc := range cases {
		res := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/mgmt/users?"+tc.query, nil)
		err := h.mgmtGetUser(&core.RequestEvent{App: app, Event: router.Event{Request: req, Response: res}})
		if status := mgmtStatus(t, err); status != tc.status {
			t.Fatalf("%q: status = %d, want %d", tc.query, status, tc.status)
		}
		if tc.status != http.StatusOK {
			continue
		}
		var body PanelUser
		if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil || body.ID != user.Id || body.AuthString != "IDLookupSecret" {
			t.Fatalf("body = %+v, err = %v", body, err)
		}
	}
}

func mgmtStatus(t *testing.T, err error) int {
	t.Helper()
	if err == nil {
		return http.StatusOK
	}
	var apiErr *router.ApiError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error %v is not an ApiError", err)
	}
	return apiErr.Status
}
