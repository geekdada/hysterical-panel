package panelserve

import (
	"net/http"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

// panelCorsMiddlewareId must differ from apis.DefaultCorsMiddlewareId ("pbCors").
// Unbind(pbCors) marks the id excluded on all child router groups; rebinding with
// the same id only clears exclusion on the root, so /api/* routes would skip CORS.
const panelCorsMiddlewareId = "panelCors"

// activeCORS holds the origins and Max-Age for the current serve invocation
// (set in the serve command RunE before apis.Serve, read from OnServe).
var activeCORS struct {
	origins []string
	maxAge  int
}

// SetActiveCORS records CORS settings for the upcoming apis.Serve call.
func SetActiveCORS(origins []string, maxAge int) {
	activeCORS.origins = origins
	activeCORS.maxAge = maxAge
}

// ApplyCORS replaces PocketBase's default CORS middleware with panel CORS settings
// (including Access-Control-Max-Age when maxAge > 0). No-op when maxAge <= 0.
func ApplyCORS(se *core.ServeEvent) {
	origins := activeCORS.origins
	if activeCORS.maxAge <= 0 {
		return
	}

	if len(origins) == 0 {
		origins = []string{"*"}
	}

	se.Router.Unbind(apis.DefaultCorsMiddlewareId)

	cors := apis.CORS(apis.CORSConfig{
		AllowOrigins: origins,
		AllowMethods: []string{
			http.MethodGet,
			http.MethodHead,
			http.MethodPut,
			http.MethodPatch,
			http.MethodPost,
			http.MethodDelete,
		},
		MaxAge: activeCORS.maxAge,
	})
	cors.Id = panelCorsMiddlewareId
	se.Router.Bind(cors)
}
