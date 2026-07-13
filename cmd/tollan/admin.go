package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/t0mer/tollan/internal/auth"
	"github.com/t0mer/tollan/internal/meta"
)

func adminCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "admin",
		Short: "Administrative commands (user bootstrap, etc.)",
	}
	cmd.AddCommand(adminCreateCmd())
	return cmd
}

func adminCreateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "create",
		Short: "Create an admin user",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadConfig(cmd)
			if err != nil {
				return err
			}
			m, err := meta.Open(filepath.Join(cfg.DataDir, "tollan.db"))
			if err != nil {
				return err
			}
			defer m.Close()

			reader := bufio.NewReader(os.Stdin)
			fmt.Print("Username: ")
			username, _ := reader.ReadString('\n')
			username = strings.TrimSpace(username)
			if username == "" {
				return fmt.Errorf("username is required")
			}

			fmt.Print("Password: ")
			pw1, err := readPassword()
			if err != nil {
				return err
			}
			fmt.Print("Confirm password: ")
			pw2, err := readPassword()
			if err != nil {
				return err
			}
			if pw1 != pw2 {
				return fmt.Errorf("passwords do not match")
			}
			if len(pw1) < 8 {
				return fmt.Errorf("password must be at least 8 characters")
			}

			hash, err := auth.HashPassword(pw1)
			if err != nil {
				return err
			}
			u, err := m.CreateUser(context.Background(), username, meta.RoleAdmin, hash)
			if err != nil {
				return fmt.Errorf("creating user (already exists?): %w", err)
			}
			fmt.Printf("Created admin user %q (%s)\n", u.Username, u.ID)
			return nil
		},
	}
}

func readPassword() (string, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		// Non-interactive: read a line.
		line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		return strings.TrimSpace(line), nil
	}
	b, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	return strings.TrimSpace(string(b)), err
}
