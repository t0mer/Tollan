package main

import (
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/t0mer/tollan/internal/app"
	"github.com/t0mer/tollan/internal/config"
	"github.com/t0mer/tollan/internal/svc"
)

// registerConfigFlags attaches the shared configuration flags as persistent
// flags on the root command.
func registerConfigFlags(cmd *cobra.Command) {
	config.RegisterFlags(cmd.PersistentFlags())
}

// loadConfig resolves configuration from the command's flags.
func loadConfig(cmd *cobra.Command) (config.Config, error) {
	return config.Load(cmd.Flags())
}

func runCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "run",
		Short: "Run the server in the foreground (also used by the service manager)",
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
			// Run through the service wrapper so behaviour is identical whether
			// launched interactively or by systemd/SCM.
			s, err := svc.New(a, log, svc.Options{})
			if err != nil {
				return err
			}
			return svc.Run(s)
		},
	}
}

// serviceArguments reconstructs the CLI arguments the service manager should use
// to launch `tollan run` with the resolved configuration baked in.
func serviceArguments(fs *pflag.FlagSet, cfg config.Config) []string {
	args := []string{
		"run",
		"--data-dir", cfg.DataDir,
		"--log-level", cfg.Log.Level,
		"--log-format", cfg.Log.Format,
		"--http-addr", cfg.HTTP.Addr,
		"--auth", cfg.Auth.Mode,
	}
	if cf, _ := fs.GetString("config"); cf != "" {
		// Bake an absolute path so the service works regardless of its cwd.
		if abs, err := filepath.Abs(cf); err == nil {
			cf = abs
		}
		args = append(args, "--config", cf)
	}
	return args
}
