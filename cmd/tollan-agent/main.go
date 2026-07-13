// Command tollan-agent is the companion log collector for Tollan: it tails
// files and journald, ships events to a Tollan server over GELF, and applies
// server-pushed configuration. It can self-register as an OS service.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/t0mer/tollan/internal/agentfleet"
	"github.com/t0mer/tollan/internal/config"
	"github.com/t0mer/tollan/internal/svc"
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
		Use:           "tollan-agent",
		Short:         "Tollan fleet collector",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       version.Version,
	}
	registerFlags(root.PersistentFlags())
	root.AddCommand(runCmd(), serviceCmd(), versionCmd())
	return root
}

func registerFlags(fs interface {
	String(name, value, usage string) *string
	StringSlice(name string, value []string, usage string) *[]string
	StringP(name, shorthand, value, usage string) *string
}) {
	fs.String("server", "", "Tollan server base URL (e.g. http://tollan:8080)")
	fs.String("token", "", "enrollment token")
	fs.String("gelf-addr", "", "GELF TCP target host:port (defaults to server host:12201)")
	fs.String("tags", "", "comma-separated tags")
	fs.StringSlice("file", nil, "file glob to tail (repeatable)")
	fs.String("data-dir", "./agent-data", "agent state directory")
	fs.String("log-level", "info", "log level")
}

func agentConfig(cmd *cobra.Command) agentflags {
	f := cmd.Flags()
	server, _ := f.GetString("server")
	token, _ := f.GetString("token")
	gelf, _ := f.GetString("gelf-addr")
	tagsStr, _ := f.GetString("tags")
	files, _ := f.GetStringSlice("file")
	dataDir, _ := f.GetString("data-dir")
	level, _ := f.GetString("log-level")
	var tags []string
	for _, t := range strings.Split(tagsStr, ",") {
		if s := strings.TrimSpace(t); s != "" {
			tags = append(tags, s)
		}
	}
	return agentflags{server, token, gelf, tags, files, dataDir, level}
}

type agentflags struct {
	server, token, gelf string
	tags, files         []string
	dataDir, level      string
}

func runCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "run",
		Short: "Run the collector in the foreground",
		RunE: func(cmd *cobra.Command, _ []string) error {
			f := agentConfig(cmd)
			log := config.NewLogger(os.Stdout, config.LogConfig{Level: f.level, Format: "text"})
			ag, err := agentfleet.New(agentfleet.Config{
				ServerURL:       f.server,
				EnrollmentToken: f.token,
				GELFAddr:        f.gelf,
				Tags:            f.tags,
				Files:           f.files,
				DataDir:         f.dataDir,
				Version:         version.Version,
			}, log)
			if err != nil {
				return err
			}
			s, err := svc.New(ag, log, svc.Options{
				Name: "tollan-agent", DisplayName: "Tollan Agent",
				Description: "Tollan fleet collector.",
			})
			if err != nil {
				return err
			}
			return svc.Run(s)
		},
	}
}

func serviceCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "service", Short: "Manage the agent OS service"}
	for _, action := range []string{"install", "uninstall", "start", "stop", "restart"} {
		action := action
		cmd.AddCommand(&cobra.Command{
			Use:   action,
			Short: fmt.Sprintf("%s the tollan-agent service", action),
			RunE: func(cmd *cobra.Command, _ []string) error {
				f := agentConfig(cmd)
				log := config.NewLogger(os.Stdout, config.LogConfig{Level: f.level, Format: "text"})
				ag, err := agentfleet.New(agentfleet.Config{
					ServerURL: f.server, EnrollmentToken: f.token, GELFAddr: f.gelf,
					Tags: f.tags, Files: f.files, DataDir: f.dataDir, Version: version.Version,
				}, log)
				if err != nil {
					return err
				}
				dataDir, _ := filepath.Abs(f.dataDir)
				opts := svc.Options{
					Name: "tollan-agent", DisplayName: "Tollan Agent",
					Description:      "Tollan fleet collector.",
					Arguments:        serviceArgs(f, dataDir),
					WorkingDirectory: dataDir,
				}
				if runtime.GOOS == "linux" {
					opts.UserName = "tollan"
				}
				s, err := svc.New(ag, log, opts)
				if err != nil {
					return err
				}
				if err := svc.Control(s, action); err != nil {
					return fmt.Errorf("%s: %w", action, err)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "agent service %sed\n", action)
				return nil
			},
		})
	}
	return cmd
}

func serviceArgs(f agentflags, dataDir string) []string {
	args := []string{"run", "--server", f.server, "--data-dir", dataDir, "--log-level", f.level}
	if f.token != "" {
		args = append(args, "--token", f.token)
	}
	if f.gelf != "" {
		args = append(args, "--gelf-addr", f.gelf)
	}
	if len(f.tags) > 0 {
		args = append(args, "--tags", strings.Join(f.tags, ","))
	}
	for _, g := range f.files {
		args = append(args, "--file", g)
	}
	return args
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version and exit",
		RunE: func(cmd *cobra.Command, _ []string) error {
			fmt.Fprintln(cmd.OutOrStdout(), version.String())
			return nil
		},
	}
}
