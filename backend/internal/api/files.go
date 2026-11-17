package api

import (
	"errors"
	"net/http"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	"hysterical-panel/internal/uploads"
)

// uploadBodyLimit leaves room for the multipart framing around a file of the
// largest allowed size. It replaces PocketBase's default 32 MiB limit on the
// upload route only.
const uploadBodyLimit = uploads.MaxBytes + 1<<20

const uploadFormField = "file"

func (h *Handlers) uploadFile(e *core.RequestEvent) error {
	files, err := e.FindUploadedFiles(uploadFormField)
	if errors.Is(err, http.ErrMissingFile) {
		return apis.NewBadRequestError("file is required", nil)
	}
	if err != nil {
		return apis.NewBadRequestError("invalid upload", err)
	}
	if len(files) != 1 {
		return apis.NewBadRequestError("upload one file at a time", nil)
	}
	stored, err := uploads.Save(h.app, e.Auth.Id, files[0], declaredUploadMIME(e))
	if errors.Is(err, uploads.ErrTooLarge) {
		return apis.NewApiError(http.StatusRequestEntityTooLarge, err.Error(), nil)
	}
	if err != nil {
		return apis.NewBadRequestError("failed to save file", err)
	}
	return ok(e, UploadedFile{ID: stored.ID, Name: stored.Name, MIME: stored.MIME, Size: stored.Size, Path: stored.Path})
}

// declaredUploadMIME reads the part's Content-Type, which the browser sets
// from the file's type. PocketBase drops part headers when it builds files.
func declaredUploadMIME(e *core.RequestEvent) string {
	form := e.Request.MultipartForm
	if form == nil || len(form.File[uploadFormField]) == 0 {
		return ""
	}
	return form.File[uploadFormField][0].Header.Get("Content-Type")
}
