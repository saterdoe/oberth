package main

import (
	"fmt"
	"os"

	"github.com/saterdoe/oberth/internal/config"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage oberth configuration",
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		return showConfig(".oberth.yaml")
	},
}

var configValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate configuration file",
	RunE: func(cmd *cobra.Command, args []string) error {
		return validateConfig(".oberth.yaml")
	},
}

func showConfig(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}
	fmt.Print(string(data))
	return nil
}

func validateConfig(path string) error {
	cfg, err := config.Load(path)
	if err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}
	out, _ := yaml.Marshal(cfg)
	fmt.Printf("Config is valid:\n%s\n", string(out))
	return nil
}

func init() {
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configValidateCmd)
}
