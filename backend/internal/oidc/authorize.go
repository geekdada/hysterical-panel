package oidc

import (
	"math"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"hysterical-panel/internal/token"
)

// Error page reasons for requests whose redirect_uri cannot be trusted, so
// the error must not be sent there (RFC 6749 4.1.2.1).
const (
	PageInvalidRequest     = "invalid_request"
	PageInvalidClient      = "invalid_client"
	PageInvalidRedirectURI = "invalid_redirect_uri"
)

// maxParamLength matches the state, nonce and scope field limits.
const maxParamLength = 1024

var codeChallengePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)

// authorizeParams may appear at most once. Others, such as ui_locales, are
// accepted and ignored (OIDC Core 15.1).
var authorizeParams = []string{
	"client_id", "redirect_uri", "response_type", "response_mode", "scope", "state",
	"nonce", "code_challenge", "code_challenge_method", "prompt", "max_age",
}

// AuthorizeRequest is a validated authorization request.
type AuthorizeRequest struct {
	Client        *core.Record
	RedirectURI   string
	Scopes        []string
	State         string
	Nonce         string
	CodeChallenge string
	Prompt        []string
	// MaxAge is 0 when absent. max_age=0 is stored as prompt=login.
	MaxAge int
}

// AuthorizeFailure is an error page reason when the client or redirect_uri
// cannot be trusted, and otherwise an OAuth error for RedirectURI.
type AuthorizeFailure struct {
	Page        string
	Err         *Error
	RedirectURI string
	State       string
}

// ParseAuthorizeRequest validates an authorization request. The client and
// redirect_uri come first because every later error is sent to that URI.
func ParseAuthorizeRequest(app core.App, form url.Values) (*AuthorizeRequest, *AuthorizeFailure) {
	if len(form["client_id"]) > 1 || len(form["redirect_uri"]) > 1 {
		return nil, &AuthorizeFailure{Page: PageInvalidRequest}
	}
	client, err := FindClient(app, form.Get("client_id"))
	if err != nil || !client.GetBool("enabled") {
		return nil, &AuthorizeFailure{Page: PageInvalidClient}
	}
	req := &AuthorizeRequest{Client: client, RedirectURI: form.Get("redirect_uri")}
	if !slices.Contains(ClientRedirectURIs(client), req.RedirectURI) {
		return nil, &AuthorizeFailure{Page: PageInvalidRedirectURI}
	}
	if oauthErr := req.parse(form); oauthErr != nil {
		return nil, &AuthorizeFailure{Err: oauthErr, RedirectURI: req.RedirectURI, State: req.State}
	}
	return req, nil
}

func (r *AuthorizeRequest) parse(form url.Values) *Error {
	steps := []func(url.Values) *Error{r.parseState, checkRequestShape, r.parseScope, r.parsePKCE, r.parseNonce, r.parsePrompt}
	for _, step := range steps {
		if oauthErr := step(form); oauthErr != nil {
			return oauthErr
		}
	}
	return nil
}

// parseState runs first so later errors can echo the state back.
func (r *AuthorizeRequest) parseState(form url.Values) *Error {
	state := form.Get("state")
	if len(form["state"]) > 1 || len(state) > maxParamLength {
		return NewError(InvalidRequest, "state must appear once and stay within 1024 characters")
	}
	r.State = state
	return nil
}

func checkRequestShape(form url.Values) *Error {
	for _, name := range authorizeParams {
		if len(form[name]) > 1 {
			return NewError(InvalidRequest, name+" must not repeat")
		}
	}
	responseType := form.Get("response_type")
	switch {
	case form.Has("request"):
		return NewError(RequestNotSupported, "request objects are not supported")
	case form.Has("request_uri"):
		return NewError(RequestURINotSupported, "request_uri is not supported")
	case responseType == "":
		return NewError(InvalidRequest, "response_type is required")
	case responseType != "code":
		return NewError(UnsupportedResponseType, "only response_type=code is supported")
	case form.Has("response_mode") && form.Get("response_mode") != "query":
		return NewError(InvalidRequest, "only response_mode=query is supported")
	}
	return nil
}

func (r *AuthorizeRequest) parseScope(form url.Values) *Error {
	raw := form.Get("scope")
	if len(raw) > maxParamLength {
		return NewError(InvalidRequest, "scope is too long")
	}
	requested := strings.Fields(raw)
	if !slices.Contains(requested, ScopeOpenID) {
		return NewError(InvalidScope, "scope must include openid")
	}
	r.Scopes = grantableScopes(r.Client, requested)
	return nil
}

func (r *AuthorizeRequest) parsePKCE(form url.Values) *Error {
	if form.Get("code_challenge_method") != "S256" {
		return NewError(InvalidRequest, "code_challenge_method must be S256")
	}
	challenge := form.Get("code_challenge")
	if !codeChallengePattern.MatchString(challenge) {
		return NewError(InvalidRequest, "code_challenge must be a base64url SHA-256 value")
	}
	r.CodeChallenge = challenge
	return nil
}

func (r *AuthorizeRequest) parseNonce(form url.Values) *Error {
	nonce := form.Get("nonce")
	if len(nonce) > maxParamLength {
		return NewError(InvalidRequest, "nonce is too long")
	}
	r.Nonce = nonce
	return nil
}

// parsePrompt reads prompt and max_age together, because max_age=0 demands
// a fresh sign-in and is stored as prompt=login.
func (r *AuthorizeRequest) parsePrompt(form url.Values) *Error {
	values := strings.Fields(form.Get("prompt"))
	for _, value := range values {
		if !slices.Contains(PromptValues, value) {
			return NewError(InvalidRequest, "unsupported prompt value")
		}
	}
	none := slices.Contains(values, PromptNone)
	if none && len(orderPrompt(values)) > 1 {
		return NewError(InvalidRequest, "prompt=none cannot be combined with other values")
	}
	maxAge, present, oauthErr := parseMaxAge(form.Get("max_age"))
	if oauthErr != nil {
		return oauthErr
	}
	if present && maxAge == 0 {
		if none {
			return NewError(LoginRequired, "max_age=0 needs an interactive sign-in")
		}
		values = append(values, PromptLogin)
	}
	r.Prompt, r.MaxAge = orderPrompt(values), maxAge
	return nil
}

func orderPrompt(values []string) []string {
	out := []string{}
	for _, value := range PromptValues {
		if slices.Contains(values, value) {
			out = append(out, value)
		}
	}
	return out
}

func parseMaxAge(raw string) (seconds int, present bool, oauthErr *Error) {
	if raw == "" {
		return 0, false, nil
	}
	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds < 0 || seconds > math.MaxInt32 {
		return 0, false, NewError(InvalidRequest, "max_age must be a non-negative integer")
	}
	return seconds, true, nil
}

// SaveAuthRequest stores a validated request for the consent page and
// returns its handle. Only the handle's hash is stored.
func SaveAuthRequest(app core.App, req *AuthorizeRequest, now time.Time) (string, error) {
	handle, err := token.New(32)
	if err != nil {
		return "", err
	}
	collection, err := app.FindCollectionByNameOrId(AuthRequestsCollection)
	if err != nil {
		return "", err
	}
	rec := core.NewRecord(collection)
	rec.Set("request_hash", token.Sha256Hex(handle))
	rec.Set("client", req.Client.Id)
	rec.Set("redirect_uri", req.RedirectURI)
	rec.Set("scope", strings.Join(req.Scopes, " "))
	rec.Set("state", req.State)
	rec.Set("nonce", req.Nonce)
	rec.Set("code_challenge", req.CodeChallenge)
	rec.Set("prompt", strings.Join(req.Prompt, " "))
	rec.Set("max_age", req.MaxAge)
	rec.Set("expires_at", now.UTC().Add(AuthRequestTTL))
	return handle, app.Save(rec)
}

// ErrorRedirect sends an OAuth error to a registered redirect_uri, with the
// RFC 9207 iss parameter.
func ErrorRedirect(redirectURI string, oauthErr *Error, state, issuer string) string {
	params := url.Values{"error": {oauthErr.Code}, "iss": {issuer}}
	if oauthErr.Description != "" {
		params.Set("error_description", oauthErr.Description)
	}
	if state != "" {
		params.Set("state", state)
	}
	return appendQuery(redirectURI, params)
}

func codeRedirect(redirectURI, code, state, issuer string) string {
	params := url.Values{"code": {code}, "iss": {issuer}}
	if state != "" {
		params.Set("state", state)
	}
	return appendQuery(redirectURI, params)
}

// appendQuery leaves the registered query untouched, because redirect_uri is
// compared byte for byte.
func appendQuery(uri string, params url.Values) string {
	if strings.Contains(uri, "?") {
		return uri + "&" + params.Encode()
	}
	return uri + "?" + params.Encode()
}
