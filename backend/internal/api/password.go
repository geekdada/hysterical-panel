package api

import (
	"errors"

	validation "github.com/pocketbase/ozzo-validation/v4"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

// passwordErrorMessages maps PocketBase password field validation codes to
// stable messages the frontend can localize. The limits themselves stay in the
// users collection settings.
var passwordErrorMessages = map[string]string{
	"validation_min_text_constraint": "password is too short",
	"validation_max_text_constraint": "password is too long",
	"validation_invalid_format":      "password format is invalid",
}

// changeOwnPassword replaces the signed-in User's password without asking for
// the current one. Saving a new password rotates the tokenKey and signs out
// every other session, so the response carries a replacement token. The empty
// auth method marks it as a session continuation: login tracking skips it and
// the OIDC auth time carries over from the previous token.
func (h *Handlers) changeOwnPassword(e *core.RequestEvent) error {
	var in PasswordChangeRequest
	if err := e.BindBody(&in); err != nil {
		return apis.NewBadRequestError("invalid body", err)
	}
	if in.Password == "" {
		return apis.NewBadRequestError("password is required", nil)
	}
	if in.Password != in.PasswordConfirm {
		return apis.NewBadRequestError("passwords do not match", nil)
	}
	u, err := h.app.FindRecordById("users", e.Request.PathValue("id"))
	if err != nil {
		return apis.NewNotFoundError("user not found", err)
	}
	u.SetPassword(in.Password)
	if err := h.app.Save(u); err != nil {
		return apis.NewBadRequestError(passwordSaveErrorMessage(err), err)
	}
	return apis.RecordAuthResponse(e, u, "", nil)
}

func passwordSaveErrorMessage(err error) string {
	var fieldErrors validation.Errors
	if errors.As(err, &fieldErrors) {
		var fieldErr validation.Error
		if errors.As(fieldErrors[core.FieldNamePassword], &fieldErr) {
			if message, ok := passwordErrorMessages[fieldErr.Code()]; ok {
				return message
			}
		}
	}
	return "failed to change password"
}
