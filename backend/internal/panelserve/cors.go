package panelserve

import (
	"net/http"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

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

// ApplyCORS replaces PocketBase's default CORS middleware with one that sets
// Access-Control-Max-Age on preflight responses. No-op when maxAge <= 0.
func ApplyCORS(se *core.ServeEvent) {
	if activeCORS.maxAge <= 0 {
		return
	}
	origins := activeCORS.origins
	if len(origins) == 0 {
		origins = []string{"*"}
	}

	se.Router.Unbind(apis.DefaultCorsMiddlewareId)
	se.Router.Bind(apis.CORS(apis.CORSConfig{
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
	}))
}
