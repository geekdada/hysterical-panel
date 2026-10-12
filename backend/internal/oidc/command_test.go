package oidc

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"hysterical-panel/internal/cryptobox"
)

type commandRun struct {
	out, errOut string
	err         error
	exitCode    int
}

// runCommand records the exit code instead of exiting, because PocketBase
// ignores errors returned by its commands.
func runCommand(t *testing.T, app core.App, box *cryptobox.Box, args ...string) commandRun {
	t.Helper()
	run := commandRun{exitCode: -1}
	cmd := newCommand(app, box, func(code int) { run.exitCode = code })
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(args)
	run.err = cmd.Execute()
	run.out, run.errOut = out.String(), errOut.String()
	return run
}

func TestRotateSigningKeyCommandPrintsNewKey(t *testing.T) {
	app := newTestApp(t)
	run := runCommand(t, app, newTestBox(t, "master-a"), "rotate-signing-key")
	if run.err != nil || run.exitCode != -1 {
		t.Fatalf("err = %v exit = %d", run.err, run.exitCode)
	}
	jwks, _ := PublicJWKS(app, time.Now())
	if len(jwks.Keys) != 1 || !strings.Contains(run.out, jwks.Keys[0].KeyID) {
		t.Fatalf("output %q does not name the active key %+v", run.out, jwks.Keys)
	}
}

func TestRotateSigningKeyCommandExplainsMissingMigrations(t *testing.T) {
	app := core.NewBaseApp(core.BaseAppConfig{DataDir: t.TempDir()})
	t.Cleanup(func() { _ = app.ResetBootstrapState() })
	if err := app.Bootstrap(); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	run := runCommand(t, app, newTestBox(t, "master-a"), "rotate-signing-key")
	if run.exitCode != 1 || !strings.Contains(run.errOut, "migrate up") {
		t.Fatalf("exit = %d stderr = %q, want exit 1 and a hint to apply migrations", run.exitCode, run.errOut)
	}
}

// Rotating with the wrong PANEL_MASTER_KEY would create a key the server
// cannot decrypt and silently replaces on its next request.
func TestRotateSigningKeyCommandRefusesKeyItCannotDecrypt(t *testing.T) {
	app := newTestApp(t)
	original, _ := ActiveSigningKey(app, newTestBox(t, "master-a"), time.Now())
	run := runCommand(t, app, newTestBox(t, "master-b"), "rotate-signing-key")
	if run.exitCode != 1 || !strings.Contains(run.errOut, "PANEL_MASTER_KEY") {
		t.Fatalf("exit = %d stderr = %q, want exit 1 naming PANEL_MASTER_KEY", run.exitCode, run.errOut)
	}
	if active, _ := ActiveSigningKey(app, newTestBox(t, "master-a"), time.Now()); active.ID != original.ID {
		t.Fatalf("active key changed to %s despite the refusal", active.ID)
	}
	forced := runCommand(t, app, newTestBox(t, "master-b"), "rotate-signing-key", "--force")
	if forced.err != nil || forced.exitCode != -1 {
		t.Fatalf("forced rotation: err = %v exit = %d", forced.err, forced.exitCode)
	}
}
