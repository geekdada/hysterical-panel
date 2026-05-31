package main

import (
	"context"
	"log"
	"os"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/plugins/migratecmd"
	"github.com/spf13/cobra"

	_ "hysterical-panel/migrations"

	"hysterical-panel/internal/api"
	"hysterical-panel/internal/collector"
	"hysterical-panel/internal/config"
	"hysterical-panel/internal/cryptobox"
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
		// register custom panel routes
		api.Register(se, app, box)

		// start the background traffic collector
		ctx, cancel := context.WithCancel(context.Background())
		c := collector.New(app, box)
		c.Start(ctx)

		// stop the collector when the app terminates
		app.OnTerminate().BindFunc(func(te *core.TerminateEvent) error {
			cancel()
			return te.Next()
		})

		return se.Next()
	})

	if err := app.Start(); err != nil {
		log.Fatal(err)
	}
	_ = os.Args
}
