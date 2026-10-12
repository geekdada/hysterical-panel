package oidc

import (
	"fmt"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/spf13/cobra"

	"hysterical-panel/internal/cryptobox"
)

// NewCommand returns the `oidc` command group. Signing keys rotate only from
// the command line (ADR-0010).
func NewCommand(app core.App, box *cryptobox.Box) *cobra.Command {
	root := &cobra.Command{Use: "oidc", Short: "Manage the OIDC provider"}
	var dropPrevious bool
	rotate := &cobra.Command{
		Use:          "rotate-signing-key",
		Short:        "Create a new ID token signing key and retire the current one",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			kid, err := RotateSigningKey(app, box, dropPrevious, time.Now())
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "signing key %s is now active\n", kid)
			return nil
		},
	}
	rotate.Flags().BoolVar(&dropPrevious, "drop-previous", false,
		"remove older keys from JWKS at once; ID tokens they signed stop verifying")
	root.AddCommand(rotate)
	return root
}
