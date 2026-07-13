package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/t0mer/tollan/internal/app"
	"github.com/t0mer/tollan/internal/config"
	"github.com/t0mer/tollan/internal/svc"
)

// serviceUserName is the dedicated account the Linux service runs as.
const serviceUserName = "tollan"

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

// serviceDataDir resolves the data directory a service should use: an explicit
// --data-dir wins; otherwise a per-OS default is chosen instead of ./data.
func serviceDataDir(cfg config.Config, fs interface{ Changed(string) bool }) string {
	if fs.Changed("data-dir") {
		if abs, err := filepath.Abs(cfg.DataDir); err == nil {
			return abs
		}
		return cfg.DataDir
	}
	switch runtime.GOOS {
	case "windows":
		return `C:\ProgramData\Tollan`
	default:
		return "/var/lib/tollan"
	}
}

// buildServiceOptions resolves config into svc.Options with the service-specific
// data dir, dedicated user (Linux) and working directory.
func buildServiceOptions(cmd *cobra.Command, cfg config.Config) svc.Options {
	dataDir := serviceDataDir(cfg, cmd.Flags())
	rcfg := cfg
	rcfg.DataDir = dataDir
	opts := svc.Options{
		Arguments:        serviceArguments(cmd.Flags(), rcfg),
		WorkingDirectory: dataDir,
	}
	if runtime.GOOS == "linux" {
		opts.UserName = serviceUserName
	}
	return opts
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
			opts := buildServiceOptions(cmd, cfg)

			if action == "install" {
				prepareServiceHost(cmd, opts.WorkingDirectory)
			}

			a, err := app.New(cfg, log)
			if err != nil {
				return err
			}
			s, err := svc.New(a, log, opts)
			if err != nil {
				return err
			}
			if err := svc.Control(s, action); err != nil {
				return fmt.Errorf("%s: %w", action, err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "service %sed\n", action)
			if action == "install" {
				fmt.Fprintf(cmd.OutOrStdout(), "data directory: %s\nstart with: tollan service start\n", opts.WorkingDirectory)
			}
			return nil
		},
	}
}

// prepareServiceHost creates the dedicated user and data directory on Linux when
// running as root; otherwise it prints guidance.
func prepareServiceHost(cmd *cobra.Command, dataDir string) {
	if runtime.GOOS != "linux" {
		_ = os.MkdirAll(dataDir, 0o750)
		return
	}
	if os.Geteuid() != 0 {
		fmt.Fprintf(cmd.ErrOrStderr(),
			"note: not running as root — create the %q user and %q directory manually,\n"+
				"      or re-run with sudo so the service is installed hardened.\n", serviceUserName, dataDir)
		return
	}
	if _, err := exec.LookPath("useradd"); err == nil {
		if _, err := exec.Command("id", serviceUserName).Output(); err != nil {
			_ = exec.Command("useradd", "--system", "--no-create-home",
				"--shell", "/usr/sbin/nologin", serviceUserName).Run()
		}
	}
	_ = os.MkdirAll(dataDir, 0o750)
	_ = exec.Command("chown", "-R", serviceUserName+":"+serviceUserName, dataDir).Run()
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
