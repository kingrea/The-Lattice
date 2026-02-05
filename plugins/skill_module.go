package plugins

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"text/template"
	"time"

	"github.com/kingrea/The-Lattice/internal/artifact"
	"github.com/kingrea/The-Lattice/internal/module"
	"github.com/kingrea/The-Lattice/internal/skills"
)

type skillModule struct {
	*module.Base
	definition ModuleDefinition
	inputs     []artifact.ArtifactRef
	outputs    []artifact.ArtifactRef
	inputIDs   []string
	config     module.Config
	terminal   skillTerminal
	windowName string
}

func newSkillModule(def ModuleDefinition, overrides module.Config) (*skillModule, error) {
	if err := def.Validate(); err != nil {
		return nil, err
	}
	normalized := def.Normalized()
	inputs, inputIDs, err := resolveBindings(normalized.Inputs)
	if err != nil {
		return nil, err
	}
	outputs, _, err := resolveBindings(normalized.Outputs)
	if err != nil {
		return nil, err
	}
	info := module.Info{
		ID:          normalized.ID,
		Name:        defaultModuleName(normalized),
		Description: normalized.Description,
		Version:     normalized.Version,
		Concurrency: normalized.Concurrency,
	}
	if err := info.Validate(); err != nil {
		return nil, err
	}
	base := module.NewBase(info)
	base.SetInputs(inputs...)
	base.SetOutputs(outputs...)
	merged := mergeConfigs(normalized.Config, overrides)
	return &skillModule{
		Base:       &base,
		definition: normalized,
		inputs:     inputs,
		outputs:    outputs,
		inputIDs:   inputIDs,
		config:     merged,
		terminal:   tmuxTerminal{},
	}, nil
}

func (m *skillModule) Run(ctx *module.ModuleContext) (module.Result, error) {
	if err := validateSkillContext(ctx); err != nil {
		return module.Result{Status: module.StatusFailed}, err
	}
	complete, err := m.IsComplete(ctx)
	if err != nil {
		return module.Result{Status: module.StatusFailed}, err
	}
	if complete {
		return module.Result{Status: module.StatusNoOp, Message: fmt.Sprintf("%s already complete", m.definition.ID)}, nil
	}
	if m.windowName != "" {
		return module.Result{Status: module.StatusNeedsInput, Message: fmt.Sprintf("%s running in %s", m.definition.ID, m.windowName)}, nil
	}
	skillPath, err := m.resolveSkillPath(ctx)
	if err != nil {
		return module.Result{Status: module.StatusFailed}, err
	}
	prompt, err := m.renderPrompt(skillPath, ctx)
	if err != nil {
		return module.Result{Status: module.StatusFailed}, err
	}
	window := m.desiredWindowName()
	if err := m.terminal.CreateWindow(window, ctx.Config.ProjectDir); err != nil {
		return module.Result{Status: module.StatusFailed}, fmt.Errorf("skill-module: create tmux window: %w", err)
	}
	if err := m.terminal.SendOpenCode(window, prompt, m.definition.Skill.Env); err != nil {
		m.terminal.KillWindow(window)
		return module.Result{Status: module.StatusFailed}, fmt.Errorf("skill-module: launch opencode: %w", err)
	}
	m.windowName = window
	return module.Result{Status: module.StatusNeedsInput, Message: fmt.Sprintf("%s running in %s", m.definition.ID, window)}, nil
}

func (m *skillModule) IsComplete(ctx *module.ModuleContext) (bool, error) {
	if err := validateSkillContext(ctx); err != nil {
		return false, err
	}
	for _, ref := range m.outputs {
		ready, err := m.ensureArtifact(ctx, ref)
		if err != nil {
			return false, err
		}
		if !ready {
			return false, nil
		}
	}
	m.stopSession()
	return true, nil
}

func (m *skillModule) ensureArtifact(ctx *module.ModuleContext, ref artifact.ArtifactRef) (bool, error) {
	if ctx.Artifacts == nil {
		return false, fmt.Errorf("skill-module: artifact store unavailable")
	}
	result, err := ctx.Artifacts.Check(ref)
	if err != nil {
		return false, err
	}
	switch result.State {
	case artifact.StateMissing:
		return false, nil
	case artifact.StateInvalid:
		if err := m.writeMetadata(ctx, ref); err != nil {
			return false, err
		}
		return false, nil
	case artifact.StateError:
		if result.Err != nil {
			return false, result.Err
		}
		return false, fmt.Errorf("skill-module: %s unknown error", ref.ID)
	case artifact.StateReady:
		if ref.Kind == artifact.KindMarker || ref.Kind == artifact.KindDirectory {
			return true, nil
		}
		if result.Metadata == nil || result.Metadata.ModuleID != m.definition.ID || result.Metadata.Version != m.definition.Version {
			if err := m.writeMetadata(ctx, ref); err != nil {
				return false, err
			}
			return false, nil
		}
		return true, nil
	default:
		return false, nil
	}
}

func (m *skillModule) writeMetadata(ctx *module.ModuleContext, ref artifact.ArtifactRef) error {
	path := ref.Path(ctx.Workflow)
	if path == "" {
		return fmt.Errorf("skill-module: %s path unresolved", ref.ID)
	}
	body, err := readArtifactBody(ref, path)
	if err != nil {
		return fmt.Errorf("skill-module: read %s: %w", ref.ID, err)
	}
	meta := artifact.Metadata{
		ArtifactID: ref.ID,
		ModuleID:   m.definition.ID,
		Version:    m.definition.Version,
		Workflow:   ctx.Workflow.Dir(),
		Inputs:     append([]string{}, m.inputIDs...),
	}
	if err := ctx.Artifacts.Write(ref, body, meta); err != nil {
		return fmt.Errorf("skill-module: write metadata for %s: %w", ref.ID, err)
	}
	return nil
}

func (m *skillModule) stopSession() {
	if m.windowName == "" {
		return
	}
	m.terminal.KillWindow(m.windowName)
	m.windowName = ""
}

func (m *skillModule) resolveSkillPath(ctx *module.ModuleContext) (string, error) {
	skill := m.definition.Skill
	if path := strings.TrimSpace(skill.Path); path != "" {
		resolved := path
		if !filepath.IsAbs(resolved) {
			resolved = filepath.Join(ctx.Config.ProjectDir, resolved)
		}
		info, err := os.Stat(resolved)
		if err != nil {
			return "", fmt.Errorf("skill-module: resolve skill path %s: %w", resolved, err)
		}
		if info.IsDir() {
			return "", fmt.Errorf("skill-module: %s is a directory", resolved)
		}
		return resolved, nil
	}
	if slug := strings.TrimSpace(skill.Slug); slug != "" {
		path, err := skills.Ensure(ctx.Config.SkillsDir(), skills.Slug(slug))
		if err != nil {
			return "", fmt.Errorf("skill-module: ensure skill %s: %w", slug, err)
		}
		return path, nil
	}
	return "", fmt.Errorf("skill-module: skill slug or path is required for %s", m.definition.ID)
}

func (m *skillModule) renderPrompt(skillPath string, ctx *module.ModuleContext) (string, error) {
	tmpl, err := template.New("skill_prompt").Funcs(template.FuncMap{
		"join": strings.Join,
	}).Parse(m.definition.Skill.Prompt)
	if err != nil {
		return "", fmt.Errorf("skill-module: parse prompt: %w", err)
	}
	data := map[string]any{
		"SkillPath":   skillPath,
		"Definition":  m.definition,
		"Inputs":      m.inputs,
		"Outputs":     m.outputs,
		"Config":      m.config,
		"Variables":   m.definition.Skill.Variables,
		"ProjectDir":  ctx.Config.ProjectDir,
		"WorkflowDir": ctx.Workflow.Dir(),
		"SkillsDir":   ctx.Config.SkillsDir(),
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("skill-module: render prompt: %w", err)
	}
	return strings.TrimSpace(buf.String()), nil
}

func (m *skillModule) desiredWindowName() string {
	if name := strings.TrimSpace(m.definition.Skill.WindowName); name != "" {
		return sanitizeWindowName(name)
	}
	return fmt.Sprintf("%s-%d", sanitizeWindowName(m.definition.ID), time.Now().Unix())
}

func defaultModuleName(def ModuleDefinition) string {
	if strings.TrimSpace(def.Name) != "" {
		return def.Name
	}
	return def.ID
}

func resolveBindings(bindings []ArtifactBinding) ([]artifact.ArtifactRef, []string, error) {
	if len(bindings) == 0 {
		return nil, nil, nil
	}
	resolved := make([]artifact.ArtifactRef, len(bindings))
	ids := make([]string, len(bindings))
	for i, binding := range bindings {
		ref, err := binding.Resolve()
		if err != nil {
			return nil, nil, err
		}
		resolved[i] = ref
		ids[i] = ref.ID
	}
	return resolved, ids, nil
}

func mergeConfigs(base module.Config, override module.Config) module.Config {
	if len(base) == 0 && len(override) == 0 {
		return nil
	}
	merged := make(module.Config)
	for k, v := range base {
		if key := strings.TrimSpace(k); key != "" {
			merged[key] = v
		}
	}
	for k, v := range override {
		if key := strings.TrimSpace(k); key != "" {
			merged[key] = v
		}
	}
	return merged
}

func validateSkillContext(ctx *module.ModuleContext) error {
	if ctx == nil {
		return fmt.Errorf("skill-module: context is nil")
	}
	if ctx.Config == nil {
		return fmt.Errorf("skill-module: config is required")
	}
	if ctx.Workflow == nil {
		return fmt.Errorf("skill-module: workflow is required")
	}
	if ctx.Artifacts == nil {
		return fmt.Errorf("skill-module: artifact store is required")
	}
	return nil
}

func readArtifactBody(ref artifact.ArtifactRef, path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if ref.Kind == artifact.KindJSON {
		return data, nil
	}
	if _, body, err := artifact.ParseFrontMatter(data); err == nil {
		return body, nil
	}
	return data, nil
}

func sanitizeWindowName(name string) string {
	re := regexp.MustCompile(`[^A-Za-z0-9_-]+`)
	clean := re.ReplaceAllString(strings.TrimSpace(name), "-")
	clean = strings.Trim(clean, "-")
	if clean == "" {
		return "skill-module"
	}
	return clean
}

type skillTerminal interface {
	CreateWindow(name, dir string) error
	SendOpenCode(window, prompt string, env map[string]string) error
	KillWindow(name string)
}

type tmuxTerminal struct{}

func (tm tmuxTerminal) CreateWindow(name, dir string) error {
	args := []string{"new-window", "-n", name}
	if strings.TrimSpace(dir) != "" {
		args = append(args, "-c", dir)
	}
	cmd := exec.Command("tmux", args...)
	return cmd.Run()
}

func (tm tmuxTerminal) SendOpenCode(window, prompt string, env map[string]string) error {
	if strings.TrimSpace(window) == "" {
		return errors.New("tmux window is required")
	}
	escapedPrompt := strings.ReplaceAll(prompt, "\"", `\"`)
	escapedPrompt = strings.ReplaceAll(escapedPrompt, "\n", " ")
	command := fmt.Sprintf("opencode --prompt \"%s\"", strings.TrimSpace(escapedPrompt))
	if prefix := formatEnvPrefix(env); prefix != "" {
		command = fmt.Sprintf("%s %s", prefix, command)
	}
	cmd := exec.Command("tmux", "send-keys", "-t", window, command, "Enter")
	return cmd.Run()
}

func (tm tmuxTerminal) KillWindow(name string) {
	if strings.TrimSpace(name) == "" {
		return
	}
	_ = exec.Command("tmux", "kill-window", "-t", name).Run()
}

func formatEnvPrefix(env map[string]string) string {
	if len(env) == 0 {
		return ""
	}
	parts := make([]string, 0, len(env))
	for key, value := range env {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		escaped := strings.ReplaceAll(value, "'", "'\\''")
		parts = append(parts, fmt.Sprintf("%s='%s'", key, escaped))
	}
	sort.Strings(parts)
	return strings.Join(parts, " ")
}
