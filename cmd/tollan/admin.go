package main

import (
	"errors"

	"github.com/spf13/cobra"
)

// errNotImplemented marks commands whose subsystem lands in a later phase.
var errNotImplemented = errors.New("not implemented yet (auth/RBAC lands in a later phase)")

func adminCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "admin",
		Short: "Administrative commands (user bootstrap, etc.)",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "create",
		Short: "Create the initial admin user",
		RunE: func(*cobra.Command, []string) error {
			return errNotImplemented
		},
	})
	return cmd
}
