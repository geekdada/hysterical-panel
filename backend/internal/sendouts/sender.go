package sendouts

import (
	"net/mail"
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

// Sender is the From address for panel mail. Recipients know the deployment by
// its Application Name, so that names the sender; PocketBase's own sender name
// is only the fallback.
func Sender(app core.App) mail.Address {
	meta := app.Settings().Meta
	name := strings.TrimSpace(meta.AppName)
	if name == "" {
		name = meta.SenderName
	}
	return mail.Address{Name: name, Address: meta.SenderAddress}
}
