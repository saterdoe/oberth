package main

import (
	"fmt"
	"runtime"

	"github.com/saterdoe/oberth/internal/doctor"
	"github.com/spf13/cobra"
)

var doctorBundlePath string

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Diagnose oberth installation",
	RunE: func(cmd *cobra.Command, args []string) error {
		checks := doctor.ComprehensiveChecks(doctor.DefaultProbes(".oberth.yaml", ".agent-vault"))

		hasFailure := false
		hasWarning := false
		for _, c := range checks {
			status := colorStatus(c.Status)
			fmt.Printf("  %s  %s\n", status, c.Name)
			if c.Status != doctor.StatusPass {
				fmt.Printf("       %s\n", c.Message)
			}
			hasFailure = hasFailure || c.Status == doctor.StatusFail
			hasWarning = hasWarning || c.Status == doctor.StatusWarn
		}
		if doctorBundlePath != "" {
			diagnostics, diagnosticErr := doctor.FetchRuntimeDiagnostics()
			var diagnosticErrors []string
			if diagnosticErr != nil {
				diagnosticErrors = append(diagnosticErrors, diagnosticErr.Error())
			}
			if err := doctor.CreateBundle(doctorBundlePath, doctor.BundleInput{
				Versions: map[string]string{"go": runtime.Version(), "oberth": Version}, Health: checks, Runtime: diagnostics, Errors: diagnosticErrors,
			}); err != nil {
				return fmt.Errorf("creating diagnostic bundle: %w", err)
			}
			fmt.Printf("Diagnostic bundle written to %s\n", doctorBundlePath)
		}

		if !hasFailure && !hasWarning {
			fmt.Println()
			fmt.Println(colorGreen("All checks passed."))
		} else if !hasFailure {
			fmt.Println()
			fmt.Println(colorYellow("Core checks passed with warnings."))
		} else {
			fmt.Println()
			fmt.Println(colorRed("Required checks failed. Follow the messages above."))
			return fmt.Errorf("doctor found required failures")
		}

		return nil
	},
}

func init() {
	doctorCmd.Flags().StringVar(&doctorBundlePath, "bundle", "", "write a redacted diagnostic ZIP bundle")
}

func colorStatus(s string) string {
	switch s {
	case doctor.StatusPass:
		return colorGreen(s)
	case doctor.StatusFail:
		return colorRed(s)
	case doctor.StatusWarn:
		return colorYellow(s)
	default:
		return s
	}
}

func colorGreen(s string) string  { return "\033[32m" + s + "\033[0m" }
func colorRed(s string) string    { return "\033[31m" + s + "\033[0m" }
func colorYellow(s string) string { return "\033[33m" + s + "\033[0m" }
