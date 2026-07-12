package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/t0mer/tollan/internal/app"
	"github.com/t0mer/tollan/internal/config"
	"github.com/t0mer/tollan/internal/svc"
)

func serviceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "service",
		Short: "Manage the OS service registration (systemd / Windows SCM)",
	}
	for _, action := range []string{"install", "uninstall", "start", "stop", "restart"} {
		cmd.AddCommand(serviceActionCmd(action))
	}
	cmd.AddCommand(serviceStatusCmd())
	return cmd
}

func serviceActionCmd(action string) *cobra.Command {
	return &cobra.Command{
		Use:   action,
		Short: fmt.Sprintf("%s the tollan service", action),
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadConfig(cmd)
			if err != nil {
				return err
			}
			log := config.NewLogger(os.Stdout, cfg.Log)
			a, err := app.New(cfg, log)
			if err != nil {
				return err
			}
			s, err := svc.New(a, log, svc.Options{
				Arguments: serviceArguments(cmd.Flags(), cfg),
			})
			if err != nil {
				return err
			}
			if err := svc.Control(s, action); err != nil {
				return fmt.Errorf("%s: %w", action, err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "service %sed\n", action)
			return nil
		},
	}
}

func serviceStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the tollan service status",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadConfig(cmd)
			if err != nil {
				return err
			}
			log := config.NewLogger(os.Stdout, cfg.Log)
			a, err := app.New(cfg, log)
			if err != nil {
				return err
			}
			s, err := svc.New(a, log, svc.Options{})
			if err != nil {
				return err
			}
			status, err := svc.Status(s)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), status)
			return nil
		},
	}
}
