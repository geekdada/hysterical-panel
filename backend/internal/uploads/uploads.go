// Package uploads stores files in the generic, unprotected files collection
// (ADR-0015). Anyone holding a file's path can download it without signing in.
package uploads

import (
	"errors"
	"mime"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/filesystem"
)

const (
	Collection = "files"
	// PathPrefix is where PocketBase serves records of Collection.
	PathPrefix = "/api/files/" + Collection + "/"
	// MaxBytes mirrors the MaxSize of the file field.
	MaxBytes     = 1 << 30
	maxNameRunes = 255
	maxMIMEBytes = 255
	fallbackMIME = "application/octet-stream"
)

var ErrTooLarge = errors.New("file is too large")

// Stored describes a saved file. Path is relative so stored content survives
// a change of API domain or CDN.
type Stored struct {
	ID   string
	Name string
	MIME string
	Size int64
	Path string
}

// Save stores one uploaded file. declaredMIME comes from the client and only
// helps the editor choose between an inline image and a download link.
func Save(app core.App, uploadedBy string, file *filesystem.File, declaredMIME string) (Stored, error) {
	if file.Size > MaxBytes {
		return Stored{}, ErrTooLarge
	}
	collection, err := app.FindCollectionByNameOrId(Collection)
	if err != nil {
		return Stored{}, err
	}
	name := displayName(file)
	contentType := normalizeMIME(declaredMIME)
	record := core.NewRecord(collection)
	record.Set("file", file)
	record.Set("name", name)
	record.Set("mime", contentType)
	record.Set("size", file.Size)
	record.Set("uploaded_by", uploadedBy)
	if err := app.Save(record); err != nil {
		return Stored{}, err
	}
	return Stored{
		ID:   record.Id,
		Name: name,
		MIME: contentType,
		Size: file.Size,
		Path: Path(record.Id, record.GetString("file")),
	}, nil
}

// Path is the relative download path of a stored file.
func Path(recordID, storedName string) string {
	return PathPrefix + recordID + "/" + url.PathEscape(storedName)
}

// displayName keeps the uploader's file name for link text, capped to the
// field limit. A blank name falls back to the stored one.
func displayName(file *filesystem.File) string {
	name := strings.TrimSpace(file.OriginalName)
	if name == "" {
		return file.Name
	}
	if utf8.RuneCountInString(name) > maxNameRunes {
		name = string([]rune(name)[:maxNameRunes])
	}
	return name
}

func normalizeMIME(declared string) string {
	mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(declared))
	if err != nil || len(mediaType) > maxMIMEBytes {
		return fallbackMIME
	}
	return mediaType
}
