package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

type capability struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Status      string `json:"status"`
	Category    string `json:"category"`
}

type capabilities struct {
	SchemaVersion string       `json:"schema_version"`
	Providers     []capability `json:"providers"`
	MCPTools      []capability `json:"mcp_tools"`
	Memory        []capability `json:"memory"`
	Prompts       []capability `json:"prompts"`
	Skills        []capability `json:"skills"`
}

var capabilitiesCmd = &cobra.Command{
	Use:   "capabilities",
	Short: "Show capabilities actually available in the running daemon",
	RunE: func(_ *cobra.Command, _ []string) error {
		var value capabilities
		if err := apiUnwrapGET("/capabilities", &value); err != nil {
			return err
		}
		fmt.Print(formatCapabilities(value))
		return nil
	},
}

func formatCapabilities(value capabilities) string {
	var output strings.Builder
	fmt.Fprintf(&output, "Capabilities schema %s\n", value.SchemaVersion)
	groups := []struct {
		name  string
		items []capability
	}{
		{"Providers", value.Providers},
		{"Skills", value.Skills},
		{"Tools", value.MCPTools},
		{"Memory", value.Memory},
		{"Prompts", value.Prompts},
	}
	for _, group := range groups {
		fmt.Fprintf(&output, "\n%s\n", group.name)
		if len(group.items) == 0 {
			output.WriteString("  (none)\n")
			continue
		}
		for _, item := range group.items {
			fmt.Fprintf(&output, "  [%s] %s — %s\n", item.Status, item.Name, item.Description)
		}
	}
	return output.String()
}

func init() {
	rootCmd.AddCommand(capabilitiesCmd)
}
