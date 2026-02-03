package main

import (
	"fmt"
	"os"

	"github.com/yourusername/lattice/internal/contracts"
)

func handleValidateAgentCommand() bool {
	if len(os.Args) < 2 || os.Args[1] != "validate-agent" {
		return false
	}
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "Usage: lattice validate-agent /path/to/agent.yaml")
		os.Exit(2)
	}
	report, err := contracts.ValidateAgentFile(os.Args[2])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Validation failed: %v\n", err)
		os.Exit(1)
	}
	if report.IsValid() {
		fmt.Printf("OK: %s (%s)\n", report.Path, report.Role)
		os.Exit(0)
	}
	fmt.Printf("Invalid: %s (%s)\n", report.Path, report.Role)
	for _, validationErr := range report.Errors {
		fmt.Printf("- %v\n", validationErr)
	}
	os.Exit(1)
	return true
}
