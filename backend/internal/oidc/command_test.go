package oidc

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

func TestRotateSigningKeyCommandPrintsNewKey(t *testing.T) {
	app := newTestApp(t)
	box := newTestBox(t, "master-a")
	cmd := NewCommand(app, box)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"rotate-signing-key"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	jwks, _ := PublicJWKS(app, time.Now())
	if len(jwks.Keys) != 1 || !strings.Contains(out.String(), jwks.Keys[0].KeyID) {
		t.Fatalf("output %q does not name the active key %+v", out.String(), jwks.Keys)
	}
}

func TestRotateSigningKeyCommandExplainsMissingMigrations(t *testing.T) {
	app := core.NewBaseApp(core.BaseAppConfig{DataDir: t.TempDir()})
	t.Cleanup(func() { _ = app.ResetBootstrapState() })
	if err := app.Bootstrap(); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	cmd := NewCommand(app, newTestBox(t, "master-a"))
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"rotate-signing-key"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "migrate up") {
		t.Fatalf("error = %v, want a hint to apply migrations", err)
	}
}
