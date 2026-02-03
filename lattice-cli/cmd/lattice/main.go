// cmd/lattice/main.go
//
// This is the entry point for the Lattice CLI.
// When you run `lattice` from any directory, this is what executes.
//
// Flow:
// 1. Check if we're already inside a tmux session
// 2. If not, start one and re-run ourselves inside it
// 3. If yes, launch the TUI

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/yourusername/lattice/internal/config"
	"github.com/yourusername/lattice/internal/tui"
)

func main() {
	// Get the current working directory - this is the "project" we're working in
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting working directory: %v\n", err)
		os.Exit(1)
	}

	// Check if we're inside tmux already
	// tmux sets TMUX env var when you're inside a session
	if os.Getenv("TMUX") == "" {
		// Not in tmux - we need to start a session and run ourselves inside it
		startTmuxSession(cwd)
		return
	}

	// We're inside tmux! Initialize the .lattice folder and start the TUI
	if err := config.InitLatticeDir(cwd); err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing .lattice directory: %v\n", err)
		os.Exit(1)
	}

	// Create and run the TUI
	// tea.NewProgram creates a new bubbletea application
	// tui.NewApp returns our main application model
	p := tea.NewProgram(
		tui.NewApp(cwd),
		tea.WithAltScreen(), // Use alternate screen buffer (like vim does)
	)

	// Run blocks until the user quits
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running TUI: %v\n", err)
		os.Exit(1)
	}
}

// startTmuxSession creates a new tmux session called "lattice" and runs
// this same binary inside it. If the session already exists, it attaches to it.
func startTmuxSession(workingDir string) {
	sessionName := "lattice"

	// Get the path to our own executable so we can run it inside tmux
	executable, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error finding executable: %v\n", err)
		os.Exit(1)
	}

	// Make sure it's an absolute path
	executable, err = filepath.Abs(executable)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error resolving executable path: %v\n", err)
		os.Exit(1)
	}

	// Check if session already exists
	checkCmd := exec.Command("tmux", "has-session", "-t", sessionName)
	sessionExists := checkCmd.Run() == nil

	if sessionExists {
		// Session exists - attach to it
		fmt.Printf("Attaching to existing lattice session...\n")
		cmd := exec.Command("tmux", "attach-session", "-t", sessionName)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to attach to tmux session %q: %v\n", sessionName, err)
			os.Exit(1)
		}
	} else {
		// Create new session
		// -s: session name
		// -c: starting directory
		// The command at the end is what runs in the new session
		fmt.Printf("Starting new lattice session...\n")
		cmd := exec.Command("tmux", "new-session", "-s", sessionName, "-c", workingDir, executable)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to start tmux session %q: %v\n", sessionName, err)
			os.Exit(1)
		}
	}
}
