package main

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

var (
	Version   = "0.1.0-alpha.1"
	Commit    = "unknown"
	BuildDate = "unknown"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show version information",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("oberth %s\n", Version)
		fmt.Printf("  commit:     %s\n", Commit)
		fmt.Printf("  built:      %s\n", BuildDate)
		fmt.Printf("  go:         %s\n", runtime.Version())
		fmt.Printf("  os/arch:    %s/%s\n", runtime.GOOS, runtime.GOARCH)
		return nil
	},
}
