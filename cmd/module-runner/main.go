package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kingrea/The-Lattice/internal/artifact"
	"github.com/kingrea/The-Lattice/internal/config"
	"github.com/kingrea/The-Lattice/internal/module"
	"github.com/kingrea/The-Lattice/internal/modules"
	"github.com/kingrea/The-Lattice/internal/workflow"
	"github.com/kingrea/The-Lattice/plugins"
	"gopkg.in/yaml.v3"
)

func main() {
	moduleID := flag.String("module", "", "module identifier to execute (e.g. anchor-docs)")
	projectDir := flag.String("project", "", "path to the project directory (defaults to cwd)")
	pollInterval := flag.Duration("poll", 3*time.Second, "poll interval while waiting for completion")
	configFile := flag.String("config-file", "", "path to YAML/JSON file with module config overrides")
	sets := keyValueFlag{}
	flag.Var(&sets, "set", "module config override (key=value, repeatable)")
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
	if err := plugins.RegisterSkillPlugins(reg, cfg); err != nil {
		die("load plugins: %v", err)
	}
	cfgOverrides, err := buildModuleConfig(*configFile, sets)
	if err != nil {
		die("load config overrides: %v", err)
	}
	mod, err := reg.Resolve(*moduleID, cfgOverrides)
	if err != nil {
		die("resolve module: %v", err)
	}
	info := mod.Info()
	label := moduleLabel(info, *moduleID)
	result, err := mod.Run(ctx)
	if err != nil {
		die("run module: %v", err)
	}
	fmt.Printf("Run status: %s\n", result.Status)
	if result.Message != "" {
		fmt.Println(result.Message)
	}
	if result.Status == module.StatusCompleted || result.Status == module.StatusNoOp {
		fmt.Printf("%s completed without polling.\n", label)
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
			fmt.Printf("%s completed successfully.\n", label)
			return
		}
		fmt.Printf("Waiting for %s outputs...\n", label)
		<-ticker.C
	}
}

func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

type keyValueFlag map[string]string

func (kv *keyValueFlag) String() string {
	if kv == nil || len(*kv) == 0 {
		return ""
	}
	var pairs []string
	for key, value := range *kv {
		pairs = append(pairs, fmt.Sprintf("%s=%s", key, value))
	}
	return strings.Join(pairs, ", ")
}

func (kv *keyValueFlag) Set(value string) error {
	parts := strings.SplitN(value, "=", 2)
	if len(parts) != 2 {
		return fmt.Errorf("expected key=value, got %q", value)
	}
	key := strings.TrimSpace(parts[0])
	if key == "" {
		return fmt.Errorf("override key is empty in %q", value)
	}
	if *kv == nil {
		*kv = keyValueFlag{}
	}
	(*kv)[key] = parts[1]
	return nil
}

func buildModuleConfig(configFile string, overrides keyValueFlag) (module.Config, error) {
	var cfg module.Config
	if path := strings.TrimSpace(configFile); path != "" {
		fileCfg, err := readModuleConfigFile(path)
		if err != nil {
			return nil, err
		}
		cfg = fileCfg
	}
	if len(overrides) > 0 {
		if cfg == nil {
			cfg = module.Config{}
		}
		for key, value := range overrides {
			cfg[key] = value
		}
	}
	if len(cfg) == 0 {
		return nil, nil
	}
	return cfg, nil
}

func moduleLabel(info module.Info, fallback string) string {
	if name := strings.TrimSpace(info.Name); name != "" {
		return name
	}
	if id := strings.TrimSpace(info.ID); id != "" {
		return id
	}
	return strings.TrimSpace(fallback)
}

func readModuleConfigFile(path string) (module.Config, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("open config file %s: %w", path, err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("%s is a directory, expected a file", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file %s: %w", path, err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil, fmt.Errorf("config file %s is empty", path)
	}
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse config file %s: %w", path, err)
	}
	if len(raw) == 0 {
		return nil, nil
	}
	cfg := make(module.Config, len(raw))
	for key, value := range raw {
		cfg[key] = value
	}
	return cfg, nil
}
