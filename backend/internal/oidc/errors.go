package oidc

import "net/url"

// OAuth 2.0 and OIDC error codes (RFC 6749 4.1.2.1 and 5.2, OIDC Core 3.1.2.6).
const (
	InvalidRequest          = "invalid_request"
	InvalidClient           = "invalid_client"
	InvalidGrant            = "invalid_grant"
	InvalidScope            = "invalid_scope"
	UnsupportedGrantType    = "unsupported_grant_type"
	UnsupportedResponseType = "unsupported_response_type"
	AccessDenied            = "access_denied"
	LoginRequired           = "login_required"
	ConsentRequired         = "consent_required"
	RequestNotSupported     = "request_not_supported"
	RequestURINotSupported  = "request_uri_not_supported"
	ServerError             = "server_error"
	TemporarilyUnavailable  = "temporarily_unavailable"
)

// Error is an OAuth error response. Description goes to the client, so it
// never carries internal details. cause is only for the server log.
type Error struct {
	Code        string
	Description string
	cause       error
}

func NewError(code, description string) *Error {
	return &Error{Code: code, Description: description}
}

// WrapServerError hides an internal failure behind server_error.
func WrapServerError(cause error) *Error {
	return &Error{Code: ServerError, cause: cause}
}

func (e *Error) Error() string {
	if e.cause != nil {
		return e.Code + ": " + e.cause.Error()
	}
	return e.Code + ": " + e.Description
}

func (e *Error) Unwrap() error { return e.cause }

// HasRepeatedParams reports whether any parameter appears more than once,
// which RFC 6749 3.1 and 3.2 forbid.
func HasRepeatedParams(form url.Values) bool {
	for _, values := range form {
		if len(values) > 1 {
			return true
		}
	}
	return false
}
