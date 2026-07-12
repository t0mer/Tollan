// Command tollan is the Tollan log management server: a single binary that
// ingests, processes, stores, searches and alerts on logs, and self-registers
// as an OS service.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/t0mer/tollan/internal/version"
)

func main() {
	if err := rootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func rootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "tollan",
		Short:         "Tollan — self-hosted log management server",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       version.Version,
	}
	// Persistent config flags available to every subcommand.
	registerConfigFlags(root)

	root.AddCommand(
		runCmd(),
		serviceCmd(),
		adminCmd(),
		versionCmd(),
	)
	return root
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information and exit",
		RunE: func(cmd *cobra.Command, _ []string) error {
			fmt.Fprintln(cmd.OutOrStdout(), version.String())
			return nil
		},
	}
}
