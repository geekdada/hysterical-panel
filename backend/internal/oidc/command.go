package oidc

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/spf13/cobra"

	"hysterical-panel/internal/cryptobox"
)

var errWrongMasterKey = errors.New("the active signing key does not decrypt with this PANEL_MASTER_KEY; " +
	"the server would replace a key created now, so check the variable or pass --force")

// NewCommand returns the `oidc` command group. Signing keys rotate only from
// the command line (ADR-0010).
func NewCommand(app core.App, box *cryptobox.Box) *cobra.Command {
	return newCommand(app, box, os.Exit)
}

// PocketBase ignores the error its root command returns, so a failure exits
// non-zero here to stay visible to scripts.
func newCommand(app core.App, box *cryptobox.Box, exit func(int)) *cobra.Command {
	root := &cobra.Command{Use: "oidc", Short: "Manage the OIDC provider"}
	var dropPrevious, force bool
	rotate := &cobra.Command{
		Use:           "rotate-signing-key",
		Short:         "Create a new ID token signing key and retire the current one",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			kid, err := rotateFromCLI(app, box, dropPrevious, force)
			if err != nil {
				fmt.Fprintln(cmd.ErrOrStderr(), "Error:", err)
				exit(1)
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "signing key %s is now active\n", kid)
			return nil
		},
	}
	rotate.Flags().BoolVar(&dropPrevious, "drop-previous", false,
		"remove older keys from JWKS at once; ID tokens they signed stop verifying")
	rotate.Flags().BoolVar(&force, "force", false,
		"rotate even when the active key does not decrypt with this PANEL_MASTER_KEY")
	root.AddCommand(rotate)
	return root
}

func rotateFromCLI(app core.App, box *cryptobox.Box, dropPrevious, force bool) (string, error) {
	if !force {
		decrypts, err := ActiveKeyDecrypts(app, box)
		if err != nil {
			return "", err
		}
		if !decrypts {
			return "", errWrongMasterKey
		}
	}
	return RotateSigningKey(app, box, dropPrevious, time.Now())
}
