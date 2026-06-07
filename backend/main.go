package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/cmd"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/plugins/migratecmd"
	"github.com/spf13/cobra"

	_ "hysterical-panel/migrations"

	"hysterical-panel/internal/api"
	"hysterical-panel/internal/collector"
	"hysterical-panel/internal/config"
	"hysterical-panel/internal/cryptobox"
	"hysterical-panel/internal/ipmeta"
	"hysterical-panel/internal/panelserve"
	"hysterical-panel/internal/version"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("startup: %v", err)
	}

	// PB_DATA_DIR / PB_ENCRYPTION_KEY are wired here so they actually take
	// effect (an empty DataDir falls back to ./pb_data; an unset encryption
	// key env leaves settings unencrypted). A CLI --dir still overrides.
	app := pocketbase.NewWithConfig(pocketbase.Config{
		DefaultDataDir:       cfg.DataDir,
		DefaultEncryptionEnv: "PB_ENCRYPTION_KEY",
	})

	// auto-apply migrations on boot
	migratecmd.MustRegister(app, app.RootCmd, migratecmd.Config{
		Automigrate: false, // migrations are committed by hand in ./migrations
	})

	// openapi-schema: generate openapi.json without starting the server
	app.RootCmd.AddCommand(&cobra.Command{
		Use:   "openapi-schema",
		Short: "Generate openapi.json to stdout (or -o file)",
		Run: func(cmd *cobra.Command, args []string) {
			outPath, _ := cmd.Flags().GetString("output")
			b, err := api.MarshalOpenAPISpec()
			if err != nil {
				log.Fatal(err)
			}
			if outPath != "" {
				if err := os.WriteFile(outPath, b, 0o644); err != nil {
					log.Fatal(err)
				}
				log.Printf("wrote %s", outPath)
			} else {
				cmd.OutOrStdout().Write(b)
			}
		},
	})
	_ = app.RootCmd // ensure cobra is initialized before flag parsing
	// Register the --output flag for the openapi-schema command
	for _, sub := range app.RootCmd.Commands() {
		if sub.Use == "openapi-schema" {
			sub.Flags().StringP("output", "o", "", "Write to file instead of stdout")
			break
		}
	}

	box, err := cryptobox.New(cfg.PanelMasterKey)
	if err != nil {
		log.Fatalf("startup: %v", err)
	}

	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		panelserve.ApplyCORS(se)

		ipLookup, err := ipmeta.New(cfg.MMDBDir)
		if err != nil {
			return fmt.Errorf("ip metadata: %w", err)
		}

		backendURL, err := cfg.PanelBackendURL()
		if err != nil {
			return err
		}
		frontendURL, err := cfg.PanelFrontendURL()
		if err != nil {
			return err
		}

		passkeyConfig, err := cfg.WebAuthn()
		if err != nil {
			return err
		}
		var passkeys *webauthn.WebAuthn
		if passkeyConfig.Enabled {
			passkeys, err = api.NewWebAuthn(passkeyConfig.RPID, passkeyConfig.Origins)
			if err != nil {
				return fmt.Errorf("passkeys: %w", err)
			}
		}

		// register custom panel routes
		api.Register(se, app, box, ipLookup, passkeys, api.PanelConfigResponse{
			APIURL:          backendURL,
			FrontendURL:     frontendURL,
			PasskeysEnabled: passkeyConfig.Enabled,
			Version:         version.Version,
		})

		// start the background traffic collector
		ctx, cancel := context.WithCancel(context.Background())
		c := collector.New(app, box)
		c.Start(ctx)

		// stop the collector when the app terminates
		app.OnTerminate().BindFunc(func(te *core.TerminateEvent) error {
			cancel()
			if err := ipLookup.Close(); err != nil {
				return err
			}
			return te.Next()
		})

		return se.Next()
	})

	corsOrigins, err := cfg.ServeAllowedOrigins()
	if err != nil {
		log.Fatalf("startup: %v", err)
	}

	// Register serve ourselves so AllowedOrigins comes from PANEL_FRONTEND_URL_BASE.
	app.RootCmd.AddCommand(cmd.NewSuperuserCommand(app))
	app.RootCmd.AddCommand(panelserve.NewCommand(app, corsOrigins, cfg.PanelCorsMaxAge, true))

	if err := app.Execute(); err != nil {
		log.Fatal(err)
	}
	_ = os.Args
}
