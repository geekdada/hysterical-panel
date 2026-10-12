package oidc

import (
	"crypto/subtle"
	"net/http"
	"net/url"
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"hysterical-panel/internal/token"
)

// unknownClientHash stands in when the client_id matches nothing, so a wrong
// client_id costs the same comparison as a wrong secret.
var unknownClientHash = strings.Repeat("0", 64)

// FindClient looks a Connected App up by its public client_id.
func FindClient(app core.App, clientID string) (*core.Record, error) {
	return app.FindFirstRecordByFilter(ClientsCollection, "client_id = {:id}", dbx.Params{"id": clientID})
}

// AuthenticateClient checks client_secret_basic or client_secret_post on a
// request whose form is already parsed. A disabled app fails like a wrong
// secret.
func AuthenticateClient(app core.App, r *http.Request) (*core.Record, *Error) {
	clientID, secret, oauthErr := clientCredentials(r)
	if oauthErr != nil {
		return nil, oauthErr
	}
	client, err := FindClient(app, clientID)
	expected := unknownClientHash
	if err == nil {
		expected = client.GetString("secret_hash")
	}
	matches := subtle.ConstantTimeCompare([]byte(token.Sha256Hex(secret)), []byte(expected)) == 1
	if err != nil || !matches || !client.GetBool("enabled") {
		return nil, NewError(InvalidClient, "client authentication failed")
	}
	return client, nil
}

// clientCredentials reads the client_id and secret. Basic credentials are
// form-encoded before base64 (RFC 6749 2.3.1), and a request may use only
// one method (RFC 6749 2.3).
func clientCredentials(r *http.Request) (clientID, secret string, oauthErr *Error) {
	basicID, basicSecret, hasBasic := r.BasicAuth()
	postID, postSecret := r.PostForm.Get("client_id"), r.PostForm.Get("client_secret")
	switch {
	case hasBasic && postSecret != "":
		return "", "", NewError(InvalidRequest, "use one client authentication method")
	case hasBasic:
		clientID, idErr := url.QueryUnescape(basicID)
		secret, secretErr := url.QueryUnescape(basicSecret)
		if idErr != nil || secretErr != nil {
			return "", "", NewError(InvalidClient, "client authentication failed")
		}
		if postID != "" && postID != clientID {
			return "", "", NewError(InvalidRequest, "client_id does not match the authenticated client")
		}
		return clientID, secret, nil
	case postID != "" && postSecret != "":
		return postID, postSecret, nil
	}
	return "", "", NewError(InvalidClient, "client authentication required")
}
