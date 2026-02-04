package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yourusername/lattice/internal/artifact"
	"github.com/yourusername/lattice/internal/config"
	"github.com/yourusername/lattice/internal/module"
	"github.com/yourusername/lattice/internal/modules"
	"github.com/yourusername/lattice/internal/workflow"
)

func main() {
	moduleID := flag.String("module", "", "module identifier to execute (e.g. anchor-docs)")
	projectDir := flag.String("project", "", "path to the project directory (defaults to cwd)")
	pollInterval := flag.Duration("poll", 3*time.Second, "poll interval while waiting for completion")
	flag.Parse()

	if strings.TrimSpace(*moduleID) == "" {
		die("--module is required")
	}

	project := *projectDir
	if project == "" {
		var err error
		project, err = os.Getwd()
		if err != nil {
			die("determine working directory: %v", err)
		}
	}
	absoluteProject, err := filepath.Abs(project)
	if err != nil {
		die("resolve project dir: %v", err)
	}
	if err := config.InitLatticeDir(absoluteProject); err != nil {
		die("init .lattice: %v", err)
	}
	cfg, err := config.NewConfig(absoluteProject)
	if err != nil {
		die("load config: %v", err)
	}
	wf := workflow.New(cfg.LatticeProjectDir)
	ctx := &module.ModuleContext{
		Config:     cfg,
		Workflow:   wf,
		Artifacts:  artifact.NewStore(wf),
		OriginMode: "module-runner",
	}
	reg := module.NewRegistry()
	modules.RegisterBuiltins(reg)
	mod, err := reg.Resolve(*moduleID, nil)
	if err != nil {
		die("resolve module: %v", err)
	}
	result, err := mod.Run(ctx)
	if err != nil {
		die("run module: %v", err)
	}
	fmt.Printf("Run status: %s\n", result.Status)
	if result.Message != "" {
		fmt.Println(result.Message)
	}
	if result.Status == module.StatusCompleted || result.Status == module.StatusNoOp {
		fmt.Println("Module completed without polling.")
		return
	}
	ticker := time.NewTicker(*pollInterval)
	defer ticker.Stop()
	for {
		complete, err := mod.IsComplete(ctx)
		if err != nil {
			die("check completion: %v", err)
		}
		if complete {
			fmt.Println("Module completed successfully.")
			return
		}
		fmt.Println("Waiting for anchor docs outputs...")
		<-ticker.C
	}
}

func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
