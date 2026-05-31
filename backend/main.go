package main

import (
	"context"
	"log"
	"os"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/plugins/migratecmd"

	_ "hysterical-panel/migrations"

	"hysterical-panel/internal/api"
	"hysterical-panel/internal/collector"
	"hysterical-panel/internal/config"
	"hysterical-panel/internal/cryptobox"
)

func main() {
	app := pocketbase.New()

	// auto-apply migrations on boot
	migratecmd.MustRegister(app, app.RootCmd, migratecmd.Config{
		Automigrate: false, // migrations are committed by hand in ./migrations
	})

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("startup: %v", err)
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
