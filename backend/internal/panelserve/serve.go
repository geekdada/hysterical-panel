// Package panelserve registers the panel's serve command with CORS from config.
package panelserve

import (
	"errors"
	"net/http"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/spf13/cobra"
)

// NewCommand returns a serve command like PocketBase's default, with AllowedOrigins
// defaulting to defaultOrigins (from PANEL_FRONTEND_URL_BASE, or "*" when unset).
// corsMaxAge is sent as Access-Control-Max-Age on preflight (0 disables).
// The --origins flag still overrides at runtime.
func NewCommand(app core.App, defaultOrigins []string, corsMaxAge int, showStartBanner bool) *cobra.Command {
	var allowedOrigins []string
	var httpAddr string
	var httpsAddr string

	command := &cobra.Command{
		Use:          "serve [domain(s)]",
		Args:         cobra.ArbitraryArgs,
		Short:        "Starts the web server (default to 127.0.0.1:8090 if no domain is specified)",
		SilenceUsage: true,
		RunE: func(command *cobra.Command, args []string) error {
			if len(args) > 0 {
				if httpAddr == "" {
					httpAddr = "0.0.0.0:80"
				}
				if httpsAddr == "" {
					httpsAddr = "0.0.0.0:443"
				}
			} else if httpAddr == "" {
				httpAddr = "127.0.0.1:8090"
			}

			SetActiveCORS(allowedOrigins, corsMaxAge)

			err := apis.Serve(app, apis.ServeConfig{
				HttpAddr:           httpAddr,
				HttpsAddr:          httpsAddr,
				ShowStartBanner:    showStartBanner,
				AllowedOrigins:     allowedOrigins,
				CertificateDomains: args,
			})
			if errors.Is(err, http.ErrServerClosed) {
				return nil
			}
			return err
		},
	}

	command.PersistentFlags().StringSliceVar(
		&allowedOrigins,
		"origins",
		defaultOrigins,
		"CORS allowed domain origins list",
	)
	command.PersistentFlags().StringVar(
		&httpAddr,
		"http",
		"",
		"TCP address to listen for the HTTP server\n(if domain args are specified - default to 0.0.0.0:80, otherwise - default to 127.0.0.1:8090)",
	)
	command.PersistentFlags().StringVar(
		&httpsAddr,
		"https",
		"",
		"TCP address to listen for the HTTPS server\n(if domain args are specified - default to 0.0.0.0:443, otherwise - default to empty string, aka. no TLS)\nThe incoming HTTP traffic also will be auto redirected to the HTTPS version",
	)

	return command
}
